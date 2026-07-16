package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gitlab-proxy/internal/apperr"
)

const (
	Version = 1
	EnvKey  = "GITLAB_PROXY_CONFIG"
)

var nameRE = regexp.MustCompile(`^[A-Za-z]{1,100}$`)

type Config struct {
	Version     int    `json:"version"`
	DefaultHost string `json:"default_host,omitempty"`
	Hosts       []Host `json:"hosts"`
}

type Host struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}

func DefaultPath() (string, error) {
	if path := os.Getenv(EnvKey); path != "" {
		return path, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", apperr.Wrap(apperr.CodeConfig, "resolve user config dir", err, nil)
	}
	return filepath.Join(dir, "gitlab-proxy", "config.json"), nil
}

func Empty() Config {
	return Config{Version: Version, Hosts: []Host{}}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return Config{}, apperr.Wrap(apperr.CodeConfig, "read config", err, nil)
	}
	cfg, err := Parse(data)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Parse(data []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, apperr.Wrap(apperr.CodeConfig, "parse config json", err, nil)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "create config dir", err, nil)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "encode config", err, nil)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return apperr.Wrap(apperr.CodeConfig, "write config", err, nil)
	}
	return nil
}

func Validate(cfg Config) error {
	if cfg.Version != Version {
		return apperr.New(apperr.CodeConfig, fmt.Sprintf("unsupported config version %d", cfg.Version), nil)
	}
	seen := map[string]struct{}{}
	for _, host := range cfg.Hosts {
		if err := ValidateHost(host); err != nil {
			return err
		}
		if _, ok := seen[host.Name]; ok {
			return apperr.New(apperr.CodeConfig, "duplicate host name", map[string]string{"name": host.Name})
		}
		seen[host.Name] = struct{}{}
	}
	if cfg.DefaultHost != "" {
		if _, ok := seen[cfg.DefaultHost]; !ok {
			return apperr.New(apperr.CodeConfig, "default host is not configured", map[string]string{"default_host": cfg.DefaultHost})
		}
	}
	return nil
}

func ValidateHost(host Host) error {
	if !nameRE.MatchString(host.Name) {
		return apperr.New(apperr.CodeInvalidArgs, "host name must contain only English letters and be at most 100 characters", map[string]string{"name": host.Name})
	}
	if strings.TrimSpace(host.Token) == "" {
		return apperr.New(apperr.CodeInvalidArgs, "token is required", nil)
	}
	normalized, err := NormalizeURL(host.URL)
	if err != nil {
		return err
	}
	if normalized != host.URL {
		return apperr.New(apperr.CodeInvalidArgs, "url must be normalized", map[string]string{"normalized_url": normalized})
	}
	return nil
}

func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", apperr.New(apperr.CodeInvalidArgs, "url is required", nil)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", apperr.Wrap(apperr.CodeInvalidArgs, "invalid url", err, map[string]string{"url": raw})
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(u.Path, "/api/v4") {
		u.Path = strings.TrimSuffix(u.Path, "/api/v4")
	}
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func UpsertHost(cfg Config, host Host, makeDefault bool) Config {
	for i := range cfg.Hosts {
		if cfg.Hosts[i].Name == host.Name {
			cfg.Hosts[i] = host
			if makeDefault || cfg.DefaultHost == "" {
				cfg.DefaultHost = host.Name
			}
			return cfg
		}
	}
	cfg.Hosts = append(cfg.Hosts, host)
	if makeDefault || cfg.DefaultHost == "" {
		cfg.DefaultHost = host.Name
	}
	return cfg
}

func FindHost(cfg Config, name string) (Host, error) {
	for _, host := range cfg.Hosts {
		if host.Name == name {
			return host, nil
		}
	}
	return Host{}, apperr.New(apperr.CodeConfig, "host is not configured", map[string]string{"host_name": name})
}

func ResolveHost(cfg Config, name string) (Host, error) {
	if name != "" {
		return FindHost(cfg, name)
	}
	if cfg.DefaultHost == "" {
		return Host{}, apperr.New(apperr.CodeConfig, "host name is required when default_host is not configured", nil)
	}
	return FindHost(cfg, cfg.DefaultHost)
}

func Mask(cfg Config) Config {
	out := cfg
	out.Hosts = append([]Host(nil), cfg.Hosts...)
	for i := range out.Hosts {
		if out.Hosts[i].Token != "" {
			out.Hosts[i].Token = ""
		}
	}
	return out
}
