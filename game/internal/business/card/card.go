// Package card holds the rules of a Game's card: what its texts and images
// may be, and what publishing requires of it.
package card

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/diegobermudez03/playhoot/game"
)

const (
	// MaxNameRunes and MaxDescriptionRunes bound the card texts.
	MaxNameRunes        = 80
	MaxDescriptionRunes = 2000
	// MaxPhotos is how many discovery photos a card shows at most.
	MaxPhotos = 8
)

// ImageKind is the role of an image on a card (game.CardImageKind).
type ImageKind = game.CardImageKind

const (
	// Logo is the card's one logo.
	Logo = game.CardLogo
	// Photo is one of the discovery photos.
	Photo = game.CardPhoto
)

// MaxImageBytes is the most an image of kind may weigh.
func MaxImageBytes(kind ImageKind) int64 {
	if kind == Logo {
		return 2 << 20
	}
	return 5 << 20
}

// imageTypes are the formats a card image may be, read from its bytes.
var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}

// ErrInvalidImage is an image a card refuses: not a supported format.
var ErrInvalidImage = errors.New("a card image must be a PNG, JPEG or WebP picture")

// ImageType is content's real type, read from its bytes, or ErrInvalidImage.
func ImageType(content []byte) (string, error) {
	detected := http.DetectContentType(content)
	if !imageTypes[detected] {
		return "", ErrInvalidImage
	}
	return detected, nil
}

// ErrInvalidText is a card text outside its limits.
var ErrInvalidText = errors.New("invalid card text")

// NormalizeName trims name and checks it fits; an empty name is allowed
// while the Game is not published.
func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxNameRunes {
		return "", fmt.Errorf("%w: the name holds at most %d characters", ErrInvalidText, MaxNameRunes)
	}
	return name, nil
}

// NormalizeDescription trims description and checks it fits.
func NormalizeDescription(description string) (string, error) {
	description = strings.TrimSpace(description)
	if !utf8.ValidString(description) || utf8.RuneCountInString(description) > MaxDescriptionRunes {
		return "", fmt.Errorf("%w: the description holds at most %d characters", ErrInvalidText, MaxDescriptionRunes)
	}
	return description, nil
}

// PublishBlocker is what a card lacks to be published.
type PublishBlocker string

// NameRequired means the card has no name yet.
const NameRequired PublishBlocker = "name_required"

// PublishBlockers is what the card with name lacks to be published.
func PublishBlockers(name string) []PublishBlocker {
	if strings.TrimSpace(name) == "" {
		return []PublishBlocker{NameRequired}
	}
	return nil
}
