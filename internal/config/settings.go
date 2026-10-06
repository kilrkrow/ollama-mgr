package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// UISettings holds user preferences for the GUI (non-secret).
// StartWithWindows is the source of truth for the HKCU Run value (applied both
// directions on launch). Start Menu presence is tracked by the .lnk file itself;
// the toggle in the UI creates/removes it.
type UISettings struct {
	StartWithWindows bool `json:"start_with_windows"`
}

func settingsPath() string {
	return filepath.Join(configDirPath(), "settings.json")
}

// LoadUISettings reads %APPDATA%\ollama-mgr\settings.json (or defaults).
func LoadUISettings() UISettings {
	b, err := os.ReadFile(settingsPath())
	if err != nil {
		return UISettings{}
	}
	var s UISettings
	if json.Unmarshal(b, &s) != nil {
		return UISettings{}
	}
	return s
}

// SaveUISettings writes settings.json.
func SaveUISettings(s UISettings) error {
	if err := os.MkdirAll(configDirPath(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath(), b, 0o644)
}