package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crertel/meshy-cli/internal/api"
	"github.com/spf13/cobra"
)

func (app *App) resourceCmd(res api.Resource) *cobra.Command {
	cmd := &cobra.Command{
		Use:   res.Name,
		Short: res.Description,
		Long: fmt.Sprintf(`%s

API path: %s
Workflow: create [--sync] → get | assets | wait | download
Statuses: PENDING → IN_PROGRESS → SUCCEEDED | FAILED | CANCELED

Use --sync (or --wait) on create to block until the job finishes.
Use assets to list downloadable URLs; download to save them locally.`, res.Description, res.Path),
	}

	if len(res.CreateModes) > 0 {
		for _, mode := range res.CreateModes {
			cmd.AddCommand(app.createModeCmd(res, mode))
		}
	} else {
		cmd.AddCommand(app.createPlainCmd(res))
	}
	cmd.AddCommand(app.getCmd(res))
	if res.HasList {
		cmd.AddCommand(app.listCmd(res))
	}
	if res.HasDelete {
		cmd.AddCommand(app.deleteCmd(res))
	}
	cmd.AddCommand(app.waitCmd(res))
	cmd.AddCommand(app.streamCmd(res))
	cmd.AddCommand(app.assetsCmd(res))
	cmd.AddCommand(app.downloadCmd(res))
	// Static catalogs for resources that take model/action ids
	if res.Name == "text-to-image" || res.Name == "image-to-image" {
		cmd.AddCommand(app.imageModelsCmd(res))
	}
	if res.Name == "animation" {
		cmd.AddCommand(app.animationLibraryCmd())
	}
	return cmd
}

type createOpts struct {
	bodyFile string
	bodyJSON string
	sync     bool
	download bool
	outDir   string
	formats  []string
	textures bool
	slug     string
	pre      []string
	post     []string
}

