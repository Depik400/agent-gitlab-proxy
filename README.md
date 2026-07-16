# gitlab-proxy

`gitlab-proxy` is a JSON-first GitLab helper for AI coding agents. It fetches merge request review context, comments and diffs, and can create or reuse a follow-up merge request after an agent fixes review comments.

The repository also contains `SKILL.md`, a Codex skill instruction file for the workflow:

1. Detect the current Git branch and GitLab project.
2. Create a `${BranchName}-comments-fix` branch.
3. Fetch unresolved MR comments with `gitlab-proxy mr-context`.
4. Fix the comments.
5. Run checks, commit, push.
6. Open a merge request back to the original branch with `gitlab-proxy create-mr`.

## Requirements

- Go 1.26 or newer.
- Git.
- A GitLab personal access token.
- `gitlab-proxy` available in `PATH` for agent workflows.

For the full workflow, the GitLab token needs:

- `api` scope to read MR context and create merge requests.
- Project permissions to read the project, push branches, and create merge requests, usually Developer or higher.

For read-only commands such as `comments` and `mr-context`, `read_api` may be enough depending on your GitLab configuration.

## Install `gitlab-proxy`

From GitHub:

```bash
go install github.com/Depik400/agent-gitlab-proxy/cmd/gitlab-proxy@latest
```

From this repository checkout:

```bash
go install -buildvcs=false ./cmd/gitlab-proxy
```

The binary is installed into:

```bash
$(go env GOPATH)/bin/gitlab-proxy
```

Add Go binaries to `PATH` on Ubuntu/bash:

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

For zsh:

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Check installation:

```bash
which gitlab-proxy
gitlab-proxy help
```

If you need a custom binary name:

```bash
go build -buildvcs=false -o "$(go env GOPATH)/bin/codex-gitlab" ./cmd/gitlab-proxy
```

## Configure GitLab Access

Interactive setup:

```bash
gitlab-proxy bootstrap --interactive
```

Non-interactive setup:

```bash
gitlab-proxy bootstrap \
  --url https://gitlab.example.com \
  --token glpat-... \
  --name Main \
  --default
```

Rules:

- `--name` must contain only English letters, no spaces, max length 100.
- The token is verified with `GET /api/v4/user`.
- The first configured host becomes `default_host` automatically. Use `--default` or `set-default` to change it later.
- The config is stored in `~/.config/gitlab-proxy/config.json`.
- Override the config path with `GITLAB_PROXY_CONFIG=/path/to/config.json`.

Show configured hosts:

```bash
gitlab-proxy config
```

Tokens are not printed by `config` or normal `export`.

Set the default host explicitly:

```bash
gitlab-proxy set-default --host-name Main
```

## Common Commands

Fetch review context by source branch:

```bash
gitlab-proxy mr-context \
  --repo group/project \
  --branch feature/my-branch
```

Fetch review context by MR IID:

```bash
gitlab-proxy mr-context \
  --repo group/project \
  --mr-iid 123
```

Fetch only comments:

```bash
gitlab-proxy comments \
  --repo group/project \
  --branch feature/my-branch
```

Create or reuse a follow-up merge request:

```bash
gitlab-proxy create-mr \
  --repo group/project \
  --source-branch feature/my-branch-comments-fix \
  --target-branch feature/my-branch \
  --title "Fix review comments for feature/my-branch"
```

`create-mr` is idempotent for the same source and target branches. If an opened MR already exists, it returns that MR with `created: false`.

Add a general comment to a merge request:

```bash
gitlab-proxy add-mr-comment \
  --repo group/project \
  --mr-iid 123 \
  --body "Review comment text"
```

Open a review thread on a changed code line:

```bash
gitlab-proxy add-mr-thread \
  --repo group/project \
  --mr-iid 123 \
  --file internal/app.go \
  --new-line 42 \
  --body "Review comment text"
```

## Install the Codex Skill

Install all embedded skills from the binary:

```bash
gitlab-proxy install-skill
```

The command writes embedded skills under `~/.codex/skills`, including `gitlab-review-comments/SKILL.md`, `gitlab/SKILL.md`, and `gitlab-review-branch/SKILL.md`.

Install only one skill when needed:

```bash
gitlab-proxy install-skill --skill gitlab
```

Restart the Codex session so the skill is discovered.

Use it from a local project repository:

```text
Use the gitlab-review-comments skill to fix unresolved comments in the current GitLab MR.
```

Review a branch or MR without publishing comments immediately:

```text
Use the gitlab-review-branch skill to review this MR: https://gitlab.example.com/group/project/-/merge_requests/123
```

Before running the skill, verify:

```bash
which gitlab-proxy
gitlab-proxy config
git branch --show-current
git remote get-url origin
```

## Use With Other Agents

### Codex

Use the installation above. Codex reads `SKILL.md` automatically when the skill is installed under `~/.codex/skills/gitlab-review-comments`.

### Claude Code or Other CLI Agents

Most agents do not auto-load Codex skills. Give the agent the file explicitly:

```text
Read /path/to/gitlabProxy/SKILL.md and follow it to fix unresolved GitLab MR comments in this repository.
```

Make sure the agent's shell environment has:

```bash
which gitlab-proxy
gitlab-proxy config
```

### Generic Automation

Treat `gitlab-proxy` as a machine-readable command-line tool:

- Successful command output is JSON on stdout.
- Skill-oriented errors are JSON on stderr.
- Non-zero exit codes indicate the failure class.

Useful exit codes:

- `2`: invalid arguments
- `3`: config error
- `4`: auth error
- `5`: not found
- `6`: ambiguous MR
- `7`: GitLab API error

## Troubleshooting

### Proxy or Network Errors

If an error mentions `127.0.0.1:2080` or another proxy address, Go is using proxy environment variables:

```bash
env | grep -i proxy
```

Disable proxy for your GitLab host:

```bash
NO_PROXY=gitlab.example.com,no_proxy=gitlab.example.com gitlab-proxy config
```

Or unset proxy variables:

```bash
unset HTTPS_PROXY HTTP_PROXY ALL_PROXY https_proxy http_proxy all_proxy
```

### Double `/api/v4`

Configure `--url` as the GitLab root:

```bash
gitlab-proxy bootstrap --url https://gitlab.example.com --token glpat-... --name Main
```

The tool normalizes `https://gitlab.example.com/api/v4` to `https://gitlab.example.com`.

### Ambiguous MR

If branch lookup finds multiple opened merge requests, the command returns `ambiguous_mr` with candidates. Rerun with the chosen MR IID:

```bash
gitlab-proxy mr-context --host-name Main --repo group/project --mr-iid 123
```

## Development

Run tests:

```bash
GOCACHE=/tmp/gitlab-proxy-gocache go test ./...
```

Build:

```bash
GOCACHE=/tmp/gitlab-proxy-gocache go build -buildvcs=false -o /tmp/gitlab-proxy ./cmd/gitlab-proxy
```

`-buildvcs=false` is useful in environments where `.git` is unavailable or not a normal repository.
