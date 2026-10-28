package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/crertel/meshy-cli/internal/api"
	"github.com/crertel/meshy-cli/internal/store"
	"github.com/spf13/cobra"
)

// App holds shared CLI state.
type App struct {
	FlagAPIKey   string
	FlagBaseURL  string
	FlagDB       string
	FlagProfile  string
	JSON         bool
	Quiet        bool
	PreferStream bool // global --stream: prefer SSE wait over poll
	Session      string
	Tags         []string
	Metadata     []string // X-Meshy-Metadata key=value pairs
	PollEvery    time.Duration
	WaitTimeout  time.Duration

	// Injectable for tests
	HTTPClient *http.Client
	Store      *store.Store
	Client     *api.Client
	Cfg        Config
	Print      *Printer
	Profile    *Profile
}

// NewRoot builds the meshy command tree.
func NewRoot() *cobra.Command {
	return (&App{}).buildRoot()
}

// ExecuteWithApp builds a root command bound to the given App (for tests).
func ExecuteWithApp(app *App, args []string) error {
	root := app.buildRoot()
	if app.Print != nil {
		if app.Print.Out != nil {
			root.SetOut(app.Print.Out)
		}
		if app.Print.Err != nil {
			root.SetErr(app.Print.Err)
		}
	}
	root.SetArgs(args)
	return root.Execute()
}

