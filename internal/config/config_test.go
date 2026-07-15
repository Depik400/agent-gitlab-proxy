package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	got, err := NormalizeURL("https://gitlab.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://gitlab.example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateHostName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "Main", wantErr: false},
		{name: "main-host", wantErr: true},
		{name: "main host", wantErr: true},
		{name: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHost(Host{Name: tt.name, URL: "https://gitlab.example.com", Token: "token"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateHost() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveLoadAndMask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Config{
		Version: Version,
		Hosts:   []Host{{Name: "Main", URL: "https://gitlab.example.com", Token: "secret"}},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %v, want 0600", got)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Hosts[0].Token != "secret" {
		t.Fatalf("token = %q", loaded.Hosts[0].Token)
	}
	masked := Mask(loaded)
	if masked.Hosts[0].Token != "" {
		t.Fatalf("masked token = %q", masked.Hosts[0].Token)
	}
}

func TestUpsertHost(t *testing.T) {
	cfg := Empty()
	cfg = UpsertHost(cfg, Host{Name: "Main", URL: "https://one.example.com", Token: "one"})
	cfg = UpsertHost(cfg, Host{Name: "Main", URL: "https://two.example.com", Token: "two"})
	if len(cfg.Hosts) != 1 {
		t.Fatalf("hosts len = %d", len(cfg.Hosts))
	}
	if cfg.Hosts[0].URL != "https://two.example.com" {
		t.Fatalf("url = %q", cfg.Hosts[0].URL)
	}
}
