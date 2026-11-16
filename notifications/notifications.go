// Package notifications is Notifications' public contract: one logical entry
// per event, shown in Playhoot and, when its type and the recipient's
// preferences allow, sent by email through the outbound providers. It
// imports only the standard library.
package notifications

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// QueueDomain names Notifications' queue: its table is
// notifications_queue_tasks.
const QueueDomain = "notifications"

// EmailRule is how a type uses email.
type EmailRule string

const (
	// EmailNone never sends email.
	EmailNone EmailRule = "none"
	// EmailNecessary always sends: access, security and billing. It ignores
	// preferences and goes before every other email.
	EmailNecessary EmailRule = "necessary"
	// EmailStandard sends whenever the recipient has an address.
	EmailStandard EmailRule = "standard"
	// EmailOptional sends only when the recipient turned its group on.
	EmailOptional EmailRule = "optional"
)

// Group is an optional email group the recipient turns on or off.
type Group string

const (
	GroupAI        Group = "ai"
	GroupCapacity  Group = "capacity"
	GroupReview    Group = "review"
	GroupMarketing Group = "marketing"
)

// Type is a kind of notification: a row of the channel matrix.
type Type string

const (
	TypeAccessCode           Type = "access-code"
	TypeWelcome              Type = "welcome"
	TypePurchaseConfirmed    Type = "purchase-confirmed"
	TypePlanChanged          Type = "plan-changed"
	TypePaymentFailed        Type = "payment-failed"
	TypeTrialUpdate          Type = "trial-update"
	TypeAccountSecurity      Type = "account-security"
	TypeGuardianAuthorized   Type = "guardian-authorization"
	TypeAIWorkFinished       Type = "ai-work-finished"
	TypeCapacityWarning      Type = "capacity-warning"
	TypeDiscoveryReview      Type = "discovery-review"
	TypeDiscoveryRestriction Type = "discovery-restriction"
	TypeClassActivity        Type = "class-activity"
	TypeMarketing            Type = "marketing"
	TypeBudgetLevelChanged   Type = "budget-level-changed"
)

// TypeSpec is a type's row of the matrix. InApp is false for a type shown
// only as the screen state of its own flow: its entry keeps no content
// once its email is decided. Group is set for optional email.
// NoEmailToStudents drops email when the recipient is a student.
// EmailValues are the only skeleton values an email may carry.
type TypeSpec struct {
	Type              Type
	InApp             bool
	Email             EmailRule
	Group             Group
	NoEmailToStudents bool
	EmailValues       []string
}

// Necessary reports whether the type ignores preferences.
func (s TypeSpec) Necessary() bool {
	return s.Email == EmailNecessary
}

// Matrix is the channel matrix: in Playhoot and by email, per type.
var Matrix = []TypeSpec{
	{Type: TypeAccessCode, InApp: false, Email: EmailNecessary, EmailValues: []string{"code"}},
	{Type: TypeWelcome, InApp: true, Email: EmailStandard},
	{Type: TypePurchaseConfirmed, InApp: true, Email: EmailNecessary},
	{Type: TypePlanChanged, InApp: true, Email: EmailNecessary},
	{Type: TypePaymentFailed, InApp: true, Email: EmailNecessary},
	{Type: TypeTrialUpdate, InApp: true, Email: EmailNecessary},
	{Type: TypeAccountSecurity, InApp: true, Email: EmailNecessary},
	{Type: TypeGuardianAuthorized, InApp: true, Email: EmailNecessary},
	{Type: TypeAIWorkFinished, InApp: true, Email: EmailOptional, Group: GroupAI},
	{Type: TypeCapacityWarning, InApp: true, Email: EmailOptional, Group: GroupCapacity},
	{Type: TypeDiscoveryReview, InApp: true, Email: EmailOptional, Group: GroupReview},
	{Type: TypeDiscoveryRestriction, InApp: true, Email: EmailNecessary},
	{Type: TypeClassActivity, InApp: true, Email: EmailStandard, NoEmailToStudents: true},
	{Type: TypeMarketing, InApp: true, Email: EmailOptional, Group: GroupMarketing},
	{Type: TypeBudgetLevelChanged, InApp: true, Email: EmailNecessary, EmailValues: []string{"level"}},
}

