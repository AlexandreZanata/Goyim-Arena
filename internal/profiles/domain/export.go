package domain

import "time"

// ExportSchemaVersion pins the schema of the personal data export
// (P14-T05; docs/PRIVACY.md §4/§6, REQ-PRIV-01). The version travels in
// every document, so a future incompatible layout must bump this constant
// and a new document shape with it; generated exports are never silently
// reinterpreted.
const ExportSchemaVersion = 1

// ExportStepUpWindow is how recent the last full authentication must be for
// the owner to request a personal export. The export is a sensitive data
// access, so a stale session is refused even for the account owner.
const ExportStepUpWindow = 15 * time.Minute

// ExportTTL is how long a generated export stays downloadable. After it
// elapses the record is expired and the retention workflow may purge the
// document.
const ExportTTL = 24 * time.Hour

// ExportMaxDownloads is the download budget frozen into every export: the
// link is single-use, so a leaked URL is worthless after the owner uses it
// once.
const ExportMaxDownloads = 1

// ExportTokenBytes is the entropy of the opaque download capability:
// 256 bits, matching the session and recovery token policy.
const ExportTokenBytes = 32

// ExportStatus is the closed lifecycle vocabulary of one export record:
// requested (awaiting generation), ready (document downloadable until
// expiry) and expired (no longer downloadable).
type ExportStatus string

const (
	// ExportStatusRequested marks a job awaiting generation.
	ExportStatusRequested ExportStatus = "requested"
	// ExportStatusReady marks a generated, downloadable document.
	ExportStatusReady ExportStatus = "ready"
	// ExportStatusExpired marks a record past its expiry.
	ExportStatusExpired ExportStatus = "expired"
)

// StepUpSatisfied reports whether a session age meets the export step-up
// rule. Negative ages are incoherent and deny instead of being treated as
// fresh; the boundary instant is accepted.
func StepUpSatisfied(sessionAge time.Duration) bool {
	if sessionAge < 0 {
		return false
	}
	return sessionAge <= ExportStepUpWindow
}

// ExportAvailable reports whether a ready export may still be downloaded at
// the given instant: the document exists, the link has not expired and the
// download budget is not exhausted. Counts and instants come from the
// record, never from a client claim.
func ExportAvailable(status ExportStatus, expiresAt time.Time, downloadCount, maxDownloads int32, now time.Time) bool {
	if status != ExportStatusReady || maxDownloads <= 0 || downloadCount < 0 || downloadCount >= maxDownloads {
		return false
	}
	return now.UTC().Before(expiresAt.UTC())
}
