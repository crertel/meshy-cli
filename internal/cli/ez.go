package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/crertel/meshy-cli/internal/api"
	"github.com/crertel/meshy-cli/internal/store"
	"github.com/spf13/cobra"
)

func (app *App) ezCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ez",
		Short: "Opinionated workflows with sane defaults (profiles, naming, sync download)",
		Long: `ez runs known-good multi-step Meshy pipelines.

Workflows:
  textured-3d    text → preview → refine (texture) → download
  image-to-3d    image → textured mesh → download
  multi-image-to-3d  multi-view images → mesh → download
  image-to-image transform reference image → download
  retexture      retexture existing task/model → download
  remesh         remesh existing task/model → download
  print-analyze  free FDM printability analysis
  print-repair   repair mesh for print → download

Profiles (also available as global --profile):
` + formatProfileHelp() + `
Examples:
  meshy ez textured-3d --prompt "beefcake minotaur" --profile game-hero --name minotaur -o ./models/mino
  meshy ez image-to-3d --image ./photo.png -o ./out --post 'xviewer {{glb}} &'
  meshy ez retexture --input-task-id <id> --texture-prompt "gold armor" -o ./out

Hooks (--pre / --post) run via sh -c with cwd = output dir on --post.
Trailing & detaches GUI apps. Env: MESHY_OUT_DIR, MESHY_GLB, MESHY_TASK_ID, …
`,
	}
	cmd.AddCommand(app.ezTextured3DCmd())
	cmd.AddCommand(app.ezImageTo3DCmd())
	cmd.AddCommand(app.ezMultiImageTo3DCmd())
	cmd.AddCommand(app.ezImageToImageCmd())
	cmd.AddCommand(app.ezRetextureCmd())
	cmd.AddCommand(app.ezRemeshCmd())
	cmd.AddCommand(app.ezPrintAnalyzeCmd())
	cmd.AddCommand(app.ezPrintRepairCmd())
	cmd.AddCommand(app.ezProfilesCmd())
	return cmd
}

func (app *App) ezProfilesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List profiles (alias for: meshy profile list)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.profileListCmd().RunE(cmd, args)
		},
	}
}

type ezCommon struct {
	name          string
	outDir        string
	profile       string
	formats       []string
	textures      bool
	quiet         bool
	verbose       bool
	noDownload    bool
	session       string
	tags          []string
	aiModel       string
	enablePBR     bool
	enablePBRSet  bool
	targetPoly    int
	shouldRemesh  bool
	remeshSet     bool
	texturePrompt string
	pre           []string // --pre hooks (repeatable)
	post          []string // --post hooks (repeatable)
}

func (app *App) ezTextured3DCmd() *cobra.Command {
	var (
		c      ezCommon
		prompt string
	)
	cmd := &cobra.Command{
		Use:   "textured-3d",
		Short: "Text → 3D preview → refine (texture) → download",
		Long: `One-shot textured mesh from a text prompt.

Runs:
  1. text-to-3d create-preview --sync
  2. text-to-3d create-refine  --sync  (PBR + texture prompt)
  3. download primary formats (profile / --format)

Writes a manifest.json next to outputs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c.enablePBRSet = cmd.Flags().Changed("enable-pbr")
			c.remeshSet = cmd.Flags().Changed("should-remesh")
			if prompt == "" {
				return fmt.Errorf("--prompt is required")
			}
			return app.runEZTextured3D(prompt, c)
		},
	}
	cmd.Flags().StringVar(&prompt, "prompt", "", "Geometry / subject prompt (required)")
	cmd.Flags().StringVar(&c.texturePrompt, "texture-prompt", "", "Texture prompt (default: same as --prompt)")
	addEZCommonFlags(cmd, &c)
	return cmd
}

func (app *App) ezImageTo3DCmd() *cobra.Command {
	var (
		c         ezCommon
		imageURL  string
		imageFile string
	)
	cmd := &cobra.Command{
		Use:   "image-to-3d",
		Short: "Image → textured 3D → download",
		Long: `One-shot image-to-3d with texture enabled by default, then download.

Provide a public URL, data URI, or local file:
  --image-url https://…
  --image-file ./photo.png
  --image-url ./photo.png   # local paths are auto-encoded to data URIs`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c.enablePBRSet = cmd.Flags().Changed("enable-pbr")
			c.remeshSet = cmd.Flags().Changed("should-remesh")
			src := imageURL
			if imageFile != "" {
				src = imageFile
			}
			if src == "" {
				return fmt.Errorf("--image-url or --image-file is required")
			}
			resolved, err := ResolveImageInput(src)
			if err != nil {
				return err
			}
			return app.runEZImageTo3D(resolved, c)
		},
	}
	cmd.Flags().StringVar(&imageURL, "image-url", "", "Image URL, data URI, or local file path")
	cmd.Flags().StringVar(&imageFile, "image-file", "", "Local image file (jpg/png → data URI)")
	cmd.Flags().StringVar(&imageFile, "image", "", "Alias for --image-file")
	addEZCommonFlags(cmd, &c)
	return cmd
}