// Spec returns t's row of the matrix.
func Spec(t Type) (TypeSpec, bool) {
	for _, spec := range Matrix {
		if spec.Type == t {
			return spec, true
		}
	}
	return TypeSpec{}, false
}

// Language is an interface language an email is written in.
type Language string

const (
	LanguageSpanish    Language = "es"
	LanguageEnglish    Language = "en"
	LanguagePortuguese Language = "pt"
)

// DefaultLanguage is used while the recipient's language is unknown.
const DefaultLanguage = LanguageEnglish

// Notice is one event a domain tells a user about. Recipient is the user's
// UUID; IdempotencyKey is unique per SourceDomain, so a retried task never
// duplicates the entry. Skeleton is the source domain's assemblable content.
// HasEmail, Student and MarketingConsent describe the recipient as the
// orchestrator found them.
type Notice struct {
	SourceDomain     string
	Recipient        string
	Type             Type
	IdempotencyKey   string
	ExternalToken    string
	Skeleton         map[string]any
	SendAt           time.Time
	HasEmail         bool
	Student          bool
	MarketingConsent bool
}

// AcceptOutcome is what AcceptEntry did.
type AcceptOutcome string

const (
	// Accepted means a new entry exists.
	Accepted AcceptOutcome = "accepted"
	// AcceptedBefore means the same source and key were accepted already.
	AcceptedBefore AcceptOutcome = "duplicate"
	// AcceptInvalid means the notice cannot be read; nothing is stored.
	AcceptInvalid AcceptOutcome = "invalid"
)

// Acceptance is AcceptEntry's answer.
type Acceptance struct {
	Outcome   AcceptOutcome
	EntryUUID string
	Email     bool
}

// Entry is a notification as its recipient sees it.
type Entry struct {
	EntryUUID     string
	SourceDomain  string
	Type          Type
	ExternalToken string
	Skeleton      map[string]any
	Email         bool
	Necessary     bool
	ReadAt        *time.Time
	CreatedAt     time.Time
}

// EntryFilter narrows a user's entries. Zero values do not filter. Before
// pages backwards: only entries created strictly before it.
type EntryFilter struct {
	SourceDomain string
	Type         Type
	From         time.Time
	To           time.Time
	Before       time.Time
	Limit        int
}

// MaxListLimit bounds one page of entries; DefaultListLimit is used when
// the filter asks for none.
const (
	MaxListLimit     = 100
	DefaultListLimit = 30
)

// EntryPage is a page of entries, newest first, and the unread count.
type EntryPage struct {
	Entries []Entry
	Unread  int
}

// MarkReadOutcome is what MarkRead did.
type MarkReadOutcome string

const (
	MarkedRead   MarkReadOutcome = "read"
	AlreadyRead  MarkReadOutcome = "already_read"
	UnknownEntry MarkReadOutcome = "unknown"
)

// Preferences are a user's optional email choices.
type Preferences struct {
	AI        bool
	Capacity  bool
	Review    bool
	Marketing bool
}

// DefaultPreferences apply until a user saves their own: AI and capacity
// email are off, review results on, marketing off.
var DefaultPreferences = Preferences{Review: true}

// Allows reports whether p turns group on.
func (p Preferences) Allows(group Group) bool {
	switch group {
	case GroupAI:
		return p.AI
	case GroupCapacity:
		return p.Capacity
	case GroupReview:
		return p.Review
	case GroupMarketing:
		return p.Marketing
	}
	return false
}

// Provider is an outbound email provider.
type Provider string

