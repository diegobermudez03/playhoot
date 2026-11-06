package versiondrafting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

func TestProvenance(t *testing.T) {
	version := &internalrepo.Version{ScriptProfile: "sdk-v1"}
	four, two := int64(4), int64(2)

	require.Equal(t, internalrepo.Provenance{Kind: "change", Origin: "web", Actor: "person", ParentRevision: &four, SDK: "sdk-v1"}, written(version, sessionpkg.DraftRevisionChange, 4, sessionpkg.DraftOriginWeb))
	require.Equal(t, "ai", written(version, sessionpkg.DraftRevisionAsset, 4, sessionpkg.DraftOriginAPI).Actor)
	require.Equal(t, "ai", written(version, sessionpkg.DraftRevisionAsset, 4, sessionpkg.DraftOriginMCP).Actor)
	require.Equal(t, internalrepo.Provenance{Kind: "restore", Origin: "mcp", Actor: "ai", ParentRevision: &four, RestoredFrom: &two, SDK: "sdk-v1"}, restoreProvenance(version, 4, 2, sessionpkg.DraftOriginMCP))
}

func TestPublicRevision(t *testing.T) {
	at := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	three := int64(3)

	edition := publicRevision(internalrepo.Revision{Number: 1, Provenance: internalrepo.Provenance{Kind: "edition", Actor: "platform", FromGameUUID: "g", FromVersionUUID: "v", FromRevision: &three, SDK: "sdk-v1"}, Files: 5, CreatedAt: at})
	require.Equal(t, sessionpkg.DraftRevision{Number: 1, Kind: "edition", Actor: "platform", From: &sessionpkg.DraftRevisionFrom{GameUUID: "g", VersionUUID: "v", Revision: &three}, SDK: "sdk-v1", Files: 5, CreatedAt: at}, edition)

	change := publicRevision(internalrepo.Revision{Number: 2, Provenance: internalrepo.Provenance{Kind: "change", Origin: "web", Actor: "person", SDK: "sdk-v1"}, Files: 5, CreatedAt: at})
	require.Nil(t, change.From, "no version to come from")
}
