// Package gametoolchain opens the game toolchain the Session domain builds
// SDK projects with.
package gametoolchain

import "github.com/diegobermudez03/playhoot/play/session/internal/gamebuild"

// Builder builds SDK projects into a version's content.
type Builder = gamebuild.Builder

// Open returns the toolchain of the frontend repository checkout at root,
// run with Node. An empty root means no toolchain is configured: every build
// then fails as the platform's, never the game's.
func Open(root string) Builder {
	return gamebuild.NewNode(root)
}
