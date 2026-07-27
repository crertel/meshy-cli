package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crertel/meshy-cli/internal/api"
)

// downloadOpts controls asset selection and output layout.
type downloadOpts struct {
	outDir   string
	slug     string   // filename prefix (default: task id short or "model")
	formats  []string // multi --format glb,fbx or repeated
	kinds    []string
	all      bool
	textures bool // also include PBR texture maps
	wait     bool
	// legacy single name (merged into formats)
	name   string
	format string
}

func (o *downloadOpts) resolveFormats() []string {
	var f []string
	f = append(f, o.formats...)
	if o.format != "" {
		f = append(f, o.format)
	}
	if o.name != "" {
		f = append(f, o.name)
	}
	return api.ParseFormatList(f)
}

func (app *App) downloadTaskAssets(res api.Resource, id string, t api.Task, opts downloadOpts) (map[string]string, error) {
	assets := t.Assets()
	if len(assets) == 0 {
		return nil, fmt.Errorf("no downloadable assets on task %s (status=%s); try: meshy %s assets %s", id, t.Status(), res.Name, id)
	}

	// Merge profile download defaults when no explicit selection
	formats := opts.resolveFormats()
	all := opts.all
	textures := opts.textures
	if app.Profile != nil && len(formats) == 0 && !all && len(opts.kinds) == 0 {
		formats = append(formats, app.Profile.DownloadFormats...)
		if app.Profile.DownloadAll {
			all = true
		}
		if app.Profile.DownloadTextures {
			textures = true
		}
	}

	selected := api.SelectAssets(assets, formats, opts.kinds, all, textures)
	if len(selected) == 0 {
		names := make([]string, 0, len(assets))
		for _, a := range assets {
			names = append(names, a.Name)
		}
		return nil, fmt.Errorf("no assets matched filters (available: %s)", strings.Join(names, ", "))
	}

	outDir := opts.outDir
	if outDir == "" {
		outDir = id
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	slug := opts.slug
	if slug == "" {
		slug = "model"
	}

	saved := map[string]string{}
	for _, a := range selected {
		data, err := app.Client.Download(app.ctx(), a.URL)
		if err != nil {
			return saved, fmt.Errorf("download %s: %w", a.Name, err)
		}
		path := filepath.Join(outDir, a.SuggestedFilename(slug))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return saved, err
		}
		saved[a.Name] = path
	}
	return saved, nil
}
