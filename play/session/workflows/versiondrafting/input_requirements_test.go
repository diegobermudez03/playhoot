package versiondrafting

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputrequirements"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

func TestVersionInputRequirements(t *testing.T) {
	yes, no := true, false
	built := func(keyboard, hover, press *bool) *internalrepo.BuiltArtifacts {
		return &internalrepo.BuiltArtifacts{Revision: 3, ScriptProfile: string(scriptcheck.ProfileSDKV1), UsesKeyboard: keyboard, UsesPointerHover: hover, UsesPointerPress: press}
	}

	t.Run("a client of SDK controls plays on desktop and mobile", func(t *testing.T) {
		requirements, err := versionInputRequirements(built(&no, &no, &yes))
		require.NoError(t, err)
		require.Equal(t, internalrepo.InputRequirements{UsesPointerPress: true, Devices: string(inputrequirements.DevicesBoth)}, requirements)
	})

	t.Run("a client that needs a keyboard plays on desktop only", func(t *testing.T) {
		requirements, err := versionInputRequirements(built(&yes, &no, &no))
		require.NoError(t, err)
		require.Equal(t, string(inputrequirements.DevicesDesktop), requirements.Devices)
	})

	t.Run("a build without requirements is broken data", func(t *testing.T) {
		_, err := versionInputRequirements(built(nil, nil, nil))
		require.Error(t, err)
	})
}
