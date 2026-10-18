package sessionlifecycle

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=sessionlifecycle -destination=step_create_session_mock_test.go . createSessionRepo

// createSessionRepo is CreateSession's own narrow persistence contract.
type createSessionRepo interface {
	ClaimCreateSessionRequest(ctx context.Context, tx *gorm.DB, userUUID string, idempotencyKey string, gameUUID string) (uint, *internalrepo.CreateSessionRequest, error)
	CompleteCreateSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, outcome string, sessionID *uint) error
	FindCurrentGameVersion(ctx context.Context, tx *gorm.DB, gameUUID string) (*internalrepo.GameVersion, error)
	CreateSession(ctx context.Context, tx *gorm.DB, gameVersionID uint, createdByUserUUID string, status string, lobbyExpiresAt time.Time, createdAt time.Time) (internalrepo.CreatedSession, error)
	CreateJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, code string, expiresAt time.Time) (bool, error)
	ReleaseExpiredJoinCode(ctx context.Context, tx *gorm.DB, code string, releasedAt time.Time) (bool, error)
	FindSessionWithJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint) (*internalrepo.SessionWithJoinCode, error)
}

const (
	// joinCodeAlphabet leaves out characters easy to confuse when read
	// aloud or typed (0/O, 1/I/L).
	joinCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	joinCodeLength   = 6

	// maxJoinCodeAttempts bounds how many candidate codes one creation tries
	// before giving up, so a nearly exhausted code space fails instead of
	// looping.
	maxJoinCodeAttempts = 10
)

var (
	createOutcomeLabel = metrics.NewLabel("outcome", "created", "game_unavailable", "replayed")

	createOutcomeCreated         = createOutcomeLabel.Value("created")
	createOutcomeGameUnavailable = createOutcomeLabel.Value("game_unavailable")
	createOutcomeReplayed        = createOutcomeLabel.Value("replayed")

	sessionCreateTotal = metrics.NewCounter(
		"play_session_create_total",
		"Completed Session creation requests, by outcome; replayed counts retries answered from a previous request.",
		createOutcomeLabel,
	)
)

// createSessionAttempt is what one creation transaction decided: the result
// to answer with, and whether it was replayed from an earlier request with
// the same idempotency key rather than decided now.
type createSessionAttempt struct {
	result   sessionpkg.CreateSessionResult
	replayed bool
}

// CreateSession creates a Session in LOBBY pinned to gameUUID's current
// version, with a join code valid until the lobby expires. It is idempotent per
// (userUUID, idempotencyKey); reusing a key for another Game returns
// sessionpkg.ErrIdempotencyConflict.
func (m *Manager) CreateSession(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (sessionpkg.CreateSessionResult, error) {
	defer logging.Step(ctx, "sessionlifecycle.create_session").Close()

	normalizedUserUUID, err := uuid.Parse(userUUID)
	if err != nil {
		return sessionpkg.CreateSessionResult{}, fmt.Errorf("user uuid is not a valid UUID")
	}
	normalizedGameUUID, err := uuid.Parse(gameUUID)
	if err != nil {
		return sessionpkg.CreateSessionResult{}, fmt.Errorf("game uuid is not a valid UUID")
	}
	if idempotencyKey == "" {
		return sessionpkg.CreateSessionResult{}, fmt.Errorf("idempotency key is required")
	}

	attempt, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (createSessionAttempt, error) {
		return m.createSessionInTx(ctx, tx, normalizedUserUUID.String(), normalizedGameUUID.String(), idempotencyKey)
	})
	if err != nil {
		return sessionpkg.CreateSessionResult{}, err
	}

	// Counted only once the transaction committed, so a rolled-back attempt
	// is never counted as a created Session.
	switch {
	case attempt.replayed:
		sessionCreateTotal.Inc(ctx, createOutcomeReplayed)
	case attempt.result.Outcome == sessionpkg.CreateSessionCreated:
		sessionCreateTotal.Inc(ctx, createOutcomeCreated)
	default:
		sessionCreateTotal.Inc(ctx, createOutcomeGameUnavailable)
	}
	return attempt.result, nil
}

