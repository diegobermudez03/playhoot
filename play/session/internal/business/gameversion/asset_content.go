package gameversion

import (
	"bytes"
	"mime"
	"net/http"
)

// SniffMIMEType is the media type content really holds, read from its bytes
// and never from a name or a declared header, among the types the asset
// catalog knows; "" when it is none of them.
func SniffMIMEType(content []byte) string {
	if len(content) == 0 {
		return ""
	}
	if isSVG(content) {
		return "image/svg+xml"
	}
	if isMP3Frame(content) {
		return "audio/mpeg"
	}
	detected, _, err := mime.ParseMediaType(http.DetectContentType(content))
	if err != nil {
		return ""
	}
	switch detected {
	case "image/png", "image/jpeg", "image/webp", "audio/mpeg", "video/mp4", "video/webm":
		return detected
	case "audio/wave":
		return "audio/wav"
	case "application/ogg":
		return "audio/ogg"
	default:
		return ""
	}
}

// isSVG reports whether content is an SVG document: text whose first element,
// after an XML declaration, comments or a doctype, is <svg.
func isSVG(content []byte) bool {
	text := bytes.TrimLeft(content, "\ufeff \t\r\n")
	for {
		switch {
		case bytes.HasPrefix(text, []byte("<?")):
			end := bytes.Index(text, []byte("?>"))
			if end < 0 {
				return false
			}
			text = bytes.TrimLeft(text[end+2:], " \t\r\n")
		case bytes.HasPrefix(text, []byte("<!--")):
			end := bytes.Index(text, []byte("-->"))
			if end < 0 {
				return false
			}
			text = bytes.TrimLeft(text[end+3:], " \t\r\n")
		case bytes.HasPrefix(text, []byte("<!")):
			end := bytes.IndexByte(text, '>')
			if end < 0 {
				return false
			}
			text = bytes.TrimLeft(text[end+1:], " \t\r\n")
		default:
			return bytes.HasPrefix(text, []byte("<svg")) && len(text) > 4 && (text[4] == ' ' || text[4] == '>' || text[4] == '\n' || text[4] == '\t' || text[4] == '\r')
		}
	}
}

// isMP3Frame reports whether content starts with an MPEG audio frame header
// without an ID3 tag, which the standard sniffer does not recognize.
func isMP3Frame(content []byte) bool {
	return len(content) >= 2 && content[0] == 0xFF && content[1]&0xE0 == 0xE0 && content[1]&0x06 != 0
}
