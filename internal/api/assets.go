package api

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
)

// Asset is a downloadable URL found on a Meshy task response.
type Asset struct {
	// Name is a stable key usable with --format (e.g. "glb", "image_0", "base_color").
	Name string `json:"name"`
	// URL is the download location.
	URL string `json:"url"`
	// Kind is a coarse type: model, image, texture, animation, other.
	Kind string `json:"kind"`
	// Ext is a suggested file extension including the dot (e.g. ".glb", ".png").
	Ext string `json:"ext,omitempty"`
}

// Assets walks the task payload and returns every downloadable asset URL.
// Covers model_urls, image_urls, texture_urls (map or []map), thumbnail_url(s), nested result.*_url, etc.
func (t Task) Assets() []Asset {
	if t == nil {
		return nil
	}
	candidates := map[string][]string{}

	add := func(name, raw string) {
		raw = strings.TrimSpace(raw)
		name = strings.TrimSpace(name)
		if raw == "" || name == "" || !looksLikeURL(raw) {
			return
		}
		for _, existing := range candidates[raw] {
			if existing == name {
				return
			}
		}
		candidates[raw] = append(candidates[raw], name)
	}

	// model_urls map
	if m, ok := t["model_urls"].(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				add("model_urls."+k, s)
				add(k, s)
			}
		}
	}

	// texture_urls: either map[string]string OR []map[string]string (Meshy refine PBR)
	switch tv := t["texture_urls"].(type) {
	case map[string]any:
		for k, v := range tv {
			if s, ok := v.(string); ok {
				add("texture_urls."+k, s)
				add(k, s)
			}
		}
	case []any:
		for i, item := range tv {
			if m, ok := item.(map[string]any); ok {
				for k, v := range m {
					if s, ok := v.(string); ok {
						// Prefer bare map key (base_color); also indexed form
						add(k, s)
						add(fmt.Sprintf("texture_%d_%s", i, k), s)
					}
				}
			} else if s, ok := item.(string); ok {
				add(fmt.Sprintf("texture_%d", i), s)
			}
		}
	}

	// thumbnail_urls map
	if m, ok := t["thumbnail_urls"].(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				add("thumbnail_"+k, s)
			}
		}
	}

	// Top-level scalar *_url / *_uri
	for k, v := range t {
		if s, ok := v.(string); ok && isURLKey(k) {
			add(k, s)
		}
	}

	// image_urls array
	if arr, ok := t["image_urls"].([]any); ok {
		for i, v := range arr {
			if s, ok := v.(string); ok {
				add(fmt.Sprintf("image_%d", i), s)
			}
		}
	}

	// Nested result / output
	for _, nestKey := range []string{"result", "output"} {
		if nested, ok := t[nestKey].(map[string]any); ok {
			walkAssetMap(nested, nestKey, add)
		}
	}

	var out []Asset
	for raw, names := range candidates {
		best := pickBestName(names)
		ext := extFromURL(raw)
		if ext == "" {
			ext = extFromName(best)
		}
		out = append(out, Asset{
			Name: best,
			URL:  raw,
			Kind: kindFrom(best, ext),
			Ext:  ext,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func pickBestName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	best := names[0]
	for _, n := range names[1:] {
		if preferredName(n, best) {
			best = n
		}
	}
	// Prefer stripping trailing _url when that improves the score
	if strings.HasSuffix(best, "_url") {
		short := strings.TrimSuffix(best, "_url")
		if preferredName(short, best) {
			best = short
		}
	}
	return best
}

func walkAssetMap(m map[string]any, prefix string, add func(name, raw string)) {
	for k, v := range m {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		switch val := v.(type) {
		case string:
			if isURLKey(k) || looksLikeURL(val) {
				add(name, val)
				if prefix != "" {
					add(k, val)
					if strings.HasSuffix(k, "_url") {
						add(strings.TrimSuffix(k, "_url"), val)
					}
				}
			}
		case map[string]any:
			walkAssetMap(val, name, add)
		case []any:
			for i, item := range val {
				itemName := fmt.Sprintf("%s_%d", name, i)
				if s, ok := item.(string); ok {
					add(itemName, s)
				} else if nested, ok := item.(map[string]any); ok {
					walkAssetMap(nested, itemName, add)
				}
			}
		}
	}
}

func preferredName(a, b string) bool {
	score := func(s string) int {
		n := 100
		if strings.Contains(s, ".") {
			n -= 30
		}
		if strings.HasSuffix(s, "_url") || strings.HasSuffix(s, "_uri") {
			n -= 15
		}
		if strings.HasPrefix(s, "texture_") && strings.Count(s, "_") >= 2 {
			n -= 10 // prefer bare base_color over texture_0_base_color
		}
		if len(s) <= 5 {
			n += 20
		}
		// Prefer known texture map keys
		switch s {
		case "base_color", "normal", "metallic", "roughness", "emission", "glb", "fbx", "obj", "usdz", "stl":
			n += 25
		}
		n -= len(s)
		return n
	}
	return score(a) > score(b)
}

func isURLKey(k string) bool {
	k = strings.ToLower(k)
	return strings.HasSuffix(k, "_url") ||
		strings.HasSuffix(k, "_uri") ||
		k == "url" ||
		k == "uri"
}

func looksLikeURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "data:")
}

