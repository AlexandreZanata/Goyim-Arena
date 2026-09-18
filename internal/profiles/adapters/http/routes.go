package http

import (
	"net/http"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
)

func init() {
	httpserver.RegisterRouteProvider(Routes)
}

// Routes returns the canonical list of versioned HTTP routes exposed by the
// profiles module (P05-T04, P14-T05): the separated public and private
// profiles plus the authenticated personal data export.
func Routes() []httpserver.Route {
	return []httpserver.Route{
		{Method: http.MethodGet, Path: "/api/v1/profiles/{username}"},
		{Method: http.MethodGet, Path: "/api/v1/me/profile"},
		{Method: http.MethodPost, Path: "/api/v1/me/exports"},
		{Method: http.MethodGet, Path: "/api/v1/me/exports/{id}/download"},
	}
}
