// Package storereplaychunk stores the replay chunks the live Runtime records
// for a Session: first the bytes as an immutable object, then their entry in
// the Session's replay manifest.
package storereplaychunk

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/storereplaychunk/internal/repo"
)

//go:generate mockgen -package=storereplaychunk -destination=service_mock_test.go . objectWriter,chunkRegistry

// objectWriter writes objects to private storage.
type objectWriter interface {
	Put(ctx context.Context, key string, content []byte, contentType string) error
}

// chunkRegistry adds stored chunks to their Session's replay manifest.
type chunkRegistry interface {
	RegisterChunk(ctx context.Context, sessionUUID string, chunk internalrepo.Chunk, registeredAt time.Time) (found bool, err error)
}

// contentType is how stored chunks are labeled: compressed with zstd.
const contentType = "application/zstd"

// Service is the replay chunk storage use case.
type Service struct {
	objects objectWriter
	chunks  chunkRegistry
	now     func() time.Time
}

// New constructs a Service registering chunks in db and storing them in store.
func New(db *gorm.DB, store storage.Store) *Service {
	return &Service{objects: store, chunks: internalrepo.New(db), now: time.Now}
}

// StoreReplayChunk stores chunk for sessionUUID. Storing the same chunk again
// is harmless; a final one marks the Session's replay finalized.
func (s *Service) StoreReplayChunk(ctx context.Context, sessionUUID string, chunk sessionpkg.ReplayChunk) error {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.ErrSessionNotFound
	}
	if _, err := uuid.Parse(chunk.RecordingUUID); err != nil {
		return fmt.Errorf("invalid recording uuid %q", chunk.RecordingUUID)
	}
	if len(chunk.Content) == 0 || chunk.Index < 0 {
		return errors.New("replay chunk has no content or a negative index")
	}
	// The object is written first: an orphan object is harmless, a manifest
	// entry without its object is not.
	key := fmt.Sprintf("replays/%s/%s/%06d.rpl", sessionUUID, chunk.RecordingUUID, chunk.Index)
	if err := s.objects.Put(ctx, key, chunk.Content, contentType); err != nil {
		return fmt.Errorf("writing replay chunk object: %w", err)
	}
	found, err := s.chunks.RegisterChunk(ctx, sessionUUID, internalrepo.Chunk{
		RecordingUUID:        chunk.RecordingUUID,
		RecordingStartedAt:   chunk.RecordingStartedAt.UTC().Truncate(time.Microsecond),
		Index:                chunk.Index,
		FormatVersion:        chunk.FormatVersion,
		FromSessionTimeMs:    chunk.FromSessionTimeMs,
		ThroughSessionTimeMs: chunk.ThroughSessionTimeMs,
		Final:                chunk.Final,
		ObjectKey:            key,
		SHA256:               objectstore.HashHex(chunk.Content),
		Size:                 int64(len(chunk.Content)),
	}, s.now().UTC().Truncate(time.Microsecond))
	if err != nil {
		return err
	}
	if !found {
		return sessionpkg.ErrSessionNotFound
	}
	return nil
}
