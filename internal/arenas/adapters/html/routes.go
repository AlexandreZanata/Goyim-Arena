package html

import (
	"net/http"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
)

func init() {
	httpserver.RegisterRouteProvider(Routes)
}

// Routes returns the canonical list of HTML routes exposed by the arenas
// module (P08-T08): the cacheable public Arena document. It stays outside
// /api/v1 because it serves HTML, not the JSON API.
func Routes() []httpserver.Route {
	return []httpserver.Route{
		{Method: http.MethodGet, Path: "/d/{slug}"},
	}
}
