package gameversion

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMeasureMedia(t *testing.T) {
	encoded := func(encode func(*bytes.Buffer, image.Image) error) []byte {
		var buffer bytes.Buffer
		require.NoError(t, encode(&buffer, image.NewRGBA(image.Rect(0, 0, 40, 30))))
		return buffer.Bytes()
	}
	pngContent := encoded(func(b *bytes.Buffer, i image.Image) error { return png.Encode(b, i) })
	jpegContent := encoded(func(b *bytes.Buffer, i image.Image) error { return jpeg.Encode(b, i, nil) })

	// A lossless WebP header: 40x30 stored as width-1 and height-1 in 14 bits each.
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8L\x00\x00\x00\x00\x2f"), le32(uint32(39)|uint32(29)<<14)...)
	webp = append(webp, make([]byte, 16)...)

	// One second of 8 kHz 16-bit mono WAV.
	wav := append([]byte("RIFF\x00\x00\x00\x00WAVEfmt "), le32(16)...)
	wav = append(wav, 1, 0, 1, 0)
	wav = append(wav, le32(8000)...)
	wav = append(wav, le32(16000)...)
	wav = append(wav, 2, 0, 16, 0)
	wav = append(wav, "data"...)
	wav = append(wav, le32(16000)...)
	wav = append(wav, make([]byte, 16000)...)

	// An MP4 whose movie header says 2.5 s at timescale 1000.
	mvhd := append(make([]byte, 12), be32(1000)...)
	mvhd = append(mvhd, be32(2500)...)
	mp4 := append(box("ftyp", []byte("isom")), box("moov", box("mvhd", mvhd))...)

	// An Ogg Vorbis stream at 44.1 kHz whose last page ends at granule 88200.
	vorbisID := append([]byte("OggS\x00\x02"), make([]byte, 22)...)
	vorbisID = append(vorbisID, "\x01vorbis"...)
	vorbisID = append(vorbisID, 0, 0, 0, 0, 2)
	vorbisID = append(vorbisID, le32(44100)...)
	lastPage := append([]byte("OggS\x00\x04"), le64(88200)...)
	ogg := append(append(vorbisID, make([]byte, 100)...), append(lastPage, make([]byte, 20)...)...)

	// A constant 128 kbit/s MP3 of 32000 bytes: 2 s.
	mp3 := append([]byte{0xFF, 0xFB, 0x90, 0x00}, make([]byte, 31996)...)

	// A WebM whose Segment Info says 1500.0 at the default 1 ms scale.
	webm := append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x80, 0x44, 0x89, 0x88}, be64(math.Float64bits(1500))...)

	for _, tc := range []struct {
		name     string
		mimeType string
		content  []byte
		expected Media
	}{
		{"png", "image/png", pngContent, Media{Width: 40, Height: 30}},
		{"jpeg", "image/jpeg", jpegContent, Media{Width: 40, Height: 30}},
		{"webp", "image/webp", webp, Media{Width: 40, Height: 30}},
		{"svg_size", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64px" height="48">`), Media{Width: 64, Height: 48}},
		{"svg_view_box", "image/svg+xml", []byte(`<svg viewBox="0 0 120 80" width="100%">`), Media{Width: 120, Height: 80}},
		{"svg_unknown", "image/svg+xml", []byte(`<svg width="10em">`), Media{}},
		{"wav", "audio/wav", wav, Media{DurationMs: 1000}},
		{"mp4", "video/mp4", mp4, Media{DurationMs: 2500}},
		{"ogg", "audio/ogg", ogg, Media{DurationMs: 2000}},
		{"mp3", "audio/mpeg", mp3, Media{DurationMs: 2000}},
		{"webm", "video/webm", webm, Media{DurationMs: 1500}},
		{"truncated", "video/mp4", []byte{0, 0, 0, 99, 'm', 'o', 'o', 'v'}, Media{}},
		{"garbage", "image/png", []byte("nope"), Media{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, MeasureMedia(tc.mimeType, tc.content))
		})
	}
}

func box(name string, body []byte) []byte {
	return append(append(be32(uint32(8+len(body))), name...), body...)
}

func le32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func le64(v uint64) []byte { return binary.LittleEndian.AppendUint64(nil, v) }
func be32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func be64(v uint64) []byte { return binary.BigEndian.AppendUint64(nil, v) }
