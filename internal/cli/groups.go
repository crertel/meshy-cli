package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/crertel/meshy-cli/internal/api"
	"github.com/crertel/meshy-cli/internal/store"
	"github.com/spf13/cobra"
)

func (app *App) printGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "print",
		Short: "3D printing: multi-color, analyze printability, repair",
		Long: `3D printing helpers:
  multi-color  Convert models to multi-color 3MF
  analyze      Analyze printability (free)
  repair       Repair non-manifold geometry for printing
  split        Split a Meshy 6/7 model into printable parts`,
	}
	// Stable order
	for _, pair := range []struct{ use, res string }{
		{"multi-color", "print-multi-color"},
		{"analyze", "print-analyze"},
		{"repair", "print-repair"},
		{"split", "print-split"},
	} {
		res, ok := api.ResourceByName(pair.res)
		if !ok {
			continue
		}
		sub := app.resourceCmd(res)
		sub.Use = pair.use
		cmd.AddCommand(sub)
	}
	return cmd
}

func (app *App) creativeLabCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "creative-lab",
		Short: "Creative Lab products (prototype → build)",
		Long: `Creative Lab workflows under product-scoped URLs:
  keychain | fridge-magnet | figure | vinyl-figure | brick-figure | lamp | keycap | fidget-pixel
  fidget-collapsible   single-stage (no prototype/build pair)

Two-stage products:
  prototype create|get|list|delete|wait|download
  build     create|get|list|delete|wait|download

Chain build with --input-task-id from a succeeded prototype.
Keycap build also needs --candidate-id from the prototype's candidate_ids.
Fidget-pixel build geometry is --shape, --grid-size, --piece-size-mm, --color-count, --piece-height-mm.`,
	}
	for _, product := range []string{"keychain", "fridge-magnet", "figure", "vinyl-figure", "brick-figure", "lamp", "keycap", "fidget-pixel"} {
		cmd.AddCommand(app.creativeProductCmd(product))
	}
	if res, ok := api.ResourceByName("cl-fidget-collapsible"); ok {
		sub := app.resourceCmd(res)
		sub.Use = "fidget-collapsible"
		sub.Short = res.Description
		cmd.AddCommand(sub)
	}
	return cmd
}

func (app *App) creativeProductCmd(product string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   product,
		Short: "Creative Lab " + product + " (prototype + build)",
	}
	for _, stage := range []string{"prototype", "build"} {
		resName := "cl-" + product + "-" + stage
		res, ok := api.ResourceByName(resName)
		if !ok {
			continue
		}
		sub := app.resourceCmd(res)
		sub.Use = stage
		sub.Short = res.Description
		cmd.AddCommand(sub)
	}
	return cmd
}

func (app *App) balanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "balance",
		Short: "Account credit balance",
	}
	get := &cobra.Command{
		Use:   "get",
		Short: "Get remaining credit balance",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			bal, err := app.Client.GetBalance(app.ctx())
			if err != nil {
				return err
			}
			return app.Print.PrintAny(bal)
		},
	}
	cmd.AddCommand(get)
	cmd.RunE = get.RunE
	return cmd
}

func (app *App) usageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Historic task usage and credit cost (Studio and Enterprise)",
	}
	var pageNum, pageSize int
	var sortBy, start, end, status string
	var endpoints []string
	list := &cobra.Command{
		Use:   "list",
		Short: "List usage records (GET /openapi/v1/usage/tasks)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := RequireAPIKey(app.Client.APIKey); err != nil {
				return err
			}
			rows, err := app.Client.ListUsage(app.ctx(), api.UsageFilter{
				PageNum:   pageNum,
				PageSize:  pageSize,
				SortBy:    sortBy,
				StartTime: start,
				EndTime:   end,
				Endpoints: strings.Join(endpoints, ","),
				Status:    status,
			})
			if err != nil {
				return err
			}
			return app.Print.PrintAny(rows)
		},
	}
	list.Flags().IntVar(&pageNum, "page", 1, "Page number")
	list.Flags().IntVar(&pageSize, "page-size", 50, "Page size (max 100)")
	list.Flags().StringVar(&sortBy, "sort", "", "Sort key; created_at is sortable, leading - is descending")
	list.Flags().StringVar(&start, "start", "", "Window start, RFC 3339 (default: 30 days before --end)")
	list.Flags().StringVar(&end, "end", "", "Window end, RFC 3339 (default: now)")
	list.Flags().StringSliceVar(&endpoints, "endpoint", nil, "Endpoint names to include (image-to-3d, animate, …)")
	list.Flags().StringVar(&status, "status", "", "Terminal status filter (SUCCEEDED, FAILED, …)")
	cmd.AddCommand(list)
	return cmd
}

func (app *App) configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Persistent settings in SQLite (api_key, base_url, ...)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "get <key>",
		Short: "Get a setting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, ok, err := app.Store.GetSetting(args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("setting %q not set", args[0])
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]string{args[0]: v})
			}
			if args[0] == "api_key" {
				fmt.Fprintln(app.Print.Out, redactSecret(v))
			} else {
				fmt.Fprintln(app.Print.Out, v)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a setting (api_key, base_url, ...)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Store.SetSetting(args[0], args[1]); err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]string{"set": args[0]})
			}
			fmt.Fprintf(app.Print.Out, "set %s\n", args[0])
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List settings (api_key redacted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := app.Store.ListSettings()
			if err != nil {
				return err
			}
			return app.Print.PrintSettings(m, true)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "unset <key>",
		Short: "Delete a setting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.Store.DeleteSetting(args[0]); err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]string{"unset": args[0]})
			}
			fmt.Fprintf(app.Print.Out, "unset %s\n", args[0])
			return nil
		},
	})
	return cmd
}

func (app *App) sessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Local work sessions for batch job tracking",
		Long: `Sessions are local-only bookkeeping (not Meshy cloud sessions).
Open a session, pass --session <id-or-name> when creating tasks, then
list/check status for the batch.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "open <name>",
		Short: "Open a new session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			id := slugID(name)
			if _, ok, _ := app.Store.GetSession(id); ok {
				id = fmt.Sprintf("%s-%d", id, time.Now().UnixMilli())
			}
			sess, err := app.Store.CreateSession(id, name)
			if err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(sess)
			}
			fmt.Fprintf(app.Print.Out, "opened session %s (%s)\n", sess.ID, sess.Name)
			return nil
		},
	})

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			openOnly, _ := cmd.Flags().GetBool("open")
			sessions, err := app.Store.ListSessions(openOnly)
			if err != nil {
				return err
			}
			return app.Print.PrintSessions(sessions)
		},
	}
	listCmd.Flags().Bool("open", false, "Only open sessions")
	cmd.AddCommand(listCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show session and its tracked tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := app.resolveSession(args[0])
			if err != nil {
				return err
			}
			tasks, err := app.Store.ListTasks(store.TaskFilter{SessionID: sess.ID})
			if err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]any{"session": sess, "tasks": tasks})
			}
			state := "open"
			if sess.ClosedAt != nil {
				state = "closed"
			}
			fmt.Fprintf(app.Print.Out, "session %s (%s) [%s]\n", sess.ID, sess.Name, state)
			return app.Print.PrintTracked(tasks)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "close <id-or-name>",
		Short: "Close a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := app.resolveSession(args[0])
			if err != nil {
				return err
			}
			if err := app.Store.CloseSession(sess.ID); err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]string{"closed": sess.ID})
			}
			fmt.Fprintf(app.Print.Out, "closed session %s\n", sess.ID)
			return nil
		},
	})

	return cmd
}

func slugID(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "session"
	}
	return out
}
