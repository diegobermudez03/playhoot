package gameversion

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/jpeg" // registers JPEG for image.DecodeConfig
	_ "image/png"  // registers PNG for image.DecodeConfig
	"math"
	"regexp"
	"strconv"
)

// Media is what an asset's content measures, for agents and people to
// choose and place it without reading its bytes: an image's width and height
// in pixels (an SVG's in its own units), and an audio or video's duration.
// A zero is unknown, never an error: the content was already accepted.
type Media struct {
	Width      int
	Height     int
	DurationMs int64
}

// MeasureMedia reads what content of mimeType measures from its headers.
func MeasureMedia(mimeType string, content []byte) Media {
	switch mimeType {
	case "image/png", "image/jpeg":
		config, _, err := image.DecodeConfig(bytes.NewReader(content))
		if err != nil {
			return Media{}
		}
		return Media{Width: config.Width, Height: config.Height}
	case "image/webp":
		return webpSize(content)
	case "image/svg+xml":
		return svgSize(content)
	case "audio/wav":
		return Media{DurationMs: wavDuration(content)}
	case "video/mp4":
		return Media{DurationMs: mp4Duration(content)}
	case "audio/ogg":
		return Media{DurationMs: oggDuration(content)}
	case "audio/mpeg":
		return Media{DurationMs: mp3Duration(content)}
	case "video/webm":
		return Media{DurationMs: webmDuration(content)}
	default:
		return Media{}
	}
}

// webpSize reads a WebP's canvas from its first chunk (VP8X, VP8 or VP8L).
func webpSize(content []byte) Media {
	if len(content) < 30 || string(content[0:4]) != "RIFF" || string(content[8:12]) != "WEBP" {
		return Media{}
	}
	chunk := content[12:]
	switch string(chunk[0:4]) {
	case "VP8X":
		width := int(chunk[12]) | int(chunk[13])<<8 | int(chunk[14])<<16
		height := int(chunk[15]) | int(chunk[16])<<8 | int(chunk[17])<<16
		return Media{Width: width + 1, Height: height + 1}
	case "VP8 ":
		if chunk[11] != 0x9d || chunk[12] != 0x01 || chunk[13] != 0x2a {
			return Media{}
		}
		return Media{Width: int(binary.LittleEndian.Uint16(chunk[14:16]) & 0x3fff), Height: int(binary.LittleEndian.Uint16(chunk[16:18]) & 0x3fff)}
	case "VP8L":
		if chunk[8] != 0x2f {
			return Media{}
		}
		bits := binary.LittleEndian.Uint32(chunk[9:13])
		return Media{Width: int(bits&0x3fff) + 1, Height: int((bits>>14)&0x3fff) + 1}
	default:
		return Media{}
	}
}

var (
	svgTag     = regexp.MustCompile(`(?s)<svg\b[^>]*>`)
	svgWidth   = regexp.MustCompile(`\swidth\s*=\s*["']\s*([0-9.]+)\s*(px)?\s*["']`)
	svgHeight  = regexp.MustCompile(`\sheight\s*=\s*["']\s*([0-9.]+)\s*(px)?\s*["']`)
	svgViewBox = regexp.MustCompile(`\sviewBox\s*=\s*["']\s*[-0-9.]+[\s,]+[-0-9.]+[\s,]+([0-9.]+)[\s,]+([0-9.]+)\s*["']`)
)

// svgSize is an SVG's width and height attributes in pixels, else its
// viewBox's size; relative sizes (percentages, em) are unknown.
func svgSize(content []byte) Media {
	tag := svgTag.Find(content)
	if tag == nil {
		return Media{}
	}
	width, height := svgNumber(svgWidth.FindSubmatch(tag)), svgNumber(svgHeight.FindSubmatch(tag))
	if width > 0 && height > 0 {
		return Media{Width: width, Height: height}
	}
	if box := svgViewBox.FindSubmatch(tag); box != nil {
		return Media{Width: svgNumber(box[:2]), Height: svgNumber([][]byte{nil, box[2]})}
	}
	return Media{}
}

func svgNumber(match [][]byte) int {
	if len(match) < 2 {
		return 0
	}
	value, err := strconv.ParseFloat(string(match[1]), 64)
	if err != nil || value <= 0 || value > math.MaxInt32 {
		return 0
	}
	return int(math.Round(value))
}

// wavDuration is a WAV's data size over its byte rate.
func wavDuration(content []byte) int64 {
	if len(content) < 12 || string(content[0:4]) != "RIFF" || string(content[8:12]) != "WAVE" {
		return 0
	}
	var byteRate uint32
	for at := 12; at+8 <= len(content); {
		id, size := string(content[at:at+4]), int(binary.LittleEndian.Uint32(content[at+4:at+8]))
		body := at + 8
		switch {
		case id == "fmt " && body+12 <= len(content):
			byteRate = binary.LittleEndian.Uint32(content[body+8 : body+12])
		case id == "data" && byteRate > 0:
			return int64(size) * 1000 / int64(byteRate)
		}
		if size < 0 || body+size < body {
			return 0
		}
		at = body + size + size%2
	}
	return 0
}

