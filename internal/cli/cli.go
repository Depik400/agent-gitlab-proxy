package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/Depik400/agent-gitlab-proxy/internal/apperr"
	"github.com/Depik400/agent-gitlab-proxy/internal/config"
	"github.com/Depik400/agent-gitlab-proxy/internal/gitlab"
	"github.com/Depik400/agent-gitlab-proxy/internal/output"
	"github.com/Depik400/agent-gitlab-proxy/internal/review"
)

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, embeddedSkills map[string]string) int {
	if len(args) == 0 {
		writeHelp(stdout, "")
		return apperr.ExitOK
	}
	if err := run(args, stdin, stdout, embeddedSkills); err != nil {
		output.Error(stderr, err)
		return apperr.ExitCode(err)
	}
	return apperr.ExitOK
}

func run(args []string, stdin io.Reader, stdout io.Writer, embeddedSkills map[string]string) error {
	if args[0] == "--help" || args[0] == "-h" {
		writeHelp(stdout, "")
		return nil
	}
	if args[0] == "help" {
		topic := ""
		if len(args) > 2 {
			return apperr.New(apperr.CodeInvalidArgs, "usage: gitlab-proxy help [command]", map[string][]string{"args": args[1:]})
		}
		if len(args) == 2 {
			topic = args[1]
		}
		return writeHelp(stdout, topic)
	}
	if hasHelpFlag(args[1:]) {
		return writeHelp(stdout, args[0])
	}
	switch args[0] {
	case "bootstrap":
		return runBootstrap(args[1:], stdin, stdout)
	case "set-default":
		return runSetDefault(args[1:], stdout)
	case "config":
		return runConfig(args[1:], stdout)
	case "import":
		return runImport(args[1:])
	case "export":
		return runExport(args[1:], stdout)
	case "comments":
		return runComments(args[1:], stdout)
	case "mr-context":
		return runMRContext(args[1:], stdout)
	case "create-mr":
		return runCreateMR(args[1:], stdout)
	case "add-mr-comment":
		return runAddMRComment(args[1:], stdout)
	case "reply-mr-discussion":
		return runReplyMRDiscussion(args[1:], stdout)
	case "add-mr-thread":
		return runAddMRThread(args[1:], stdout)
	case "install-skill":
		return runInstallSkill(args[1:], stdout, embeddedSkills)
	default:
		return apperr.New(apperr.CodeInvalidArgs, "unknown command", map[string]string{"command": args[0]})
	}
}

func runBootstrap(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := newFlagSet("bootstrap")
	interactive := fs.Bool("interactive", false, "prompt for url, token and name")
	rawURL := fs.String("url", "", "GitLab URL")
	token := fs.String("token", "", "GitLab access token")
	name := fs.String("name", "", "host name")
	makeDefault := fs.Bool("default", false, "make this host default")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *interactive {
		values, err := promptBootstrap(stdin, stdout)
		if err != nil {
			return err
		}
		*rawURL = values.URL
		*token = values.Token
		*name = values.Name
	}
	if *rawURL == "" || *token == "" || *name == "" {
		return apperr.New(apperr.CodeInvalidArgs, "--url, --token and --name are required unless --interactive is set", nil)
	}
	normalizedURL, err := config.NormalizeURL(*rawURL)
	if err != nil {
		return err
	}
	host := config.Host{Name: *name, URL: normalizedURL, Token: *token}
	if err := config.ValidateHost(host); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := gitlab.NewClient(host.URL, host.Token).VerifyToken(ctx); err != nil {
		return err
	}
	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	cfg = config.UpsertHost(cfg, host, *makeDefault)
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	return output.JSON(stdout, map[string]any{"status": "ok", "host": config.Host{Name: host.Name, URL: host.URL}, "default_host": cfg.DefaultHost})
}

func runSetDefault(args []string, stdout io.Writer) error {
	fs := newFlagSet("set-default")
	hostName := fs.String("host-name", "", "configured host name")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *hostName == "" {
		return apperr.New(apperr.CodeInvalidArgs, "--host-name is required", nil)
	}
	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if _, err := config.FindHost(cfg, *hostName); err != nil {
		return err
	}
	cfg.DefaultHost = *hostName
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	return output.JSON(stdout, map[string]any{"status": "ok", "default_host": cfg.DefaultHost})
}

