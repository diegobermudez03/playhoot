package draftsources

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
)

func TestFromTemplate(t *testing.T) {
	draft := Revision{
		Files: map[string]File{
			"notes.md":       {SHA256: "draft-notes"},
			"brief.json":     {SHA256: "draft-brief"},
			"server/game.ts": {SHA256: "draft-server"},
		},
		Assets: map[string]AssetBinding{
			"cardBack": {SHA256: "draft-back", MIMEType: "image/png"},
			"flip":     {SHA256: "draft-flip", MIMEType: "image/png"},
			"logo":     {SHA256: "draft-logo", MIMEType: "image/png"},
		},
	}
	template := Revision{
		Files: map[string]File{
			"server/game.ts":    {SHA256: "template-server"},
			"game.project.json": {SHA256: "template-project"},
			"notes.md":          {SHA256: "template-notes"},
		},
		Assets: map[string]AssetBinding{"cardBack": {SHA256: "template-back", MIMEType: "image/webp"}},
	}
	declared := map[string]AssetDeclaration{
		"cardBack": {Type: gameversion.AssetImage},
		"flip":     {Type: gameversion.AssetAudio},
	}

	switched := FromTemplate(draft, template, declared)

	require.Equal(t, map[string]File{
		"server/game.ts":    {SHA256: "template-server"},
		"game.project.json": {SHA256: "template-project"},
		"notes.md":          {SHA256: "draft-notes"},
		"brief.json":        {SHA256: "draft-brief"},
	}, switched.Files, "the template's code with the draft's notes and brief")
	require.Equal(t, map[string]AssetBinding{
		"cardBack": {SHA256: "draft-back", MIMEType: "image/png"},
	}, switched.Assets, "the person's upload for a key the template declares with the same media; nothing undeclared or of another kind")

	bare := FromTemplate(Revision{}, template, nil)
	require.Equal(t, template.Files, bare.Files)
	require.Equal(t, template.Assets, bare.Assets)
}