const (
	ProviderResend Provider = "resend"
	ProviderSES    Provider = "ses"
	// ProviderNone marks a delivery decided before any provider was called.
	ProviderNone Provider = "none"
)

// Mode is how the outbound provider is chosen.
type Mode string

const (
	// ModeAutomatic uses Resend while its daily and monthly quota remain,
	// then SES; a renewed quota returns to Resend.
	ModeAutomatic Mode = "automatic"
	// ModeResendOnly uses only Resend's available quota; beyond it sends wait.
	ModeResendOnly Mode = "resend-only"
	// ModeSESOnly uses only SES.
	ModeSESOnly Mode = "ses-only"
)

// ValidMode reports whether m is a known mode.
func ValidMode(m Mode) bool {
	return m == ModeAutomatic || m == ModeResendOnly || m == ModeSESOnly
}

// Resend's free quota, joint for the whole provider account.
const (
	ResendDailyQuota   = 100
	ResendMonthlyQuota = 3000
)

// DeliveryState is the state of one email attempt.
type DeliveryState string

const (
	DeliveryPending  DeliveryState = "pending"
	DeliverySent     DeliveryState = "sent"
	DeliveryRejected DeliveryState = "rejected"
	DeliveryFailed   DeliveryState = "failed"
)

// DeliveryStates lists every state, in lifecycle order.
var DeliveryStates = []DeliveryState{DeliveryPending, DeliverySent, DeliveryRejected, DeliveryFailed}

// Delivery details: why an attempt ended as it did, never its content.
const (
	DetailSuppressed      = "address suppressed"
	DetailNoAddress       = "recipient has no address"
	DetailQuota           = "provider quota exhausted"
	DetailRefused         = "provider refused the message"
	DetailProviderFailure = "provider failed before accepting"
	DetailUncertain       = "uncertain provider response"
	DetailInterrupted     = "attempt interrupted before the provider answered"
)

// Recipient is where an entry's email goes, as Identity knows it now.
type Recipient struct {
	Email    string
	Language Language
}

// SendOutcome is what SendEmail decided for an entry's email.
type SendOutcome string

const (
	// EmailSent means a provider accepted the message.
	EmailSent SendOutcome = "sent"
	// EmailRejected means the address is suppressed or missing, or the
	// providers refused it; nothing will be retried.
	EmailRejected SendOutcome = "rejected"
	// EmailFailed means the attempt failed for good (an uncertain response
	// is never resent).
	EmailFailed SendOutcome = "failed"
	// EmailRetry means no provider took the message yet: Resend only is out
	// of quota or a provider failed clearly. The task retries later.
	EmailRetry SendOutcome = "retry"
	// EmailAlreadyDecided means an earlier attempt already ended this email.
	EmailAlreadyDecided SendOutcome = "already_decided"
	// EmailNotApplicable means the entry is unknown or has no email channel.
	EmailNotApplicable SendOutcome = "not_applicable"
)

// SendTaskType is the per-entry email task on Notifications' queue, due at
// the entry's send_at. The orchestrator handles it (flow
// send-notification-email).
const SendTaskType = "send-email"

// SendTask is a send-email task's payload: the entry, its recipient's UUID
// and whether it is necessary (necessary email is never discretionary).
type SendTask struct {
	EntryUUID string `json:"entry_uuid"`
	Recipient string `json:"recipient"`
	Necessary bool   `json:"necessary"`
}

