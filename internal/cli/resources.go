package cli

import (
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
}

func (app *App) createPlainCmd(res api.Resource) *cobra.Command {
	var opts createOpts
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a " + res.Name + " task",
		Long: `Create a task. Pass JSON via --body/--body-file and/or convenience flags.

--sync / --wait: do not exit until the task reaches a terminal status.
--download: after a successful sync wait, download all assets (implies --sync).`,
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
}

func addCommonCreateFlags(cmd *cobra.Command) {
	cmd.Flags().String("prompt", "", "Text prompt")
	cmd.Flags().String("image-url", "", "Image URL (or data URI)")
	cmd.Flags().StringSlice("image-urls", nil, "Multiple image URLs")
	cmd.Flags().String("model-url", "", "Input model URL")
	cmd.Flags().String("input-task-id", "", "Upstream Meshy task id")
	cmd.Flags().String("preview-task-id", "", "Preview task id (text-to-3d refine)")
	cmd.Flags().String("ai-model", "", "AI model (latest, meshy-6, meshy-5, ...)")
	cmd.Flags().Bool("should-texture", false, "Enable texturing")
	cmd.Flags().Bool("enable-pbr", false, "Enable PBR maps")
	cmd.Flags().String("texture-prompt", "", "Texture text prompt")
	cmd.Flags().String("texture-image-url", "", "Texture image URL")
	cmd.Flags().String("target-format", "", "Target format (convert)")
	cmd.Flags().Int("target-polycount", 0, "Target polycount")
	cmd.Flags().String("action-id", "", "Animation action id")
	cmd.Flags().String("rig-task-id", "", "Rigging task id (animation)")
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
			if v != 0 {
				body[key] = v
			}
		}
	}
	setStr("prompt", "prompt")
	setStr("image-url", "image_url")
	setStr("model-url", "model_url")
	setStr("input-task-id", "input_task_id")
	setStr("preview-task-id", "preview_task_id")
	setStr("ai-model", "ai_model")
	setStr("texture-prompt", "texture_prompt")
	setStr("texture-image-url", "texture_image_url")
	setStr("target-format", "target_formats")
	setStr("action-id", "action_id")
	setStr("rig-task-id", "rig_task_id")
	setBool("should-texture", "should_texture")
	setBool("enable-pbr", "enable_pbr")
	setInt("target-polycount", "target_polycount")
	if cmd.Flags().Changed("image-urls") {
		v, _ := cmd.Flags().GetStringSlice("image-urls")
		if len(v) > 0 {
			body["image_urls"] = v
		}
	}
	if tf, ok := body["target_formats"].(string); ok && tf != "" {
		body["target_formats"] = []string{tf}
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
	sync := opts.sync || opts.download
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
	if opts.download {
		if t.Status() != "SUCCEEDED" {
			_ = app.Print.PrintAny(t)
			return fmt.Errorf("task %s finished with status %s; nothing to download", id, t.Status())
		}
		saved, err := app.downloadTaskAssets(res, id, t, downloadOpts{
			outDir: opts.outDir,
			all:    true,
		})
		if err != nil {
			return err
		}
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
	t, err := app.Client.Wait(app.ctx(), res.TaskPath(id), api.WaitOptions{
		Interval: app.PollEvery,
		Timeout:  app.WaitTimeout,
		OnUpdate: func(task api.Task) {
			if !app.JSON {
				fmt.Fprintf(os.Stderr, "\rstatus=%s progress=%v   ", task.Status(), task.Progress())
			}
			if app.Store != nil {
				_ = app.Store.UpdateTaskStatus(id, task.Status())
			}
		},
	})
	if !app.JSON {
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
	return &cobra.Command{
		Use:     "wait <task-id>",
		Aliases: []string{"sync"},
		Short:   "Poll until " + res.Name + " task is terminal",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			t, err := app.waitForTask(res, args[0])
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

type downloadOpts struct {
	outDir string
	name   string
	format string // alias of name
	kind   string
	all    bool
	wait   bool
}

func (app *App) downloadCmd(res api.Resource) *cobra.Command {
	var opts downloadOpts
	cmd := &cobra.Command{
		Use:   "download <task-id>",
		Short: "Download assets from a " + res.Name + " task",
		Long: `Download task assets to disk.

Discovers URLs from model_urls, image_urls, nested result fields, thumbnails, etc.
Use assets first to see names:

  meshy ` + res.Name + ` assets <task-id>
  meshy ` + res.Name + ` download <task-id> --all
  meshy ` + res.Name + ` download <task-id> --name glb
  meshy ` + res.Name + ` download <task-id> --kind image -o ./out

Default without filters: downloads glb if present, otherwise all assets.`,
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
			// format is alias for name
			if opts.name == "" {
				opts.name = opts.format
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
	cmd.Flags().StringVar(&opts.name, "name", "", "Asset name to download (see assets command)")
	cmd.Flags().StringVar(&opts.format, "format", "", "Alias for --name (e.g. glb, fbx, image_0)")
	cmd.Flags().StringVar(&opts.kind, "kind", "", "Download all assets of kind (model, image, ...)")
	cmd.Flags().BoolVar(&opts.all, "all", false, "Download every available asset")
	cmd.Flags().BoolVar(&opts.wait, "sync", false, "Wait for terminal status before downloading")
	cmd.Flags().BoolVar(&opts.wait, "wait", false, "Alias for --sync")
	return cmd
}

func (app *App) downloadTaskAssets(res api.Resource, id string, t api.Task, opts downloadOpts) (map[string]string, error) {
	assets := t.Assets()
	if len(assets) == 0 {
		return nil, fmt.Errorf("no downloadable assets on task %s (status=%s); try: meshy %s assets %s", id, t.Status(), res.Name, id)
	}

	var selected []api.Asset
	switch {
	case opts.all || opts.kind != "" || opts.name != "":
		selected = api.FilterAssets(assets, opts.name, opts.kind)
	default:
		// Prefer glb when present; else download everything (images, etc.)
		glb := api.FilterAssets(assets, "glb", "")
		if len(glb) > 0 {
			selected = glb
		} else {
			selected = assets
		}
	}
	if len(selected) == 0 {
		names := make([]string, 0, len(assets))
		for _, a := range assets {
			names = append(names, a.Name)
		}
		return nil, fmt.Errorf("no assets matched filters (available: %s)", strings.Join(names, ", "))
	}

	outDir := opts.outDir
	if outDir == "" {
		outDir = id
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	saved := map[string]string{}
	for _, a := range selected {
		data, err := app.Client.Download(app.ctx(), a.URL)
		if err != nil {
			return saved, fmt.Errorf("download %s: %w", a.Name, err)
		}
		path := filepath.Join(outDir, a.SuggestedFilename(id))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return saved, err
		}
		saved[a.Name] = path
	}
	return saved, nil
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
