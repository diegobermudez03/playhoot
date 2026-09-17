package joinsession

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

const (
	operationJoin = "JOIN"
	outcomeJoined = "JOINED"
)

type repoAPI interface {
	resolveActiveSessionForJoinCode(ctx context.Context, joinCode uint) (*joinCodeResolution, error)
	joinSession(ctx context.Context, params joinParams) (Result, error)
}

type repo struct {
	db *gorm.DB
}

func newRepo(db *gorm.DB) *repo {
	return &repo{db: db}
}

// GetDB satisfies utils.DBServicer so repo can own its own transaction
// boundary via utils.RunInDBTransaction.
func (r *repo) GetDB() *gorm.DB { return r.db }

type joinCodeResolution struct {
	SessionID          uint
	GameDefinitionUUID string
}

// resolveActiveSessionForJoinCode is an unlocked lookup: it identifies which
// Session an active JoinCode currently belongs to, and that Session's pinned
// Definition/Version UUID, so the caller can read the Game Management
// pinned-definition capability before opening the mutation transaction. The
// Session's admissibility itself is always re-validated under lock inside
// joinSession - see WORK-0001's Game Management Dependency (either order is
// correctness-safe since the pinned definition cannot change underneath the
// Session).
func (r *repo) resolveActiveSessionForJoinCode(ctx context.Context, joinCode uint) (*joinCodeResolution, error) {
	var resolution joinCodeResolution
	result := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS session_id, s.game_definition_uuid
		FROM sessions s
		INNER JOIN join_codes jc ON jc.session_id = s.id
		WHERE jc.code = ? AND jc.revoked_at IS NULL
	`, joinCode).Scan(&resolution)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving join code: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &resolution, nil
}

type joinParams struct {
	SessionID      uint
	JoinCode       uint
	UserUUID       string
	DisplayName    string
	IdempotencyKey string
	PlayersMax     int
}

// joinRequestPayload is JOIN's meaningful-field idempotency payload.
type joinRequestPayload struct {
	JoinCode    uint   `json:"join_code"`
	UserUUID    string `json:"user_uuid"`
	DisplayName string `json:"display_name"`
}

// joinSession obtains the Session's per-Session DB mutation lock, revalidates
// LOBBY admissibility (lazily materializing an already-expired lobby),
// enforces the pinned Definition's players.max, and activates a Participant
// - reusing an existing SessionActor/Participant where one already exists
// for (session, user_uuid). See WORK-0001's Session/Actor/Participant
// Operation Behavior - JoinSession.
func (r *repo) joinSession(ctx context.Context, params joinParams) (Result, error) {
	return utils.RunInDBTransaction(ctx, r, func(ctx context.Context, tx *gorm.DB) (Result, error) {
		row, err := sessionlock.LockByID(ctx, tx, params.SessionID)
		if err != nil {
			return Result{}, err
		}
		if row == nil {
			return Result{}, session.ErrSessionNotFound
		}

		now := time.Now().UTC()
		expired, err := sessionlock.MaterializeExpirationIfDue(ctx, tx, row, now)
		if err != nil {
			return Result{}, err
		}
		if expired || row.Phase != session.PhaseLobby {
			return Result{}, session.ErrLobbyExpired
		}

		incomingPayload := joinRequestPayload{
			JoinCode:    params.JoinCode,
			UserUUID:    params.UserUUID,
			DisplayName: params.DisplayName,
		}
		payloadBytes, err := json.Marshal(incomingPayload)
		if err != nil {
			return Result{}, fmt.Errorf("marshaling join request payload: %s", err)
		}

		sessionID := row.ID
		requestID, claimed, existing, err := idempotency.Claim(ctx, tx, idempotency.ClaimInput{
			Operation:      operationJoin,
			UserUUID:       params.UserUUID,
			IdempotencyKey: params.IdempotencyKey,
			SessionID:      &sessionID,
			RequestPayload: string(payloadBytes),
		})
		if err != nil {
			return Result{}, fmt.Errorf("claiming join idempotency key: %s", err)
		}
		if !claimed {
			return interpretExistingJoinClaim(existing, incomingPayload)
		}

		actorID, err := findOrCreateActor(ctx, tx, row.ID, params.UserUUID, now)
		if err != nil {
			return Result{}, err
		}

		if err := admitParticipant(ctx, tx, row.ID, actorID, params.DisplayName, params.PlayersMax, now); err != nil {
			return Result{}, err
		}

		result := Result{SessionUUID: row.UUID, DisplayName: params.DisplayName}
		responseBytes, err := json.Marshal(result)
		if err != nil {
			return Result{}, fmt.Errorf("marshaling join response payload: %s", err)
		}
		if err := idempotency.Complete(ctx, tx, requestID, &sessionID, outcomeJoined, string(responseBytes)); err != nil {
			return Result{}, err
		}

		return result, nil
	})
}

// actorRow is Join's local view of a persisted session_actors record - a
// small persistence-code duplication with leavesession's own actorRow is
// preferred here over a shared horizontal actors package, since Join and
// Leave are separate behavior-local consumers whose query/locking needs may
// evolve independently (see repositories.md's Sharing Rule).
type actorRow struct {
	ID        uint
	SessionID uint
	UserUUID  string
}

// findActor returns the SessionActor for (sessionID, userUUID), or nil, nil
// if none exists yet.
func findActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*actorRow, error) {
	var row actorRow
	result := tx.WithContext(ctx).Raw(`
		SELECT id, session_id, user_uuid
		FROM session_actors
		WHERE session_id = ? AND user_uuid = ?
	`, sessionID, userUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("finding session actor: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

type actorInsert struct {
	ID               uint      `gorm:"column:id"`
	SessionID        uint      `gorm:"column:session_id"`
	UserUUID         string    `gorm:"column:user_uuid"`
	SemanticPresence string    `gorm:"column:semantic_presence"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (actorInsert) TableName() string { return "session_actors" }

// createActor inserts a new SessionActor for (sessionID, userUUID), starting
// semantic_presence CONNECTED, and returns its id.
func createActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string, now time.Time) (uint, error) {
	row := actorInsert{
		SessionID:        sessionID,
		UserUUID:         userUUID,
		SemanticPresence: session.PresenceConnected,
		CreatedAt:        now,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating session actor: %s", err)
	}
	return row.ID, nil
}