// createSessionInTx is CreateSession's work inside its transaction. The
// arguments are already validated and normalized.
func (m *Manager) createSessionInTx(ctx context.Context, tx *gorm.DB, userUUID string, gameUUID string, idempotencyKey string) (createSessionAttempt, error) {
	requestID, existing, err := m.createSessionRepo.ClaimCreateSessionRequest(ctx, tx, userUUID, idempotencyKey, gameUUID)
	if err != nil {
		return createSessionAttempt{}, err
	}
	if existing != nil {
		result, err := m.replayCreateSession(ctx, tx, existing, gameUUID)
		if err != nil {
			return createSessionAttempt{}, err
		}
		logging.LogFields(ctx, logging.Field("create_session_replayed", true))
		return createSessionAttempt{result: result, replayed: true}, nil
	}

	version, err := m.createSessionRepo.FindCurrentGameVersion(ctx, tx, gameUUID)
	if err != nil {
		return createSessionAttempt{}, err
	}
	if version == nil {
		if err := m.createSessionRepo.CompleteCreateSessionRequest(ctx, tx, requestID, string(sessionpkg.CreateSessionGameUnavailable), nil); err != nil {
			return createSessionAttempt{}, err
		}
		return createSessionAttempt{result: sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionGameUnavailable}}, nil
	}
	if !gameversion.Status(version.Status).AllowsSessions() {
		// A Game's current version must always be executable (immutable,
		// scripts ready to run); anything else is broken data, not a Game
		// that is merely unavailable.
		monitoring.Alert(ctx, "play game points to a current version that is not executable")
		return createSessionAttempt{}, fmt.Errorf("current version %d of game %s is %s, not executable", version.ID, gameUUID, version.Status)
	}

	// Postgres keeps microseconds; truncating keeps the first answer equal
	// to what a replay reads back.
	now := m.now().UTC().Truncate(time.Microsecond)
	lobbyExpiresAt := now.Add(m.lobbyTTL)

	created, err := m.createSessionRepo.CreateSession(ctx, tx, version.ID, userUUID, string(sessionstatus.Lobby), lobbyExpiresAt, now)
	if err != nil {
		return createSessionAttempt{}, err
	}
	joinCode, err := m.allocateJoinCode(ctx, tx, created.ID, lobbyExpiresAt, now)
	if err != nil {
		return createSessionAttempt{}, err
	}
	if err := m.createSessionRepo.CompleteCreateSessionRequest(ctx, tx, requestID, string(sessionpkg.CreateSessionCreated), &created.ID); err != nil {
		return createSessionAttempt{}, err
	}

	logging.LogFields(ctx, logging.Field("session_uuid", created.UUID))
	return createSessionAttempt{result: sessionpkg.CreateSessionResult{
		Outcome:     sessionpkg.CreateSessionCreated,
		SessionUUID: created.UUID,
		JoinCode:    joinCode,
		ExpiresAt:   lobbyExpiresAt,
	}}, nil
}

// replayCreateSession answers a request whose idempotency identity was
// already claimed: the same Game replays the earlier outcome, a different
// Game is a conflict.
func (m *Manager) replayCreateSession(ctx context.Context, tx *gorm.DB, existing *internalrepo.CreateSessionRequest, gameUUID string) (sessionpkg.CreateSessionResult, error) {
	if existing.GameUUID != gameUUID {
		return sessionpkg.CreateSessionResult{}, sessionpkg.ErrIdempotencyConflict
	}
	if existing.Outcome == nil {
		// Claims are completed in the transaction that made them, so a
		// committed claim without an outcome is broken data.
		monitoring.Alert(ctx, "play create session request committed without an outcome")
		return sessionpkg.CreateSessionResult{}, fmt.Errorf("create session request %d has no outcome", existing.ID)
	}

	switch sessionpkg.CreateSessionOutcome(*existing.Outcome) {
	case sessionpkg.CreateSessionGameUnavailable:
		return sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionGameUnavailable}, nil
	case sessionpkg.CreateSessionCreated:
		if existing.SessionID == nil {
			monitoring.Alert(ctx, "play create session request is created but references no session")
			return sessionpkg.CreateSessionResult{}, fmt.Errorf("create session request %d has no session", existing.ID)
		}
		found, err := m.createSessionRepo.FindSessionWithJoinCode(ctx, tx, *existing.SessionID)
		if err != nil {
			return sessionpkg.CreateSessionResult{}, err
		}
		if found == nil {
			monitoring.Alert(ctx, "play create session request references a session or join code that does not exist")
			return sessionpkg.CreateSessionResult{}, fmt.Errorf("session %d of create session request %d not found", *existing.SessionID, existing.ID)
		}
		return sessionpkg.CreateSessionResult{
			Outcome:     sessionpkg.CreateSessionCreated,
			SessionUUID: found.UUID,
			JoinCode:    found.JoinCode,
			ExpiresAt:   found.LobbyExpiresAt.UTC(),
		}, nil
	default:
		monitoring.Alert(ctx, "play create session request has an unknown outcome")
		return sessionpkg.CreateSessionResult{}, fmt.Errorf("create session request %d has unknown outcome %q", existing.ID, *existing.Outcome)
	}
}

// allocateJoinCode gives sessionID a join code no other active join code
// holds. A candidate held by a join code that already expired is released
// and reused; a candidate still in use is skipped for a new one.
func (m *Manager) allocateJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, expiresAt time.Time, now time.Time) (string, error) {
	for range maxJoinCodeAttempts {
		code, err := m.newJoinCode()
		if err != nil {
			return "", fmt.Errorf("generating join code: %s", err)
		}

		created, err := m.createSessionRepo.CreateJoinCode(ctx, tx, sessionID, code, expiresAt)
		if err != nil {
			return "", err
		}
		if created {
			return code, nil
		}

		released, err := m.createSessionRepo.ReleaseExpiredJoinCode(ctx, tx, code, now)
		if err != nil {
			return "", err
		}
		if !released {
			continue
		}
		created, err = m.createSessionRepo.CreateJoinCode(ctx, tx, sessionID, code, expiresAt)
		if err != nil {
			return "", err
		}
		if created {
			return code, nil
		}
	}
	return "", errors.New("no free join code found")
}

// newJoinCode returns a random join code of joinCodeLength characters from
// joinCodeAlphabet.
func newJoinCode() (string, error) {
	code := make([]byte, joinCodeLength)
	alphabetSize := big.NewInt(int64(len(joinCodeAlphabet)))
	for i := range code {
		index, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", err
		}
		code[i] = joinCodeAlphabet[index.Int64()]
	}
	return string(code), nil
}
