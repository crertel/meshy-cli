package api_test

import (
	"testing"

	"github.com/crertel/meshy-cli/internal/api"
)

func TestAssetsFromImageTask(t *testing.T) {
	task := api.Task{
		"id":     "img-1",
		"status": "SUCCEEDED",
		"image_urls": []any{
			"https://assets.meshy.ai/x/output/image.png?Expires=1",
			"https://assets.meshy.ai/x/output/image_side.png?Expires=1",
		},
	}
	assets := task.Assets()
	if len(assets) != 2 {
		t.Fatalf("assets = %#v", assets)
	}
	if assets[0].Kind != "image" || assets[0].Ext != ".png" {
		t.Fatalf("asset0 = %#v", assets[0])
	}
}

func TestAssetsFromModelAndNestedResult(t *testing.T) {
	task := api.Task{
		"id":     "m-1",
		"status": "SUCCEEDED",
		"model_urls": map[string]any{
			"glb": "https://cdn.example/m.glb",
			"fbx": "https://cdn.example/m.fbx",
		},
		"thumbnail_url": "https://cdn.example/thumb.png",
		"result": map[string]any{
			"animation_glb_url": "https://cdn.example/anim.glb",
			"basic_animations": map[string]any{
				"walking_glb_url": "https://cdn.example/walk.glb",
			},
		},
	}
	assets := task.Assets()
	if len(assets) < 4 {
		t.Fatalf("expected several assets, got %#v", assets)
	}
	names := map[string]bool{}
	for _, a := range assets {
		names[a.Name] = true
	}
	// short glb/fbx keys from model_urls
	if !names["glb"] && !names["model_urls.glb"] {
		t.Fatalf("missing glb: %v", names)
	}
	foundAnim := false
	for _, a := range assets {
		if a.URL == "https://cdn.example/anim.glb" {
			foundAnim = true
		}
	}
	if !foundAnim {
		t.Fatalf("missing nested animation url: %#v", assets)
	}
}

func TestFilterAssets(t *testing.T) {
	assets := []api.Asset{
		{Name: "glb", URL: "https://x/a.glb", Kind: "model", Ext: ".glb"},
		{Name: "image_0", URL: "https://x/a.png", Kind: "image", Ext: ".png"},
	}
	got := api.FilterAssets(assets, "glb", "")
	if len(got) != 1 || got[0].Name != "glb" {
		t.Fatalf("%#v", got)
	}
	got = api.FilterAssets(assets, "", "image")
	if len(got) != 1 || got[0].Kind != "image" {
		t.Fatalf("%#v", got)
	}
}

func TestSuggestedFilename(t *testing.T) {
	a := api.Asset{Name: "image_0", Ext: ".png"}
	if got := a.SuggestedFilename("t1"); got != "t1-image_0.png" {
		t.Fatalf("got %q", got)
	}
	glb := api.Asset{Name: "glb", Ext: ".glb"}
	if got := glb.SuggestedFilename("minotaur"); got != "minotaur-model.glb" {
		t.Fatalf("glb name = %q", got)
	}
}

func TestTextureURLsArray(t *testing.T) {
	task := api.Task{
		"status": "SUCCEEDED",
		"model_urls": map[string]any{
			"glb": "https://cdn.example/m.glb",
		},
		"texture_urls": []any{
			map[string]any{
				"base_color": "https://cdn.example/bc.png",
				"normal":     "https://cdn.example/n.png",
				"metallic":   "https://cdn.example/m.png",
				"roughness":  "https://cdn.example/r.png",
			},
		},
	}
	assets := task.Assets()
	names := map[string]bool{}
	for _, a := range assets {
		names[a.Name] = true
	}
	for _, want := range []string{"glb", "base_color", "normal", "metallic", "roughness"} {
		if !names[want] {
			t.Fatalf("missing %s in %#v", want, names)
		}
	}
	// Select defaults: glb only
	sel := api.SelectAssets(assets, nil, nil, false, false)
	if len(sel) != 1 || sel[0].Name != "glb" {
		t.Fatalf("default select = %#v", sel)
	}
	// Multi format + textures
	sel = api.SelectAssets(assets, []string{"glb", "fbx"}, nil, false, true)
	if len(sel) < 2 {
		t.Fatalf("multi+tex = %#v", sel)
	}
	foundBC := false
	for _, a := range sel {
		if a.Name == "base_color" {
			foundBC = true
		}
	}
	if !foundBC {
		t.Fatalf("expected base_color in %#v", sel)
	}
}

func TestSlugify(t *testing.T) {
	if got := api.Slugify("Beefcake Minotaur!!!"); got != "beefcake-minotaur" {
		t.Fatalf("got %q", got)
	}
}
