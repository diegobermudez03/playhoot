package game

import (
	"errors"
	"time"
)

// ErrIdempotencyConflict is an idempotency key reused with another request.
var ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")

// ErrInvalidCard is a card value that breaks a card rule, such as a name
// that is empty or too long.
var ErrInvalidCard = errors.New("invalid card value")

// CardImageKind is the role of an image on a card.
type CardImageKind string

const (
	// CardLogo is the card's one logo.
	CardLogo CardImageKind = "logo"
	// CardPhoto is one of the discovery photos.
	CardPhoto CardImageKind = "photo"
)

// CardImage is one image of a card. URL, a signed read link valid until
// URLExpiresAt, is set only when the card is read.
type CardImage struct {
	UUID         string
	MIMEType     string
	Size         int64
	URL          string
	URLExpiresAt time.Time
}

// Card is what a Game shows of itself, with where its lifecycle stands.
type Card struct {
	GameUUID    string
	Name        string
	Description string
	Logo        *CardImage
	Photos      []CardImage
	Visibility  VisibilityType
	Published   bool
	PublishedAt *time.Time
	// Editing means an edition is open; Publishing that a release is still
	// being published.
	Editing    bool
	Publishing bool
}

// CardOutcome is what a card operation decided.
type CardOutcome string

const (
	// CardDone means the read or change happened.
	CardDone CardOutcome = "done"
	// CardUnchanged means the change asked for was already the card's state.
	CardUnchanged CardOutcome = "unchanged"
	// CardNotFound means the user owns no Game with that uuid.
	CardNotFound CardOutcome = "not_found"
	// CardInvalid means the request breaks a card rule: Message says which.
	CardInvalid CardOutcome = "invalid"
	// CardImageTooLarge means the image is over its kind's limit.
	CardImageTooLarge CardOutcome = "too_large"
	// CardTooManyPhotos means the card already shows as many photos as it may.
	CardTooManyPhotos CardOutcome = "too_many_photos"
	// CardImageNotFound means the card shows no such image.
	CardImageNotFound CardOutcome = "image_not_found"
)

// CardRead is a card, set when Outcome is CardDone.
type CardRead struct {
	Outcome CardOutcome
	Card    Card
}

// CardEdit is what a change of a card decided. Image is the image an upload
// bound; Message explains CardInvalid.
type CardEdit struct {
	Outcome CardOutcome
	Image   *CardImage
	Message string
}

// CardChange is a change of a card's texts; a nil field keeps its value.
type CardChange struct {
	Name        *string
	Description *string
}

// PublicationOutcome is what starting a publication decided.
type PublicationOutcome string

const (
	// PublicationStarted means a release is recorded, being published.
	PublicationStarted PublicationOutcome = "started"
	// PublicationPending means another release of the Game is still being
	// published: it is answered, to be completed first.
	PublicationPending PublicationOutcome = "pending"
	// PublicationCompleted and PublicationAbandoned answer a retry of a
	// release that already ended.
	PublicationCompleted PublicationOutcome = "completed"
	PublicationAbandoned PublicationOutcome = "abandoned"
	// PublicationNotFound means the user owns no Game with that uuid.
	PublicationNotFound PublicationOutcome = "not_found"
	// PublicationNotEditable means the Game has no edition open to publish.
	PublicationNotEditable PublicationOutcome = "not_editable"
	// PublicationNotReady means the card lacks what publishing requires:
	// Blockers say what.
	PublicationNotReady PublicationOutcome = "not_ready"
)

// Release is one publication of an edition at a revision of its version.
type Release struct {
	UUID        string
	GameUUID    string
	Sequence    int64
	VersionUUID string
	Revision    int64
	Visibility  VisibilityType
	CompletedAt *time.Time
	AbandonedAt *time.Time
}

// Publication is what starting a publication decided, with the release it
// started or found.
type Publication struct {
	Outcome  PublicationOutcome
	Release  *Release
	Blockers []string
}

// EditionOutcome is what opening a new edition decided.
type EditionOutcome string

const (
	// EditionOpened means a new edition is open; a retry answers the same.
	EditionOpened EditionOutcome = "opened"
	// EditionNotFound means the user owns no Game with that uuid.
	EditionNotFound EditionOutcome = "not_found"
	// EditionNotPublished means the Game was never published: its first
	// edition is still the one to edit.
	EditionNotPublished EditionOutcome = "not_published"
	// EditionAlreadyOpen means the Game already has an edition open.
	EditionAlreadyOpen EditionOutcome = "already_open"
	// EditionPublishing means a release is still being published.
	EditionPublishing EditionOutcome = "publishing"
)

// Edition is a new edition, built in the Play version VersionUUID, which
// starts from the published BaseVersionUUID.
type Edition struct {
	Outcome         EditionOutcome
	VersionUUID     string
	BaseVersionUUID string
}
