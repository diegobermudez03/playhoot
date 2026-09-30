package sessionlifecycle

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/session"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

// contentAccessRepoAPI is the frontend-script and asset access reads' own
// narrow persistence contract - every method is a plain, unlocked read, since
// these calls never mutate state.
type contentAccessRepoAPI interface {
	ResolveSessionForClientState(ctx context.Context, sessionUUID string) (*internalrepo.Session, error)
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	ResolveFrontendScriptObject(ctx context.Context, definitionUUID string) (*internalrepo.ContentObject, error)
	ResolveAssetObject(ctx context.Context, definitionUUID, key string) (*internalrepo.ContentObject, error)
}

// GetFrontendScriptAccess returns short-lived signed access to the frontend
// script of the exact game version sessionUUID is pinned to, for Playhoot's
// own trusted host frontend to fetch directly from storage. It is a pure
// read: it never mutates Session state and never locks the Session row, and
// it is available to any participant in any phase, since the pinned version
// exists from Create.
//
// The signed URL is a bearer credential until it expires and must never be
// handed to game code running in the sandboxed iframe. Session Runtime does
// not proxy the bytes.
func (m *Manager) GetFrontendScriptAccess(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID) (session.ContentAccessResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.GetFrontendScriptAccess").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
	)

	return m.grantContentAccess(ctx, sessionUUID, userUUID, func(definitionUUID string) (*internalrepo.ContentObject, error) {
		return m.contentAccessRepo.ResolveFrontendScriptObject(ctx, definitionUUID)
	})
}

// GetAssetAccess returns short-lived signed access to the asset declared
// under key by the exact game version sessionUUID is pinned to. key is the
// game's own logical asset key; a key the pinned version never declared
// returns ContentAccessOutcomeContentNotFound. Same read-only, trusted-host-
// frontend-only contract as GetFrontendScriptAccess.
func (m *Manager) GetAssetAccess(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, key string) (session.ContentAccessResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.GetAssetAccess").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
		logging.Field("asset_key", key),
	)

	return m.grantContentAccess(ctx, sessionUUID, userUUID, func(definitionUUID string) (*internalrepo.ContentObject, error) {
		return m.contentAccessRepo.ResolveAssetObject(ctx, definitionUUID, key)
	})
}

// grantContentAccess is the shared flow: resolve the Session, require the
// caller to be a participant, resolve the pinned version's object through
// resolve, and sign exactly that object. Signed URLs are never logged.
func (m *Manager) grantContentAccess(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, resolve func(definitionUUID string) (*internalrepo.ContentObject, error)) (session.ContentAccessResult, error) {
	found, err := m.contentAccessRepo.ResolveSessionForClientState(ctx, string(sessionUUID))
	if err != nil {
		return session.ContentAccessResult{}, err
	}
	if found == nil {
		return session.ContentAccessResult{}, session.ErrSessionNotFound
	}

	actor, err := m.contentAccessRepo.FindActor(ctx, m.dbServicer.GetDB(), found.ID, string(userUUID))
	if err != nil {
		return session.ContentAccessResult{}, err
	}
	if actor == nil {
		return session.ContentAccessResult{Outcome: session.ContentAccessOutcomeNotAParticipant}, nil
	}

	object, err := resolve(found.GameDefinitionUUID)
	if err != nil {
		return session.ContentAccessResult{}, err
	}
	if object == nil {
		return session.ContentAccessResult{Outcome: session.ContentAccessOutcomeContentNotFound}, nil
	}

	signed, err := m.objectStore.PresignGet(ctx, object.Locator.ObjectKey, m.signedURLTTL)
	if err != nil {
		return session.ContentAccessResult{}, fmt.Errorf("signing content access: %w", err)
	}

	return session.ContentAccessResult{
		Outcome:     session.ContentAccessOutcomeGranted,
		URL:         signed.URL,
		ExpiresAt:   signed.ExpiresAt,
		SHA256:      object.Locator.SHA256,
		ContentType: object.ContentType,
		Size:        object.Locator.Size,
	}, nil
}