func (app *App) createPlainCmd(res api.Resource) *cobra.Command {
	var opts createOpts
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a " + res.Name + " task",
		Long: `Create a task. Pass JSON via --body/--body-file and/or convenience flags.

--sync / --wait: do not exit until the task reaches a terminal status.
--download: after success, download assets (implies --sync; default glb / profile formats).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := parseBody(opts.bodyFile, opts.bodyJSON)
			if err != nil {
				return err
			}
			if body == nil {
				body = map[string]any{}
			}
			if err := mergeCreateFlags(cmd, body); err != nil {
				return err
			}
			if app.Profile != nil {
				ApplyProfileToBody(body, *app.Profile)
			}
			return app.runCreate(res, body, opts)
		},
	}
	cmd.Flags().StringVar(&opts.bodyJSON, "body", "", "JSON request body")
	cmd.Flags().StringVar(&opts.bodyFile, "body-file", "", "Path to JSON request body file")
	addSyncDownloadFlags(cmd, &opts)
	addCommonCreateFlags(cmd)
	return cmd
}

func (app *App) createModeCmd(res api.Resource, mode string) *cobra.Command {
	var opts createOpts
	cmd := &cobra.Command{
		Use:   "create-" + mode,
		Short: "Create a " + res.Name + " " + mode + " task",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := parseBody(opts.bodyFile, opts.bodyJSON)
			if err != nil {
				return err
			}
			if body == nil {
				body = map[string]any{}
			}
			body["mode"] = mode
			if err := mergeCreateFlags(cmd, body); err != nil {
				return err
			}
			if app.Profile != nil {
				ApplyProfileToBody(body, *app.Profile)
			}
			return app.runCreate(res, body, opts)
		},
	}
	cmd.Flags().StringVar(&opts.bodyJSON, "body", "", "JSON request body")
	cmd.Flags().StringVar(&opts.bodyFile, "body-file", "", "Path to JSON request body file")
	addSyncDownloadFlags(cmd, &opts)
	addCommonCreateFlags(cmd)
	return cmd
}

func addSyncDownloadFlags(cmd *cobra.Command, opts *createOpts) {
	cmd.Flags().BoolVar(&opts.sync, "sync", false, "Block until the task finishes (same as --wait)")
	cmd.Flags().BoolVar(&opts.sync, "wait", false, "Alias for --sync")
	cmd.Flags().BoolVar(&opts.download, "download", false, "After success, download assets (implies --sync)")
	cmd.Flags().StringVarP(&opts.outDir, "output", "o", "", "Directory for --download (default: <task-id>)")
	cmd.Flags().StringSliceVar(&opts.formats, "format", nil, "Download format(s): glb,fbx or repeat --format (default: glb / profile)")
	cmd.Flags().BoolVar(&opts.textures, "textures", false, "Also download loose PBR texture maps")
	cmd.Flags().StringVar(&opts.slug, "name", "", "Filename prefix for downloads (default: model)")
	cmd.Flags().StringArrayVar(&opts.pre, "pre", nil, "Shell before create (repeatable); {{out}}/MESHY_* available after out dir exists")
	cmd.Flags().StringArrayVar(&opts.post, "post", nil, "Shell after success (repeatable); cwd=output when downloading")
}

func addCommonCreateFlags(cmd *cobra.Command) {
	// Inputs / prompts
	cmd.Flags().String("prompt", "", "Text prompt")
	cmd.Flags().String("image-url", "", "Image URL, data URI, or local file path")
	cmd.Flags().String("image-file", "", "Local image file (converted to data URI)")
	cmd.Flags().StringSlice("image-urls", nil, "Multiple image URLs, data URIs, or local paths")
	cmd.Flags().StringSlice("image-files", nil, "Local image files (converted to data URIs)")
	cmd.Flags().StringSlice("reference-image-urls", nil, "image-to-image reference images (URL/path)")
	cmd.Flags().String("model-url", "", "Input model URL or local path (data URI)")
	cmd.Flags().String("input-task-id", "", "Upstream Meshy task id")
	cmd.Flags().String("preview-task-id", "", "Preview task id (text-to-3d refine)")

	// Model / mesh
	cmd.Flags().String("ai-model", "", "AI model (latest, meshy-6, meshy-5, nano-banana, …)")
	cmd.Flags().String("model-type", "", "Mesh type: standard | smart-topology | lowpoly")
	cmd.Flags().String("topology", "", "triangle | quad")
	cmd.Flags().Int("target-polycount", 0, "Target polycount")
	cmd.Flags().Int("decimation-mode", 0, "Adaptive decimation 1–4 (ultra…low)")
	cmd.Flags().Bool("should-remesh", false, "Enable remesh phase")
	cmd.Flags().Bool("save-pre-remeshed-model", false, "Also store pre-remesh GLB")
	cmd.Flags().String("pose-mode", "", "a-pose | t-pose | empty")
	cmd.Flags().StringSlice("target-formats", nil, "Output formats, CSV or repeatable (glb,fbx,obj,stl,usdz,3mf,blend)")

	// Texturing (image/text-to-3d refine, multi-image)
	cmd.Flags().Bool("should-texture", false, "Enable texturing")
	cmd.Flags().Bool("enable-pbr", false, "Enable PBR maps")
	cmd.Flags().String("texture-prompt", "", "Texture text prompt (generation refine / i2 3d)")
	cmd.Flags().String("texture-image-url", "", "Texture image URL, data URI, or local file")
	cmd.Flags().String("texture-image-file", "", "Local texture guide image file")
	cmd.Flags().String("texture-resolution", "", "Base color resolution: 2k | 4k | 8k")
	cmd.Flags().Bool("image-enhancement", false, "Input image enhancement (meshy-6+)")
	cmd.Flags().Bool("remove-lighting", false, "Remove baked lighting from textures")

	// Retexture-specific
	cmd.Flags().String("text-style-prompt", "", "Retexture: text style prompt")
	cmd.Flags().String("image-style-url", "", "Retexture: style image URL, data URI, or local path")
	cmd.Flags().String("image-style-file", "", "Retexture: local style image file")
	cmd.Flags().Bool("enable-original-uv", false, "Retexture: keep existing UV layout")

	// 2D image APIs
	cmd.Flags().String("aspect-ratio", "", "Image aspect ratio (1:1, 16:9, …)")
	cmd.Flags().Bool("generate-multi-view", false, "Generate multi-view images (2D APIs)")

	// Sizing / previews
	cmd.Flags().Bool("moderation", false, "Content moderation on inputs")
	cmd.Flags().Bool("auto-size", false, "AI real-world size estimation")
	cmd.Flags().String("origin-at", "", "bottom | center (with auto-size / resize)")
	cmd.Flags().Bool("alpha-thumbnail", false, "Also render RGBA thumbnail")
	cmd.Flags().Bool("multi-view-thumbnails", false, "Also render 4-view thumbnails")
	cmd.Flags().Float64("resize-height", 0, "Resize height in meters")
	cmd.Flags().Float64("resize-longest-side", 0, "Resize longest side in meters")

	// Rigging / animation
	cmd.Flags().Float64("height-meters", 0, "Character height for rigging")
	cmd.Flags().Int("action-id", 0, "Animation action_id (see animation library)")
	cmd.Flags().String("rig-task-id", "", "Rigging task id (animation)")
	cmd.Flags().String("post-process-operation", "", "Animation post_process.operation_type: change_fps|fbx2usdz|extract_armature")
	cmd.Flags().Int("post-process-fps", 0, "Animation post_process.fps (24|25|30|60)")

	// Print
	cmd.Flags().Int("max-colors", 0, "Multi-color print: palette size 1–16")
}

func mergeCreateFlags(cmd *cobra.Command, body map[string]any) error {
	setStr := func(flag, key string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetString(flag)
			if v != "" {
				body[key] = v
			}
		}
	}
	setBool := func(flag, key string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetBool(flag)
			body[key] = v
		}
	}
	setInt := func(flag, key string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetInt(flag)
			body[key] = v
		}
	}
	setFloat := func(flag, key string) {
		if cmd.Flags().Changed(flag) {
			v, _ := cmd.Flags().GetFloat64(flag)
			if v != 0 {
				body[key] = v
			}
		}
	}
	setStr("prompt", "prompt")
	setStr("input-task-id", "input_task_id")
	setStr("preview-task-id", "preview_task_id")
	setStr("ai-model", "ai_model")
	setStr("model-type", "model_type")
	setStr("texture-prompt", "texture_prompt")
	setStr("text-style-prompt", "text_style_prompt")
	setStr("texture-resolution", "texture_resolution")
	setStr("topology", "topology")
	setStr("pose-mode", "pose_mode")
	setStr("aspect-ratio", "aspect_ratio")
	setStr("origin-at", "origin_at")
	setStr("rig-task-id", "rig_task_id")
	setBool("should-texture", "should_texture")
	setBool("enable-pbr", "enable_pbr")
	setBool("should-remesh", "should_remesh")
	setBool("save-pre-remeshed-model", "save_pre_remeshed_model")
	setBool("generate-multi-view", "generate_multi_view")
	setBool("image-enhancement", "image_enhancement")
	setBool("remove-lighting", "remove_lighting")
	setBool("moderation", "moderation")
	setBool("auto-size", "auto_size")
	setBool("alpha-thumbnail", "alpha_thumbnail")
	setBool("multi-view-thumbnails", "multi_view_thumbnails")
	setBool("enable-original-uv", "enable_original_uv")
	setInt("target-polycount", "target_polycount")
	setInt("decimation-mode", "decimation_mode")
	setInt("action-id", "action_id")
	setInt("max-colors", "max_colors")
	setFloat("resize-height", "resize_height")
	setFloat("resize-longest-side", "resize_longest_side")
	setFloat("height-meters", "height_meters")

	// Animation nested post_process
	if cmd.Flags().Changed("post-process-operation") {
		op, _ := cmd.Flags().GetString("post-process-operation")
		if op != "" {
			pp := map[string]any{"operation_type": op}
			if cmd.Flags().Changed("post-process-fps") {
				fps, _ := cmd.Flags().GetInt("post-process-fps")
				pp["fps"] = fps
			}
			body["post_process"] = pp
		}
	}

	// target_formats
	if cmd.Flags().Changed("target-formats") {
		v, _ := cmd.Flags().GetStringSlice("target-formats")
		var formats []string
		for _, item := range v {
			for _, p := range strings.Split(item, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					formats = append(formats, p)
				}
			}
		}
		if len(formats) > 0 {
			body["target_formats"] = formats
		}
	}

	// Image inputs: URL / data URI / local file → data URI
	if cmd.Flags().Changed("image-file") {
		path, _ := cmd.Flags().GetString("image-file")
		uri, err := ImageFileToDataURI(path)
		if err != nil {
			return err
		}
		body["image_url"] = uri
	} else if cmd.Flags().Changed("image-url") {
		v, _ := cmd.Flags().GetString("image-url")
		uri, err := ResolveImageInput(v)
		if err != nil {
			return err
		}
		body["image_url"] = uri
	}
	if cmd.Flags().Changed("texture-image-file") {
		path, _ := cmd.Flags().GetString("texture-image-file")
		uri, err := ImageFileToDataURI(path)
		if err != nil {
			return err
		}
		body["texture_image_url"] = uri
	} else if cmd.Flags().Changed("texture-image-url") {
		v, _ := cmd.Flags().GetString("texture-image-url")
		uri, err := ResolveImageInput(v)
		if err != nil {
			return err
		}
		body["texture_image_url"] = uri
	}
	// retexture style image
	if cmd.Flags().Changed("image-style-file") {
		path, _ := cmd.Flags().GetString("image-style-file")
		uri, err := ImageFileToDataURI(path)
		if err != nil {
			return err
		}
		body["image_style_url"] = uri
	} else if cmd.Flags().Changed("image-style-url") {
		v, _ := cmd.Flags().GetString("image-style-url")
		uri, err := ResolveImageInput(v)
		if err != nil {
			return err
		}
		body["image_style_url"] = uri
	}
	if cmd.Flags().Changed("model-url") {
		v, _ := cmd.Flags().GetString("model-url")
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "data:") {
			body["model_url"] = v
		} else if data, err := os.ReadFile(v); err == nil {
			mime := "application/octet-stream"
			if strings.HasSuffix(strings.ToLower(v), ".glb") {
				mime = "model/gltf-binary"
			}
			body["model_url"] = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
		} else {
			return fmt.Errorf("model-url: %w", err)
		}
	}

	var imageList []string
	if cmd.Flags().Changed("image-urls") {
		v, _ := cmd.Flags().GetStringSlice("image-urls")
		imageList = append(imageList, v...)
	}
	if cmd.Flags().Changed("image-files") {
		v, _ := cmd.Flags().GetStringSlice("image-files")
		imageList = append(imageList, v...)
	}
	if len(imageList) > 0 {
		resolved := make([]string, 0, len(imageList))
		for _, item := range imageList {
			uri, err := ResolveImageInput(item)
			if err != nil {
				return err
			}
			resolved = append(resolved, uri)
		}
		body["image_urls"] = resolved
	}

	// image-to-image only: reference_image_urls (do not set from image_urls for multi-image-to-3d)
	if cmd.Flags().Changed("reference-image-urls") {
		v, _ := cmd.Flags().GetStringSlice("reference-image-urls")
		resolved := make([]string, 0, len(v))
		for _, item := range v {
			uri, err := ResolveImageInput(item)
			if err != nil {
				return err
			}
			resolved = append(resolved, uri)
		}
		body["reference_image_urls"] = resolved
	} else if img, ok := body["image_url"].(string); ok && img != "" {
		// Single image-file/url also seeds reference_image_urls for image-to-image convenience
		// only when not already set and not doing multi-image (no image_urls)
		if _, has := body["reference_image_urls"]; !has {
			if _, multi := body["image_urls"]; !multi {
				body["reference_image_urls"] = []string{img}
			}
		}
	}

	return nil
}

func parseBody(file, raw string) (map[string]any, error) {
	var data []byte
	switch {
	case file != "" && raw != "":
		return nil, fmt.Errorf("use only one of --body or --body-file")
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		data = b
	case raw != "":
		data = []byte(raw)
	default:
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse JSON body: %w", err)
	}
	return m, nil
}

func (app *App) runCreate(res api.Resource, body map[string]any, opts createOpts) error {
	if err := RequireAPIKey(app.Client.APIKey); err != nil {
		return err
	}
	sync := opts.sync || opts.download || len(opts.post) > 0

	slug := opts.slug
	if slug == "" {
		slug = "model"
	}
	outDir := opts.outDir
	if outDir == "" && (opts.download || len(opts.pre) > 0 || len(opts.post) > 0) {
		outDir = slug
	}
	if outDir != "" {
		abs, err := filepath.Abs(outDir)
		if err != nil {
			return err
		}
		outDir = abs
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
	}

	hook := HookContext{
		Workflow: res.Name,
		Name:     slug,
		OutDir:   outDir,
		Session:  app.Session,
	}
	if app.Profile != nil {
		hook.Profile = app.Profile.Name
	}
	logHook := func(format string, args ...any) {
		if !app.JSON && !app.Quiet {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}
	if len(opts.pre) > 0 {
		if err := RunHooks(opts.pre, hook, outDir, logHook); err != nil {
			return fmt.Errorf("pre hook: %w", err)
		}
	}

	id, err := app.Client.Create(app.ctx(), res.Path, body)
	if err != nil {
		return err
	}
	if err := app.trackTask(res.Name, id, res.TaskPath(id), "PENDING"); err != nil {
		return fmt.Errorf("track task: %w", err)
	}
	if !sync {
		return app.Print.PrintCreate(id, map[string]any{"resource": res.Name})
	}
	t, err := app.waitForTask(res, id)
	if err != nil {
		return err
	}

	var saved map[string]string
	if opts.download || len(opts.post) > 0 {
		if t.Status() != "SUCCEEDED" {
			_ = app.Print.PrintAny(t)
			return fmt.Errorf("task %s finished with status %s; nothing to download", id, t.Status())
		}
		if opts.download || len(opts.post) > 0 {
			// Always materialize assets when post hooks need paths
			wantDL := opts.download || len(opts.post) > 0
			if wantDL {
				saved, err = app.downloadTaskAssets(res, id, t, downloadOpts{
					outDir:   outDir,
					formats:  opts.formats,
					textures: opts.textures,
					slug:     slug,
				})
				if err != nil {
					return err
				}
				saved = absFileMap(saved)
			}
		}
	}

	hook.TaskID = id
	hook.Status = t.Status()
	hook.Files = saved
	if outDir != "" {
		hook.OutDir = outDir
	}
	if len(opts.post) > 0 {
		if err := RunHooks(opts.post, hook, outDir, logHook); err != nil {
			return fmt.Errorf("post hook: %w", err)
		}
	}

	if opts.download {
		if app.JSON {
			return app.Print.JSONValue(map[string]any{
				"task":  t,
				"files": saved,
			})
		}
		_ = app.Print.PrintAny(t)
		for k, p := range saved {
			fmt.Fprintf(app.Print.Out, "saved %s → %s\n", k, p)
		}
		return nil
	}
	return app.Print.PrintAny(t)
}

func (app *App) waitForTask(res api.Resource, id string) (api.Task, error) {
	return app.waitForTaskMode(res, id, app.PreferStream)
}

func (app *App) waitForTaskMode(res api.Resource, id string, useStream bool) (api.Task, error) {
	showProgress := !app.JSON && !app.Quiet
	opts := api.WaitOptions{
		Interval: app.PollEvery,
		Timeout:  app.WaitTimeout,
		OnUpdate: func(task api.Task) {
			if showProgress {
				fmt.Fprintf(os.Stderr, "\rstatus=%s progress=%v   ", task.Status(), task.Progress())
			}
			if app.Store != nil {
				_ = app.Store.UpdateTaskStatus(id, task.Status())
			}
		},
	}
	var (
		t   api.Task
		err error
	)
	if useStream {
		t, err = app.Client.WaitStream(app.ctx(), res.StreamPath(id), opts)
	} else {
		t, err = app.Client.Wait(app.ctx(), res.TaskPath(id), opts)
	}
	if showProgress {
		fmt.Fprintln(os.Stderr)
	}
	if err != nil {
		return t, err
	}
	if app.Store != nil {
		_ = app.Store.UpdateTaskStatus(id, t.Status())
	}
	_ = app.trackTask(res.Name, id, res.TaskPath(id), t.Status())
	return t, nil
}

func (app *App) getCmd(res api.Resource) *cobra.Command {
	return &cobra.Command{
		Use:   "get <task-id>",
		Short: "Get a " + res.Name + " task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			id := args[0]
			t, err := app.Client.Get(app.ctx(), res.TaskPath(id))
			if err != nil {
				return err
			}
			_ = app.trackTask(res.Name, id, res.TaskPath(id), t.Status())
			return app.Print.PrintAny(t)
		},
	}
}

func (app *App) listCmd(res api.Resource) *cobra.Command {
	var pageNum, pageSize int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List " + res.Name + " tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			tasks, err := app.Client.List(app.ctx(), res.Path, pageNum, pageSize)
			if err != nil {
				return err
			}
			return app.Print.PrintAny(tasks)
		},
	}
	cmd.Flags().IntVar(&pageNum, "page", 1, "Page number")
	cmd.Flags().IntVar(&pageSize, "page-size", 10, "Page size")
	return cmd
}

func (app *App) deleteCmd(res api.Resource) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <task-id>",
		Short: "Delete a " + res.Name + " task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			id := args[0]
			if err := app.Client.Delete(app.ctx(), res.TaskPath(id)); err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]any{"deleted": id, "resource": res.Name})
			}
			fmt.Fprintf(app.Print.Out, "deleted %s\n", id)
			return nil
		},
	}
}

func (app *App) waitCmd(res api.Resource) *cobra.Command {
	var useStream bool
	cmd := &cobra.Command{
		Use:     "wait <task-id>",
		Aliases: []string{"sync"},
		Short:   "Wait until " + res.Name + " task is terminal (poll or SSE)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			t, err := app.waitForTaskMode(res, args[0], useStream || app.PreferStream)
			if err != nil {
				return err
			}
			return app.Print.PrintAny(t)
		},
	}
	cmd.Flags().BoolVar(&useStream, "stream", false, "Use SSE stream instead of polling")
	return cmd
}

func (app *App) streamCmd(res api.Resource) *cobra.Command {
	return &cobra.Command{
		Use:   "stream <task-id>",
		Short: "SSE-wait until " + res.Name + " task is terminal",
		Long:  `Connect to GET …/:id/stream for real-time status updates (falls back to poll if stream unavailable).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			t, err := app.waitForTaskMode(res, args[0], true)
			if err != nil {
				return err
			}
			return app.Print.PrintAny(t)
		},
	}
}