// mp4Duration is the movie header's duration over its timescale.
func mp4Duration(content []byte) int64 {
	moov := mp4Box(content, "moov")
	mvhd := mp4Box(moov, "mvhd")
	if len(mvhd) < 4 {
		return 0
	}
	var timescale, duration uint64
	if mvhd[0] == 1 {
		if len(mvhd) < 32 {
			return 0
		}
		timescale, duration = uint64(binary.BigEndian.Uint32(mvhd[20:24])), binary.BigEndian.Uint64(mvhd[24:32])
	} else {
		if len(mvhd) < 20 {
			return 0
		}
		timescale, duration = uint64(binary.BigEndian.Uint32(mvhd[12:16])), uint64(binary.BigEndian.Uint32(mvhd[16:20]))
	}
	if timescale == 0 || duration > math.MaxInt64/1000 {
		return 0
	}
	return int64(duration * 1000 / timescale)
}

// mp4Box is the body of the first box named name among content's boxes.
func mp4Box(content []byte, name string) []byte {
	for at := 0; at+8 <= len(content); {
		size, header := uint64(binary.BigEndian.Uint32(content[at:at+4])), 8
		if size == 1 {
			if at+16 > len(content) {
				return nil
			}
			size, header = binary.BigEndian.Uint64(content[at+8:at+16]), 16
		} else if size == 0 {
			size = uint64(len(content) - at)
		}
		if size < uint64(header) || size > uint64(len(content)-at) {
			return nil
		}
		if string(content[at+4:at+8]) == name {
			return content[at+header : at+int(size)]
		}
		at += int(size)
	}
	return nil
}

// oggDuration is the last page's granule position over the stream's sample
// rate (Vorbis declares it; Opus always counts at 48 kHz).
func oggDuration(content []byte) int64 {
	var rate uint64
	switch {
	case bytes.Contains(content[:min(len(content), 512)], []byte("OpusHead")):
		rate = 48000
	default:
		at := bytes.Index(content[:min(len(content), 512)], []byte("\x01vorbis"))
		if at < 0 || at+16 > len(content) {
			return 0
		}
		rate = uint64(binary.LittleEndian.Uint32(content[at+12 : at+16]))
	}
	last := bytes.LastIndex(content, []byte("OggS"))
	if rate == 0 || last < 0 || last+14 > len(content) {
		return 0
	}
	granule := binary.LittleEndian.Uint64(content[last+6 : last+14])
	if granule == math.MaxUint64 || granule > math.MaxInt64/1000 {
		return 0
	}
	return int64(granule * 1000 / rate)
}

// mp3Bitrates are MPEG-1 Layer III bitrates in kbit/s by header index;
// mp3Rates the MPEG-1 sample rates.
var (
	mp3Bitrates = [16]int64{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	mp3Rates    = [4]int64{44100, 48000, 32000, 0}
)

// mp3Duration is an MPEG-1 Layer III file's duration: from its Xing/Info
// frame count when it has one, else estimated from the first frame's
// bitrate (exact for constant bitrate).
func mp3Duration(content []byte) int64 {
	audio := content
	if len(audio) >= 10 && string(audio[0:3]) == "ID3" {
		size := int(audio[6]&0x7f)<<21 | int(audio[7]&0x7f)<<14 | int(audio[8]&0x7f)<<7 | int(audio[9]&0x7f)
		if 10+size > len(audio) {
			return 0
		}
		audio = audio[10+size:]
	}
	if len(audio) < 4 || audio[0] != 0xFF || audio[1]&0xFE != 0xFA {
		return 0 // only MPEG-1 Layer III is measured
	}
	bitrate, rate := mp3Bitrates[audio[2]>>4], mp3Rates[(audio[2]>>2)&0x3]
	if bitrate == 0 || rate == 0 {
		return 0
	}
	// A Xing (variable bitrate) or Info (constant) header in the first frame
	// may count the frames.
	head := audio[:min(len(audio), 64)]
	tag := bytes.Index(head, []byte("Xing"))
	if tag < 0 {
		tag = bytes.Index(head, []byte("Info"))
	}
	if tag >= 0 && tag+12 <= len(audio) && binary.BigEndian.Uint32(audio[tag+4:tag+8])&1 == 1 {
		frames := int64(binary.BigEndian.Uint32(audio[tag+8 : tag+12]))
		return frames * 1152 * 1000 / rate
	}
	return int64(len(audio)) * 8 / bitrate
}

// webmDuration reads the Segment Info's Duration (a float in TimecodeScale
// units, 1 ms by default) from the file's first 64 KiB.
func webmDuration(content []byte) int64 {
	head := content[:min(len(content), 64<<10)]
	if len(head) < 4 || !bytes.HasPrefix(head, []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return 0
	}
	scale := uint64(1_000_000)
	if at := bytes.Index(head, []byte{0x2A, 0xD7, 0xB1}); at >= 0 && at+4 <= len(head) && head[at+3]&0x80 != 0 {
		length := int(head[at+3] & 0x7f)
		if length >= 1 && length <= 8 && at+4+length <= len(head) {
			var value uint64
			for _, b := range head[at+4 : at+4+length] {
				value = value<<8 | uint64(b)
			}
			if value > 0 {
				scale = value
			}
		}
	}
	at := bytes.Index(head, []byte{0x44, 0x89})
	if at < 0 || at+3 > len(head) {
		return 0
	}
	var duration float64
	switch {
	case head[at+2] == 0x84 && at+7 <= len(head):
		duration = float64(math.Float32frombits(binary.BigEndian.Uint32(head[at+3 : at+7])))
	case head[at+2] == 0x88 && at+11 <= len(head):
		duration = math.Float64frombits(binary.BigEndian.Uint64(head[at+3 : at+11]))
	default:
		return 0
	}
	ms := duration * float64(scale) / 1e6
	if ms <= 0 || math.IsNaN(ms) || ms > math.MaxInt64/2 {
		return 0
	}
	return int64(math.Round(ms))
}
