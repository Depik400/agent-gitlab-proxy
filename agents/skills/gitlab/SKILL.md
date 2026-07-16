---
name: gitlab
description: "Use this skill when Codex needs to inspect GitLab merge requests, branches, comments, diffs, or create a merge request through gitlab-proxy without necessarily fixing review comments."
---

# GitLab

Use `gitlab-proxy` for GitLab API reads and merge request creation. Use `git` for local repository state, branches, commits, remotes, and pushes.

## Setup

1. Run `gitlab-proxy config`.
2. If `default_host` is configured, omit `--host-name` in GitLab commands unless the user asks for another host.
3. If no default host is configured, choose a configured host only when the repository remote clearly matches one host URL. Ask the user when it is ambiguous.
4. Determine the repository path from `git remote get-url origin` when the user did not provide `--repo`.

## Common Tasks

- Fetch merge request context by branch:

```bash
gitlab-proxy mr-context --repo group/project --branch feature/name
```

- Fetch merge request context by IID:

```bash
gitlab-proxy mr-context --repo group/project --mr-iid 123
```

- Fetch comments only:

```bash
gitlab-proxy comments --repo group/project --branch feature/name
```

- Create or reuse a merge request:

```bash
gitlab-proxy create-mr --repo group/project --source-branch feature/name --target-branch main --title "Merge feature/name"
```

Add `--host-name <name>` to these commands only when no default host is configured or when the user requests a specific host.

## Rules

- Preserve unrelated local changes.
- Treat command stdout as JSON and stderr as structured JSON errors.
- If GitLab returns `ambiguous_mr`, present the candidates and ask the user which MR IID to use.
- Do not expose access tokens from `gitlab-proxy export --include-secrets` unless the user explicitly asks.
