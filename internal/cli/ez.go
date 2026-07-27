package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
  textured-3d   text → preview → refine (texture) → download
  image-to-3d   image → textured mesh → download

Profiles (also available as global --profile):
` + formatProfileHelp() + `
Examples:
  meshy ez textured-3d --prompt "beefcake minotaur" --profile game-hero --name minotaur -o ./models/mino
  meshy ez image-to-3d --image-url https://.../photo.png -o ./out --post 'echo done: $MESHY_GLB'
  meshy ez textured-3d --prompt "mug" --post 'cp {{glb}} ~/Dropbox/' --post 'notify-send meshy {{name}}'

Hooks (--pre / --post) run via sh -c with cwd = output dir on --post.
Environment: MESHY_OUT_DIR, MESHY_NAME, MESHY_GLB, MESHY_TASK_ID, MESHY_MANIFEST, MESHY_FILE_*, …
Placeholders: {{out}}, {{name}}, {{glb}}, {{task_id}}, {{manifest}}, {{preview_id}}, {{refine_id}}, …
`,
	}
	cmd.AddCommand(app.ezTextured3DCmd())
	cmd.AddCommand(app.ezImageTo3DCmd())
	cmd.AddCommand(app.ezProfilesCmd())
	return cmd
}

func (app *App) ezProfilesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List built-in profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.JSON {
				return app.Print.JSONValue(BuiltinProfiles)
			}
			fmt.Fprint(app.Print.Out, formatProfileHelp())
			return nil
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
		c        ezCommon
		imageURL string
	)
	cmd := &cobra.Command{
		Use:   "image-to-3d",
		Short: "Image → textured 3D → download",
		Long:  `One-shot image-to-3d with texture enabled by default, then download.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			c.enablePBRSet = cmd.Flags().Changed("enable-pbr")
			c.remeshSet = cmd.Flags().Changed("should-remesh")
			if imageURL == "" {
				return fmt.Errorf("--image-url is required")
			}
			return app.runEZImageTo3D(imageURL, c)
		},
	}
	cmd.Flags().StringVar(&imageURL, "image-url", "", "Source image URL or data URI (required)")
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
	p, ok := ProfileByName(name)
	if !ok {
		return nil, fmt.Errorf("unknown profile %q (try: meshy ez profiles)", name)
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
