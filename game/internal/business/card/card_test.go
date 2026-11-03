package card

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTexts(t *testing.T) {
	name, err := NormalizeName("  Memoria  ")
	require.NoError(t, err)
	require.Equal(t, "Memoria", name)
	_, err = NormalizeName(strings.Repeat("ñ", MaxNameRunes+1))
	require.ErrorIs(t, err, ErrInvalidText)
	_, err = NormalizeName(strings.Repeat("ñ", MaxNameRunes))
	require.NoError(t, err)
	_, err = NormalizeDescription(strings.Repeat("a", MaxDescriptionRunes+1))
	require.ErrorIs(t, err, ErrInvalidText)
}

func TestImageType(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	detected, err := ImageType(png)
	require.NoError(t, err)
	require.Equal(t, "image/png", detected)
	_, err = ImageType([]byte("GIF89a......"))
	require.ErrorIs(t, err, ErrInvalidImage)
	_, err = ImageType([]byte("<svg xmlns='http://www.w3.org/2000/svg'/>"))
	require.ErrorIs(t, err, ErrInvalidImage)
}

func TestPublishBlockers(t *testing.T) {
	require.Equal(t, []PublishBlocker{NameRequired}, PublishBlockers("  "))
	require.Empty(t, PublishBlockers("Memoria"))
	require.Equal(t, int64(2<<20), MaxImageBytes(Logo))
	require.Equal(t, int64(5<<20), MaxImageBytes(Photo))
}
