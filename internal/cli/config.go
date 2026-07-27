package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crertel/meshy-cli/internal/store"
)

// Config holds resolved runtime configuration.
type Config struct {
	APIKey  string
	BaseURL string
	DBPath  string
	JSON    bool
	Session string
	Tags    []string
	// Store is opened lazily / by root; may be nil until OpenStore.
	Store *store.Store
}

// ResolveSources documents where a value came from (for tests/debug).
type ResolveSources struct {
	APIKey  string // "flag", "env", "sqlite", or ""
	BaseURL string
	DBPath  string
}

// ResolveConfig applies precedence: flag > env > SQLite > default.
// Empty flag strings mean "unset".
func ResolveConfig(flagAPIKey, flagBaseURL, flagDB string, st *store.Store) (Config, ResolveSources, error) {
	var cfg Config
	var src ResolveSources

	// DB path first so we can open store if needed.
	cfg.DBPath = firstNonEmpty(flagDB, os.Getenv("MESHY_DB"))
	if cfg.DBPath != "" {
		src.DBPath = pickSource(flagDB, os.Getenv("MESHY_DB"), "")
	} else {
		p, err := store.DefaultPath()
		if err != nil {
			return cfg, src, err
		}
		cfg.DBPath = p
		src.DBPath = "default"
	}

	// API key
	if flagAPIKey != "" {
		cfg.APIKey = flagAPIKey
		src.APIKey = "flag"
	} else if v := os.Getenv("MESHY_API_KEY"); v != "" {
		cfg.APIKey = v
		src.APIKey = "env"
	} else if st != nil {
		if v, ok, err := st.GetSetting("api_key"); err != nil {
			return cfg, src, err
		} else if ok && v != "" {
			cfg.APIKey = v
			src.APIKey = "sqlite"
		}
	}

	// Base URL
	if flagBaseURL != "" {
		cfg.BaseURL = flagBaseURL
		src.BaseURL = "flag"
	} else if v := os.Getenv("MESHY_BASE_URL"); v != "" {
		cfg.BaseURL = v
		src.BaseURL = "env"
	} else if st != nil {
		if v, ok, err := st.GetSetting("base_url"); err != nil {
			return cfg, src, err
		} else if ok && v != "" {
			cfg.BaseURL = v
			src.BaseURL = "sqlite"
		}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.meshy.ai"
		if src.BaseURL == "" {
			src.BaseURL = "default"
		}
	}

	return cfg, src, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func pickSource(flag, env, sqlite string) string {
	if flag != "" {
		return "flag"
	}
	if env != "" {
		return "env"
	}
	if sqlite != "" {
		return "sqlite"
	}
	return "default"
}

// EnsureDBDir creates parent directories for the DB path.
func EnsureDBDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

// RequireAPIKey returns a clear error if the key is missing.
func RequireAPIKey(key string) error {
	if key == "" {
		return fmt.Errorf("missing API key: set --api-key, MESHY_API_KEY, or run `meshy config set api_key <key>`")
	}
	return nil
}