// participantRow is Join's local view of a persisted session_participants
// record.
type participantRow struct {
	ID             uint
	SessionActorID uint
	DisplayName    string
	Active         bool
}

// findParticipant returns the Participant owned by actorID, or nil, nil if
// none has ever been created for it.
func findParticipant(ctx context.Context, tx *gorm.DB, actorID uint) (*participantRow, error) {
	var row participantRow
	result := tx.WithContext(ctx).Raw(`
		SELECT id, session_actor_id, display_name, active
		FROM session_participants
		WHERE session_actor_id = ?
	`, actorID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("finding participant: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

type participantInsert struct {
	ID             uint      `gorm:"column:id"`
	SessionActorID uint      `gorm:"column:session_actor_id"`
	DisplayName    string    `gorm:"column:display_name"`
	Active         bool      `gorm:"column:active"`
	JoinedAt       time.Time `gorm:"column:joined_at"`
}

func (participantInsert) TableName() string { return "session_participants" }

// createParticipant inserts the first, active Participant for actorID.
func createParticipant(ctx context.Context, tx *gorm.DB, actorID uint, displayName string, now time.Time) error {
	row := participantInsert{
		SessionActorID: actorID,
		DisplayName:    displayName,
		Active:         true,
		JoinedAt:       now,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating participant: %s", err)
	}
	return nil
}

// reactivateParticipant re-admits a previously deactivated Participant,
// refreshing its display-name snapshot and joined_at.
func reactivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, displayName string, now time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_participants
		SET active = TRUE, left_at = NULL, display_name = ?, joined_at = ?
		WHERE id = ?
	`, displayName, now, participantID).Error; err != nil {
		return fmt.Errorf("reactivating participant: %s", err)
	}
	return nil
}

// refreshActiveDisplayName updates the display-name snapshot of an already
// active Participant, keeping a repeated identical Join idempotent without
// mutating joined_at/left_at.
func refreshActiveDisplayName(ctx context.Context, tx *gorm.DB, participantID uint, displayName string) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_participants
		SET display_name = ?
		WHERE id = ?
	`, displayName, participantID).Error; err != nil {
		return fmt.Errorf("refreshing participant display name: %s", err)
	}
	return nil
}

// countActiveParticipants returns the number of currently active
// Participants for sessionID.
func countActiveParticipants(ctx context.Context, tx *gorm.DB, sessionID uint) (int, error) {
	var count int
	if err := tx.WithContext(ctx).Raw(`
		SELECT COUNT(*)
		FROM session_participants p
		INNER JOIN session_actors a ON a.id = p.session_actor_id
		WHERE a.session_id = ? AND p.active = TRUE
	`, sessionID).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("counting active participants: %s", err)
	}
	return count, nil
}

func findOrCreateActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string, now time.Time) (uint, error) {
	actor, err := findActor(ctx, tx, sessionID, userUUID)
	if err != nil {
		return 0, err
	}
	if actor != nil {
		return actor.ID, nil
	}
	return createActor(ctx, tx, sessionID, userUUID, now)
}

func admitParticipant(ctx context.Context, tx *gorm.DB, sessionID, actorID uint, displayName string, playersMax int, now time.Time) error {
	participant, err := findParticipant(ctx, tx, actorID)
	if err != nil {
		return err
	}

	if participant != nil && participant.Active {
		// Repeated identical active Join is the same logical admission, not
		// a new slot - just refresh the display-name snapshot.
		return refreshActiveDisplayName(ctx, tx, participant.ID, displayName)
	}

	activeCount, err := countActiveParticipants(ctx, tx, sessionID)
	if err != nil {
		return err
	}
	if playersMax > 0 && activeCount >= playersMax {
		return session.ErrLobbyFull
	}

	if participant == nil {
		return createParticipant(ctx, tx, actorID, displayName, now)
	}
	return reactivateParticipant(ctx, tx, participant.ID, displayName, now)
}

func interpretExistingJoinClaim(existing *idempotency.Row, incoming joinRequestPayload) (Result, error) {
	if existing.Status != idempotency.StatusCompleted {
		return Result{}, session.ErrIdempotencyInFlight
	}

	var storedPayload joinRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &storedPayload); err != nil {
		return Result{}, fmt.Errorf("decoding stored join request payload: %s", err)
	}
	if storedPayload != incoming {
		return Result{}, session.ErrIdempotencyConflict
	}

	if existing.ResponsePayload == nil {
		return Result{}, fmt.Errorf("completed join idempotency record missing response payload")
	}
	var result Result
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return Result{}, fmt.Errorf("decoding stored join response payload: %s", err)
	}
	return result, nil
}
