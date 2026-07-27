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
Workflow: create → get/list → wait (poll until terminal) → download model_urls.
Statuses: PENDING → IN_PROGRESS → SUCCEEDED | FAILED | CANCELED`, res.Description, res.Path),
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
	cmd.AddCommand(app.downloadCmd(res))
	return cmd
}

func (app *App) createPlainCmd(res api.Resource) *cobra.Command {
	var (
		bodyFile string
		bodyJSON string
		wait     bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a " + res.Name + " task",
		Long:  `Create a task. Pass JSON via --body/--body-file and/or convenience flags.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := parseBody(bodyFile, bodyJSON)
			if err != nil {
				return err
			}
			if body == nil {
				body = map[string]any{}
			}
			if err := mergeCreateFlags(cmd, body); err != nil {
				return err
			}
			return app.runCreate(res, body, wait)
		},
	}
	cmd.Flags().StringVar(&bodyJSON, "body", "", "JSON request body")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Path to JSON request body file")
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll until terminal status")
	addCommonCreateFlags(cmd)
	return cmd
}

func (app *App) createModeCmd(res api.Resource, mode string) *cobra.Command {
	var (
		bodyFile string
		bodyJSON string
		wait     bool
	)
	cmd := &cobra.Command{
		Use:   "create-" + mode,
		Short: "Create a " + res.Name + " " + mode + " task",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := parseBody(bodyFile, bodyJSON)
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
			return app.runCreate(res, body, wait)
		},
	}
	cmd.Flags().StringVar(&bodyJSON, "body", "", "JSON request body")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Path to JSON request body file")
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll until terminal status")
	addCommonCreateFlags(cmd)
	return cmd
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

func (app *App) runCreate(res api.Resource, body map[string]any, wait bool) error {
	if err := RequireAPIKey(app.Client.APIKey); err != nil {
		return err
	}
	id, err := app.Client.Create(app.ctx(), res.Path, body)
	if err != nil {
		return err
	}
	if err := app.trackTask(res.Name, id, res.TaskPath(id), "PENDING"); err != nil {
		return fmt.Errorf("track task: %w", err)
	}
	if !wait {
		return app.Print.PrintCreate(id, map[string]any{"resource": res.Name})
	}
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
		return err
	}
	if app.Store != nil {
		_ = app.Store.UpdateTaskStatus(id, t.Status())
	}
	return app.Print.PrintAny(t)
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
		Use:   "wait <task-id>",
		Short: "Poll until " + res.Name + " task is terminal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			id := args[0]
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
				return err
			}
			_ = app.trackTask(res.Name, id, res.TaskPath(id), t.Status())
			return app.Print.PrintAny(t)
		},
	}
}

func (app *App) downloadCmd(res api.Resource) *cobra.Command {
	var (
		outDir string
		format string
		all    bool
	)
	cmd := &cobra.Command{
		Use:   "download <task-id>",
		Short: "Download model_urls for a succeeded " + res.Name + " task",
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
			urls := t.ModelURLs()
			if len(urls) == 0 {
				urls = extractAssetURLs(t)
			}
			if len(urls) == 0 {
				return fmt.Errorf("no downloadable URLs on task %s (status=%s)", id, t.Status())
			}
			if outDir == "" {
				outDir = id
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			saved := map[string]string{}
			_, hasGLB := urls["glb"]
			for fmtName, u := range urls {
				if !all && format != "" && !strings.EqualFold(fmtName, format) {
					continue
				}
				if !all && format == "" && hasGLB && fmtName != "glb" && len(urls) > 1 {
					continue
				}
				data, err := app.Client.Download(app.ctx(), u)
				if err != nil {
					return fmt.Errorf("download %s: %w", fmtName, err)
				}
				ext := fmtName
				if !strings.Contains(ext, ".") {
					ext = "." + ext
				} else {
					ext = ""
				}
				path := filepath.Join(outDir, id+ext)
				if err := os.WriteFile(path, data, 0o644); err != nil {
					return err
				}
				saved[fmtName] = path
			}
			if len(saved) == 0 {
				ks := make([]string, 0, len(urls))
				for k := range urls {
					ks = append(ks, k)
				}
				return fmt.Errorf("no formats matched (available: %v)", ks)
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]any{"task_id": id, "files": saved})
			}
			for k, p := range saved {
				fmt.Fprintf(app.Print.Out, "saved %s → %s\n", k, p)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&outDir, "output", "o", "", "Output directory (default: task id)")
	cmd.Flags().StringVar(&format, "format", "", "Single format key from model_urls (e.g. glb, fbx)")
	cmd.Flags().BoolVar(&all, "all", false, "Download all available formats")
	return cmd
}

func extractAssetURLs(t api.Task) map[string]string {
	out := map[string]string{}
	if u, ok := t["image_url"].(string); ok && u != "" {
		out["image"] = u
	}
	if arr, ok := t["image_urls"].([]any); ok {
		for i, v := range arr {
			if s, ok := v.(string); ok {
				out[fmt.Sprintf("image_%d", i)] = s
			}
		}
	}
	for _, key := range []string{"result", "output"} {
		if nested, ok := t[key].(map[string]any); ok {
			if mu, ok := nested["model_urls"].(map[string]any); ok {
				for k, v := range mu {
					if s, ok := v.(string); ok {
						out[k] = s
					}
				}
			}
		}
	}
	return out
}
