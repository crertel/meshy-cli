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
}
