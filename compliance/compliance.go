// Package compliance is Compliance's public contract: which functions each
// market allows and from what age, people's declared residence and age,
// known minority, and the versioned texts people accept. It imports only the
// standard library.
package compliance

import (
	"errors"
	"time"
)

// Function is something the market matrix enables or disables per market.
type Function string

const (
	// FunctionCreateGames is creating, editing and publishing games.
	FunctionCreateGames Function = "create-games"
	// FunctionSubscribe is subscribing and purchasing.
	FunctionSubscribe Function = "subscribe"
	// FunctionTeach is running classes.
	FunctionTeach Function = "teach"
	// FunctionMinorClassParticipation is a minor taking part in a class with
	// an authorization.
	FunctionMinorClassParticipation Function = "minor-class-participation"
	// FunctionMinorFamilyParticipation is a minor taking part in a private
	// family group with an authorization.
	FunctionMinorFamilyParticipation Function = "minor-family-participation"
	// FunctionInteractiveUse is any interactive use beyond public
	// information, such as playing.
	FunctionInteractiveUse Function = "interactive-use"
)

// Functions lists every Function.
var Functions = []Function{
	FunctionCreateGames, FunctionSubscribe, FunctionTeach,
	FunctionMinorClassParticipation, FunctionMinorFamilyParticipation, FunctionInteractiveUse,
}

// AccessOutcome is what CheckMarketAccess decided.
type AccessOutcome string

const (
	// AccessAllowed means the function is available to the person here.
	AccessAllowed AccessOutcome = "allowed"
	// AccessNeedsDeclaration means the person must declare residence and age
	// first.
	AccessNeedsDeclaration AccessOutcome = "needs_declaration"
	// AccessBlocked means the function is not available; Reason says why.
	AccessBlocked AccessOutcome = "blocked"
)

// BlockReason is why a function is blocked, worded as a service restriction.
type BlockReason string

const (
	// BlockMarketUnavailable means the service does not offer the function
	// in the person's market.
	BlockMarketUnavailable BlockReason = "market_unavailable"
	// BlockAgeRequirement means the function needs an age the person did not
	// declare, or is known not to have.
	BlockAgeRequirement BlockReason = "age_requirement"
)

// AccessRequest asks whether Function is available to UserUUID (empty for
// someone without an identity). ApproximateCountry is the ISO country the
// caller's network address suggests, empty when unknown.
type AccessRequest struct {
	UserUUID           string
	Function           Function
	ApproximateCountry string
}

// AccessDecision is CheckMarketAccess's answer. Reason is set only when
// blocked. LocationDiscrepancy reports that the approximate location differs
// from the declared residence; it never blocks on its own.
type AccessDecision struct {
	Outcome             AccessOutcome
	Reason              BlockReason
	Country             string
	Subdivision         string
	LocationDiscrepancy bool
}

// DeclarationOrigin is who recorded a declaration.
type DeclarationOrigin string

const (
	// DeclaredByUser is the person's own declaration.
	DeclaredByUser DeclarationOrigin = "user"
	// DeclaredBySupport is a correction support recorded.
	DeclaredBySupport DeclarationOrigin = "support"
)

// Declaration is a residence-and-age declaration: country, state or province
// where the market needs one, and whether the person has reached the age the
// place requires. It is declared, never verified.
type Declaration struct {
	UserUUID    string
	Country     string
	Subdivision string
	MeetsAge    bool
	Origin      DeclarationOrigin
}

// DeclarationOutcome is what RecordDeclaration decided.
type DeclarationOutcome string

const (
	// DeclarationRecorded means the declaration is now the current one.
	DeclarationRecorded DeclarationOutcome = "recorded"
	// DeclarationUnchanged means the same answer was already recorded.
	DeclarationUnchanged DeclarationOutcome = "unchanged"
	// DeclarationConflict means a different answer was already recorded:
	// only support corrects it.
	DeclarationConflict DeclarationOutcome = "conflict"
	// DeclarationInvalid means the country or subdivision is malformed, or
	// the market needs a subdivision that is missing.
	DeclarationInvalid DeclarationOutcome = "invalid"
)

