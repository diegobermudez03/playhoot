package game

import "github.com/diegobermudez03/playhoot/game/language/v1/program"

// visibility types
const (
	Draft   VisibilityType = "draft"
	Private VisibilityType = "private"
	Hidden  VisibilityType = "hidden"
	Public  VisibilityType = "public"
)

type VisibilityType string

type Game struct {
	UUID         string
	Name         string
	Description  string
	OwnerUUID    string
	LogoImageURL string
	Visibility   VisibilityType
	VersionUUID  string
	Definition   program.Definition
}
