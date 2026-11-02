package gameversion

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSniffMIMEType(t *testing.T) {
	tests := map[string]struct {
		content  []byte
		expected string
	}{
		"png":             {content: append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...), expected: "image/png"},
		"jpeg":            {content: []byte("\xff\xd8\xff\xe0\x00\x10JFIF"), expected: "image/jpeg"},
		"webp":            {content: []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), expected: "image/webp"},
		"svg":             {content: []byte(`<?xml version="1.0"?><!-- logo --><svg xmlns="http://www.w3.org/2000/svg"></svg>`), expected: "image/svg+xml"},
		"wav":             {content: []byte("RIFF\x00\x00\x00\x00WAVEfmt "), expected: "audio/wav"},
		"ogg":             {content: []byte("OggS\x00\x02\x00\x00"), expected: "audio/ogg"},
		"mp3_with_id3":    {content: []byte("ID3\x03\x00\x00\x00"), expected: "audio/mpeg"},
		"mp3_frame":       {content: []byte{0xFF, 0xFB, 0x90, 0x00}, expected: "audio/mpeg"},
		"mp4":             {content: []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), expected: "video/mp4"},
		"webm":            {content: []byte("\x1a\x45\xdf\xa3\x01\x00\x00\x00\x00\x00\x00\x1f\x42\x86\x81\x01"), expected: "video/webm"},
		"html_is_not_svg": {content: []byte("<html><svg></svg></html>"), expected: ""},
		"plain_text":      {content: []byte("hello"), expected: ""},
		"empty":           {content: nil, expected: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, SniffMIMEType(tc.content))
		})
	}
}