// DeclarationResult is what RecordDeclaration decided. ThresholdAge is the
// age the declaration was about.
type DeclarationResult struct {
	Outcome      DeclarationOutcome
	ThresholdAge int
}

// TextKind is a kind of text people accept.
type TextKind string

const (
	// TextTerms is the terms of use and purchase.
	TextTerms TextKind = "terms"
	// TextOwnData is the processing of the person's own data.
	TextOwnData TextKind = "own-data"
	// TextClassDataAgreement is the class data processing agreement; it stays
	// downloadable.
	TextClassDataAgreement TextKind = "class-data-agreement"
	// TextMarketing is the optional marketing consent; it can be withdrawn.
	TextMarketing TextKind = "marketing"
)

// TextKinds lists every TextKind.
var TextKinds = []TextKind{TextTerms, TextOwnData, TextClassDataAgreement, TextMarketing}

// Languages are the languages texts are published in.
var Languages = []string{"es", "en", "pt"}

// AcceptanceAction is what a person did with a text.
type AcceptanceAction string

const (
	// ActionAccepted is accepting a text version.
	ActionAccepted AcceptanceAction = "accepted"
	// ActionWithdrawn is withdrawing a consent; only marketing allows it.
	ActionWithdrawn AcceptanceAction = "withdrawn"
)

// AcceptanceRequest records UserUUID accepting the text Kind, Version,
// Language, or withdrawing it, in Context (registration, purchase, class
// setup). A withdrawal needs only Kind.
type AcceptanceRequest struct {
	UserUUID string
	Kind     TextKind
	Version  int
	Language string
	Action   AcceptanceAction
	Context  string
}

// AcceptanceOutcome is what RecordAcceptance decided.
type AcceptanceOutcome string

const (
	// AcceptanceRecorded means the acceptance or withdrawal is recorded.
	AcceptanceRecorded AcceptanceOutcome = "recorded"
	// AcceptanceUnchanged means it was already so: nothing new is recorded.
	AcceptanceUnchanged AcceptanceOutcome = "unchanged"
	// AcceptanceUnknownText means no published text has that kind, version
	// and language.
	AcceptanceUnknownText AcceptanceOutcome = "unknown_text"
	// AcceptanceNotWithdrawable means the kind is necessary and cannot be
	// withdrawn here.
	AcceptanceNotWithdrawable AcceptanceOutcome = "not_withdrawable"
	// AcceptanceInvalid means the request is malformed.
	AcceptanceInvalid AcceptanceOutcome = "invalid"
)

// AcceptanceText is a published text version. Accepted reports whether the
// caller has a standing acceptance of this kind's latest version.
type AcceptanceText struct {
	Kind        TextKind
	Version     int
	Language    string
	Content     string
	PublishedAt time.Time
	Accepted    bool
}

// ErrTextNotFound means no text of that kind is published in that language.
var ErrTextNotFound = errors.New("acceptance text not found")

// MarketRule is one row of the market matrix: whether Function is enabled
// in Country (and Subdivision, empty for the whole country) from EffectiveAt,
// and from what declared age. ConditionsNote names operator conditions still
// pending, which keep the rule disabled.
type MarketRule struct {
	Country           string
	Subdivision       string
	Function          Function
	Enabled           bool
	MinAge            int
	ConditionsNote    string
	EffectiveAt       time.Time
	CreatedByUserUUID string
	Reason            string
	CreatedAt         time.Time
}

// MarketRules is the matrix as admin reads it: the rule in force for each
// market and function, rules scheduled to take effect, and superseded ones.
type MarketRules struct {
	Current   []MarketRule
	Scheduled []MarketRule
	History   []MarketRule
}

// SetMarketRuleOutcome is what AdminSetMarketRule decided.
type SetMarketRuleOutcome string

const (
	// MarketRuleRecorded means the rule is appended.
	MarketRuleRecorded SetMarketRuleOutcome = "recorded"
	// MarketRuleConditionsPending means enabling was asked while operator
	// conditions are still noted: the admin clears the note once they are met.
	MarketRuleConditionsPending SetMarketRuleOutcome = "conditions_pending"
	// MarketRuleInvalid means the rule is malformed, its effective date is in
	// the past, or the reason is missing.
	MarketRuleInvalid SetMarketRuleOutcome = "invalid"
)
