package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crertel/meshy-cli/internal/cli"
	"github.com/crertel/meshy-cli/internal/store"
)

func TestProfileCRUD(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "p.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// list includes builtins
	all, err := cli.ListAllProfiles(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 4 {
		t.Fatalf("expected builtins, got %d", len(all))
	}

	// create from game-hero
	var out bytes.Buffer
	app := &cli.App{Store: st, Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app, []string{
		"--db", dbPath, "--json",
		"profile", "create", "my-game",
		"--from", "game-lowpoly",
		"--target-polycount", "15000",
		"--description", "my custom lowpoly",
	}); err != nil {
		t.Fatal(err)
	}

	p, ok, err := cli.ResolveProfile(st, "my-game")
	if err != nil || !ok {
		t.Fatalf("resolve: %v ok=%v", err, ok)
	}
	if p.TargetPolycount != 15000 || p.Source != "user" {
		t.Fatalf("%+v", p)
	}

	// shadow builtin
	if err := cli.ExecuteWithApp(&cli.App{Store: st, Print: &cli.Printer{Out: io.Discard, Err: io.Discard}}, []string{
		"--db", dbPath,
		"profile", "update", "game-hero",
		"--target-polycount", "42000",
	}); err != nil {
		t.Fatal(err)
	}
	p, ok, err = cli.ResolveProfile(st, "game-hero")
	if err != nil || !ok || p.TargetPolycount != 42000 || p.Source != "user" {
		t.Fatalf("shadow: %+v ok=%v err=%v", p, ok, err)
	}

	// list JSON
	out.Reset()
	if err := cli.ExecuteWithApp(&cli.App{Store: st, Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}, []string{
		"--db", dbPath, "--json", "profile", "list",
	}); err != nil {
		t.Fatal(err)
	}
	var listed []map[string]any
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	// game-hero user shadow: only one game-hero entry
	nHero := 0
	for _, row := range listed {
		if row["name"] == "game-hero" {
			nHero++
		}
	}
	if nHero != 1 {
		t.Fatalf("expected one game-hero, got %d in %s", nHero, out.String())
	}

	// delete user override restores builtin
	if err := cli.ExecuteWithApp(&cli.App{Store: st, Print: &cli.Printer{Out: io.Discard, Err: io.Discard}}, []string{
		"--db", dbPath, "profile", "delete", "game-hero",
	}); err != nil {
		t.Fatal(err)
	}
	p, ok, err = cli.ResolveProfile(st, "game-hero")
	if err != nil || !ok || p.Source != "builtin" || p.TargetPolycount != 50000 {
		t.Fatalf("restored builtin: %+v", p)
	}

	// cannot delete pure builtin
	err = cli.ExecuteWithApp(&cli.App{Store: st, Print: &cli.Printer{Out: io.Discard, Err: io.Discard}}, []string{
		"--db", dbPath, "profile", "delete", "cinema",
	})
	if err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("expected builtin delete error, got %v", err)
	}
}
