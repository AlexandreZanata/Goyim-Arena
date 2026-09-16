// Package httperror maps the HTTP-free application error vocabulary
// (internal/platform/apperr) to RFC 9457 Problem Details responses
// (P02-T03). It is the only place where the vocabulary gains HTTP semantics:
// statuses, media type and public bodies live here.
//
// Security invariants, enforced by table tests:
//   - the wrapped internal cause never serializes into the body;
//   - unknown errors render as generic internal problems, never echoing
//     foreign error strings to the client;
//   - every problem carries a stable code, a title derived from the kind,
//     and the request correlation ID.
package httperror

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/apperr"
)

// mediaType is the RFC 9457 problem media type mandated by the master plan.
const mediaType = "application/problem+json"

// genericInternalTitle mirrors the plan rule for unexpected failures: the
// public body stays generic; details go to logs, not to clients.
const genericInternalMessage = "internal error"

// statusByKind fixes the public HTTP status for each stable kind.
var statusByKind = map[apperr.Kind]int{
	apperr.KindValidation:   http.StatusBadRequest,
	apperr.KindUnauthorized: http.StatusUnauthorized,
	apperr.KindForbidden:    http.StatusForbidden,
	apperr.KindNotFound:     http.StatusNotFound,
	apperr.KindConflict:     http.StatusConflict,
	apperr.KindRateLimited:  http.StatusTooManyRequests,
	apperr.KindInternal:     http.StatusInternalServerError,
}

// titleByKind fixes the public title for each stable kind. Internal errors
// never expose their real title or detail.
var titleByKind = map[apperr.Kind]string{
	apperr.KindValidation:   "the request is invalid",
	apperr.KindUnauthorized: "authentication is required",
	apperr.KindForbidden:    "you are not allowed to do this",
	apperr.KindNotFound:     "resource not found",
	apperr.KindConflict:     "the request conflicts with the current state",
	apperr.KindRateLimited:  "too many requests",
	apperr.KindInternal:     genericInternalMessage,
}

// Problem is the RFC 9457 problem details document sent to clients.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// WriteProblem renders an error as an application/problem+json response and
// returns the rendered status, so handlers stay one-liners while tests can
// assert on the mapping. requestID correlates the response with logs.
func WriteProblem(writer http.ResponseWriter, requestID string, err error) int {
	problem := ProblemFor(requestID, err)

	writer.Header().Set("Content-Type", mediaType)
	writer.WriteHeader(problem.Status)
	_ = json.NewEncoder(writer).Encode(problem)
	return problem.Status
}

// ProblemFor builds the public problem document for an error. Foreign errors
// collapse into a generic internal problem; wrapped causes never leak.
func ProblemFor(requestID string, err error) Problem {
	kind := apperr.KindOf(err)
	status := statusByKind[kind]
	title := titleByKind[kind]
	detail := ""

	var appError *apperr.Error
	if errors.As(err, &appError) {
		if code := appError.Code(); code != "" {
			return Problem{
				Type:      typeFor(kind),
				Title:     title,
				Status:    status,
				Code:      appError.Code(),
				RequestID: requestID,
				Detail:    publicDetail(appError, kind),
			}
		}
	}

	return Problem{
		Type:      typeFor(kind),
		Title:     title,
		Status:    status,
		Code:      codeFor(kind),
		RequestID: requestID,
		Detail:    detail,
	}
}

// publicDetail exposes the apperr detail only when the kind is safe to do
// so; internal failures keep the body generic.
func publicDetail(appError *apperr.Error, kind apperr.Kind) string {
	if kind == apperr.KindInternal {
		return ""
	}
	return appError.Detail()
}

// typeFor renders the problem type URI; a stable fragment keeps clients from
// string-matching titles.
func typeFor(kind apperr.Kind) string {
	return "https://goyim-arena.dev/problems/" + string(kind)
}

// codeFor renders the fallback code for non-vocabulary errors.
func codeFor(kind apperr.Kind) string {
	return "ARENA-" + string(kind)
}
