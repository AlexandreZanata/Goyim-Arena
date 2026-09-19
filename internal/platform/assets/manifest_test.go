package assets_test

import (
	"strings"
	"testing"
	"text/template"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/assets"
)

const manifestJSON = `{"version":1,"assets":{"main.js":{"path":"/assets/main-abc123.js","sha256":"abc123"}}}`

func TestManifestResolvesOnlyDeclaredAssets(t *testing.T) {
	manifest, err := assets.Load(strings.NewReader(manifestJSON))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := manifest.URL("main.js"); err != nil || got != "/assets/main-abc123.js" {
		t.Fatalf("URL(main.js) = %q, %v", got, err)
	}
	if _, err := manifest.URL("missing.js"); err == nil {
		t.Fatal("missing asset must fail instead of falling back to an unhashed URL")
	}
}

func TestManifestTemplateHelperResolvesHashedURL(t *testing.T) {
	manifest, err := assets.Load(strings.NewReader(manifestJSON))
	if err != nil {
		t.Fatal(err)
	}
	page, err := template.New("page").Funcs(manifest.TemplateFuncs()).Parse(`<script src="{{asset "main.js"}}"></script>`)
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := page.Execute(&output, nil); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != `<script src="/assets/main-abc123.js"></script>` {
		t.Fatalf("rendered page = %q", got)
	}
}

func TestManifestRejectsMalformedRecords(t *testing.T) {
	for _, input := range []string{
		`{"version":2,"assets":{"main.js":{"path":"/main.js","sha256":"x"}}}`,
		`{"version":1,"assets":{"main.js":{"path":"","sha256":"x"}}}`,
		`{"version":1,"assets":{}}`,
	} {
		if _, err := assets.Load(strings.NewReader(input)); err == nil {
			t.Fatalf("manifest %s was accepted", input)
		}
	}
}
