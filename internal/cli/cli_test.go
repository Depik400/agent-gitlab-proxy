package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab-proxy/internal/apperr"
	"gitlab-proxy/internal/config"
)

func TestRunConfigMasksToken(t *testing.T) {
	configPath := writeTestConfig(t)
	t.Setenv(config.EnvKey, configPath)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"config"}, strings.NewReader(""), &stdout, &stderr)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	var got config.Config
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Hosts[0].Token != "" {
		t.Fatalf("token = %q", got.Hosts[0].Token)
	}
}

func TestRunExportIncludesSecretsWhenRequested(t *testing.T) {
	configPath := writeTestConfig(t)
	t.Setenv(config.EnvKey, configPath)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"export", "--include-secrets"}, strings.NewReader(""), &stdout, &stderr)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	var got config.Config
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Hosts[0].Token != "secret" {
		t.Fatalf("token = %q", got.Hosts[0].Token)
	}
}

func TestRunUnknownCommandReturnsJSONError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"unknown"}, strings.NewReader(""), &stdout, &stderr)
	if code != apperr.ExitInvalidArgs {
		t.Fatalf("code = %d", code)
	}
	var got apperr.Error
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Code != apperr.CodeInvalidArgs {
		t.Fatalf("error code = %q", got.Code)
	}
}

func writeTestConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Config{
		Version: config.Version,
		Hosts: []config.Host{{
			Name:  "Main",
			URL:   "https://gitlab.example.com",
			Token: "secret",
		}},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
