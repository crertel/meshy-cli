package cli

import (
	"fmt"

	"github.com/crertel/meshy-cli/internal/api"
	"github.com/crertel/meshy-cli/internal/store"
	"github.com/spf13/cobra"
)

func (app *App) tasksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "Locally tracked tasks (sessions/tags)",
		Long: `List and check status of tasks the CLI has created or tracked.
Use --session / --tag filters. check refreshes remote status for a batch.`,
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List tracked tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			f := store.TaskFilter{
				SessionID: flagStr(cmd, "session-filter"),
				Tag:       flagStr(cmd, "tag-filter"),
				Resource:  flagStr(cmd, "resource"),
				Status:    flagStr(cmd, "status"),
			}
			if f.SessionID == "" && app.Session != "" {
				if sess, err := app.resolveSession(app.Session); err == nil {
					f.SessionID = sess.ID
				} else {
					f.SessionID = app.Session
				}
			}
			if f.Tag == "" && len(app.Tags) > 0 {
				f.Tag = app.Tags[0]
			}
			tasks, err := app.Store.ListTasks(f)
			if err != nil {
				return err
			}
			return app.Print.PrintTracked(tasks)
		},
	}
	list.Flags().String("session-filter", "", "Filter by session id")
	list.Flags().String("tag-filter", "", "Filter by tag")
	list.Flags().String("resource", "", "Filter by resource name")
	list.Flags().String("status", "", "Filter by last known status")
	cmd.AddCommand(list)

	cmd.AddCommand(&cobra.Command{
		Use:   "tag <task-id> <tag>...",
		Short: "Add tags to a tracked task",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if _, ok, err := app.Store.GetTask(id); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("task %q is not tracked locally; get/create it first", id)
			}
			if err := app.Store.AddTags(id, args[1:]); err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]any{"task_id": id, "tags": args[1:]})
			}
			fmt.Fprintf(app.Print.Out, "tagged %s with %v\n", id, args[1:])
			return nil
		},
	})

	check := &cobra.Command{
		Use:   "check",
		Short: "Refresh remote status for filtered tracked tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			f := store.TaskFilter{
				SessionID: flagStr(cmd, "session-filter"),
				Tag:       flagStr(cmd, "tag-filter"),
				Resource:  flagStr(cmd, "resource"),
				Status:    flagStr(cmd, "status"),
			}
			if f.SessionID == "" && app.Session != "" {
				if sess, err := app.resolveSession(app.Session); err == nil {
					f.SessionID = sess.ID
				} else {
					f.SessionID = app.Session
				}
			}
			if f.Tag == "" && len(app.Tags) > 0 {
				f.Tag = app.Tags[0]
			}
			tasks, err := app.Store.ListTasks(f)
			if err != nil {
				return err
			}
			var results []map[string]any
			for _, t := range tasks {
				path := t.RemotePath
				if path == "" {
					if res, ok := api.ResourceByName(t.Resource); ok {
						path = res.TaskPath(t.ID)
					}
				}
				if path == "" {
					results = append(results, map[string]any{"id": t.ID, "error": "unknown resource path"})
					continue
				}
				remote, err := app.Client.Get(app.ctx(), path)
				if err != nil {
					results = append(results, map[string]any{"id": t.ID, "error": err.Error()})
					continue
				}
				_ = app.Store.UpdateTaskStatus(t.ID, remote.Status())
				results = append(results, map[string]any{
					"id":       t.ID,
					"resource": t.Resource,
					"status":   remote.Status(),
					"progress": remote.Progress(),
				})
			}
			if app.JSON {
				return app.Print.JSONValue(results)
			}
			for _, r := range results {
				if e, ok := r["error"]; ok {
					fmt.Fprintf(app.Print.Out, "%s  error=%v\n", r["id"], e)
					continue
				}
				fmt.Fprintf(app.Print.Out, "%s  %s  status=%v progress=%v\n",
					r["id"], r["resource"], r["status"], r["progress"])
			}
			if len(results) == 0 {
				fmt.Fprintln(app.Print.Out, "(no tracked tasks matched)")
			}
			return nil
		},
	}
	check.Flags().String("session-filter", "", "Filter by session id")
	check.Flags().String("tag-filter", "", "Filter by tag")
	check.Flags().String("resource", "", "Filter by resource")
	check.Flags().String("status", "", "Filter by status")
	cmd.AddCommand(check)

	return cmd
}

func flagStr(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func (app *App) resolveSession(idOrName string) (store.Session, error) {
	if sess, ok, err := app.Store.GetSession(idOrName); err != nil {
		return store.Session{}, err
	} else if ok {
		return sess, nil
	}
	sessions, err := app.Store.ListSessions(false)
	if err != nil {
		return store.Session{}, err
	}
	for _, s := range sessions {
		if s.Name == idOrName || s.ID == idOrName {
			return s, nil
		}
	}
	return store.Session{}, fmt.Errorf("session %q not found", idOrName)
}
