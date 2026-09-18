package domain

import (
	"strings"
	"time"
)

// TargetType names what a report contests. It mirrors the CHECK constraint
// of app.moderation_reports and app.moderation_cases (migration 00022) so a
// value accepted here is a value the database accepts.
type TargetType string

const (
	// TargetArena contests an Arena.
	TargetArena TargetType = "arena"
	// TargetArgument contests an argument or reply.
	TargetArgument TargetType = "argument"
	// TargetProfile contests a profile (account).
	TargetProfile TargetType = "profile"
)

// AllTargetTypes returns the closed vocabulary in canonical order.
func AllTargetTypes() []TargetType {
	return []TargetType{TargetArena, TargetArgument, TargetProfile}
}

// ParseTargetType validates a requested target type against the exact
// vocabulary.
func ParseTargetType(raw string) (TargetType, error) {
	target := TargetType(strings.TrimSpace(raw))
	switch target {
	case TargetArena, TargetArgument, TargetProfile:
		return target, nil
	default:
		return "", ErrInvalidTargetType
	}
}

// IsValid reports whether the target type is an authorized enum value.
func (t TargetType) IsValid() bool {
	switch t {
	case TargetArena, TargetArgument, TargetProfile:
		return true
	default:
		return false
	}
}

// String returns the stored target type value.
func (t TargetType) String() string {
	return string(t)
}

// Reason names the structured report category. It mirrors the CHECK
// constraint of app.moderation_reports (migration 00022): the closed
// vocabulary of MODERATION §3. Free text lives only in the optional
// context, never in the reason.
type Reason string

const (
	ReasonViolence     Reason = "violence"
	ReasonDoxxing      Reason = "doxxing"
	ReasonHarassment   Reason = "harassment"
	ReasonSexual       Reason = "sexual"
	ReasonFraud        Reason = "fraud"
	ReasonSpam         Reason = "spam"
	ReasonIllegal      Reason = "illegal"
	ReasonEvasion      Reason = "evasion"
	ReasonMultiaccount Reason = "multiaccount"
	ReasonMalware      Reason = "malware"
	ReasonOther        Reason = "other"
)

// AllReasons returns the closed vocabulary in canonical order.
func AllReasons() []Reason {
	return []Reason{
		ReasonViolence, ReasonDoxxing, ReasonHarassment, ReasonSexual,
		ReasonFraud, ReasonSpam, ReasonIllegal, ReasonEvasion,
		ReasonMultiaccount, ReasonMalware, ReasonOther,
	}
}

// ParseReason validates a requested reason against the exact vocabulary.
func ParseReason(raw string) (Reason, error) {
	reason := Reason(strings.TrimSpace(raw))
	switch reason {
	case ReasonViolence, ReasonDoxxing, ReasonHarassment, ReasonSexual,
		ReasonFraud, ReasonSpam, ReasonIllegal, ReasonEvasion,
		ReasonMultiaccount, ReasonMalware, ReasonOther:
		return reason, nil
	default:
		return "", ErrInvalidReason
	}
}

// IsValid reports whether the reason is an authorized enum value.
func (r Reason) IsValid() bool {
	switch r {
	case ReasonViolence, ReasonDoxxing, ReasonHarassment, ReasonSexual,
		ReasonFraud, ReasonSpam, ReasonIllegal, ReasonEvasion,
		ReasonMultiaccount, ReasonMalware, ReasonOther:
		return true
	default:
		return false
	}
}

// String returns the stored reason value.
func (r Reason) String() string {
	return string(r)
}

// MaxReportContextLength bounds the optional reporter context. It mirrors
// the CHECK constraint of app.moderation_reports (migration 00022).
const MaxReportContextLength = 2000

// ParseReportContext validates the optional reporter context: empty means
// absent, otherwise trimmed non-blank text within the bound. The context is
// restricted evidence: it is stored, never logged, and never projected
// publicly.
func ParseReportContext(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > MaxReportContextLength {
		return "", ErrInvalidContext
	}
	for _, r := range trimmed {
		if r == 0 || (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
			return "", ErrInvalidContext
		}
	}
	return trimmed, nil
}

// DuplicateWindow bounds deduplication: the same reporter contesting the
// same target for the same reason inside the window resolves the original
// report instead of inserting a second row.
const DuplicateWindow = 24 * time.Hour

// RateWindow bounds the brigading signal: reports filed by one reporter
// inside the window are counted, and reaching RateThreshold marks the
// result as rate-limited without refusing the report.
const RateWindow = time.Hour

// RateThreshold is how many reports inside RateWindow mark the result.
// It is a policy constant, not configuration: changing it moves the
// brigading signal for every reporter at once.
const RateThreshold = 10