func (app *App) buildRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "meshy",
		Short: "CLI for the Meshy.AI API (3D generation, post-process, print, Creative Lab)",
		Long: `meshy is a resource-oriented CLI for https://api.meshy.ai.

Resource families:
  generation   text-to-3d, image-to-3d, multi-image-to-3d
  postprocess  remesh, convert, resize, uv-unwrap, retexture, rigging, animation, text-to-motion
  image        text-to-image, image-to-image
  print        print multi-color | analyze | repair | split
  creative-lab creative-lab <product> prototype|build, or fidget-collapsible
  account      balance, usage

Shared task verbs:
  create [--sync] [--download] | get | list | delete | wait [--stream] | stream | assets | download

Workflows (sane defaults):
  ez textured-3d | image-to-3d | multi-image-to-3d | image-to-image
  ez retexture | remesh | print-analyze | print-repair

Profiles (global --profile): game-lowpoly, game-hero, cinema, print, fast

Config precedence: flag > env > profile > SQLite (~/.config/meshy/meshy.db).

Install (Nix flakes):
  nix run github:crertel/meshy-cli -- --help
  nix profile install github:crertel/meshy-cli

Examples:
  meshy ez textured-3d --prompt "beefcake minotaur" --profile game-hero --name minotaur
  meshy ez image-to-3d --image ./photo.png -o ./out --post 'xviewer {{glb}} &'
  meshy --profile cinema text-to-3d create-preview --prompt "mug" --sync
  meshy text-to-3d download <id> --format glb,fbx --textures
  meshy balance get --json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "help" || cmd.Name() == "completion" {
				return nil
			}
			return app.setup()
		},
	}
	root.PersistentFlags().StringVar(&app.FlagAPIKey, "api-key", "", "Meshy API key (or MESHY_API_KEY / config)")
	root.PersistentFlags().StringVar(&app.FlagBaseURL, "base-url", "", "API base URL (or MESHY_BASE_URL / config)")
	root.PersistentFlags().StringVar(&app.FlagDB, "db", "", "SQLite DB path (or MESHY_DB)")
	root.PersistentFlags().StringVar(&app.FlagProfile, "profile", "", "Built-in profile defaults (game-lowpoly, game-hero, cinema, print, fast)")
	root.PersistentFlags().BoolVar(&app.JSON, "json", false, "Emit structured JSON on stdout")
	root.PersistentFlags().BoolVar(&app.Quiet, "quiet", false, "Suppress progress logs on stderr")
	root.PersistentFlags().BoolVar(&app.PreferStream, "stream", false, "Prefer SSE stream wait over polling (where supported)")
	root.PersistentFlags().StringVar(&app.Session, "session", "", "Local session id/name to attach created tasks to")
	root.PersistentFlags().StringSliceVar(&app.Tags, "tag", nil, "Tag(s) to attach to created/tracked tasks (repeatable)")
	root.PersistentFlags().StringSliceVar(&app.Metadata, "metadata", nil, "X-Meshy-Metadata key=value (repeatable; Studio/Enterprise). Creative Lab ignores it")
	root.PersistentFlags().DurationVar(&app.PollEvery, "poll-interval", 5*time.Second, "Polling interval for wait")
	root.PersistentFlags().DurationVar(&app.WaitTimeout, "wait-timeout", 0, "Max wait duration (0 = no limit)")

	for _, res := range api.AllResources {
		if strings.HasPrefix(res.Name, "cl-") || strings.HasPrefix(res.Name, "print-") {
			continue
		}
		root.AddCommand(app.resourceCmd(res))
	}
	root.AddCommand(app.printGroupCmd())
	root.AddCommand(app.creativeLabCmd())
	root.AddCommand(app.ezCmd())
	root.AddCommand(app.profileCmd())
	root.AddCommand(app.balanceCmd())
	root.AddCommand(app.usageCmd())
	root.AddCommand(app.configCmd())
	root.AddCommand(app.sessionCmd())
	root.AddCommand(app.tasksCmd())
	root.AddCommand(app.versionCmd())
	return root
}

func (app *App) setup() error {
	if app.Print == nil {
		app.Print = NewPrinter(app.JSON)
	} else {
		app.Print.JSON = app.JSON
	}

	if app.Store == nil {
		dbPath := firstNonEmpty(app.FlagDB, os.Getenv("MESHY_DB"))
		if dbPath == "" {
			p, err := store.DefaultPath()
			if err != nil {
				return err
			}
			dbPath = p
		}
		st, err := store.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		app.Store = st
		cobra.OnFinalize(func() {
			_ = app.Store.Close()
		})
	}

	cfg, _, err := ResolveConfig(app.FlagAPIKey, app.FlagBaseURL, app.FlagDB, app.Store)
	if err != nil {
		return err
	}
	cfg.JSON = app.JSON
	cfg.Session = app.Session
	cfg.Tags = app.Tags
	cfg.Store = app.Store
	app.Cfg = cfg

	if app.FlagProfile != "" {
		p, ok, err := ResolveProfile(app.Store, app.FlagProfile)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("unknown profile %q (try: meshy profile list)", app.FlagProfile)
		}
		app.Profile = &p
	}

	if app.Client == nil {
		app.Client = api.NewClient(cfg.BaseURL, cfg.APIKey, app.HTTPClient)
	} else {
		if app.Client.APIKey == "" {
			app.Client.APIKey = cfg.APIKey
		}
		if app.Client.BaseURL == "" || app.Client.BaseURL == api.DefaultBaseURL {
			if cfg.BaseURL != "" {
				app.Client.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
			}
		}
		if app.HTTPClient != nil {
			app.Client.HTTPClient = app.HTTPClient
		}
	}
	if len(app.Metadata) > 0 {
		app.Client.Metadata = strings.Join(app.Metadata, ",")
	}
	return nil
}

func (app *App) ctx() context.Context {
	return context.Background()
}

// Version is the meshy CLI release.
const Version = "0.2.0"

func (app *App) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.JSON {
				return app.Print.JSONValue(map[string]string{"version": Version, "name": "meshy"})
			}
			fmt.Fprintln(app.Print.Out, "meshy "+Version)
			return nil
		},
	}
}

func (app *App) trackTask(resource, id, remotePath, status string) error {
	if app.Store == nil {
		return nil
	}
	sessionID := app.Session
	if sessionID != "" {
		if sess, err := app.resolveSession(sessionID); err == nil {
			sessionID = sess.ID
		}
	}
	t := store.TrackedTask{
		ID:         id,
		Resource:   resource,
		SessionID:  sessionID,
		Status:     status,
		RemotePath: remotePath,
	}
	if err := app.Store.UpsertTask(t); err != nil {
		return err
	}
	if len(app.Tags) > 0 {
		return app.Store.AddTags(id, app.Tags)
	}
	return nil
}