func (app *App) assetsCmd(res api.Resource) *cobra.Command {
	var (
		kind string
		wait bool
	)
	cmd := &cobra.Command{
		Use:   "assets <task-id>",
		Short: "List downloadable assets on a " + res.Name + " task",
		Long: `List asset names, kinds, and URLs available on the task payload.

Works for model_urls (3D), image_urls (text/image-to-image), nested result.*
URLs (rigging/animation), thumbnails, and textures.

Use the name column with:  meshy ` + res.Name + ` download <id> --name <name>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			id := args[0]
			var t api.Task
			var err error
			if wait {
				t, err = app.waitForTask(res, id)
			} else {
				t, err = app.Client.Get(app.ctx(), res.TaskPath(id))
			}
			if err != nil {
				return err
			}
			assets := api.FilterAssets(t.Assets(), "", kind)
			if app.JSON {
				return app.Print.JSONValue(map[string]any{
					"task_id": id,
					"status":  t.Status(),
					"assets":  assets,
				})
			}
			if len(assets) == 0 {
				fmt.Fprintf(app.Print.Out, "no assets on task %s (status=%s)\n", id, t.Status())
				return nil
			}
			fmt.Fprintf(app.Print.Out, "task %s  status=%s  %d asset(s)\n", id, t.Status(), len(assets))
			for _, a := range assets {
				fmt.Fprintf(app.Print.Out, "  %-28s  %-10s  %s\n", a.Name, a.Kind, a.URL)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind: model, image, texture, animation, other")
	cmd.Flags().BoolVar(&wait, "sync", false, "Wait for terminal status before listing")
	cmd.Flags().BoolVar(&wait, "wait", false, "Alias for --sync")
	return cmd
}

func (app *App) downloadCmd(res api.Resource) *cobra.Command {
	var opts downloadOpts
	var kind string
	cmd := &cobra.Command{
		Use:   "download <task-id>",
		Short: "Download assets from a " + res.Name + " task",
		Long: `Download task assets to disk.

Discovers model_urls, image_urls, texture_urls (PBR maps), nested result fields, etc.

  meshy ` + res.Name + ` assets <task-id>
  meshy ` + res.Name + ` download <task-id>                  # default: glb (or images)
  meshy ` + res.Name + ` download <task-id> --format glb,fbx
  meshy ` + res.Name + ` download <task-id> --format glb --textures
  meshy ` + res.Name + ` download <task-id> --all -o ./out

Default: glb if present; else generated images; profile may add formats.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			id := args[0]
			var t api.Task
			var err error
			if opts.wait {
				t, err = app.waitForTask(res, id)
			} else {
				t, err = app.Client.Get(app.ctx(), res.TaskPath(id))
			}
			if err != nil {
				return err
			}
			if kind != "" {
				opts.kinds = []string{kind}
			}
			saved, err := app.downloadTaskAssets(res, id, t, opts)
			if err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]any{"task_id": id, "status": t.Status(), "files": saved})
			}
			for k, p := range saved {
				fmt.Fprintf(app.Print.Out, "saved %s → %s\n", k, p)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.outDir, "output", "o", "", "Output directory (default: task id)")
	cmd.Flags().StringVar(&opts.slug, "name", "", "Filename prefix (default: model)")
	cmd.Flags().StringSliceVar(&opts.formats, "format", nil, "Format(s) to download: glb,fbx,base_color (repeatable or CSV)")
	cmd.Flags().StringVar(&kind, "kind", "", "Download all assets of kind (model, image, texture, ...)")
	cmd.Flags().BoolVar(&opts.all, "all", false, "Download every available asset")
	cmd.Flags().BoolVar(&opts.textures, "textures", false, "Also download loose PBR texture maps")
	cmd.Flags().BoolVar(&opts.wait, "sync", false, "Wait for terminal status before downloading")
	cmd.Flags().BoolVar(&opts.wait, "wait", false, "Alias for --sync")
	return cmd
}

