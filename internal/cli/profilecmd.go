package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (app *App) profileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage generation/download profiles (built-in + user CRUD in SQLite)",
		Long: `Profiles bundle create/download defaults. Built-ins ship with the CLI;
user profiles are stored in the local SQLite DB and shadow built-ins of the same name.

  meshy profile list
  meshy profile show game-hero
  meshy profile create my-game --from game-lowpoly --target-polycount 20000
  meshy profile update my-game --download-formats glb,fbx --textures
  meshy profile delete my-game
  meshy --profile my-game ez textured-3d --prompt "…"
`,
	}
	cmd.AddCommand(app.profileListCmd())
	cmd.AddCommand(app.profileShowCmd())
	cmd.AddCommand(app.profileCreateCmd())
	cmd.AddCommand(app.profileUpdateCmd())
	cmd.AddCommand(app.profileDeleteCmd())
	// alias under ez profiles still works
	return cmd
}

func (app *App) profileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List built-in and user profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			all, err := ListAllProfiles(app.Store)
			if err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(all)
			}
			if len(all) == 0 {
				fmt.Fprintln(app.Print.Out, "(no profiles)")
				return nil
			}
			for _, p := range all {
				src := p.Source
				if src == "" {
					if p.Builtin {
						src = "builtin"
					} else {
						src = "user"
					}
				}
				fmt.Fprintf(app.Print.Out, "%-16s  %-8s  %s\n", p.Name, src, p.Description)
			}
			return nil
		},
	}
}

func (app *App) profileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a profile (user or built-in)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, ok, err := ResolveProfile(app.Store, args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("profile %q not found", args[0])
			}
			return app.Print.PrintAny(p)
		},
	}
}

func (app *App) profileCreateCmd() *cobra.Command {
	var from string
	var flags profileFlagSet
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a user profile (optionally clone --from a built-in)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := normalizeProfileName(args[0])
			if name == "" {
				return fmt.Errorf("name required")
			}
			// refuse if user profile already exists
			if app.Store != nil {
				if _, ok, err := app.Store.GetProfile(name); err != nil {
					return err
				} else if ok {
					return fmt.Errorf("user profile %q already exists (use profile update)", name)
				}
			}
			var base Profile
			if from != "" {
				p, ok, err := ResolveProfile(app.Store, from)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("source profile %q not found", from)
				}
				base = p
			}
			base.Name = name
			base.Builtin = false
			base.Source = "user"
			applyProfileFlagSet(&base, flags, cmd)
			if err := SaveProfile(app.Store, base); err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(base)
			}
			fmt.Fprintf(app.Print.Out, "created profile %s\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Clone from existing profile name")
	addProfileFieldFlags(cmd, &flags)
	return cmd
}

func (app *App) profileUpdateCmd() *cobra.Command {
	var flags profileFlagSet
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update fields on a user profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := normalizeProfileName(args[0])
			sp, ok, err := app.Store.GetProfile(name)
			if err != nil {
				return err
			}
			if !ok {
				// allow update-by-cloning builtin into user profile
				if bp, isB := ProfileByName(name); isB {
					base := bp
					base.Name = name
					base.Builtin = false
					applyProfileFlagSet(&base, flags, cmd)
					if err := SaveProfile(app.Store, base); err != nil {
						return err
					}
					if app.JSON {
						return app.Print.JSONValue(base)
					}
					fmt.Fprintf(app.Print.Out, "created user override for built-in %s\n", name)
					return nil
				}
				return fmt.Errorf("user profile %q not found (create it first, or update a built-in name to shadow it)", name)
			}
			base, err := decodeStoredProfile(sp)
			if err != nil {
				return err
			}
			applyProfileFlagSet(&base, flags, cmd)
			if err := SaveProfile(app.Store, base); err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(base)
			}
			fmt.Fprintf(app.Print.Out, "updated profile %s\n", name)
			return nil
		},
	}
	addProfileFieldFlags(cmd, &flags)
	return cmd
}

func (app *App) profileDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a user profile (built-ins cannot be deleted)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := DeleteUserProfile(app.Store, args[0]); err != nil {
				return err
			}
			if app.JSON {
				return app.Print.JSONValue(map[string]string{"deleted": normalizeProfileName(args[0])})
			}
			fmt.Fprintf(app.Print.Out, "deleted profile %s\n", normalizeProfileName(args[0]))
			return nil
		},
	}
}

type profileFlagSet struct {
	description      string
	aiModel          string
	targetPoly       int
	downloadFormats  string
	shouldRemesh     bool
	shouldTexture    bool
	enablePBR        bool
	downloadTextures bool
	downloadAll      bool
}

func addProfileFieldFlags(cmd *cobra.Command, f *profileFlagSet) {
	cmd.Flags().StringVar(&f.description, "description", "", "Profile description")
	cmd.Flags().StringVar(&f.aiModel, "ai-model", "", "Default ai_model")
	cmd.Flags().IntVar(&f.targetPoly, "target-polycount", 0, "Default target_polycount")
	cmd.Flags().StringVar(&f.downloadFormats, "download-formats", "", "Comma-separated formats (glb,fbx,…)")
	cmd.Flags().BoolVar(&f.shouldRemesh, "should-remesh", false, "Default should_remesh (set with flag to enable)")
	cmd.Flags().BoolVar(&f.shouldTexture, "should-texture", false, "Default should_texture")
	cmd.Flags().BoolVar(&f.enablePBR, "enable-pbr", false, "Default enable_pbr")
	cmd.Flags().BoolVar(&f.downloadTextures, "textures", false, "Also download PBR maps by default")
	cmd.Flags().BoolVar(&f.downloadAll, "download-all", false, "Download all assets by default")
}

func applyProfileFlagSet(p *Profile, f profileFlagSet, cmd *cobra.Command) {
	if cmd.Flags().Changed("description") {
		p.Description = f.description
	}
	if cmd.Flags().Changed("ai-model") {
		p.AIModel = f.aiModel
	}
	if cmd.Flags().Changed("target-polycount") {
		p.TargetPolycount = f.targetPoly
	}
	if cmd.Flags().Changed("download-formats") {
		p.DownloadFormats = splitCSV(f.downloadFormats)
	}
	if cmd.Flags().Changed("should-remesh") {
		v := f.shouldRemesh
		p.ShouldRemesh = &v
	}
	if cmd.Flags().Changed("should-texture") {
		v := f.shouldTexture
		p.ShouldTexture = &v
	}
	if cmd.Flags().Changed("enable-pbr") {
		v := f.enablePBR
		p.EnablePBR = &v
	}
	if cmd.Flags().Changed("textures") {
		p.DownloadTextures = f.downloadTextures
	}
	if cmd.Flags().Changed("download-all") {
		p.DownloadAll = f.downloadAll
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

