package gameversion

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// The SDK's list must be the same: a game declares its languages against it,
// and this domain validates them against Languages.
func TestLanguagesMatchTheSDK(t *testing.T) {
	source, err := os.ReadFile("../../../../../../playhoot_frontend/packages/game-core/src/index.ts")
	if os.IsNotExist(err) {
		t.Skip("the frontend checkout is not next to this one")
	}
	require.NoError(t, err)
	match := regexp.MustCompile(`gameLanguages = (\[[^\]]*\]) as const`).FindSubmatch(source)
	require.NotNil(t, match, "game-core no longer declares gameLanguages as an array literal")
	var sdk []Language
	require.NoError(t, json.Unmarshal(regexp.MustCompile(`'`).ReplaceAll(match[1], []byte(`"`)), &sdk))
	require.Equal(t, Languages, sdk)
}
