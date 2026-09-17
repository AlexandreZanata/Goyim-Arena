package http

import (
	"net/http"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
)

func init() {
	httpserver.RegisterRouteProvider(Routes)
}

// Routes returns the canonical list of versioned HTTP routes exposed by the
// persuasion module (P11-T06): the authenticated attribution recording under
// /api/v1/me plus the public count reads per argument and per profile under
// /api/v1. No route ever lists attributors.
func Routes() []httpserver.Route {
	return []httpserver.Route{
		{Method: http.MethodPost, Path: "/api/v1/me/position-changes/{id}/attributions"},
		{Method: http.MethodGet, Path: "/api/v1/arguments/{id}/attributions"},
		{Method: http.MethodGet, Path: "/api/v1/profiles/{username}/reputation"},
	}
}
