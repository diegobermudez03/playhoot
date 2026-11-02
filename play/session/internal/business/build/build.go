// Package build defines a DRAFT revision's build: its status lifecycle, the
// stages its diagnostics come from, and the hash of everything it is built
// from.
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
)

// Status is where a build is in its lifecycle: queued, running, then one of
// the terminal statuses. A running build goes back to queued only when the
// instance running it stopped.
type Status string

const (
	Queued    Status = "QUEUED"
	Running   Status = "RUNNING"
	Succeeded Status = "SUCCEEDED"
	Failed    Status = "FAILED"
	// Superseded is a queued build of a revision a newer one replaced
	// before it ran.
	Superseded Status = "SUPERSEDED"
)

// Terminal reports whether s never changes again.
func (s Status) Terminal() bool {
	return s == Succeeded || s == Failed || s == Superseded
}

// Known reports whether s is a status of the catalog.
func (s Status) Known() bool {
	return s == Queued || s == Running || s.Terminal()
}

// Stage is the step of a build a diagnostic comes from. The first five are
// the game tools' own (@playhoot/game-tools); the rest are the backend's.
type Stage string

const (
	StageProject Stage = "project"
	StageSource  Stage = "source"
	StageTypes   Stage = "types"
	StageBundle  Stage = "bundle"
	StageRuntime Stage = "runtime"
	// StageSandbox is the backend script refused by scriptcheck's profile.
	StageSandbox Stage = "sandbox"
	// StageManifest is a manifest the runtime would not accept.
	StageManifest Stage = "manifest"
	// StageInfrastructure is the platform failing, never the game: the
	// author has nothing to fix.
	StageInfrastructure Stage = "infrastructure"
)

// Codes of infrastructure diagnostics.
const (
	CodeToolchainUnavailable = "toolchain_unavailable"
	CodeTimeout              = "build_timeout"
	CodeToolchainFailed      = "toolchain_failed"
	CodeStorageFailed        = "storage_failed"
	CodeInternal             = "internal_error"
)

// Diagnostic is one problem a build found, located in a source file when it
// can be.
type Diagnostic struct {
	Stage   Stage  `json:"stage"`
	Code    string `json:"code,omitempty"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

// InputsSHA256 is the hash of everything a build of revision with toolchain
// produces its output from: equal hashes build the same artifacts.
func InputsSHA256(revision draftsources.Revision, toolchainVersion string) string {
	type entry struct {
		Path   string `json:"p"`
		SHA256 string `json:"h"`
	}
	files := make([]entry, 0, len(revision.Files))
	for path, file := range revision.Files {
		files = append(files, entry{Path: path, SHA256: file.SHA256})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	assets := make([]entry, 0, len(revision.Assets))
	for key, binding := range revision.Assets {
		assets = append(assets, entry{Path: key, SHA256: binding.SHA256})
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	encoded, _ := json.Marshal(struct {
		Toolchain string  `json:"t"`
		Files     []entry `json:"f"`
		Assets    []entry `json:"a"`
	}{toolchainVersion, files, assets})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
