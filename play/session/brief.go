package session

import "github.com/diegobermudez03/playhoot/play/session/internal/business/gamebrief"

// BriefPath is where a project keeps its brief (playhoot.brief.v1): the game
// agreed with the person and the phase the work is in.
const BriefPath = gamebrief.Path

// BriefPhase is the phase content, a brief.json, says the work is in
// (brief, foundation, building, testing, assets, ready; brief when it names
// none), and false when content is not a valid brief.
func BriefPhase(content string) (string, bool) {
	brief, problems := gamebrief.Parse([]byte(content))
	if len(problems) > 0 {
		return "", false
	}
	return brief.Phase, true
}
