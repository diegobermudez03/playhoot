package sessionlifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=sessionlifecycle -destination=step_commit_start_mock_test.go . commitStartRepo

// commitStartRepo is CommitStart's own narrow persistence contract.
type commitStartRepo interface {
	LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string) (*internalrepo.LockedSession, error)
	CreateParticipants(ctx context.Context, tx *gorm.DB, sessionID uint, participants []internalrepo.Participant) error
	SaveCheckpoint(ctx context.Context, tx *gorm.DB, sessionID uint, contextRevision int64, content []byte, savedAt time.Time) error
	ChangeSessionStatus(ctx context.Context, tx *gorm.DB, sessionID uint, fromStatus string, toStatus string, endCause *string, changedBy *string, changedAt time.Time) error
	RenewOwnerLease(ctx context.Context, tx *gorm.DB, sessionID uint, expiresAt time.Time) error
	ReleaseJoinCodes(ctx context.Context, tx *gorm.DB, sessionID uint, releasedAt time.Time) error
}

var (
	commitStartOutcomeLabel = metrics.NewLabel("outcome",
		string(sessionpkg.CommitStartCommitted),
		string(sessionpkg.CommitStartNotInLobby),
	)

	sessionStartCommits = metrics.NewCounter(
		"play_session_start_commit_total",
		"Attempts to record a Session as started, by outcome.",
		commitStartOutcomeLabel,
	)
)

var errInvalidStartCommit = errors.New("invalid start commit")

// CommitStart makes sessionUUID RUNNING with the commit's roster and initial
// checkpoint, and releases its join code, all in one transaction, if it is
// still in a lobby that has not expired and userUUID created it.
func (m *Manager) CommitStart(ctx context.Context, sessionUUID string, userUUID string, commit sessionpkg.StartCommit) (sessionpkg.CommitStartOutcome, error) {
	defer logging.Step(ctx, "sessionlifecycle.commit_start").Close()

	if _, err := uuid.Parse(sessionUUID); err != nil {
		return "", sessionpkg.ErrSessionNotFound
	}
	participants, err := validStartCommit(commit)
	if err != nil {
		return "", err
	}
	outcome, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.CommitStartOutcome, error) {
		return m.commitStartInTx(ctx, tx, sessionUUID, userUUID, participants, commit.Checkpoint)
	})
	if err != nil {
		return "", err
	}
	// Counted once committed, so a rolled-back start is never counted.
	sessionStartCommits.Inc(ctx, commitStartOutcomeLabel.Value(string(outcome)))
	return outcome, nil
}

func (m *Manager) commitStartInTx(ctx context.Context, tx *gorm.DB, sessionUUID string, userUUID string, participants []internalrepo.Participant, checkpoint sessionpkg.Checkpoint) (sessionpkg.CommitStartOutcome, error) {
	session, err := m.commitStartRepo.LockSession(ctx, tx, sessionUUID)
	if err != nil {
		return "", err
	}
	if session == nil {
		return "", sessionpkg.ErrSessionNotFound
	}
	if !sameUUID(session.CreatedByUserUUID, userUUID) {
		// Only the creator's admin connection can start, as checked before.
		return "", fmt.Errorf("user %s did not create session %s", userUUID, sessionUUID)
	}
	// Postgres keeps microseconds; truncating keeps what is written equal to
	// what is read back.
	now := m.now().UTC().Truncate(time.Microsecond)
	if sessionstatus.Status(session.Status) != sessionstatus.Lobby || !now.Before(session.LobbyExpiresAt) {
		return sessionpkg.CommitStartNotInLobby, nil
	}

	if err := m.commitStartRepo.CreateParticipants(ctx, tx, session.ID, participants); err != nil {
		return "", err
	}
	if err := m.commitStartRepo.SaveCheckpoint(ctx, tx, session.ID, checkpoint.ContextRevision, checkpoint.Content, now); err != nil {
		return "", err
	}
	startedBy := session.CreatedByUserUUID
	if err := m.commitStartRepo.ChangeSessionStatus(ctx, tx, session.ID, string(sessionstatus.Lobby), string(sessionstatus.Running), nil, &startedBy, now); err != nil {
		return "", err
	}
	// The instance committing it is its live owner now.
	if err := m.commitStartRepo.RenewOwnerLease(ctx, tx, session.ID, now.Add(m.ownerLease)); err != nil {
		return "", err
	}
	if err := m.commitStartRepo.ReleaseJoinCodes(ctx, tx, session.ID, now); err != nil {
		return "", err
	}
	return sessionpkg.CommitStartCommitted, nil
}

// validStartCommit checks commit and returns its roster: at least one
// participant, each user and ParticipantID once, and a checkpoint.
func validStartCommit(commit sessionpkg.StartCommit) ([]internalrepo.Participant, error) {
	if len(commit.Participants) == 0 {
		return nil, fmt.Errorf("%w: no participants", errInvalidStartCommit)
	}
	if len(commit.Checkpoint.Content) == 0 {
		return nil, fmt.Errorf("%w: no checkpoint", errInvalidStartCommit)
	}
	users := make(map[uuid.UUID]bool, len(commit.Participants))
	participantIDs := make(map[string]bool, len(commit.Participants))
	participants := make([]internalrepo.Participant, 0, len(commit.Participants))
	for _, participant := range commit.Participants {
		user, err := uuid.Parse(participant.UserUUID)
		if err != nil || participant.ParticipantID == "" {
			return nil, fmt.Errorf("%w: participant %q of user %q", errInvalidStartCommit, participant.ParticipantID, participant.UserUUID)
		}
		if users[user] || participantIDs[participant.ParticipantID] {
			return nil, fmt.Errorf("%w: participant %q of user %q is repeated", errInvalidStartCommit, participant.ParticipantID, participant.UserUUID)
		}
		users[user], participantIDs[participant.ParticipantID] = true, true
		participants = append(participants, internalrepo.Participant{UserUUID: user.String(), ParticipantID: participant.ParticipantID, DisplayName: participant.DisplayName})
	}
	return participants, nil
}
