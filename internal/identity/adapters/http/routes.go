package http

import (
	"net/http"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
)

func init() {
	httpserver.RegisterRouteProvider(Routes)
}

// Routes returns the canonical list of versioned HTTP routes exposed by the identity module (P04-T08).
func Routes() []httpserver.Route {
	return []httpserver.Route{
		{Method: http.MethodPost, Path: "/api/v1/auth/register"},
		{Method: http.MethodGet, Path: "/api/v1/auth/verify"},
		{Method: http.MethodPost, Path: "/api/v1/auth/login"},
		{Method: http.MethodPost, Path: "/api/v1/auth/logout"},
		{Method: http.MethodPost, Path: "/api/v1/auth/password-reset/request"},
		{Method: http.MethodGet, Path: "/api/v1/auth/password-reset"},
		{Method: http.MethodPost, Path: "/api/v1/auth/password-reset/confirm"},
	}
}
