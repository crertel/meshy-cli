package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crertel/meshy-cli/internal/cli"
	"github.com/crertel/meshy-cli/internal/store"
)

func TestResolveConfigPrecedence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "t.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SetSetting("api_key", "from-sqlite"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting("base_url", "https://from-sqlite"); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MESHY_API_KEY", "")
	t.Setenv("MESHY_BASE_URL", "")
	// Clear may not empty if parent set; force
	_ = os.Unsetenv("MESHY_API_KEY")
	_ = os.Unsetenv("MESHY_BASE_URL")

	cfg, src, err := cli.ResolveConfig("", "", dbPath, st)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "from-sqlite" || src.APIKey != "sqlite" {
		t.Fatalf("sqlite key: cfg=%q src=%q", cfg.APIKey, src.APIKey)
	}
	if cfg.BaseURL != "https://from-sqlite" || src.BaseURL != "sqlite" {
		t.Fatalf("sqlite url: cfg=%q src=%q", cfg.BaseURL, src.BaseURL)
	}

	t.Setenv("MESHY_API_KEY", "from-env")
	t.Setenv("MESHY_BASE_URL", "https://from-env")
	cfg, src, err = cli.ResolveConfig("", "", dbPath, st)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "from-env" || src.APIKey != "env" {
		t.Fatalf("env key: cfg=%q src=%q", cfg.APIKey, src.APIKey)
	}
	if cfg.BaseURL != "https://from-env" || src.BaseURL != "env" {
		t.Fatalf("env url: cfg=%q src=%q", cfg.BaseURL, src.BaseURL)
	}

	cfg, src, err = cli.ResolveConfig("from-flag", "https://from-flag", dbPath, st)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "from-flag" || src.APIKey != "flag" {
		t.Fatalf("flag key: cfg=%q src=%q", cfg.APIKey, src.APIKey)
	}
	if cfg.BaseURL != "https://from-flag" || src.BaseURL != "flag" {
		t.Fatalf("flag url: cfg=%q src=%q", cfg.BaseURL, src.BaseURL)
	}
}

func TestRequireAPIKey(t *testing.T) {
	if err := cli.RequireAPIKey(""); err == nil {
		t.Fatal("expected error")
	}
	if err := cli.RequireAPIKey("msy_x"); err != nil {
		t.Fatal(err)
	}
}
