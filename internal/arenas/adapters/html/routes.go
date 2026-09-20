package html

import (
	"net/http"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
)

func init() {
	httpserver.RegisterRouteProvider(Routes)
}

// Routes returns the canonical list of HTML routes exposed by the arenas
// module: the cacheable public Arena document (P08-T08) and the browser
// participation journey (P18-T06). They stay outside /api/v1 because they serve
// HTML, not the JSON API, and the journey is listed route by route because each
// one is a decision about who may call it.
func Routes() []httpserver.Route {
	return []httpserver.Route{
		{Method: http.MethodGet, Path: "/d/{slug}"},
		{Method: http.MethodGet, Path: "/arenas/{slug}"},
		{Method: http.MethodPost, Path: "/arenas/{slug}/position"},
		{Method: http.MethodPost, Path: "/arenas/{slug}/position/change"},
		{Method: http.MethodPost, Path: "/arenas/{slug}/arguments"},
		{Method: http.MethodPost, Path: "/arenas/{slug}/attributions"},
	}
}
