package cli

import (
	"fmt"
	"strings"
)

// Profile is a named bundle of create/download defaults.
// Explicit flags always win over profile values.
type Profile struct {
	Name        string
	Description string

	// Generation defaults (merged into create bodies when unset)
	AIModel         string
	ShouldRemesh    *bool
	TargetPolycount int
	ShouldTexture   *bool
	EnablePBR       *bool
	TextureRichness string // high, medium, low if API accepts

	// Download defaults
	DownloadFormats  []string // e.g. glb, fbx
	DownloadTextures bool     // also pull loose PBR maps
	DownloadAll      bool
}

// BuiltinProfiles are the shipped opt-in defaults.
var BuiltinProfiles = []Profile{
	{
		Name:            "game-lowpoly",
		Description:     "Realtime / engine: remesh, lower poly, glb only",
		AIModel:         "latest",
		ShouldRemesh:    boolPtr(true),
		TargetPolycount: 30000,
		EnablePBR:       boolPtr(true),
		DownloadFormats: []string{"glb"},
	},
	{
		Name:            "game-hero",
		Description:     "Character focal: medium poly, PBR, glb",
		AIModel:         "latest",
		ShouldRemesh:    boolPtr(true),
		TargetPolycount: 50000,
		EnablePBR:       boolPtr(true),
		DownloadFormats: []string{"glb"},
		DownloadTextures: false, // glb embeds maps
	},
	{
		Name:             "cinema",
		Description:      "Higher fidelity: more poly, PBR, glb+fbx+texture maps",
		AIModel:          "latest",
		ShouldRemesh:     boolPtr(true),
		TargetPolycount:  100000,
		EnablePBR:        boolPtr(true),
		TextureRichness:  "high",
		DownloadFormats:  []string{"glb", "fbx"},
		DownloadTextures: true,
	},
	{
		Name:            "print",
		Description:     "Print-oriented downloads (stl); moderate poly",
		AIModel:         "latest",
		ShouldRemesh:    boolPtr(true),
		TargetPolycount: 40000,
		DownloadFormats: []string{"stl", "glb"},
	},
	{
		Name:            "fast",
		Description:     "Cheap/quick defaults: latest model, glb only",
		AIModel:         "latest",
		DownloadFormats: []string{"glb"},
	},
}

func boolPtr(b bool) *bool { return &b }

// ProfileByName looks up a built-in profile (case-insensitive).
func ProfileByName(name string) (Profile, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range BuiltinProfiles {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// ApplyProfileToBody fills body keys only when not already set.
func ApplyProfileToBody(body map[string]any, p Profile) {
	if body == nil {
		return
	}
	setIfMissing := func(key string, val any) {
		if _, ok := body[key]; ok {
			return
		}
		if val == nil {
			return
		}
		switch v := val.(type) {
		case string:
			if v != "" {
				body[key] = v
			}
		case int:
			if v != 0 {
				body[key] = v
			}
		default:
			body[key] = val
		}
	}
	setIfMissing("ai_model", p.AIModel)
	if p.ShouldRemesh != nil {
		setIfMissing("should_remesh", *p.ShouldRemesh)
	}
	setIfMissing("target_polycount", p.TargetPolycount)
	if p.ShouldTexture != nil {
		setIfMissing("should_texture", *p.ShouldTexture)
	}
	if p.EnablePBR != nil {
		setIfMissing("enable_pbr", *p.EnablePBR)
	}
	setIfMissing("texture_richness", p.TextureRichness)
}

// ProfileNames lists built-in profile ids.
func ProfileNames() []string {
	var n []string
	for _, p := range BuiltinProfiles {
		n = append(n, p.Name)
	}
	return n
}

func formatProfileHelp() string {
	var b strings.Builder
	for _, p := range BuiltinProfiles {
		fmt.Fprintf(&b, "  %-14s  %s\n", p.Name, p.Description)
	}
	return b.String()
}
