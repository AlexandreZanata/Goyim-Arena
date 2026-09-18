package http

import (
	"net/http"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
)

func init() {
	httpserver.RegisterRouteProvider(Routes)
}

// Routes returns the canonical list of public transparency routes (P14-T03):
// the versioned JSON metrics document and the HTML transparency document.
// Both are unauthenticated and cacheable.
func Routes() []httpserver.Route {
	return []httpserver.Route{
		{Method: http.MethodGet, Path: "/api/v1/public/transparency"},
		{Method: http.MethodGet, Path: "/transparency"},
	}
}
