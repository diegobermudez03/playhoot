// Package sdkguide reads the SDK guide for agents from the toolchain
// checkout Play builds with, so the guide an agent reads describes the SDK
// its builds use.
package sdkguide

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// Dir is where the guide lives in the toolchain checkout (the frontend
// repository): the same files the web app serves.
const Dir = "apps/web/public/docs/sdk/v1"

// MaxPageBytes bounds a page read.
const MaxPageBytes = 256 * 1024

// pageName is what a page may be called: a Markdown file at the guide's top,
// or one folder down (examples/quiz.md).
var pageName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*(/[a-z0-9][a-z0-9-]*)?\.md$`)

// Guide reads the guide of the toolchain at root; an empty root has none.
type Guide struct {
	root string
}

// New returns the guide of the toolchain checkout at root.
func New(root string) Guide {
	return Guide{root: root}
}

// Read returns page's Markdown: sessionpkg.ErrSDKGuidePageNotFound for a
// name that is not a page, sessionpkg.ErrSDKGuideUnavailable without a
// toolchain.
func (g Guide) Read(page string) (string, error) {
	if !pageName.MatchString(page) {
		return "", fmt.Errorf("page %q: %w", page, sessionpkg.ErrSDKGuidePageNotFound)
	}
	if g.root == "" {
		return "", sessionpkg.ErrSDKGuideUnavailable
	}
	path := filepath.Join(g.root, filepath.FromSlash(Dir), filepath.FromSlash(page))
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if _, dirErr := os.Stat(filepath.Join(g.root, filepath.FromSlash(Dir))); dirErr != nil {
			return "", fmt.Errorf("%w: %v", sessionpkg.ErrSDKGuideUnavailable, dirErr)
		}
		return "", fmt.Errorf("page %q: %w", page, sessionpkg.ErrSDKGuidePageNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", sessionpkg.ErrSDKGuideUnavailable, err)
	}
	if info.IsDir() || info.Size() > MaxPageBytes {
		return "", fmt.Errorf("page %q: %w", page, sessionpkg.ErrSDKGuidePageNotFound)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", sessionpkg.ErrSDKGuideUnavailable, err)
	}
	return string(content), nil
}
