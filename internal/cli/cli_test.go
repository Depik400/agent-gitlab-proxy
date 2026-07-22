package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Depik400/agent-gitlab-proxy/internal/apperr"
	"github.com/Depik400/agent-gitlab-proxy/internal/config"
)

func TestRunConfigMasksToken(t *testing.T) {
	configPath := writeTestConfig(t)
	t.Setenv(config.EnvKey, configPath)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"config"}, strings.NewReader(""), &stdout, &stderr, nil)
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
	code := Run([]string{"export", "--include-secrets"}, strings.NewReader(""), &stdout, &stderr, nil)
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
	code := Run([]string{"unknown"}, strings.NewReader(""), &stdout, &stderr, nil)
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

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, strings.NewReader(""), &stdout, &stderr, nil)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "create-mr") {
		t.Fatalf("help = %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRunHelpTopic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help", "comments"}, strings.NewReader(""), &stdout, &stderr, nil)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "gitlab-proxy comments") {
		t.Fatalf("help = %s", stdout.String())
	}
}

func TestRunCommandHelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"comments", "--help"}, strings.NewReader(""), &stdout, &stderr, nil)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--include-resolved") {
		t.Fatalf("help = %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRunAddMRCommentRequiresOneBodySource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"add-mr-comment", "--repo", "group/project", "--mr-iid", "1"}, strings.NewReader(""), &stdout, &stderr, nil)
	if code != apperr.ExitInvalidArgs {
		t.Fatalf("code = %d", code)
	}
	var got apperr.Error
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Message != "exactly one of --body or --body-file is required" {
		t.Fatalf("message = %q", got.Message)
	}
}

func TestRunAddMRCommentHelpDescribesMarkdownFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help", "add-mr-comment"}, strings.NewReader(""), &stdout, &stderr, nil)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--body-file") || !strings.Contains(stdout.String(), "Markdown") {
		t.Fatalf("help = %s", stdout.String())
	}
}

func TestRunAddMRThreadHelpDescribesInlineMarkdown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help", "add-mr-thread"}, strings.NewReader(""), &stdout, &stderr, nil)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "GitLab Flavored Markdown") || strings.Contains(stdout.String(), "--body-file") {
		t.Fatalf("help = %s", stdout.String())
	}
}

func TestRunReplyMRDiscussionHelpDescribesMarkdown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help", "reply-mr-discussion"}, strings.NewReader(""), &stdout, &stderr, nil)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--discussion-id") || !strings.Contains(stdout.String(), "GitLab Flavored Markdown") {
		t.Fatalf("help = %s", stdout.String())
	}
}

func TestRunUnknownHelpTopicReturnsJSONError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help", "missing"}, strings.NewReader(""), &stdout, &stderr, nil)
	if code != apperr.ExitInvalidArgs {
		t.Fatalf("code = %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %s", stdout.String())
	}
	var got apperr.Error
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Code != apperr.CodeInvalidArgs {
		t.Fatalf("error code = %q", got.Code)
	}
}

func TestRunInstallSkillWritesAllSkillsByDefault(t *testing.T) {
	target := t.TempDir()
	embedded := map[string]string{
		"gitlab-review-comments": "---\nname: gitlab-review-comments\ndescription: test\n---\n",
		"gitlab":                 "---\nname: gitlab\ndescription: test\n---\n",
		"gitlab-review-branch":   "---\nname: gitlab-review-branch\ndescription: test\n---\n",
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"install-skill", "--target-dir", target}, strings.NewReader(""), &stdout, &stderr, embedded)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	path := filepath.Join(target, "gitlab-review-comments", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "gitlab-review-comments") {
		t.Fatalf("skill = %s", string(data))
	}
	path = filepath.Join(target, "gitlab", "SKILL.md")
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: gitlab") {
		t.Fatalf("skill = %s", string(data))
	}
	path = filepath.Join(target, "gitlab-review-branch", "SKILL.md")
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: gitlab-review-branch") {
		t.Fatalf("skill = %s", string(data))
	}
}

func TestRunInstallSkillCanInstallSingleSkill(t *testing.T) {
	target := t.TempDir()
	embedded := map[string]string{
		"gitlab-review-comments": "---\nname: gitlab-review-comments\ndescription: test\n---\n",
		"gitlab":                 "---\nname: gitlab\ndescription: test\n---\n",
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"install-skill", "--target-dir", target, "--skill", "gitlab"}, strings.NewReader(""), &stdout, &stderr, embedded)
	if code != apperr.ExitOK {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(target, "gitlab", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "gitlab-review-comments", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("review skill stat error = %v, want not exist", err)
	}
}

func writeTestConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Config{
		Version:     config.Version,
		DefaultHost: "Main",
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
