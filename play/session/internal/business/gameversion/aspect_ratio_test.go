package gameversion

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAspectRatioResolution(t *testing.T) {
	type test struct {
		ratio AspectRatio

		expected   Resolution
		expectedOk bool
	}

	tests := map[string]func(t *testing.T) test{
		"landscape": func(t *testing.T) test {
			return test{ratio: AspectRatio16x9, expected: Resolution{Width: 1920, Height: 1080}, expectedOk: true}
		},
		"portrait": func(t *testing.T) test {
			return test{ratio: AspectRatio9x16, expected: Resolution{Width: 1080, Height: 1920}, expectedOk: true}
		},
		"classic_landscape": func(t *testing.T) test {
			return test{ratio: AspectRatio4x3, expected: Resolution{Width: 1440, Height: 1080}, expectedOk: true}
		},
		"classic_portrait": func(t *testing.T) test {
			return test{ratio: AspectRatio3x4, expected: Resolution{Width: 1080, Height: 1440}, expectedOk: true}
		},
		"square": func(t *testing.T) test {
			return test{ratio: AspectRatio1x1, expected: Resolution{Width: 1080, Height: 1080}, expectedOk: true}
		},
		"not_offered": func(t *testing.T) test { return test{ratio: AspectRatio("21:9")} },
		"empty":       func(t *testing.T) test { return test{ratio: AspectRatio("")} },
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			resolution, ok := tc.ratio.Resolution()

			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expected, resolution)
		})
	}
}
