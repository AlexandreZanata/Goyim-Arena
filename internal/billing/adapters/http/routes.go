package http

import (
	"net/http"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
)

func init() {
	httpserver.RegisterRouteProvider(Routes)
}

// Routes returns the canonical list of versioned HTTP routes exposed by the
// billing module (P07-T06): authenticated read-only pass summary and
// consumption history. Granting and consuming have no public endpoints.
func Routes() []httpserver.Route {
	return []httpserver.Route{
		{Method: http.MethodGet, Path: "/api/v1/me/passes"},
		{Method: http.MethodGet, Path: "/api/v1/me/passes/history"},
	}
}
