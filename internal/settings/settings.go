package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"markvault/internal/domain"
)

const (
	appDirName        = "markvault"
	configName        = "config.json"
	configVersion     = 1
	defaultAutosaveMs = 750
)

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) ConfigDir() (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, appDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", appDirName), nil
}

func (s *Service) DataDir() (string, error) {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, appDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", appDirName), nil
}

func (s *Service) DBPath() (string, error) {
	dir, err := s.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "markvault.db"), nil
}

func (s *Service) ConfigPath() (string, error) {
	dir, err := s.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configName), nil
}

func defaults() domain.Settings {
	return domain.Settings{
		RecentVaults:   []string{},
		AutosaveMs:     defaultAutosaveMs,
		PreviewDefault: false,
		ConfigVersion:  configVersion,
	}
}

func (s *Service) Load() (domain.Settings, error) {
	p, err := s.ConfigPath()
	if err != nil {
		return defaults(), err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaults(), nil
		}
		return defaults(), err
	}
	cfg := defaults()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return defaults(), err
	}
	if cfg.AutosaveMs <= 0 {
		cfg.AutosaveMs = defaultAutosaveMs
	}
	if cfg.RecentVaults == nil {
		cfg.RecentVaults = []string{}
	}
	if cfg.ConfigVersion == 0 {
		cfg.ConfigVersion = configVersion
	}
	return cfg, nil
}

func (s *Service) Save(cfg domain.Settings) (domain.Settings, error) {
	if cfg.AutosaveMs <= 0 {
		cfg.AutosaveMs = defaultAutosaveMs
	}
	if cfg.RecentVaults == nil {
		cfg.RecentVaults = []string{}
	}
	cfg.ConfigVersion = configVersion
	dir, err := s.ConfigDir()
	if err != nil {
		return cfg, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return cfg, err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return cfg, err
	}
	b = append(b, '\n')
	p, err := s.ConfigPath()
	if err != nil {
		return cfg, err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return cfg, err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return cfg, err
	}
	return cfg, nil
}

func (s *Service) EnsureDataDir() (string, error) {
	dir, err := s.DataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
