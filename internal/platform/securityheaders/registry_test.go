// Blank imports of every inbound adapter that registers routes, mirroring the
// import list of internal/contract. The header scanner in
// securityheaders_test.go has to walk the same registry the arena binary
// serves; without these imports it would only ever see the health routes and
// would pass while scanning almost nothing.
package securityheaders_test

import (
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/arenas/adapters/html"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/arenas/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/arguments/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/billing/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/identity/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/jobs/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/moderation/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/persuasion/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/positions/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/profiles/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/transparency/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/wallet/adapters/http"
)
