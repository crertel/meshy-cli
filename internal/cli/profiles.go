package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crertel/meshy-cli/internal/store"
)

// Profile is a named bundle of create/download defaults.
// Explicit flags always win over profile values.
type Profile struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Builtin     bool     `json:"builtin,omitempty"`
	Source      string   `json:"source,omitempty"` // "builtin" | "user"

	// Generation defaults (merged into create bodies when unset)
	AIModel         string `json:"ai_model,omitempty"`
	ShouldRemesh    *bool  `json:"should_remesh,omitempty"`
	TargetPolycount int    `json:"target_polycount,omitempty"`
	ShouldTexture *bool `json:"should_texture,omitempty"`
	EnablePBR     *bool `json:"enable_pbr,omitempty"`

	// Download defaults
	DownloadFormats  []string `json:"download_formats,omitempty"`
	DownloadTextures bool     `json:"download_textures,omitempty"`
	DownloadAll      bool     `json:"download_all,omitempty"`
}

// BuiltinProfiles are the shipped opt-in defaults.
var BuiltinProfiles = []Profile{
	{
		Name: "game-lowpoly", Description: "Realtime / engine: remesh, lower poly, glb only",
		Builtin: true, Source: "builtin",
		AIModel: "latest", ShouldRemesh: boolPtr(true), TargetPolycount: 30000,
		EnablePBR: boolPtr(true), DownloadFormats: []string{"glb"},
	},
	{
		Name: "game-hero", Description: "Character focal: medium poly, PBR, glb",
		Builtin: true, Source: "builtin",
		AIModel: "latest", ShouldRemesh: boolPtr(true), TargetPolycount: 50000,
		EnablePBR: boolPtr(true), DownloadFormats: []string{"glb"},
	},
	{
		Name: "cinema", Description: "Higher fidelity: more poly, PBR, glb+fbx+texture maps",
		Builtin: true, Source: "builtin",
		AIModel: "latest", ShouldRemesh: boolPtr(true), TargetPolycount: 100000,
		EnablePBR: boolPtr(true),
		DownloadFormats: []string{"glb", "fbx"}, DownloadTextures: true,
	},
	{
		Name: "print", Description: "Print-oriented downloads (stl); moderate poly",
		Builtin: true, Source: "builtin",
		AIModel: "latest", ShouldRemesh: boolPtr(true), TargetPolycount: 40000,
		DownloadFormats: []string{"stl", "glb"},
	},
	{
		Name: "fast", Description: "Cheap/quick defaults: latest model, glb only",
		Builtin: true, Source: "builtin",
		AIModel: "latest", DownloadFormats: []string{"glb"},
	},
}

func boolPtr(b bool) *bool { return &b }

// ProfileByName looks up a built-in profile only.
func ProfileByName(name string) (Profile, bool) {
	name = normalizeProfileName(name)
	for _, p := range BuiltinProfiles {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// ResolveProfile looks up user DB first, then built-ins.
// User profiles with the same name shadow builtins.
func ResolveProfile(st *store.Store, name string) (Profile, bool, error) {
	name = normalizeProfileName(name)
	if name == "" {
		return Profile{}, false, nil
	}
	if st != nil {
		sp, ok, err := st.GetProfile(name)
		if err != nil {
			return Profile{}, false, err
		}
		if ok {
			p, err := decodeStoredProfile(sp)
			if err != nil {
				return Profile{}, false, err
			}
			p.Source = "user"
			p.Builtin = false
			return p, true, nil
		}
	}
	if p, ok := ProfileByName(name); ok {
		return p, true, nil
	}
	return Profile{}, false, nil
}

// ListAllProfiles returns builtins (minus those shadowed) plus user profiles.
func ListAllProfiles(st *store.Store) ([]Profile, error) {
	userNames := map[string]bool{}
	var out []Profile
	if st != nil {
		rows, err := st.ListProfiles()
		if err != nil {
			return nil, err
		}
		for _, sp := range rows {
			p, err := decodeStoredProfile(sp)
			if err != nil {
				return nil, err
			}
			p.Source = "user"
			p.Builtin = false
			userNames[p.Name] = true
			out = append(out, p)
		}
	}
	for _, p := range BuiltinProfiles {
		if userNames[p.Name] {
			continue // shadowed
		}
		cp := p
		cp.Source = "builtin"
		cp.Builtin = true
		out = append(out, cp)
	}
	return out, nil
}

func decodeStoredProfile(sp store.StoredProfile) (Profile, error) {
	var p Profile
	if err := json.Unmarshal([]byte(sp.DataJSON), &p); err != nil {
		return Profile{}, fmt.Errorf("decode profile %q: %w", sp.Name, err)
	}
	p.Name = sp.Name
	if sp.Description != "" {
		p.Description = sp.Description
	}
	return p, nil
}

// SaveProfile writes a user profile to the store.
func SaveProfile(st *store.Store, p Profile) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	p.Name = normalizeProfileName(p.Name)
	if p.Name == "" {
		return fmt.Errorf("profile name required")
	}
	p.Builtin = false
	p.Source = "user"
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return st.UpsertProfile(p.Name, p.Description, string(b))
}

// DeleteUserProfile removes a user profile (not builtins).
func DeleteUserProfile(st *store.Store, name string) error {
	if st == nil {
		return fmt.Errorf("no store")
	}
	name = normalizeProfileName(name)
	_, ok, err := st.GetProfile(name)
	if err != nil {
		return err
	}
	if !ok {
		if _, isBuiltin := ProfileByName(name); isBuiltin {
			return fmt.Errorf("cannot delete built-in profile %q (delete a user override if you created one)", name)
		}
		return fmt.Errorf("profile %q not found", name)
	}
	return st.DeleteProfile(name)
}

// MergeProfile overlays non-zero fields from patch onto base.
func MergeProfile(base, patch Profile) Profile {
	out := base
	if patch.Description != "" {
		out.Description = patch.Description
	}
	if patch.AIModel != "" {
		out.AIModel = patch.AIModel
	}
	if patch.ShouldRemesh != nil {
		out.ShouldRemesh = patch.ShouldRemesh
	}
	if patch.TargetPolycount != 0 {
		out.TargetPolycount = patch.TargetPolycount
	}
	if patch.ShouldTexture != nil {
		out.ShouldTexture = patch.ShouldTexture
	}
	if patch.EnablePBR != nil {
		out.EnablePBR = patch.EnablePBR
	}
	if len(patch.DownloadFormats) > 0 {
		out.DownloadFormats = patch.DownloadFormats
	}
	// bool flags: only apply if patch sets them via explicit means — use pointer for DownloadTextures?
	// For simplicity, DownloadTextures/DownloadAll from patch always applied when merging from flags via applyProfileFlags
	out.DownloadTextures = patch.DownloadTextures
	out.DownloadAll = patch.DownloadAll
	return out
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
}

func normalizeProfileName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
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
	fmt.Fprintf(&b, "\nUser profiles: meshy profile list | create | update | delete | show\n")
	return b.String()
}