func runConfig(args []string, stdout io.Writer) error {
	fs := newFlagSet("config")
	if err := parse(fs, args); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	return output.JSON(stdout, config.Mask(cfg))
}

func runImport(args []string) error {
	fs := newFlagSet("import")
	path := fs.String("path", "", "config json path")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *path == "" {
		return apperr.New(apperr.CodeInvalidArgs, "--path is required", nil)
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "read import file", err, nil)
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return err
	}
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	return config.Save(cfgPath, cfg)
}

func runExport(args []string, stdout io.Writer) error {
	fs := newFlagSet("export")
	outputPath := fs.String("output-path", "", "write config json to path")
	includeSecrets := fs.Bool("include-secrets", false, "include tokens")
	if err := parse(fs, args); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if !*includeSecrets {
		cfg = config.Mask(cfg)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return apperr.Wrap(apperr.CodeConfig, "encode config", err, nil)
	}
	data = append(data, '\n')
	if *outputPath == "" {
		_, err = stdout.Write(data)
		if err != nil {
			return apperr.Wrap(apperr.CodeConfig, "write stdout", err, nil)
		}
		return nil
	}
	return os.WriteFile(*outputPath, data, 0o600)
}

func runComments(args []string, stdout io.Writer) error {
	opts, err := parseReviewFlags("comments", args)
	if err != nil {
		return err
	}
	client, err := clientForHost(opts.HostName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	comments, err := review.Comments(ctx, client, opts.Repo, opts.selector(), opts.IncludeResolved)
	if err != nil {
		return err
	}
	return output.JSON(stdout, comments)
}

func runMRContext(args []string, stdout io.Writer) error {
	opts, err := parseReviewFlags("mr-context", args)
	if err != nil {
		return err
	}
	client, err := clientForHost(opts.HostName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctxData, err := review.MRContext(ctx, client, opts.HostName, opts.Repo, opts.selector(), opts.IncludeResolved)
	if err != nil {
		return err
	}
	return output.JSON(stdout, ctxData)
}

func runCreateMR(args []string, stdout io.Writer) error {
	fs := newFlagSet("create-mr")
	hostName := fs.String("host-name", "", "configured host name")
	repo := fs.String("repo", "", "GitLab project path")
	sourceBranch := fs.String("source-branch", "", "source branch")
	targetBranch := fs.String("target-branch", "", "target branch")
	title := fs.String("title", "", "merge request title")
	description := fs.String("description", "", "merge request description")
	removeSourceBranch := fs.Bool("remove-source-branch", false, "remove source branch after merge")
	allowCollaboration := fs.Bool("allow-collaboration", false, "allow target project members to push to source branch")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *repo == "" || *sourceBranch == "" || *targetBranch == "" || *title == "" {
		return apperr.New(apperr.CodeInvalidArgs, "--repo, --source-branch, --target-branch and --title are required", nil)
	}
	client, err := clientForHost(*hostName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := review.CreateMergeRequest(ctx, client, *repo, review.CreateMergeRequestInput{
		SourceBranch:       *sourceBranch,
		TargetBranch:       *targetBranch,
		Title:              *title,
		Description:        *description,
		RemoveSourceBranch: *removeSourceBranch,
		AllowCollaboration: *allowCollaboration,
	})
	if err != nil {
		return err
	}
	return output.JSON(stdout, result)
}

func runAddMRComment(args []string, stdout io.Writer) error {
	fs := newFlagSet("add-mr-comment")
	hostName := fs.String("host-name", "", "configured host name")
	repo := fs.String("repo", "", "GitLab project path")
	branch := fs.String("branch", "", "source branch")
	mrIID := fs.String("mr-iid", "", "merge request IID")
	body := fs.String("body", "", "comment body (GitLab Flavored Markdown)")
	bodyFile := fs.String("body-file", "", "path to a Markdown file containing the comment body")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *repo == "" {
		return apperr.New(apperr.CodeInvalidArgs, "--repo is required", nil)
	}
	if (*branch == "" && *mrIID == "") || (*branch != "" && *mrIID != "") {
		return apperr.New(apperr.CodeInvalidArgs, "exactly one of --branch or --mr-iid is required", nil)
	}
	if (*body == "" && *bodyFile == "") || (*body != "" && *bodyFile != "") {
		return apperr.New(apperr.CodeInvalidArgs, "exactly one of --body or --body-file is required", nil)
	}
	if *bodyFile != "" {
		data, err := os.ReadFile(*bodyFile)
		if err != nil {
			return apperr.Wrap(apperr.CodeInvalidArgs, "read --body-file", err, map[string]string{"path": *bodyFile})
		}
		*body = string(data)
	}
	iid := 0
	if *mrIID != "" {
		parsed, err := review.ParseMRIID(*mrIID)
		if err != nil {
			return err
		}
		iid = parsed
	}
	client, err := clientForHost(*hostName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := review.AddMergeRequestComment(ctx, client, *repo, review.MRSelector{Branch: *branch, MRIID: iid}, *body)
	if err != nil {
		return err
	}
	return output.JSON(stdout, result)
}

func runReplyMRDiscussion(args []string, stdout io.Writer) error {
	fs := newFlagSet("reply-mr-discussion")
	hostName := fs.String("host-name", "", "configured host name")
	repo := fs.String("repo", "", "GitLab project path")
	branch := fs.String("branch", "", "source branch")
	mrIID := fs.String("mr-iid", "", "merge request IID")
	discussionID := fs.String("discussion-id", "", "GitLab discussion ID")
	body := fs.String("body", "", "reply body (GitLab Flavored Markdown)")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *repo == "" {
		return apperr.New(apperr.CodeInvalidArgs, "--repo is required", nil)
	}
	if (*branch == "" && *mrIID == "") || (*branch != "" && *mrIID != "") {
		return apperr.New(apperr.CodeInvalidArgs, "exactly one of --branch or --mr-iid is required", nil)
	}
	iid := 0
	if *mrIID != "" {
		parsed, err := review.ParseMRIID(*mrIID)
		if err != nil {
			return err
		}
		iid = parsed
	}
	client, err := clientForHost(*hostName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := review.ReplyToMergeRequestDiscussion(ctx, client, *repo, review.MRSelector{Branch: *branch, MRIID: iid}, *discussionID, *body)
	if err != nil {
		return err
	}
	return output.JSON(stdout, result)
}

func runAddMRThread(args []string, stdout io.Writer) error {
	fs := newFlagSet("add-mr-thread")
	hostName := fs.String("host-name", "", "configured host name")
	repo := fs.String("repo", "", "GitLab project path")
	branch := fs.String("branch", "", "source branch")
	mrIID := fs.String("mr-iid", "", "merge request IID")
	body := fs.String("body", "", "comment body (GitLab Flavored Markdown)")
	file := fs.String("file", "", "new file path in the diff")
	oldFile := fs.String("old-file", "", "old file path in the diff for renamed files")
	newLine := fs.Int("new-line", 0, "new line number in the diff")
	oldLine := fs.Int("old-line", 0, "old line number in the diff")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *repo == "" {
		return apperr.New(apperr.CodeInvalidArgs, "--repo is required", nil)
	}
	if (*branch == "" && *mrIID == "") || (*branch != "" && *mrIID != "") {
		return apperr.New(apperr.CodeInvalidArgs, "exactly one of --branch or --mr-iid is required", nil)
	}
	iid := 0
	if *mrIID != "" {
		parsed, err := review.ParseMRIID(*mrIID)
		if err != nil {
			return err
		}
		iid = parsed
	}
	client, err := clientForHost(*hostName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := review.AddMergeRequestThread(ctx, client, *repo, review.MRSelector{Branch: *branch, MRIID: iid}, review.AddMergeRequestThreadInput{
		Body:    *body,
		File:    *file,
		OldFile: *oldFile,
		NewLine: *newLine,
		OldLine: *oldLine,
	})
	if err != nil {
		return err
	}
	return output.JSON(stdout, result)
}

type reviewOptions struct {
	HostName        string
	Repo            string
	Branch          string
	MRIID           int
	IncludeResolved bool
}

func (o reviewOptions) selector() review.MRSelector {
	return review.MRSelector{Branch: o.Branch, MRIID: o.MRIID}
}

func parseReviewFlags(name string, args []string) (reviewOptions, error) {
	fs := newFlagSet(name)
	hostName := fs.String("host-name", "", "configured host name")
	repo := fs.String("repo", "", "GitLab project path")
	branch := fs.String("branch", "", "source branch")
	mrIID := fs.String("mr-iid", "", "merge request IID")
	includeResolved := fs.Bool("include-resolved", false, "include resolved comments")
	if err := parse(fs, args); err != nil {
		return reviewOptions{}, err
	}
	if *repo == "" {
		return reviewOptions{}, apperr.New(apperr.CodeInvalidArgs, "--repo is required", nil)
	}
	if (*branch == "" && *mrIID == "") || (*branch != "" && *mrIID != "") {
		return reviewOptions{}, apperr.New(apperr.CodeInvalidArgs, "exactly one of --branch or --mr-iid is required", nil)
	}
	iid := 0
	if *mrIID != "" {
		parsed, err := review.ParseMRIID(*mrIID)
		if err != nil {
			return reviewOptions{}, err
		}
		iid = parsed
	}
	return reviewOptions{HostName: *hostName, Repo: *repo, Branch: *branch, MRIID: iid, IncludeResolved: *includeResolved}, nil
}

func clientForHost(hostName string) (*gitlab.Client, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	host, err := config.ResolveHost(cfg, hostName)
	if err != nil {
		return nil, err
	}
	return gitlab.NewClient(host.URL, host.Token), nil
}

func runInstallSkill(args []string, stdout io.Writer, embeddedSkills map[string]string) error {
	fs := newFlagSet("install-skill")
	targetDir := fs.String("target-dir", "", "Codex skills directory")
	skillName := fs.String("skill", "", "install only this embedded skill")
	all := fs.Bool("all", false, "install all embedded skills")
	if err := parse(fs, args); err != nil {
		return err
	}
	base := *targetDir
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return apperr.Wrap(apperr.CodeConfig, "resolve home dir", err, nil)
		}
		base = filepath.Join(home, ".codex", "skills")
	}
	var names []string
	if *skillName != "" && !*all {
		names = []string{*skillName}
	} else {
		for name := range embeddedSkills {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	installed := make([]map[string]string, 0, len(names))
	for _, name := range names {
		content := embeddedSkills[name]
		if strings.TrimSpace(content) == "" {
			return apperr.New(apperr.CodeConfig, "embedded skill is empty or not found", map[string]string{"skill": name})
		}
		skillDir := filepath.Join(base, name)
		if err := os.MkdirAll(skillDir, 0o700); err != nil {
			return apperr.Wrap(apperr.CodeConfig, "create skill dir", err, map[string]string{"path": skillDir})
		}
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(content), 0o600); err != nil {
			return apperr.Wrap(apperr.CodeConfig, "write skill", err, map[string]string{"path": skillPath})
		}
		installed = append(installed, map[string]string{"name": name, "path": skillPath})
	}
	return output.JSON(stdout, map[string]any{
		"status":    "ok",
		"installed": installed,
		"recommendations": []string{
			"Run: gitlab-proxy config",
			"If no host is configured, run: gitlab-proxy bootstrap --interactive",
			"Use gitlab-proxy set-default --host-name <name> to choose the default host.",
		},
	})
}

func loadConfig() (config.Config, error) {
	path, err := config.DefaultPath()
	if err != nil {
		return config.Config{}, err
	}
	return config.Load(path)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return apperr.Wrap(apperr.CodeInvalidArgs, "parse flags", err, nil)
	}
	if fs.NArg() != 0 {
		return apperr.New(apperr.CodeInvalidArgs, "unexpected positional arguments", map[string][]string{"args": fs.Args()})
	}
	return nil
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func writeHelp(stdout io.Writer, topic string) error {
	if topic == "" {
		_, _ = fmt.Fprint(stdout, rootHelp)
		return nil
	}
	text, ok := commandHelp[topic]
	if !ok {
		return apperr.New(apperr.CodeInvalidArgs, "unknown help topic", map[string]string{"topic": topic})
	}
	_, _ = fmt.Fprint(stdout, text)
	return nil
}

const rootHelp = `gitlab-proxy is a JSON-first GitLab helper for Codex review workflows.

Usage:
  gitlab-proxy help [command]
  gitlab-proxy <command> --help
  gitlab-proxy <command> [flags]

Commands:
  bootstrap   Configure a GitLab host and verify the token.
  set-default Set default configured host.
  config      Print configured GitLab hosts without tokens.
  import      Replace local configuration from a JSON file.
  export      Export local configuration.
  comments    Print unresolved merge request comments as JSON.
  mr-context  Print merge request metadata, diffs and comments as JSON.
  create-mr   Create or reuse an opened merge request.
  add-mr-comment Add a general comment to a merge request.
  reply-mr-discussion Reply to an existing merge request discussion.
  add-mr-thread Add a code-position thread to a merge request.
  install-skill Install embedded Codex skills.

Examples:
  gitlab-proxy help mr-context
  gitlab-proxy config
`

var commandHelp = map[string]string{
	"bootstrap": `Usage:
  gitlab-proxy bootstrap --interactive
  gitlab-proxy bootstrap --url <url> --token <token> --name <name> [--default]

Configure a GitLab host. Host name must contain only English letters and be at most 100 characters.
The token is verified with GET /api/v4/user before saving.
The first configured host becomes default automatically.

Example:
  gitlab-proxy bootstrap --url https://gitlab.example.com --token glpat-... --name Main --default
`,
	"set-default": `Usage:
  gitlab-proxy set-default --host-name <name>

Set the default configured host used by commands when --host-name is omitted.

Example:
  gitlab-proxy set-default --host-name Main
`,
	"config": `Usage:
  gitlab-proxy config

Print configured GitLab hosts and default_host as JSON. Tokens are never printed.

Example:
  gitlab-proxy config
`,
	"import": `Usage:
  gitlab-proxy import --path <file>

Replace the current configuration with a validated JSON config file.

Example:
  gitlab-proxy import --path config.json
`,
	"export": `Usage:
  gitlab-proxy export [--output-path <file>] [--include-secrets]

Export the current configuration. Tokens are masked unless --include-secrets is passed.

Example:
  gitlab-proxy export --output-path config.json
`,
	"comments": `Usage:
  gitlab-proxy comments [--host-name <name>] --repo <project-path> (--branch <branch> | --mr-iid <iid>) [--include-resolved]

Print merge request comments as JSON. By default only unresolved resolvable comments are returned.
If --host-name is omitted, default_host from the config is used.

Example:
  gitlab-proxy comments --repo group/project --branch feature/review
`,
	"mr-context": `Usage:
  gitlab-proxy mr-context [--host-name <name>] --repo <project-path> (--branch <branch> | --mr-iid <iid>) [--include-resolved]

Print merge request metadata, diffs and comments as one JSON object.
If --host-name is omitted, default_host from the config is used.

Example:
  gitlab-proxy mr-context --repo group/project --branch feature/review
`,
	"create-mr": `Usage:
  gitlab-proxy create-mr [--host-name <name>] --repo <project-path> --source-branch <branch> --target-branch <branch> --title <title> [--description <text>] [--remove-source-branch] [--allow-collaboration]

Create an opened merge request or return an existing opened MR with the same source and target branches.
If --host-name is omitted, default_host from the config is used.

Example:
  gitlab-proxy create-mr --repo group/project --source-branch feature-comments-fix --target-branch feature --title "Fix review comments for feature"
`,
	"add-mr-comment": `Usage:
	  gitlab-proxy add-mr-comment [--host-name <name>] --repo <project-path> (--branch <branch> | --mr-iid <iid>) (--body <markdown> | --body-file <path>)

Add a general GitLab Flavored Markdown comment to a merge request. Pass inline Markdown with --body or read it from a Markdown file with --body-file. If --host-name is omitted, default_host from the config is used.

Example:
  gitlab-proxy add-mr-comment --repo group/project --mr-iid 123 --body "## Review\n\nPlease add a test."
  gitlab-proxy add-mr-comment --repo group/project --mr-iid 123 --body-file review-comment.md
`,
	"reply-mr-discussion": `Usage:
  gitlab-proxy reply-mr-discussion [--host-name <name>] --repo <project-path> (--branch <branch> | --mr-iid <iid>) --discussion-id <id> --body <markdown>

Reply to an existing merge request discussion with GitLab Flavored Markdown. Get the discussion ID from comments or mr-context. If --host-name is omitted, default_host from the config is used.

Example:
  gitlab-proxy reply-mr-discussion --repo group/project --mr-iid 123 --discussion-id abc123 --body "**Resolved:** fixed in 1a2b3c4."
`,
	"add-mr-thread": `Usage:
	  gitlab-proxy add-mr-thread [--host-name <name>] --repo <project-path> (--branch <branch> | --mr-iid <iid>) --file <path> (--new-line <n> | --old-line <n>) --body <markdown> [--old-file <path>]

Open a GitLab Flavored Markdown review thread on a specific changed code line. Use --new-line for added or unchanged new-side lines, --old-line for removed old-side lines, and both for unchanged lines when needed. Only inline Markdown through --body is supported.
If --host-name is omitted, default_host from the config is used.

Example:
  gitlab-proxy add-mr-thread --repo group/project --mr-iid 123 --file internal/app.go --new-line 42 --body "**Required:** add a test."
`,
	"install-skill": `Usage:
  gitlab-proxy install-skill [--target-dir <dir>] [--skill <name>]

Install all embedded Codex skills. Use --skill to install only one skill.

Examples:
  gitlab-proxy install-skill
  gitlab-proxy install-skill --skill gitlab
`,
}

type bootstrapInput struct {
	URL   string
	Token string
	Name  string
}

func promptBootstrap(stdin io.Reader, stdout io.Writer) (bootstrapInput, error) {
	reader := bufio.NewReader(stdin)
	urlValue, err := promptLine(reader, stdout, "GitLab URL: ")
	if err != nil {
		return bootstrapInput{}, err
	}
	tokenValue, err := promptToken(reader, stdout)
	if err != nil {
		return bootstrapInput{}, err
	}
	nameValue, err := promptLine(reader, stdout, "Host name: ")
	if err != nil {
		return bootstrapInput{}, err
	}
	return bootstrapInput{URL: urlValue, Token: tokenValue, Name: nameValue}, nil
}

func promptLine(reader *bufio.Reader, stdout io.Writer, prompt string) (string, error) {
	_, _ = fmt.Fprint(stdout, prompt)
	value, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", apperr.Wrap(apperr.CodeInvalidArgs, "read interactive input", err, nil)
	}
	return strings.TrimSpace(value), nil
}

func promptToken(reader *bufio.Reader, stdout io.Writer) (string, error) {
	_, _ = fmt.Fprint(stdout, "Access token: ")
	if !isTerminal(int(os.Stdin.Fd())) {
		return promptLine(reader, stdout, "")
	}
	oldState, err := getTermios(int(os.Stdin.Fd()))
	if err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidArgs, "read terminal state", err, nil)
	}
	newState := *oldState
	newState.Lflag &^= syscall.ECHO
	if err := setTermios(int(os.Stdin.Fd()), &newState); err != nil {
		return "", apperr.Wrap(apperr.CodeInvalidArgs, "disable terminal echo", err, nil)
	}
	defer func() {
		_ = setTermios(int(os.Stdin.Fd()), oldState)
		_, _ = fmt.Fprintln(stdout)
	}()
	value, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", apperr.Wrap(apperr.CodeInvalidArgs, "read token", err, nil)
	}
	return strings.TrimSpace(value), nil
}

func isTerminal(fd int) bool {
	_, err := getTermios(fd)
	return err == nil
}

func getTermios(fd int) (*syscall.Termios, error) {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return nil, errno
	}
	return &termios, nil
}

func setTermios(fd int, termios *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(termios)))
	if errno != 0 {
		return errno
	}
	return nil
}
