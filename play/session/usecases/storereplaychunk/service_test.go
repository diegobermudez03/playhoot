package storereplaychunk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/storereplaychunk/internal/repo"
)

func TestStoreReplayChunk(t *testing.T) {
	type dependencies struct {
		objects *MockobjectWriter
		chunks  *MockchunkRegistry
	}
	type test struct {
		sessionUUID string
		chunk       sessionpkg.ReplayChunk

		expectedErr error
		expectErr   bool
	}

	const (
		sessionUUID   = "5e55a000-0000-4000-8000-000000000001"
		recordingUUID = "7ec00000-0000-4000-8000-000000000001"
		key           = "replays/" + sessionUUID + "/" + recordingUUID + "/000003.rpl"
	)
	ctx := context.Background()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC)
	content := []byte("compressed chunk")
	chunk := sessionpkg.ReplayChunk{
		RecordingUUID: recordingUUID, RecordingStartedAt: startedAt, Index: 3, FormatVersion: 1,
		FromSessionTimeMs: 180_000, ThroughSessionTimeMs: 240_000, Content: content,
	}
	registered := internalrepo.Chunk{
		RecordingUUID: recordingUUID, RecordingStartedAt: startedAt, Index: 3, FormatVersion: 1,
		FromSessionTimeMs: 180_000, ThroughSessionTimeMs: 240_000,
		ObjectKey: key, SHA256: objectstore.HashHex(content), Size: int64(len(content)),
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"object_written_then_registered": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.objects.EXPECT().Put(ctx, key, content, contentType).Return(nil),
				deps.chunks.EXPECT().RegisterChunk(ctx, sessionUUID, registered, now).Return(true, nil),
			)
			return test{sessionUUID: sessionUUID, chunk: chunk}
		},
		"final_chunk_registered_as_final": func(t *testing.T, deps dependencies) test {
			final, finalRegistered := chunk, registered
			final.Final, finalRegistered.Final = true, true
			gomock.InOrder(
				deps.objects.EXPECT().Put(ctx, key, content, contentType).Return(nil),
				deps.chunks.EXPECT().RegisterChunk(ctx, sessionUUID, finalRegistered, now).Return(true, nil),
			)
			return test{sessionUUID: sessionUUID, chunk: final}
		},
		"failed_write_registers_nothing": func(t *testing.T, deps dependencies) test {
			deps.objects.EXPECT().Put(ctx, key, content, contentType).Return(errors.New("storage down"))
			return test{sessionUUID: sessionUUID, chunk: chunk, expectErr: true}
		},
		"failed_registration_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.objects.EXPECT().Put(ctx, key, content, contentType).Return(nil),
				deps.chunks.EXPECT().RegisterChunk(ctx, sessionUUID, registered, now).Return(false, errors.New("database down")),
			)
			return test{sessionUUID: sessionUUID, chunk: chunk, expectErr: true}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.objects.EXPECT().Put(ctx, key, content, contentType).Return(nil),
				deps.chunks.EXPECT().RegisterChunk(ctx, sessionUUID, registered, now).Return(false, nil),
			)
			return test{sessionUUID: sessionUUID, chunk: chunk, expectedErr: sessionpkg.ErrSessionNotFound}
		},
		"invalid_session_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", chunk: chunk, expectedErr: sessionpkg.ErrSessionNotFound}
		},
		"invalid_recording_uuid": func(t *testing.T, deps dependencies) test {
			invalid := chunk
			invalid.RecordingUUID = "not-a-uuid"
			return test{sessionUUID: sessionUUID, chunk: invalid, expectErr: true}
		},
		"empty_content": func(t *testing.T, deps dependencies) test {
			empty := chunk
			empty.Content = nil
			return test{sessionUUID: sessionUUID, chunk: empty, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{objects: NewMockobjectWriter(ctrl), chunks: NewMockchunkRegistry(ctrl)}
			tc := setup(t, deps)
			service := &Service{objects: deps.objects, chunks: deps.chunks, now: func() time.Time { return now }}

			err := service.StoreReplayChunk(ctx, tc.sessionUUID, tc.chunk)

			switch {
			case tc.expectedErr != nil:
				require.ErrorIs(t, err, tc.expectedErr)
			case tc.expectErr:
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}
		})
	}
}