func (app *App) imageModelsCmd(res api.Resource) *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List known ai_model values for " + res.Name,
		Long:  `Static catalog of documented Meshy image ai_model IDs (from API docs). Pass with --ai-model or --body.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			type row struct {
				ID          string `json:"id"`
				Description string `json:"description"`
			}
			var rows []row
			for _, m := range api.KnownImageModels {
				rows = append(rows, row{ID: m.ID, Description: m.Description})
			}
			if app.JSON {
				return app.Print.JSONValue(rows)
			}
			fmt.Fprintln(app.Print.Out, "Known ai_model values (from docs):")
			for _, m := range rows {
				fmt.Fprintf(app.Print.Out, "  %-18s  %s\n", m.ID, m.Description)
			}
			fmt.Fprintf(app.Print.Out, "\nExample:\n  meshy %s create --ai-model nano-banana --prompt \"...\" --sync\n", res.Name)
			return nil
		},
	}
}

func (app *App) animationLibraryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "library",
		Short: "Fetch the public Meshy animation library (action_id catalog)",
		Long:  `GET https://api.meshy.ai/web/public/animations/resources — use action_id values with animation create --action-id.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Public endpoint; still use client HTTP for injectability
			base := app.Client.BaseURL
			if base == "" {
				base = api.DefaultBaseURL
			}
			url := strings.TrimRight(base, "/") + "/web/public/animations/resources"
			// Animation catalog is on the main host, not always under openapi base only
			if !strings.Contains(url, "api.meshy.ai") && app.Cfg.BaseURL != "" {
				// keep as-is for mocks
			}
			data, err := app.Client.Download(app.ctx(), url)
			if err != nil {
				// fallback hard-coded public URL if base was openapi-only mock
				data, err = app.Client.Download(app.ctx(), "https://api.meshy.ai/web/public/animations/resources")
				if err != nil {
					return fmt.Errorf("fetch animation library: %w", err)
				}
			}
			if app.JSON {
				// pass through raw JSON
				var v any
				if err := json.Unmarshal(data, &v); err != nil {
					return err
				}
				return app.Print.JSONValue(v)
			}
			fmt.Fprintln(app.Print.Out, string(data))
			return nil
		},
	}
}
