package http

import (
	"net/http"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
)

func init() {
	httpserver.RegisterRouteProvider(Routes)
}

// Routes returns the canonical list of versioned HTTP routes exposed by the
// arenas module (P08-T07): the authenticated draft CRUD, publication and
// closing, plus the public feed and the public Arena document. Moderation
// stays internal: no public route restricts or removes an Arena.
func Routes() []httpserver.Route {
	return []httpserver.Route{
		{Method: http.MethodPost, Path: "/api/v1/me/arenas/drafts"},
		{Method: http.MethodGet, Path: "/api/v1/me/arenas/drafts"},
		{Method: http.MethodGet, Path: "/api/v1/me/arenas/drafts/{id}"},
		{Method: http.MethodPatch, Path: "/api/v1/me/arenas/drafts/{id}"},
		{Method: http.MethodDelete, Path: "/api/v1/me/arenas/drafts/{id}"},
		{Method: http.MethodPost, Path: "/api/v1/me/arenas/drafts/{id}/publish"},
		{Method: http.MethodPost, Path: "/api/v1/me/arenas/{id}/close"},
		{Method: http.MethodGet, Path: "/api/v1/arenas"},
		{Method: http.MethodGet, Path: "/api/v1/arenas/{slug}"},
	}
}
