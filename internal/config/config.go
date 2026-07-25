package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultEndpoint = "http://localhost:11434"
	AppName         = "ollama-mgr"
)

// Version is set at link time via -ldflags "-X .../config.Version=...".
var Version = "0.1.0"

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

// FetchedPath is the on-disk list of library families added via "+ Family".
func (c Config) FetchedPath() string {
	return filepath.Join(c.ConfigDir, "fetched.json")
}

type fetchedFile struct {
	Bases []string `json:"bases"`
}

// LoadFetchedBases returns persisted empty-family board names.
func (c Config) LoadFetchedBases() []string {
	b, err := os.ReadFile(c.FetchedPath())
	if err != nil {
		return nil
	}
	var f fetchedFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil
	}
	out := make([]string, 0, len(f.Bases))
	seen := map[string]bool{}
	for _, name := range f.Bases {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// SaveFetchedBases writes the fetched-family board (sorted unique).
func (c Config) SaveFetchedBases(bases []string) error {
	if err := c.EnsureDirs(); err != nil {
		return err
	}
	seen := map[string]bool{}
	var clean []string
	for _, name := range bases {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		clean = append(clean, name)
	}
	sort.Strings(clean)
	b, err := json.MarshalIndent(fetchedFile{Bases: clean}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.FetchedPath(), b, 0o644)
}