func extFromURL(raw string) string {
	if strings.HasPrefix(raw, "data:") {
		rest := strings.TrimPrefix(raw, "data:")
		if i := strings.Index(rest, ";"); i >= 0 {
			rest = rest[:i]
		}
		if i := strings.Index(rest, "/"); i >= 0 {
			return "." + rest[i+1:]
		}
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if i := strings.LastIndex(base, "."); i >= 0 && i < len(base)-1 {
		ext := strings.ToLower(base[i:])
		if len(ext) <= 6 {
			return ext
		}
	}
	return ""
}

func extFromName(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "glb"):
		return ".glb"
	case strings.Contains(n, "fbx"):
		return ".fbx"
	case strings.Contains(n, "obj"):
		return ".obj"
	case strings.Contains(n, "usdz"):
		return ".usdz"
	case strings.Contains(n, "stl"):
		return ".stl"
	case strings.Contains(n, "3mf"):
		return ".3mf"
	case strings.Contains(n, "mtl"):
		return ".mtl"
	case strings.Contains(n, "blend"):
		return ".blend"
	case strings.Contains(n, "base_color") || strings.Contains(n, "normal") ||
		strings.Contains(n, "metallic") || strings.Contains(n, "roughness") ||
		strings.Contains(n, "emission") || strings.Contains(n, "png") ||
		strings.HasPrefix(n, "image") || strings.Contains(n, "thumbnail") || strings.Contains(n, "texture"):
		return ".png"
	case strings.Contains(n, "jpg") || strings.Contains(n, "jpeg"):
		return ".jpg"
	case strings.Contains(n, "mp4") || strings.Contains(n, "video"):
		return ".mp4"
	}
	return ""
}

func kindFrom(name, ext string) string {
	n := strings.ToLower(name + ext)
	switch {
	case strings.Contains(n, "base_color") || strings.Contains(n, "normal") ||
		strings.Contains(n, "metallic") || strings.Contains(n, "roughness") ||
		strings.Contains(n, "emission") || strings.Contains(n, "texture"):
		return "texture"
	case strings.Contains(n, "image") || strings.Contains(n, "thumbnail") ||
		ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp":
		return "image"
	case strings.Contains(n, "anim") || strings.Contains(n, "walking") || strings.Contains(n, "running"):
		return "animation"
	case ext == ".glb" || ext == ".fbx" || ext == ".obj" || ext == ".usdz" || ext == ".stl" || ext == ".3mf" ||
		strings.Contains(n, "model") || strings.Contains(n, "mesh") || strings.Contains(n, "rigged"):
		return "model"
	default:
		return "other"
	}
}

// SuggestedFilename returns a clean filesystem name.
// Prefer "{slug}-{name}{ext}" or "{name}{ext}" without doubling extensions (no "glb.glb").
func (a Asset) SuggestedFilename(slug string) string {
	base := a.Name
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	base = strings.ReplaceAll(base, ".", "_")
	base = strings.TrimSuffix(base, "_url")
	base = strings.TrimSuffix(base, "_uri")
	ext := a.Ext
	if ext == "" {
		ext = ".bin"
	}
	// Avoid model_glb.glb style when name is exactly the format key
	extBare := strings.TrimPrefix(ext, ".")
	if strings.EqualFold(base, extBare) {
		base = "model"
	}
	// image_0 stays image_0; base_color stays base_color
	if slug != "" {
		return slug + "-" + base + ext
	}
	return base + ext
}

