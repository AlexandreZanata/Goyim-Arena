// Package httpcache centralizes response-cache policy for inbound HTTP adapters.
// Private responses are never reusable, public responses are explicitly bounded,
// and validators are derived from stable facts rather than moving annotations.
package httpcache

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
)

const (
	privatePolicy = "private, no-store, no-cache, must-revalidate"
	noStorePolicy = "no-store"
)

// Private marks a response as account- or request-specific. This policy is
// also appropriate for rejection responses from authenticated middleware.
func Private(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", privatePolicy)
	w.Header().Set("Pragma", "no-cache")
}

// NoStore marks a response that must not be retained by any cache.
func NoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", noStorePolicy)
}

// Public marks a response as explicitly cacheable for maxAge seconds. Public
// responses vary only by controlled representation encoding; callers that
// negotiate locale must add their own controlled locale dimension.
func Public(w http.ResponseWriter, maxAge int) {
	if len(w.Header().Values("Set-Cookie")) > 0 {
		Private(w)
		return
	}
	if maxAge < 0 {
		maxAge = 0
	}
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAge))
	vary := w.Header().Get("Vary")
	if vary == "" {
		w.Header().Set("Vary", "Accept-Encoding")
	} else if !strings.Contains(vary, "Accept-Encoding") {
		w.Header().Set("Vary", vary+", Accept-Encoding")
	}
}

// Validator returns a strong quoted ETag for an exact representation.
func Validator(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// WeakValidator returns a weak quoted ETag for stable facts whose presentation
// may carry changing metadata, such as a derivation timestamp.
func WeakValidator(facts []byte) string {
	sum := sha256.Sum256(facts)
	return `W/"` + hex.EncodeToString(sum[:]) + `"`
}

// Matches performs the weak comparison required by If-None-Match.
func Matches(header, etag string) bool {
	header = strings.TrimSpace(header)
	etag = strings.TrimPrefix(etag, "W/")
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "W/"))
		if candidate == etag {
			return true
		}
	}
	return false
}

// SetValidator applies a validator to a response.
func SetValidator(w http.ResponseWriter, etag string) {
	w.Header().Set("ETag", etag)
}
