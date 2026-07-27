package api

// Resource describes a Meshy async task resource family.
type Resource struct {
	// Name is the CLI subcommand name (e.g. "image-to-3d").
	Name string
	// Group is a logical family for help (generation, postprocess, image, print, creative-lab).
	Group string
	// Description is short help text.
	Description string
	// Path is the collection path (e.g. /openapi/v1/image-to-3d).
	Path string
	// CreateModes lists special create modes (preview/refine) for dual-mode endpoints.
	// Empty means a single create verb.
	CreateModes []string
	// HasList is false for resources that only support get by id (e.g. some rigging list gaps).
	HasList bool
	// HasDelete indicates DELETE is supported.
	HasDelete bool
}

// TaskPath returns path for a specific task id.
func (r Resource) TaskPath(id string) string {
	return r.Path + "/" + id
}

// StreamPath returns SSE stream path for a task id.
func (r Resource) StreamPath(id string) string {
	return r.Path + "/" + id + "/stream"
}

// AllResources is the Meshy API resource map exposed by the CLI.
var AllResources = []Resource{
	// 3D generation
	{
		Name: "text-to-3d", Group: "generation",
		Description: "Generate 3D models from text (preview + refine)",
		Path:        "/openapi/v2/text-to-3d",
		CreateModes: []string{"preview", "refine"},
		HasList:     true, HasDelete: true,
	},
	{
		Name: "image-to-3d", Group: "generation",
		Description: "Generate 3D models from a single image",
		Path:        "/openapi/v1/image-to-3d",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "multi-image-to-3d", Group: "generation",
		Description: "Generate 3D models from multiple images",
		Path:        "/openapi/v1/multi-image-to-3d",
		HasList:     true, HasDelete: true,
	},
	// Post-processing
	{
		Name: "remesh", Group: "postprocess",
		Description: "Remesh / re-export existing models",
		Path:        "/openapi/v1/remesh",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "convert", Group: "postprocess",
		Description: "Convert models between formats",
		Path:        "/openapi/v1/convert",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "resize", Group: "postprocess",
		Description: "Resize models to physical dimensions",
		Path:        "/openapi/v1/resize",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "uv-unwrap", Group: "postprocess",
		Description: "Generate clean UV layouts",
		Path:        "/openapi/v1/uv-unwrap",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "retexture", Group: "postprocess",
		Description: "Apply new textures via text or image",
		Path:        "/openapi/v1/retexture",
		HasList:     true, HasDelete: true,
	},
	{
		// Docs only document create/get/delete/stream (no list collection).
		Name: "rigging", Group: "postprocess",
		Description: "Add a skeleton to a humanoid character",
		Path:        "/openapi/v1/rigging",
		HasList:     false, HasDelete: true,
	},
	{
		// Docs only document create/get/delete/stream (no list collection).
		Name: "animation", Group: "postprocess",
		Description: "Apply animations to a rigged character",
		Path:        "/openapi/v1/animations",
		HasList:     false, HasDelete: true,
	},
	// 2D
	{
		Name: "text-to-image", Group: "image",
		Description: "Generate images from text prompts",
		Path:        "/openapi/v1/text-to-image",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "image-to-image", Group: "image",
		Description: "Transform images via prompt guidance",
		Path:        "/openapi/v1/image-to-image",
		HasList:     true, HasDelete: true,
	},
	// 3D printing
	{
		Name: "print-multi-color", Group: "print",
		Description: "Convert models to multi-color 3MF for printing",
		Path:        "/openapi/v1/print/multi-color",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "print-analyze", Group: "print",
		Description: "Analyze FDM printability (free)",
		Path:        "/openapi/v1/print/analyze",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "print-repair", Group: "print",
		Description: "Repair models for FDM printability",
		Path:        "/openapi/v1/print/repair",
		HasList:     true, HasDelete: true,
	},
	// Creative Lab
	{
		Name: "cl-keychain-prototype", Group: "creative-lab",
		Description: "Creative Lab keychain: prototype concept image",
		Path:        "/openapi/creative-lab/keychain/v1/prototype",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "cl-keychain-build", Group: "creative-lab",
		Description: "Creative Lab keychain: build 3D mesh",
		Path:        "/openapi/creative-lab/keychain/v1/build",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "cl-fridge-magnet-prototype", Group: "creative-lab",
		Description: "Creative Lab fridge magnet: prototype",
		Path:        "/openapi/creative-lab/fridge-magnet/v1/prototype",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "cl-fridge-magnet-build", Group: "creative-lab",
		Description: "Creative Lab fridge magnet: build",
		Path:        "/openapi/creative-lab/fridge-magnet/v1/build",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "cl-figure-prototype", Group: "creative-lab",
		Description: "Creative Lab figure: prototype",
		Path:        "/openapi/creative-lab/figure/v1/prototype",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "cl-figure-build", Group: "creative-lab",
		Description: "Creative Lab figure: build",
		Path:        "/openapi/creative-lab/figure/v1/build",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "cl-vinyl-figure-prototype", Group: "creative-lab",
		Description: "Creative Lab vinyl figure: prototype",
		Path:        "/openapi/creative-lab/vinyl-figure/v1/prototype",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "cl-vinyl-figure-build", Group: "creative-lab",
		Description: "Creative Lab vinyl figure: build",
		Path:        "/openapi/creative-lab/vinyl-figure/v1/build",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "cl-lamp-prototype", Group: "creative-lab",
		Description: "Creative Lab lamp: prototype",
		Path:        "/openapi/creative-lab/lamp/v1/prototype",
		HasList:     true, HasDelete: true,
	},
	{
		Name: "cl-lamp-build", Group: "creative-lab",
		Description: "Creative Lab lamp: build",
		Path:        "/openapi/creative-lab/lamp/v1/build",
		HasList:     true, HasDelete: true,
	},
}

// ResourceByName looks up a resource definition.
func ResourceByName(name string) (Resource, bool) {
	for _, r := range AllResources {
		if r.Name == name {
			return r, true
		}
	}
	return Resource{}, false
}
