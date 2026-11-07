package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndSave(t *testing.T) {
	cfgHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	s := NewService()
	cfg, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AutosaveMs <= 0 || cfg.ConfigVersion != 1 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	cfg.LastActiveVault = "/tmp/vault"
	cfg, err = s.Save(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfgHome, "markvault", "config.json")); err != nil {
		t.Fatal(err)
	}
	cfg2, err := s.Load()
	if err != nil || cfg2.LastActiveVault != "/tmp/vault" {
		t.Fatalf("reload: cfg=%+v err=%v", cfg2, err)
	}
	db, err := s.DBPath()
	if err != nil || db != filepath.Join(dataHome, "markvault", "markvault.db") {
		t.Fatalf("db path: %q %v", db, err)
	}
}
