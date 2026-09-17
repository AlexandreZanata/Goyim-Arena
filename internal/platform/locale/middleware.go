package locale

import (
	"net/http"
	"sync/atomic"
)

// fallbackCount tracks how many requests resolved without any explicit
// signal (precedence 5). It is an anonymous process-local counter: no tag,
// no request data, no PII — the I18N standard asks for a metric, not a
// profile.
var fallbackCount atomic.Uint64

// FallbackCount reports how many negotiations ended on the default because
// the request carried no usable locale signal.
func FallbackCount() uint64 { return fallbackCount.Load() }

// InterfaceLocaleHeader echoes the resolved interface locale to clients.
const InterfaceLocaleHeader = "X-Interface-Locale"

// SetHandler wraps a handler with locale negotiation (I18N_STANDARD.md §4).
// On every request it:
//
//   - resolves the locale through the full precedence chain;
//   - increments the anonymous fallback metric when the resolution lands
//     on the default with no explicit signal from the request;
//   - stores the resolved locale in the request context (presentation
//     layer only);
//   - echoes the resolved locale to the client in X-Interface-Locale.
//
// The header is set before the handler runs, so it is present even on
// panics and short-circuit paths.
func SetHandler(resolver *Resolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		tag, resolvedFrom := resolver.resolve(request)
		if resolvedFrom == sourceNone {
			fallbackCount.Add(1)
		}
		writer.Header().Set(InterfaceLocaleHeader, string(tag))
		next.ServeHTTP(writer, request.WithContext(WithLocale(request.Context(), tag)))
	})
}
