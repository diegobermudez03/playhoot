package versiondrafting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

func TestApplyChangesInTx(t *testing.T) {
	type dependencies struct {
		repo *MockapplyChangesRepo
	}
	type test struct {
		change          sessionpkg.DraftChange
		expected        sessionpkg.DraftChangeResult
		expectErr       bool
		expectedErrorIs error
	}

	const (
		versionUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
		versionID   = uint(21)
		attemptID   = uint(5)
		fingerprint = "fingerprint"
	)
	ctx := context.Background()
	tx := &gorm.DB{}
	revision := func(n int64) *int64 { return &n }
	draft := func(current int64) *internalrepo.Version {
		return &internalrepo.Version{ID: versionID, UUID: versionUUID, GameUUID: "game", Status: "DRAFT", CurrentRevision: revision(current)}
	}
	starter := draftsources.Apply(nil, draftsources.Template(), nil).Files
	change := func(base int64, files ...sessionpkg.DraftFileContent) sessionpkg.DraftChange {
		return sessionpkg.DraftChange{BaseRevision: base, Files: files, IdempotencyKey: "key", Origin: "api"}
	}
	claims := func(deps dependencies, version *internalrepo.Version, base int64) {
		deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(version, nil)
		deps.repo.EXPECT().ClaimChangeAttempt(ctx, tx, versionID, "change", "key", fingerprint, base, "api").Return(attemptID, nil, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"applied_as_the_next_revision": func(t *testing.T, deps dependencies) test {
			claims(deps, draft(3), 3)
			gomock.InOrder(
				deps.repo.EXPECT().FindRevision(ctx, tx, versionID, int64(3)).Return(draftsources.Revision{Files: starter}, true, nil),
				deps.repo.EXPECT().AdvanceRevision(ctx, tx, versionID, "DRAFT", int64(3)).Return(true, nil),
				deps.repo.EXPECT().StoreSourceContents(ctx, tx, map[string]string{draftsources.SHA256Hex("x"): "x"}).Return(nil),
				deps.repo.EXPECT().CreateRevision(ctx, tx, versionID, int64(4), revisionWithFiles(len(starter)+1), "api").Return(nil),
				deps.repo.EXPECT().QueueBuild(ctx, tx, versionID, int64(4), "build-uuid", "QUEUED", "SUPERSEDED").Return(1, nil),
				deps.repo.EXPECT().CompleteChangeAttempt(ctx, tx, attemptID, "applied", revision(4), nil).Return(nil),
			)
			return test{
				change:   change(3, sessionpkg.DraftFileContent{Path: "shared/x.ts", Content: "x"}),
				expected: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 4},
			}
		},
		"stale_base_is_a_conflict": func(t *testing.T, deps dependencies) test {
			claims(deps, draft(5), 3)
			deps.repo.EXPECT().CompleteChangeAttempt(ctx, tx, attemptID, "conflict", revision(5), nil).Return(nil)
			return test{
				change:   change(3, sessionpkg.DraftFileContent{Path: "shared/x.ts", Content: "x"}),
				expected: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: 5},
			}
		},
		"a_concurrent_change_won_the_base": func(t *testing.T, deps dependencies) test {
			claims(deps, draft(3), 3)
			gomock.InOrder(
				deps.repo.EXPECT().FindRevision(ctx, tx, versionID, int64(3)).Return(draftsources.Revision{Files: starter}, true, nil),
				deps.repo.EXPECT().AdvanceRevision(ctx, tx, versionID, "DRAFT", int64(3)).Return(false, nil),
				deps.repo.EXPECT().CompleteChangeAttempt(ctx, tx, attemptID, "conflict", revision(3), nil).Return(nil),
			)
			return test{
				change:   change(3, sessionpkg.DraftFileContent{Path: "shared/x.ts", Content: "x"}),
				expected: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: 3},
			}
		},
		"invalid_change_is_recorded_with_its_diagnostics": func(t *testing.T, deps dependencies) test {
			claims(deps, draft(2), 2)
			diagnostics := draftsources.Apply(starter, []draftsources.Write{{Path: "dist/x.js"}}, nil).Diagnostics
			gomock.InOrder(
				deps.repo.EXPECT().FindRevision(ctx, tx, versionID, int64(2)).Return(draftsources.Revision{Files: starter}, true, nil),
				deps.repo.EXPECT().CompleteChangeAttempt(ctx, tx, attemptID, "invalid", revision(2), diagnostics).Return(nil),
			)
			return test{
				change: change(2, sessionpkg.DraftFileContent{Path: "dist/x.js"}),
				expected: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeInvalid, Revision: 2, Diagnostics: []sessionpkg.DraftDiagnostic{
					{Path: "dist/x.js", Code: draftsources.CodePathNotAllowed, Message: diagnostics[0].Message},
				}},
			}
		},
		"version_no_longer_a_draft": func(t *testing.T, deps dependencies) test {
			executable := draft(6)
			executable.Status = "EXECUTABLE"
			claims(deps, executable, 6)
			deps.repo.EXPECT().CompleteChangeAttempt(ctx, tx, attemptID, "not_editable", revision(6), nil).Return(nil)
			return test{
				change:   change(6, sessionpkg.DraftFileContent{Path: "shared/x.ts", Content: "x"}),
				expected: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeNotEditable, Revision: 6},
			}
		},
		"retry_replays_the_first_decision": func(t *testing.T, deps dependencies) test {
			outcome := "conflict"
			deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(draft(5), nil)
			deps.repo.EXPECT().ClaimChangeAttempt(ctx, tx, versionID, "change", "key", fingerprint, int64(3), "api").
				Return(uint(0), &internalrepo.ChangeAttempt{ID: attemptID, RequestSHA256: fingerprint, Outcome: &outcome, Revision: revision(4)}, nil)
			return test{
				change:   change(3, sessionpkg.DraftFileContent{Path: "shared/x.ts", Content: "x"}),
				expected: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: 4, Replayed: true},
			}
		},
		"same_key_with_another_change_is_an_idempotency_conflict": func(t *testing.T, deps dependencies) test {
			outcome := "applied"
			deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(draft(5), nil)
			deps.repo.EXPECT().ClaimChangeAttempt(ctx, tx, versionID, "change", "key", fingerprint, int64(3), "api").
				Return(uint(0), &internalrepo.ChangeAttempt{ID: attemptID, RequestSHA256: "other", Outcome: &outcome, Revision: revision(4)}, nil)
			return test{change: change(3), expectErr: true, expectedErrorIs: sessionpkg.ErrIdempotencyConflict}
		},
		"unknown_version": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(nil, nil)
			return test{change: change(1), expectErr: true, expectedErrorIs: sessionpkg.ErrDraftNotFound}
		},
		"missing_current_revision_is_broken_data": func(t *testing.T, deps dependencies) test {
			claims(deps, draft(2), 2)
			deps.repo.EXPECT().FindRevision(ctx, tx, versionID, int64(2)).Return(draftsources.Revision{}, false, nil)
			return test{change: change(2, sessionpkg.DraftFileContent{Path: "shared/x.ts", Content: "x"}), expectErr: true}
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockapplyChangesRepo(ctrl)}
			tc := setup(t, deps)
			m := &Manager{applyChangesRepo: deps.repo, newUUID: func() string { return "build-uuid" }}

			got, err := m.applyChangesInTx(ctx, tx, versionUUID, tc.change, fingerprint)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}
