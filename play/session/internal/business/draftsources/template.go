package draftsources

import (
	"strings"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/gamebrief"
)

// keptFromDraft are the files a draft keeps when it switches to a template:
// what was agreed with the person and what is worth knowing, not the code.
var keptFromDraft = []string{"notes.md", gamebrief.Path}

// FromTemplate is the revision a draft becomes when it starts over from a
// template: the template's files and asset bindings, but the draft's
// notes.md and brief.json when it has them, and the draft's upload for each
// key the template declares (declared, from its game.project.json) with the
// same kind of media, so nothing the person uploaded is lost.
func FromTemplate(draft Revision, template Revision, declared map[string]AssetDeclaration) Revision {
	files := make(map[string]File, len(template.Files))
	for filePath, file := range template.Files {
		files[filePath] = file
	}
	for _, kept := range keptFromDraft {
		if file, found := draft.Files[kept]; found {
			files[kept] = file
		}
	}
	assets := make(map[string]AssetBinding, len(template.Assets))
	for key, binding := range template.Assets {
		assets[key] = binding
	}
	for key, declaration := range declared {
		uploaded, found := draft.Assets[key]
		if found && strings.HasPrefix(uploaded.MIMEType, string(declaration.Type)+"/") {
			assets[key] = uploaded
		}
	}
	return Revision{Files: files, Assets: assets}
}