// SpendTask is the spend-event payload Notifications leaves in its queue
// for each accepted email.
type SpendTask struct {
	Category       string    `json:"category"`
	Service        string    `json:"service"`
	Reference      string    `json:"reference"`
	Quantity       float64   `json:"quantity"`
	Unit           string    `json:"unit"`
	Measured       bool      `json:"measured"`
	IdempotencyKey string    `json:"idempotency_key"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// EmailReference names an entry's email as cost-bearing work.
func EmailReference(entryUUID string) string {
	return "email:" + entryUUID
}

// Spend reported for each accepted email, in the budget's email category.
const (
	SpendCategory = "email"
	SpendUnit     = "emails"
)

// SpendService names the budget service of an accepted email by provider.
func SpendService(p Provider) string {
	return string(p) + "-outbound"
}

// OutboundEmail is one rendered message. Text is the whole body: plain text,
// no tracking. Unsubscribe is the one-click link, set for optional types.
// IdempotencyKey lets a provider refuse a repeated send.
type OutboundEmail struct {
	To             string
	Subject        string
	Text           string
	Unsubscribe    string
	IdempotencyKey string
}

// ProviderOutcome is how a provider answered a send.
type ProviderOutcome string

const (
	// ProviderAccepted means the provider took the message.
	ProviderAccepted ProviderOutcome = "accepted"
	// ProviderQuotaRejected is a clear refusal for lack of quota.
	ProviderQuotaRejected ProviderOutcome = "quota_rejected"
	// ProviderRefused is a clear permanent refusal of this message.
	ProviderRefused ProviderOutcome = "refused"
	// ProviderFailed is a clear failure before accepting: safe to retry.
	ProviderFailed ProviderOutcome = "failed"
	// ProviderUncertain means the message may or may not have been taken.
	ProviderUncertain ProviderOutcome = "uncertain"
)

// ProviderAnswer is a provider's answer to one send.
type ProviderAnswer struct {
	Outcome   ProviderOutcome
	MessageID string
}

// FeedbackReason is why a provider tells us to stop emailing an address.
type FeedbackReason string

const (
	ReasonUnsubscribe FeedbackReason = "unsubscribe"
	ReasonBounce      FeedbackReason = "bounce"
	ReasonComplaint   FeedbackReason = "complaint"
)

// Feedback is one provider report about an address.
type Feedback struct {
	Email  string
	Reason FeedbackReason
}

// EmailProvider is the outbound port. Send never returns an error for a
// provider answer: every answer is a ProviderOutcome. VerifyWebhook
// authenticates a provider callback and reads its bounces and complaints.
// USDPerEmail is the reference price of one accepted email.
type EmailProvider interface {
	Provider() Provider
	Send(ctx context.Context, email OutboundEmail) ProviderAnswer
	VerifyWebhook(header http.Header, body []byte) ([]Feedback, error)
	USDPerEmail() float64
}

// SuppressionOrigin is who reported a suppression.
type SuppressionOrigin string

const (
	OriginResend   SuppressionOrigin = "resend"
	OriginSES      SuppressionOrigin = "ses"
	OriginPlayhoot SuppressionOrigin = "playhoot"
)

// SuppressionOutcome is what RecordSuppression did.
type SuppressionOutcome string

const (
	SuppressionRecorded  SuppressionOutcome = "recorded"
	SuppressionDuplicate SuppressionOutcome = "duplicate"
	SuppressionInvalid   SuppressionOutcome = "invalid"
)

// ProviderUsage is one provider's accepted sends today and this month (UTC).
type ProviderUsage struct {
	Provider     Provider
	Today        int
	Month        int
	EstimatedUSD float64
}

// EmailStatus is admin's view of outbound email: the mode, consumption per
// provider and the last 30 days of deliveries by state.
type EmailStatus struct {
	Mode       Mode
	ModeSince  time.Time
	ModeReason string
	Usage      []ProviderUsage
	Deliveries map[DeliveryState]int
}

// SetModeOutcome is what AdminSetEmailMode did.
type SetModeOutcome string

const (
	ModeChanged   SetModeOutcome = "changed"
	ModeUnchanged SetModeOutcome = "unchanged"
	ModeInvalid   SetModeOutcome = "invalid"
)

// NormalizeEmail is the stored and compared form of an address.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ErrWebhookRejected means a provider callback names no configured provider
// or fails its verification.
var ErrWebhookRejected = errors.New("email webhook rejected")
