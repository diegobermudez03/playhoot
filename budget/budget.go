// Package budget is Budget's public contract: what Playhoot spends, the caps
// that bound it, the admission of cost-bearing work and the restriction
// ladder up to the emergency shutdown. It imports only the standard library.
package budget

import "time"

// QueueDomain names Budget's queue: its table is budget_queue_tasks.
const QueueDomain = "budget"

// Category is a kind of cost-bearing consumption.
type Category string

const (
	CategoryAI          Category = "ai"
	CategoryImages      Category = "images"
	CategoryEmail       Category = "email"
	CategoryFiles       Category = "files"
	CategorySimulations Category = "simulations"
	CategoryExports     Category = "exports"
	CategoryTraffic     Category = "traffic"
	CategoryStorage     Category = "storage"
	CategoryCompute     Category = "compute"
)

// Categories lists every category, in a stable order.
var Categories = []Category{
	CategoryAI, CategoryImages, CategoryEmail, CategoryFiles, CategorySimulations,
	CategoryExports, CategoryTraffic, CategoryStorage, CategoryCompute,
}

// ValidCategory reports whether c is a known category.
func ValidCategory(c Category) bool {
	for _, known := range Categories {
		if c == known {
			return true
		}
	}
	return false
}

// ScopeGlobal is the cap scope covering every category; any other scope is a
// category.
const ScopeGlobal = "global"

// Window is the period a cap bounds, aligned to UTC calendar boundaries.
type Window string

const (
	WindowHour  Window = "hour"
	WindowDay   Window = "day"
	WindowMonth Window = "month"
)

// Windows lists every window, shortest first.
var Windows = []Window{WindowHour, WindowDay, WindowMonth}

// Level is a step of the restriction ladder.
type Level string

const (
	// LevelNormal admits work within caps.
	LevelNormal Level = "normal"
	// LevelRestricted refuses discretionary and free work first.
	LevelRestricted Level = "restricted"
	// LevelPaused also stops the categories threatening their caps.
	LevelPaused Level = "paused"
	// LevelShutdown refuses everything until a manual reopening.
	LevelShutdown Level = "shutdown"
)

// ActorAutomatic is the actor of the ladder moves made by the evaluation.
const ActorAutomatic = "automatic"

// SpendEvent is a cost that occurred, as a domain reports it. Quantity is in
// Unit; the rate registered for Service turns it into USD. Reference names
// the work, so a measured event replaces the estimate with the same one.
type SpendEvent struct {
	SourceDomain   string
	Category       Category
	Service        string
	Reference      string
	Quantity       float64
	Unit           string
	Measured       bool
	IdempotencyKey string
	OccurredAt     time.Time
}

// RecordSpendOutcome is what RecordSpend decided.
type RecordSpendOutcome string

const (
	// SpendRecorded means the event is stored, priced when a rate existed.
	SpendRecorded RecordSpendOutcome = "recorded"
	// SpendRateMissing means the event is stored without price and its
	// category refuses admission until the rate is registered.
	SpendRateMissing RecordSpendOutcome = "rate_missing"
	// SpendDuplicate means the same source and key were already recorded.
	SpendDuplicate RecordSpendOutcome = "duplicate"
	// SpendInvalid means the event cannot be read; nothing is stored.
	SpendInvalid RecordSpendOutcome = "invalid"
)

// WorkRequest asks to admit cost-bearing work: its estimated Quantity of
// Service in Category, a Reference naming it, whether it is discretionary
// or free (refused first under restriction) and how long it may take.
type WorkRequest struct {
	Category      Category
	Service       string
	Quantity      float64
	Reference     string
	Discretionary bool
	MaxDuration   time.Duration
}

// AdmissionOutcome is whether work was admitted.
type AdmissionOutcome string

const (
	Admitted AdmissionOutcome = "admitted"
	Refused  AdmissionOutcome = "refused"
)

// RefusalReason is why work was refused. Every reason is a service
// restriction; none is the caller's own balance.
type RefusalReason string

const (
	RefusedShutdown       RefusalReason = "shutdown"
	RefusedPaused         RefusalReason = "category_paused"
	RefusedRestricted     RefusalReason = "restricted"
	RefusedNoControl      RefusalReason = "no_control"
	RefusedCapReached     RefusalReason = "cap_reached"
	RefusedConcurrency    RefusalReason = "concurrency_reached"
	RefusedVolume         RefusalReason = "volume_reached"
	RefusedInvalidRequest RefusalReason = "invalid_request"
)

