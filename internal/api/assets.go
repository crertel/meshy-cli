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
	// Name is a stable key usable with --name / --format (e.g. "glb", "image_0", "animation_glb").
	Name string `json:"name"`
	// URL is the download location.
	URL string `json:"url"`
	// Kind is a coarse type: model, image, texture, animation, other.
	Kind string `json:"kind"`
	// Ext is a suggested file extension including the dot (e.g. ".glb", ".png").
	Ext string `json:"ext,omitempty"`
}

// Assets walks the task payload and returns every downloadable asset URL.
// Covers model_urls, image_urls, texture_urls, thumbnail_url(s), nested result.*_url, etc.
func (t Task) Assets() []Asset {
	if t == nil {
		return nil
	}
	// url → candidate names
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

	// Top-level map-style buckets
	for _, key := range []string{"model_urls", "texture_urls", "thumbnail_urls"} {
		if m, ok := t[key].(map[string]any); ok {
			for k, v := range m {
				if s, ok := v.(string); ok {
					add(key+"."+k, s)
					if key == "model_urls" || key == "texture_urls" {
						add(k, s)
					}
				}
			}
		}
	}

	// Top-level scalar *_url / *_uri
	for k, v := range t {
		if s, ok := v.(string); ok && isURLKey(k) {
			add(k, s)
		}
	}

	// image_urls array (text-to-image, image-to-image)
	if arr, ok := t["image_urls"].([]any); ok {
		for i, v := range arr {
			if s, ok := v.(string); ok {
				add(fmt.Sprintf("image_%d", i), s)
			}
		}
	}

	// Nested result / output objects (rigging, animation, print, etc.)
	for _, nestKey := range []string{"result", "output"} {
		if nested, ok := t[nestKey].(map[string]any); ok {
			walkAssetMap(nested, nestKey, add)
		}
	}

	var out []Asset
	for raw, names := range candidates {
		best := names[0]
		for _, n := range names[1:] {
			if preferredName(n, best) {
				best = n
			}
		}
		// Prefer stripping trailing _url for cleaner CLI keys when that form exists
		for _, n := range names {
			if strings.HasSuffix(n, "_url") {
				short := strings.TrimSuffix(n, "_url")
				if preferredName(short, best) || short == best {
					// keep short if it's in candidates or derived
					best = short
					break
				}
			}
		}
		// re-check: if we shortened, fine; also prefer bare "glb" over "model_urls.glb"
		for _, n := range names {
			if preferredName(n, best) {
				best = n
			}
		}
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
		// prefer short keys like glb, fbx
		if len(s) <= 5 {
			n += 20
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
	case strings.Contains(n, "blend"):
		return ".blend"
	case strings.Contains(n, "png") || strings.HasPrefix(n, "image") || strings.Contains(n, "thumbnail") || strings.Contains(n, "texture"):
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
	case strings.Contains(n, "image") || strings.Contains(n, "thumbnail") || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp":
		return "image"
	case strings.Contains(n, "texture"):
		return "texture"
	case strings.Contains(n, "anim") || strings.Contains(n, "walking") || strings.Contains(n, "running"):
		return "animation"
	case ext == ".glb" || ext == ".fbx" || ext == ".obj" || ext == ".usdz" || ext == ".stl" || ext == ".3mf" ||
		strings.Contains(n, "model") || strings.Contains(n, "mesh") || strings.Contains(n, "rigged"):
		return "model"
	default:
		return "other"
	}
}

// SuggestedFilename returns a filesystem-safe name for saving the asset.
func (a Asset) SuggestedFilename(taskID string) string {
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
	if taskID != "" {
		return taskID + "-" + base + ext
	}
	return base + ext
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
	{"nano-banana", "Standard model (3 credits per image)"},
	{"nano-banana-2", "Balanced / stronger (6 credits per image)"},
	{"nano-banana-pro", "Pro quality (9 credits per image)"},
	{"gpt-image-2", "OpenAI GPT Image 2 (higher cost; limited aspect ratios)"},
}