// ParseFormatList splits comma-separated and/or multi-value format flags.
func ParseFormatList(vals []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range vals {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			k := strings.ToLower(p)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// SelectAssets picks assets for download.
// formats: explicit names (glb, fbx, base_color, image_0, …); empty = use defaults.
// kinds: filter by kind when non-empty.
// all: everything.
// wantTextures: also include kind=texture when selecting model defaults.
func SelectAssets(assets []Asset, formats []string, kinds []string, all, wantTextures bool) []Asset {
	if len(assets) == 0 {
		return nil
	}
	if all {
		return assets
	}
	formats = ParseFormatList(formats)
	if len(formats) > 0 {
		var out []Asset
		for _, f := range formats {
			matched := FilterAssets(assets, f, "")
			out = append(out, matched...)
		}
		// also allow kind names in --format for convenience
		for _, f := range formats {
			switch f {
			case "texture", "textures":
				out = append(out, FilterAssets(assets, "", "texture")...)
			case "image", "images":
				out = append(out, FilterAssets(assets, "", "image")...)
			case "model", "models":
				out = append(out, FilterAssets(assets, "", "model")...)
			}
		}
		if wantTextures {
			out = append(out, FilterAssets(assets, "", "texture")...)
		}
		return dedupeAssets(out)
	}
	if len(kinds) > 0 {
		var out []Asset
		for _, k := range kinds {
			out = append(out, FilterAssets(assets, "", k)...)
		}
		return dedupeAssets(out)
	}

	// Defaults: glb if present; else all images; else first model; always optional textures flag
	var out []Asset
	glb := FilterAssets(assets, "glb", "")
	if len(glb) > 0 {
		out = append(out, glb...)
	} else {
		images := FilterAssets(assets, "", "image")
		// Prefer image_* over thumbnail when both exist
		var primary []Asset
		for _, a := range images {
			if strings.HasPrefix(a.Name, "image_") {
				primary = append(primary, a)
			}
		}
		if len(primary) > 0 {
			out = append(out, primary...)
		} else if len(images) > 0 {
			out = append(out, images...)
		} else {
			models := FilterAssets(assets, "", "model")
			if len(models) > 0 {
				out = append(out, models[0])
			} else if len(assets) > 0 {
				out = append(out, assets[0])
			}
		}
	}
	if wantTextures {
		out = append(out, FilterAssets(assets, "", "texture")...)
	}
	return dedupeAssets(out)
}

func dedupeAssets(in []Asset) []Asset {
	seen := map[string]bool{}
	var out []Asset
	for _, a := range in {
		if seen[a.URL] {
			continue
		}
		seen[a.URL] = true
		out = append(out, a)
	}
	return out
}

// FilterAssets returns assets matching name or kind (case-insensitive). Empty filters match all.
func FilterAssets(assets []Asset, name, kind string) []Asset {
	if name == "" && kind == "" {
		return assets
	}
	var out []Asset
	name = strings.ToLower(name)
	kind = strings.ToLower(kind)
	for _, a := range assets {
		if name != "" {
			an := strings.ToLower(a.Name)
			match := an == name ||
				strings.TrimSuffix(an, "_url") == name ||
				strings.HasSuffix(an, "."+name) ||
				strings.HasSuffix(an, "_"+name) ||
				strings.TrimPrefix(a.Ext, ".") == name
			if !match {
				continue
			}
		}
		if kind != "" && !strings.EqualFold(a.Kind, kind) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// KnownImageModels are documented ai_model values for text-to-image / image-to-image.
var KnownImageModels = []struct {
	ID          string
	Description string
}{
	{"nano-banana", "Standard image model"},
	{"nano-banana-2", "More capable than nano-banana"},
	{"nano-banana-2-lite", "Lite version of nano-banana-2"},
	{"nano-banana-pro", "Pro quality (Pro plan or higher)"},
	{"gpt-image-2", "OpenAI GPT Image 2"},
	{"gpt-image-2-5-flare", "OpenAI GPT Image 2.5 (Flare)"},
	{"gpt-image-2-5-sunburst", "OpenAI GPT Image 2.5 (Sunburst)"},
}

// Slugify makes a filesystem-safe short name from a prompt or label.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 48 {
		out = out[:48]
		out = strings.Trim(out, "-")
	}
	if out == "" {
		return "meshy"
	}
	return out
}
