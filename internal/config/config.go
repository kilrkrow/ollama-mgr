package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultEndpoint = "http://localhost:11434"
	AppName         = "ollama-mgr"
	Version         = "0.1.0"
)

// Config holds runtime settings for CLI and GUI.
type Config struct {
	Endpoint  string
	CacheTTL  time.Duration
	Pinned    map[string]bool
	CacheDir  string
	ConfigDir string
}

// Default returns config with sensible defaults, env overrides applied.
func Default() Config {
	endpoint := os.Getenv("OLLAMA_HOST")
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	// OLLAMA_HOST may be host:port without scheme
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}
	endpoint = strings.TrimRight(endpoint, "/")

	cacheTTL := 24 * time.Hour
	if v := os.Getenv("OLLAMA_MGR_CACHE_TTL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cacheTTL = time.Duration(n) * time.Hour
		}
	}

	configDir := configDirPath()
	cacheDir := filepath.Join(localAppData(), AppName, "cache")

	return Config{
		Endpoint:  endpoint,
		CacheTTL:  cacheTTL,
		Pinned:    map[string]bool{},
		CacheDir:  cacheDir,
		ConfigDir: configDir,
	}
}

func configDirPath() string {
	if d := os.Getenv("OLLAMA_MGR_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(appData(), AppName)
}

func appData() string {
	if d := os.Getenv("APPDATA"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func localAppData() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache")
}

// EnsureDirs creates config and cache directories if missing.
func (c Config) EnsureDirs() error {
	if err := os.MkdirAll(c.ConfigDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(c.CacheDir, 0o755)
}

// IsPinned reports whether a model name or base name is pinned.
func (c Config) IsPinned(name string) bool {
	if c.Pinned[name] {
		return true
	}
	base := name
	if i := strings.Index(name, ":"); i >= 0 {
		base = name[:i]
	}
	return c.Pinned[base]
}
