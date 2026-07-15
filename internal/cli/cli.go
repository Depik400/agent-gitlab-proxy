package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"gitlab-proxy/internal/apperr"
	"gitlab-proxy/internal/config"
	"gitlab-proxy/internal/gitlab"
	"gitlab-proxy/internal/output"
	"gitlab-proxy/internal/review"
)

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		output.Usage(stderr, "usage: gitlab-proxy <bootstrap|config|import|export|comments|mr-context> [flags]")
		return apperr.ExitInvalidArgs
	}
	if err := run(args, stdin, stdout); err != nil {
		output.Error(stderr, err)
		return apperr.ExitCode(err)
	}
	return apperr.ExitOK
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	switch args[0] {
	case "bootstrap":
		return runBootstrap(args[1:], stdin, stdout)
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
	cfg = config.UpsertHost(cfg, host)
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	return output.JSON(stdout, map[string]any{"status": "ok", "host": config.Host{Name: host.Name, URL: host.URL}})
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
	if *hostName == "" || *repo == "" {
		return reviewOptions{}, apperr.New(apperr.CodeInvalidArgs, "--host-name and --repo are required", nil)
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
	host, err := config.FindHost(cfg, hostName)
	if err != nil {
		return nil, err
	}
	return gitlab.NewClient(host.URL, host.Token), nil
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