// RestrictionMessage is how a refusal is explained to people: a temporary
// service restriction, never an exhausted balance.
const RestrictionMessage = "This service is temporarily restricted to protect the platform. Your balance and entitlements are unchanged; please try again later."

// Admission is the answer to a WorkRequest. ReservationUUID is set when
// admitted: the caller settles it when the work ends.
type Admission struct {
	Outcome         AdmissionOutcome
	ReservationUUID string
	Reason          RefusalReason
	Message         string
}

// Settlement is the measured cost of admitted work: Quantity of Service.
// A zero Quantity releases the whole reservation (cancelled work).
type Settlement struct {
	Service  string
	Quantity float64
}

// SettleOutcome is what SettleWork did.
type SettleOutcome string

const (
	// Settled means the reservation is closed with the measured cost.
	Settled SettleOutcome = "settled"
	// AlreadyReleased means the reservation was closed before (settled,
	// cancelled or expired); a measured cost still enters the spend record.
	AlreadyReleased SettleOutcome = "already_released"
	// UnknownReservation means no reservation has that identifier.
	UnknownReservation SettleOutcome = "unknown"
)

// Cap is a ceiling for a scope (ScopeGlobal or a category) over a window.
// MaxConcurrency and MaxVolume are zero when not limited by count.
type Cap struct {
	Scope          string
	Window         Window
	USDLimit       float64
	MaxConcurrency int
	MaxVolume      int
}

// Rate is what one Unit of Service in Category costs.
type Rate struct {
	Category   Category
	Service    string
	Unit       string
	USDPerUnit float64
}

// Controls is an admin change of caps and rates, effective from
// EffectiveAt (now when zero), with a reason.
type Controls struct {
	Caps        []Cap
	Rates       []Rate
	EffectiveAt time.Time
	Reason      string
}

// SetControlsOutcome is what AdminSetControls decided.
type SetControlsOutcome string

const (
	ControlsRecorded SetControlsOutcome = "recorded"
	ControlsInvalid  SetControlsOutcome = "invalid"
)

// CutoffOutcome is what AdminCutoff did.
type CutoffOutcome string

const (
	CutoffApplied        CutoffOutcome = "shut_down"
	CutoffAlreadyApplied CutoffOutcome = "already_shut_down"
	CutoffInvalid        CutoffOutcome = "invalid"
)

// ReopenOutcome is what Reopen did.
type ReopenOutcome string

const (
	// ReopenStarted means the shutdown is lifted into restricted with every
	// category still closed.
	ReopenStarted ReopenOutcome = "reopening"
	// ReopenOpened means the named categories are open again.
	ReopenOpened ReopenOutcome = "opened"
	// ReopenNotShutDown means there is no shutdown or reopening to act on.
	ReopenNotShutDown ReopenOutcome = "not_shut_down"
	// ReopenAlreadyOpen means a named category is not closed.
	ReopenAlreadyOpen ReopenOutcome = "already_open"
	ReopenInvalid     ReopenOutcome = "invalid"
)

// LadderState is the current step of the ladder: the categories paused by
// the evaluation and those still closed by a gradual reopening.
type LadderState struct {
	Level            Level
	PausedCategories []Category
	ReopenClosed     []Category
	Reason           string
	Actor            string
	Since            time.Time
}

// WindowUsage is one scope's consumption in one window, in USD. Measured
// and Estimated are spend; Reserved is admitted work still running.
type WindowUsage struct {
	Scope     string
	Window    Window
	Measured  float64
	Estimated float64
	Reserved  float64
	// Limit is the current cap; HasCap is false when the window has none.
	Limit  float64
	HasCap bool
}

// Status is the control panel: ladder, caps, rates, window usage, active
// reservations, last-hour spend speed and the projection of the global
// month cap. HoursToMonthCap is negative when spend is not advancing.
type Status struct {
	Ladder             LadderState
	Caps               []Cap
	Rates              []Rate
	Usage              []WindowUsage
	ActiveReservations int
	LastHourUSD        float64
	HoursToMonthCap    float64
	RateMissing        []Category
}
