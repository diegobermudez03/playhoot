package gameversion

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// validSHA256 is a well-formed digest for assets whose content does not matter.
var validSHA256 = strings.Repeat("a", 64)

func TestValidateAsset(t *testing.T) {
	type test struct {
		asset Asset

		expectErr bool
	}

	tests := map[string]func(t *testing.T) test{
		"png_image": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "ship", Type: AssetImage, MIMEType: "image/png", Size: 120}}
		},
		"svg_sprite_sheet": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "ship_sheet_2", Type: AssetImage, MIMEType: "image/svg+xml", Size: 5 << 20}}
		},
		"wav_audio": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "thrust", Type: AssetAudio, MIMEType: "audio/wav", Size: 10 << 20}}
		},
		"webm_video": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "Intro", Type: AssetVideo, MIMEType: "video/webm", Size: 50 << 20}}
		},
		"key_of_64_characters": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "a" + strings.Repeat("b", 63), Type: AssetImage, MIMEType: "image/png", Size: 1}}
		},
		"key_of_65_characters": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "a" + strings.Repeat("b", 64), Type: AssetImage, MIMEType: "image/png", Size: 1}, expectErr: true}
		},
		"key_starting_with_a_digit": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "1ship", Type: AssetImage, MIMEType: "image/png", Size: 1}, expectErr: true}
		},
		"key_with_a_dash": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "space-ship", Type: AssetImage, MIMEType: "image/png", Size: 1}, expectErr: true}
		},
		"empty_key": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Type: AssetImage, MIMEType: "image/png", Size: 1}, expectErr: true}
		},
		"unknown_type": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "font", Type: AssetType("font"), MIMEType: "font/woff2", Size: 1}, expectErr: true}
		},
		"mime_type_of_another_type": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "ship", Type: AssetImage, MIMEType: "audio/wav", Size: 1}, expectErr: true}
		},
		"mime_type_not_offered": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "ship", Type: AssetImage, MIMEType: "image/gif", Size: 1}, expectErr: true}
		},
		"sha256_in_uppercase": func(t *testing.T) test {
			return test{asset: Asset{SHA256: strings.Repeat("A", 64), Key: "ship", Type: AssetImage, MIMEType: "image/png", Size: 1}, expectErr: true}
		},
		"sha256_too_short": func(t *testing.T) test {
			return test{asset: Asset{SHA256: "abc", Key: "ship", Type: AssetImage, MIMEType: "image/png", Size: 1}, expectErr: true}
		},
		"image_over_its_limit": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "ship", Type: AssetImage, MIMEType: "image/png", Size: 5<<20 + 1}, expectErr: true}
		},
		"audio_over_its_limit": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "thrust", Type: AssetAudio, MIMEType: "audio/ogg", Size: 10<<20 + 1}, expectErr: true}
		},
		"video_over_its_limit": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "intro", Type: AssetVideo, MIMEType: "video/mp4", Size: 50<<20 + 1}, expectErr: true}
		},
		"empty_file": func(t *testing.T) test {
			return test{asset: Asset{SHA256: validSHA256, Key: "ship", Type: AssetImage, MIMEType: "image/png"}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			err := ValidateAsset(tc.asset)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateAssets(t *testing.T) {
	type test struct {
		assets []Asset

		expectErr bool
	}

	video := func(key string) Asset {
		return Asset{SHA256: validSHA256, Key: key, Type: AssetVideo, MIMEType: "video/mp4", Size: 50 << 20}
	}

	tests := map[string]func(t *testing.T) test{
		"no_assets": func(t *testing.T) test { return test{} },
		"assets_at_the_total_limit": func(t *testing.T) test {
			return test{assets: []Asset{video("a"), video("b")}}
		},
		"assets_over_the_total_limit": func(t *testing.T) test {
			return test{assets: []Asset{video("a"), video("b"), {SHA256: validSHA256, Key: "c", Type: AssetImage, MIMEType: "image/png", Size: 1}}, expectErr: true}
		},
		"repeated_key": func(t *testing.T) test {
			return test{assets: []Asset{{SHA256: validSHA256, Key: "a", Type: AssetImage, MIMEType: "image/png", Size: 1}, {SHA256: validSHA256, Key: "a", Type: AssetAudio, MIMEType: "audio/wav", Size: 1}}, expectErr: true}
		},
		"one_invalid_asset": func(t *testing.T) test {
			return test{assets: []Asset{{SHA256: validSHA256, Key: "a", Type: AssetImage, MIMEType: "image/png", Size: 1}, {SHA256: validSHA256, Key: "b", Type: AssetImage, MIMEType: "image/gif", Size: 1}}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			err := ValidateAssets(tc.assets)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