func addEZCommonFlags(cmd *cobra.Command, c *ezCommon) {
	cmd.Flags().StringVar(&c.name, "name", "", "Output name / slug (default: from prompt)")
	cmd.Flags().StringVarP(&c.outDir, "output", "o", "", "Output directory (default: ./out/<name>; created if missing)")
	cmd.Flags().StringVar(&c.profile, "profile", "", "Profile defaults (game-lowpoly, game-hero, cinema, print, fast)")
	cmd.Flags().StringSliceVar(&c.formats, "format", nil, "Download format(s), comma or repeatable (default: profile or glb)")
	cmd.Flags().BoolVar(&c.textures, "textures", false, "Also download loose PBR texture maps")
	cmd.Flags().BoolVar(&c.quiet, "quiet", false, "Errors only on stderr")
	cmd.Flags().BoolVar(&c.verbose, "verbose", false, "Log equivalent raw steps")
	cmd.Flags().BoolVar(&c.noDownload, "no-download", false, "Skip download after success")
	cmd.Flags().StringVar(&c.session, "session", "", "Session id/name (default: ez-<name>)")
	cmd.Flags().StringSliceVar(&c.tags, "tag", nil, "Extra tags")
	cmd.Flags().StringVar(&c.aiModel, "ai-model", "", "AI model override")
	cmd.Flags().BoolVar(&c.enablePBR, "enable-pbr", true, "Enable PBR (refine / image-to-3d)")
	cmd.Flags().IntVar(&c.targetPoly, "target-polycount", 0, "Target polycount")
	cmd.Flags().BoolVar(&c.shouldRemesh, "should-remesh", true, "Enable remesh phase")
	cmd.Flags().StringArrayVar(&c.pre, "pre", nil, "Shell command before the job (repeatable). See env/{{placeholders}}")
	cmd.Flags().StringArrayVar(&c.post, "post", nil, "Shell command after success (repeatable). cwd=output dir")
}

func (app *App) resolveEZProfile(name string) (*Profile, error) {
	if name == "" && app.FlagProfile != "" {
		name = app.FlagProfile
	}
	if name == "" {
		return app.Profile, nil
	}
	p, ok, err := ResolveProfile(app.Store, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("unknown profile %q (try: meshy profile list)", name)
	}
	app.Profile = &p
	return &p, nil
}

