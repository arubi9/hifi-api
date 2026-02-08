package config

import (
	"os"
	"path/filepath"

	json "github.com/goccy/go-json"
)

// Settings holds user-configurable settings, compatible with the Python format.
type Settings struct {
	DownloadDir   string `json:"download_dir"`
	Quality       string `json:"quality"`
	ParallelCount int    `json:"parallel_count"`
	LastQuery     string `json:"last_query"`
}

// DefaultSettings returns settings with sensible defaults.
func DefaultSettings() Settings {
	home, _ := os.UserHomeDir()
	return Settings{
		DownloadDir:   filepath.Join(home, "Music", "HiFi"),
		Quality:       "HI_RES_LOSSLESS",
		ParallelCount: 8,
		LastQuery:     "",
	}
}

// configDir returns ~/.tidal_tui, creating it if needed.
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".tidal_tui")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// settingsPath returns the path to settings.json.
func settingsPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// Load reads settings from ~/.tidal_tui/settings.json.
// Returns defaults if the file doesn't exist.
func Load() (Settings, error) {
	s := DefaultSettings()
	path, err := settingsPath()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettings(), err
	}
	// Clamp parallel_count
	if s.ParallelCount < 1 {
		s.ParallelCount = 1
	}
	if s.ParallelCount > 50 {
		s.ParallelCount = 50
	}
	return s, nil
}

// Save writes settings to ~/.tidal_tui/settings.json atomically.
func Save(s Settings) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ConfigDir returns the config directory path.
func ConfigDir() (string, error) {
	return configDir()
}
