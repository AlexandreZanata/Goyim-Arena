// Package assets resolves the immutable frontend URLs emitted by cmd/assetgen.
// The manifest is loaded by composition and passed to html/template; no
// template guesses filenames or serves an un-hashed fallback.
package assets

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
)

type Manifest struct {
	Version int               `json:"version"`
	Assets  map[string]Record `json:"assets"`
}

type Record struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func Load(reader io.Reader) (Manifest, error) {
	if reader == nil {
		return Manifest{}, errors.New("assets: manifest reader is required")
	}
	var manifest Manifest
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("assets: decode manifest: %w", err)
	}
	if manifest.Version != 1 || len(manifest.Assets) == 0 {
		return Manifest{}, errors.New("assets: unsupported or empty manifest")
	}
	for name, record := range manifest.Assets {
		if name == "" || record.Path == "" || record.SHA256 == "" {
			return Manifest{}, fmt.Errorf("assets: invalid record %q", name)
		}
	}
	return manifest, nil
}

// URL resolves a logical source path. Missing assets return an error so a
// broken build cannot silently render a page with an unversioned URL.
func (manifest Manifest) URL(name string) (string, error) {
	record, ok := manifest.Assets[name]
	if !ok {
		return "", fmt.Errorf("assets: asset %q is not in the manifest", name)
	}
	return record.Path, nil
}

// TemplateFuncs returns the only asset helper templates should use.
func (manifest Manifest) TemplateFuncs() template.FuncMap {
	return template.FuncMap{"asset": manifest.URL}
}