func (app *App) ezLog(c ezCommon, format string, args ...any) {
	if c.quiet {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func (app *App) runEZTextured3D(prompt string, c ezCommon) error {
	if err := RequireAPIKey(app.Client.APIKey); err != nil {
		return err
	}
	prof, err := app.resolveEZProfile(c.profile)
	if err != nil {
		return err
	}

	name := c.name
	if name == "" {
		name = api.Slugify(prompt)
	} else {
		name = api.Slugify(name)
	}
	outDir, err := ResolveOutDir(c.outDir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Session + tags
	sess := c.session
	if sess == "" {
		if app.Session != "" {
			sess = app.Session
		} else {
			sess = "ez-" + name
		}
	}
	app.Session = sess
	if _, err := app.ensureSession(sess, sess); err != nil {
		return err
	}
	tags := append([]string{"ez", "textured-3d"}, c.tags...)
	if prof != nil {
		tags = append(tags, "profile:"+prof.Name)
	}
	app.Tags = append(app.Tags, tags...)

	texPrompt := c.texturePrompt
	if texPrompt == "" {
		texPrompt = prompt
	}

	hook := HookContext{
		Workflow: "textured-3d",
		Name:     name,
		OutDir:   outDir,
		Profile:  profileName(prof),
		Session:  sess,
	}
	logHook := func(format string, args ...any) {
		if c.verbose || !c.quiet {
			app.ezLog(c, format, args...)
		}
	}
	if err := RunHooks(c.pre, hook, outDir, logHook); err != nil {
		return fmt.Errorf("pre hook: %w", err)
	}

	// Preview body
	previewBody := map[string]any{
		"mode":   "preview",
		"prompt": prompt,
	}
	if prof != nil {
		ApplyProfileToBody(previewBody, *prof)
	}
	applyEZOverrides(previewBody, c)
	// preview doesn't need texture flags
	delete(previewBody, "enable_pbr")
	delete(previewBody, "texture_prompt")
	delete(previewBody, "should_texture")

	resT23, _ := api.ResourceByName("text-to-3d")
	app.ezLog(c, "ez textured-3d: preview…")
	if c.verbose {
		app.ezLog(c, "  POST %s %v", resT23.Path, previewBody)
	}
	previewID, err := app.Client.Create(app.ctx(), resT23.Path, previewBody)
	if err != nil {
		return fmt.Errorf("preview create: %w", err)
	}
	_ = app.trackTask("text-to-3d", previewID, resT23.TaskPath(previewID), "PENDING")
	previewTask, err := app.waitForTask(resT23, previewID)
	if err != nil {
		return fmt.Errorf("preview wait: %w", err)
	}
	if previewTask.Status() != "SUCCEEDED" {
		return fmt.Errorf("preview failed: status=%s err=%s", previewTask.Status(), previewTask.TaskErrorMessage())
	}
	app.ezLog(c, "ez textured-3d: preview ok (%s)", previewID)

	// Refine
	refineBody := map[string]any{
		"mode":            "refine",
		"preview_task_id": previewID,
		"texture_prompt":  texPrompt,
		"enable_pbr":      true,
	}
	if prof != nil {
		ApplyProfileToBody(refineBody, *prof)
	}
	applyEZOverrides(refineBody, c)
	// ensure pbr default true unless user set false
	if c.enablePBRSet {
		refineBody["enable_pbr"] = c.enablePBR
	}

	app.ezLog(c, "ez textured-3d: refine (texture)…")
	if c.verbose {
		app.ezLog(c, "  POST %s %v", resT23.Path, refineBody)
	}
	refineID, err := app.Client.Create(app.ctx(), resT23.Path, refineBody)
	if err != nil {
		return fmt.Errorf("refine create: %w", err)
	}
	_ = app.trackTask("text-to-3d", refineID, resT23.TaskPath(refineID), "PENDING")
	refineTask, err := app.waitForTask(resT23, refineID)
	if err != nil {
		return fmt.Errorf("refine wait: %w", err)
	}
	if refineTask.Status() != "SUCCEEDED" {
		return fmt.Errorf("refine failed: status=%s err=%s", refineTask.Status(), refineTask.TaskErrorMessage())
	}
	app.ezLog(c, "ez textured-3d: refine ok (%s)", refineID)

	var files map[string]string
	if !c.noDownload {
		dopts := downloadOpts{
			outDir:   outDir,
			slug:     name,
			formats:  c.formats,
			textures: c.textures,
		}
		files, err = app.downloadTaskAssets(resT23, refineID, refineTask, dopts)
		if err != nil {
			return err
		}
		files = absFileMap(files)
		for k, p := range files {
			app.ezLog(c, "  saved %s → %s", k, p)
		}
	}

	manPath := filepath.Join(outDir, "manifest.json")
	manifest := map[string]any{
		"workflow":       "textured-3d",
		"name":           name,
		"prompt":         prompt,
		"texture_prompt": texPrompt,
		"profile":        profileName(prof),
		"preview_id":     previewID,
		"refine_id":      refineID,
		"session":        sess,
		"tags":           app.Tags,
		"out_dir":        outDir,
		"files":          files,
		"finished_at":    time.Now().UTC().Format(time.RFC3339),
		"preview":        previewTask,
		"refine":         refineTask,
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manPath, mb, 0o644); err != nil {
		return err
	}

	hook.PreviewID = previewID
	hook.RefineID = refineID
	hook.TaskID = refineID
	hook.Manifest = manPath
	hook.Status = refineTask.Status()
	hook.Files = files
	if err := RunHooks(c.post, hook, outDir, logHook); err != nil {
		return fmt.Errorf("post hook: %w", err)
	}

	app.ezLog(c, "ez textured-3d: done → %s", outDir)

	if app.JSON {
		return app.Print.JSONValue(manifest)
	}
	fmt.Fprintf(app.Print.Out, "preview: %s\nrefine:  %s\nout:     %s\n", previewID, refineID, outDir)
	for k, p := range files {
		fmt.Fprintf(app.Print.Out, "  %s: %s\n", k, p)
	}
	fmt.Fprintf(app.Print.Out, "manifest: %s\n", manPath)
	return nil
}

func (app *App) runEZImageTo3D(imageURL string, c ezCommon) error {
	if err := RequireAPIKey(app.Client.APIKey); err != nil {
		return err
	}
	prof, err := app.resolveEZProfile(c.profile)
	if err != nil {
		return err
	}

	name := c.name
	if name == "" {
		name = "image-to-3d"
	}
	name = api.Slugify(name)
	outDir, err := ResolveOutDir(c.outDir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	sess := c.session
	if sess == "" {
		if app.Session != "" {
			sess = app.Session
		} else {
			sess = "ez-" + name
		}
	}
	app.Session = sess
	if _, err := app.ensureSession(sess, sess); err != nil {
		return err
	}
	tags := append([]string{"ez", "image-to-3d"}, c.tags...)
	if prof != nil {
		tags = append(tags, "profile:"+prof.Name)
	}
	app.Tags = append(app.Tags, tags...)

	hook := HookContext{
		Workflow: "image-to-3d",
		Name:     name,
		OutDir:   outDir,
		Profile:  profileName(prof),
		Session:  sess,
	}
	logHook := func(format string, args ...any) {
		if c.verbose || !c.quiet {
			app.ezLog(c, format, args...)
		}
	}
	if err := RunHooks(c.pre, hook, outDir, logHook); err != nil {
		return fmt.Errorf("pre hook: %w", err)
	}

	body := map[string]any{
		"image_url":      imageURL,
		"should_texture": true,
		"enable_pbr":     true,
	}
	if prof != nil {
		ApplyProfileToBody(body, *prof)
	}
	applyEZOverrides(body, c)
	if c.enablePBRSet {
		body["enable_pbr"] = c.enablePBR
	}

	res, _ := api.ResourceByName("image-to-3d")
	app.ezLog(c, "ez image-to-3d: create…")
	if c.verbose {
		app.ezLog(c, "  POST %s (image_url set)", res.Path)
	}
	id, err := app.Client.Create(app.ctx(), res.Path, body)
	if err != nil {
		return err
	}
	_ = app.trackTask("image-to-3d", id, res.TaskPath(id), "PENDING")
	task, err := app.waitForTask(res, id)
	if err != nil {
		return err
	}
	if task.Status() != "SUCCEEDED" {
		return fmt.Errorf("task failed: status=%s err=%s", task.Status(), task.TaskErrorMessage())
	}

	var files map[string]string
	if !c.noDownload {
		files, err = app.downloadTaskAssets(res, id, task, downloadOpts{
			outDir:   outDir,
			slug:     name,
			formats:  c.formats,
			textures: c.textures,
		})
		if err != nil {
			return err
		}
		files = absFileMap(files)
	}

	manPath := filepath.Join(outDir, "manifest.json")
	manifest := map[string]any{
		"workflow":    "image-to-3d",
		"name":        name,
		"image_url":   imageURL,
		"profile":     profileName(prof),
		"task_id":     id,
		"session":     sess,
		"out_dir":     outDir,
		"files":       files,
		"finished_at": time.Now().UTC().Format(time.RFC3339),
		"task":        task,
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(manPath, mb, 0o644)

	hook.TaskID = id
	hook.Manifest = manPath
	hook.Status = task.Status()
	hook.Files = files
	if err := RunHooks(c.post, hook, outDir, logHook); err != nil {
		return fmt.Errorf("post hook: %w", err)
	}

	app.ezLog(c, "ez image-to-3d: done → %s", outDir)

	if app.JSON {
		return app.Print.JSONValue(manifest)
	}
	fmt.Fprintf(app.Print.Out, "task: %s\nout:  %s\n", id, outDir)
	for k, p := range files {
		fmt.Fprintf(app.Print.Out, "  %s: %s\n", k, p)
	}
	return nil
}

// --- additional ez workflows ---

func (app *App) ezMultiImageTo3DCmd() *cobra.Command {
	var (
		c     ezCommon
		urls  []string
		files []string
	)
	cmd := &cobra.Command{
		Use:   "multi-image-to-3d",
		Short: "Multi-view images → 3D mesh → download",
		RunE: func(cmd *cobra.Command, args []string) error {
			c.enablePBRSet = cmd.Flags().Changed("enable-pbr")
			c.remeshSet = cmd.Flags().Changed("should-remesh")
			list := append([]string{}, urls...)
			list = append(list, files...)
			if len(list) == 0 {
				return fmt.Errorf("provide --image-urls and/or --image-files")
			}
			resolved := make([]string, 0, len(list))
			for _, item := range list {
				uri, err := ResolveImageInput(item)
				if err != nil {
					return err
				}
				resolved = append(resolved, uri)
			}
			body := map[string]any{
				"image_urls":     resolved,
				"should_texture": true,
				"enable_pbr":     true,
			}
			return app.runEZSimple("multi-image-to-3d", "multi-image-to-3d", body, c, "multi")
		},
	}
	cmd.Flags().StringSliceVar(&urls, "image-urls", nil, "Image URLs or paths")
	cmd.Flags().StringSliceVar(&files, "image-files", nil, "Local image files")
	addEZCommonFlags(cmd, &c)
	return cmd
}

func (app *App) ezImageToImageCmd() *cobra.Command {
	var (
		c         ezCommon
		prompt    string
		imageURL  string
		imageFile string
		aiModel   string
	)
	cmd := &cobra.Command{
		Use:   "image-to-image",
		Short: "Transform a reference image via prompt → download",
		RunE: func(cmd *cobra.Command, args []string) error {
			if prompt == "" {
				return fmt.Errorf("--prompt is required")
			}
			src := imageURL
			if imageFile != "" {
				src = imageFile
			}
			if src == "" {
				return fmt.Errorf("--image-url or --image-file is required")
			}
			uri, err := ResolveImageInput(src)
			if err != nil {
				return err
			}
			if aiModel == "" {
				aiModel = "nano-banana"
			}
			body := map[string]any{
				"prompt":               prompt,
				"ai_model":             aiModel,
				"reference_image_urls": []string{uri},
			}
			if c.aiModel != "" {
				body["ai_model"] = c.aiModel
			}
			return app.runEZSimple("image-to-image", "image-to-image", body, c, "i2i")
		},
	}
	cmd.Flags().StringVar(&prompt, "prompt", "", "Edit / style prompt (required)")
	cmd.Flags().StringVar(&imageURL, "image-url", "", "Reference image URL, data URI, or path")
	cmd.Flags().StringVar(&imageFile, "image-file", "", "Local reference image")
	cmd.Flags().StringVar(&imageFile, "image", "", "Alias for --image-file")
	cmd.Flags().StringVar(&aiModel, "image-model", "nano-banana", "Image model (nano-banana, …)")
	addEZCommonFlags(cmd, &c)
	return cmd
}

func (app *App) ezRetextureCmd() *cobra.Command {
	var (
		c             ezCommon
		inputTaskID   string
		modelURL      string
		texturePrompt string
	)
	cmd := &cobra.Command{
		Use:   "retexture",
		Short: "Retexture a model (task id or model URL) → download",
		RunE: func(cmd *cobra.Command, args []string) error {
			c.enablePBRSet = cmd.Flags().Changed("enable-pbr")
			if inputTaskID == "" && modelURL == "" {
				return fmt.Errorf("--input-task-id or --model-url is required")
			}
			body := map[string]any{"enable_pbr": true}
			if inputTaskID != "" {
				body["input_task_id"] = inputTaskID
			}
			if modelURL != "" {
				v := strings.TrimSpace(modelURL)
				if !(strings.HasPrefix(v, "http") || strings.HasPrefix(v, "data:")) {
					data, err := os.ReadFile(v)
					if err != nil {
						return err
					}
					body["model_url"] = "data:model/gltf-binary;base64," + encodeStdB64(data)
				} else {
					body["model_url"] = v
				}
			}
			if texturePrompt != "" {
				body["texture_prompt"] = texturePrompt
			}
			return app.runEZSimple("retexture", "retexture", body, c, "retexture")
		},
	}
	cmd.Flags().StringVar(&inputTaskID, "input-task-id", "", "Succeeded Meshy task to retexture")
	cmd.Flags().StringVar(&modelURL, "model-url", "", "Model URL or local .glb")
	cmd.Flags().StringVar(&texturePrompt, "texture-prompt", "", "Texture guidance prompt")
	addEZCommonFlags(cmd, &c)
	return cmd
}

func (app *App) ezRemeshCmd() *cobra.Command {
	var (
		c           ezCommon
		inputTaskID string
		modelURL    string
	)
	cmd := &cobra.Command{
		Use:   "remesh",
		Short: "Remesh a model (task id or model URL) → download",
		RunE: func(cmd *cobra.Command, args []string) error {
			c.remeshSet = cmd.Flags().Changed("should-remesh")
			if inputTaskID == "" && modelURL == "" {
				return fmt.Errorf("--input-task-id or --model-url is required")
			}
			body := map[string]any{}
			if inputTaskID != "" {
				body["input_task_id"] = inputTaskID
			}
			if modelURL != "" {
				body["model_url"] = modelURL
			}
			return app.runEZSimple("remesh", "remesh", body, c, "remesh")
		},
	}
	cmd.Flags().StringVar(&inputTaskID, "input-task-id", "", "Succeeded Meshy task to remesh")
	cmd.Flags().StringVar(&modelURL, "model-url", "", "Model URL or local path")
	addEZCommonFlags(cmd, &c)
	return cmd
}

func (app *App) ezPrintAnalyzeCmd() *cobra.Command {
	var (
		c           ezCommon
		inputTaskID string
		modelURL    string
	)
	cmd := &cobra.Command{
		Use:   "print-analyze",
		Short: "Analyze FDM printability (free) and print result",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputTaskID == "" && modelURL == "" {
				return fmt.Errorf("--input-task-id or --model-url is required")
			}
			body := map[string]any{}
			if inputTaskID != "" {
				body["input_task_id"] = inputTaskID
			}
			if modelURL != "" {
				body["model_url"] = modelURL
			}
			// analyze is free; often no mesh download — still allow
			c.noDownload = true
			return app.runEZSimple("print-analyze", "print-analyze", body, c, "print-analyze")
		},
	}
	cmd.Flags().StringVar(&inputTaskID, "input-task-id", "", "Succeeded Meshy task")
	cmd.Flags().StringVar(&modelURL, "model-url", "", "Model URL")
	addEZCommonFlags(cmd, &c)
	return cmd
}

func (app *App) ezPrintRepairCmd() *cobra.Command {
	var (
		c           ezCommon
		inputTaskID string
		modelURL    string
	)
	cmd := &cobra.Command{
		Use:   "print-repair",
		Short: "Repair mesh for FDM printability → download",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputTaskID == "" && modelURL == "" {
				return fmt.Errorf("--input-task-id or --model-url is required")
			}
			body := map[string]any{}
			if inputTaskID != "" {
				body["input_task_id"] = inputTaskID
			}
			if modelURL != "" {
				body["model_url"] = modelURL
			}
			return app.runEZSimple("print-repair", "print-repair", body, c, "print-repair")
		},
	}
	cmd.Flags().StringVar(&inputTaskID, "input-task-id", "", "Succeeded Meshy task")
	cmd.Flags().StringVar(&modelURL, "model-url", "", "Model URL")
	addEZCommonFlags(cmd, &c)
	return cmd
}

// runEZSimple is a shared create→wait→download→manifest pipeline for single-task workflows.
func (app *App) runEZSimple(workflow, resourceName string, body map[string]any, c ezCommon, defaultName string) error {
	if err := RequireAPIKey(app.Client.APIKey); err != nil {
		return err
	}
	prof, err := app.resolveEZProfile(c.profile)
	if err != nil {
		return err
	}
	name := c.name
	if name == "" {
		name = defaultName
	}
	name = api.Slugify(name)
	outDir, err := ResolveOutDir(c.outDir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	sess := c.session
	if sess == "" {
		if app.Session != "" {
			sess = app.Session
		} else {
			sess = "ez-" + name
		}
	}
	app.Session = sess
	if _, err := app.ensureSession(sess, sess); err != nil {
		return err
	}
	tags := append([]string{"ez", workflow}, c.tags...)
	if prof != nil {
		tags = append(tags, "profile:"+prof.Name)
	}
	app.Tags = append(app.Tags, tags...)

	if prof != nil {
		ApplyProfileToBody(body, *prof)
	}
	applyEZOverrides(body, c)
	if c.enablePBRSet {
		body["enable_pbr"] = c.enablePBR
	}

	hook := HookContext{Workflow: workflow, Name: name, OutDir: outDir, Profile: profileName(prof), Session: sess}
	logHook := func(format string, args ...any) {
		if c.verbose || !c.quiet {
			app.ezLog(c, format, args...)
		}
	}
	if err := RunHooks(c.pre, hook, outDir, logHook); err != nil {
		return fmt.Errorf("pre hook: %w", err)
	}

	res, ok := api.ResourceByName(resourceName)
	if !ok {
		return fmt.Errorf("unknown resource %q", resourceName)
	}
	app.ezLog(c, "ez %s: create…", workflow)
	id, err := app.Client.Create(app.ctx(), res.Path, body)
	if err != nil {
		return err
	}
	_ = app.trackTask(resourceName, id, res.TaskPath(id), "PENDING")
	task, err := app.waitForTask(res, id)
	if err != nil {
		return err
	}
	if task.Status() != "SUCCEEDED" {
		return fmt.Errorf("task failed: status=%s err=%s", task.Status(), task.TaskErrorMessage())
	}

	var files map[string]string
	if !c.noDownload {
		files, err = app.downloadTaskAssets(res, id, task, downloadOpts{
			outDir: outDir, slug: name, formats: c.formats, textures: c.textures,
		})
		if err != nil {
			// analyze may have no model assets — soft-fail if no assets
			if !strings.Contains(err.Error(), "no downloadable assets") {
				return err
			}
			app.ezLog(c, "ez %s: no downloadable assets (ok for analyze)", workflow)
		} else {
			files = absFileMap(files)
		}
	}

	manPath := filepath.Join(outDir, "manifest.json")
	manifest := map[string]any{
		"workflow": workflow, "name": name, "profile": profileName(prof),
		"task_id": id, "session": sess, "out_dir": outDir, "files": files,
		"finished_at": time.Now().UTC().Format(time.RFC3339), "task": task, "body": body,
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(manPath, mb, 0o644)

	hook.TaskID = id
	hook.Manifest = manPath
	hook.Status = task.Status()
	hook.Files = files
	if err := RunHooks(c.post, hook, outDir, logHook); err != nil {
		return fmt.Errorf("post hook: %w", err)
	}
	app.ezLog(c, "ez %s: done → %s", workflow, outDir)
	if app.JSON {
		return app.Print.JSONValue(manifest)
	}
	fmt.Fprintf(app.Print.Out, "task: %s\nout:  %s\n", id, outDir)
	for k, p := range files {
		fmt.Fprintf(app.Print.Out, "  %s: %s\n", k, p)
	}
	return nil
}

func encodeStdB64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func absFileMap(files map[string]string) map[string]string {
	if files == nil {
		return nil
	}
	out := make(map[string]string, len(files))
	for k, p := range files {
		if abs, err := filepath.Abs(p); err == nil {
			out[k] = abs
		} else {
			out[k] = p
		}
	}
	return out
}

func applyEZOverrides(body map[string]any, c ezCommon) {
	if c.aiModel != "" {
		body["ai_model"] = c.aiModel
	}
	if c.targetPoly != 0 {
		body["target_polycount"] = c.targetPoly
	}
	if c.remeshSet {
		body["should_remesh"] = c.shouldRemesh
	}
}

func profileName(p *Profile) string {
	if p == nil {
		return ""
	}
	return p.Name
}

func (app *App) ensureSession(id, name string) (store.Session, error) {
	if app.Store == nil {
		return store.Session{}, nil
	}
	if sess, ok, err := app.Store.GetSession(id); err != nil {
		return store.Session{}, err
	} else if ok {
		return sess, nil
	}
	// try resolve by name
	if sess, err := app.resolveSession(id); err == nil {
		return sess, nil
	}
	return app.Store.CreateSession(api.Slugify(id), name)
}
