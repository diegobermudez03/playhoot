package sdkguide

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestRead(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(Dir))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "examples"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "capabilities.md"), []byte("# Capabilities\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "examples", "quiz.md"), []byte("# Quiz\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret.md"), []byte("no"), 0o644))
	guide := New(root)

	page, err := guide.Read("capabilities.md")
	require.NoError(t, err)
	require.Equal(t, "# Capabilities\n", page)
	page, err = guide.Read("examples/quiz.md")
	require.NoError(t, err)
	require.Equal(t, "# Quiz\n", page)

	for _, name := range []string{"missing.md", "../../../../../secret.md", "examples", "/etc/passwd", "a/b/c.md", "Index.md", "x.txt", ""} {
		_, err := guide.Read(name)
		require.ErrorIs(t, err, sessionpkg.ErrSDKGuidePageNotFound, name)
	}

	_, err = New("").Read("index.md")
	require.ErrorIs(t, err, sessionpkg.ErrSDKGuideUnavailable)
	_, err = New(t.TempDir()).Read("index.md")
	require.ErrorIs(t, err, sessionpkg.ErrSDKGuideUnavailable, "a checkout without the guide")
}
