package draftsources

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
)

// CodeInvalidLanguages is the diagnostic of a missing or invalid "languages".
const CodeInvalidLanguages = "invalid_languages"

// DeclaredLanguages reads the languages game.project.json declares the game
// is written in, its main one first: one or more of Playhoot's languages,
// without repeats. Diagnostics name what makes them invalid; then the result
// is nil. The frontend's game tools apply the same rule.
func DeclaredLanguages(projectFile string) ([]gameversion.Language, []Diagnostic) {
	var project struct {
		Languages json.RawMessage `json:"languages"`
	}
	if err := json.Unmarshal([]byte(projectFile), &project); err != nil {
		return nil, []Diagnostic{{Path: ProjectFile, Code: CodeInvalidProjectFile, Message: fmt.Sprintf("%s is not a JSON object: %s", ProjectFile, err)}}
	}
	known := make([]string, len(gameversion.Languages))
	for i, language := range gameversion.Languages {
		known[i] = string(language)
	}
	message := fmt.Sprintf(`"languages" lists the languages the game is written in, its main one first: one or more of %s`, strings.Join(known, ", "))
	refuse := func(detail string) ([]gameversion.Language, []Diagnostic) {
		return nil, []Diagnostic{{Path: ProjectFile, Code: CodeInvalidLanguages, Message: message + detail}}
	}
	var declared []string
	if len(project.Languages) == 0 || json.Unmarshal(project.Languages, &declared) != nil || len(declared) == 0 {
		return refuse("")
	}
	languages := make([]gameversion.Language, 0, len(declared))
	for _, tag := range declared {
		language := gameversion.Language(tag)
		if !gameversion.ValidLanguage(language) {
			return refuse(fmt.Sprintf("; %q is not one of them", tag))
		}
		for _, seen := range languages {
			if seen == language {
				return refuse(fmt.Sprintf("; %q is listed twice", tag))
			}
		}
		languages = append(languages, language)
	}
	return languages, nil
}
