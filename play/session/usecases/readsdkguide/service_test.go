package readsdkguide

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

type fakePages map[string]string

func (f fakePages) Read(page string) (string, error) {
	content, found := f[page]
	if !found {
		return "", sessionpkg.ErrSDKGuidePageNotFound
	}
	return content, nil
}

type fakeToolchain struct {
	version string
	err     error
}

func (f fakeToolchain) Version(ctx context.Context) (string, error) { return f.version, f.err }

func TestReadPage(t *testing.T) {
	ctx := context.Background()
	pages := fakePages{"capabilities.md": "# Capabilities\n"}

	page, err := (&Service{pages: pages, toolchain: fakeToolchain{version: "tc-1"}}).ReadPage(ctx, sessionpkg.SDKGuideIndex)
	require.NoError(t, err)
	require.Equal(t, sessionpkg.SDKGuidePage{Name: "capabilities.md", Content: "# Capabilities\n", ToolchainVersion: "tc-1"}, page)

	page, err = (&Service{pages: pages, toolchain: fakeToolchain{err: errors.New("unreadable")}}).ReadPage(ctx, "capabilities.md")
	require.NoError(t, err, "the guide is answered without the toolchain's version")
	require.Empty(t, page.ToolchainVersion)

	_, err = (&Service{pages: pages, toolchain: fakeToolchain{}}).ReadPage(ctx, "nope.md")
	require.ErrorIs(t, err, sessionpkg.ErrSDKGuidePageNotFound)
}
