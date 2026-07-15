---
name: gitlab-review-comments
description: "Use this skill when Codex must address GitLab merge request review comments in a local repository: discover the GitLab project and current branch, fetch MR comments/context with gitlab-proxy, create a comments-fix branch, edit code, run checks, push the branch, and open a GitLab merge request back to the original branch."
---

# GitLab Review Comments

Use `gitlab-proxy` for GitLab API operations and `git` for local branch, commit, and push operations.

## Workflow

1. Confirm the local repository:
   - Run `git rev-parse --show-toplevel`.
   - Run `git status --short` and preserve unrelated user changes.
2. Determine the source branch:
   - Run `git branch --show-current`.
   - If the branch is empty or detached, ask the user which branch to use.
3. Determine the GitLab project path:
   - Run `git remote get-url origin`.
   - Parse GitLab SSH/HTTPS remotes into `group/project` without `.git`.
   - If parsing is ambiguous, ask the user for `--repo`.
4. Determine `--host-name`:
   - Run `gitlab-proxy config`.
   - Match the remote host against configured host URLs.
   - If zero or multiple hosts match, ask the user which configured host to use.
5. Create the fix branch from the original branch:
   - Use `git checkout -b "${BranchName}-comments-fix"`.
   - If the branch already exists, inspect it before reusing; do not overwrite user work.
6. Fetch review context:
   - Run `gitlab-proxy mr-context --host-name <host> --repo <repo> --branch <BranchName>`.
   - If GitLab returns `ambiguous_mr`, show the candidates and ask the user which MR IID to use, then rerun with `--mr-iid`.
7. Fix only the unresolved comments unless the user asks for broader changes.
   - Use `comments[].file_path`, `old_line`, `new_line`, `body`, and `suggestions`.
   - Keep changes scoped to the review feedback.
8. Run the repository's relevant tests, linters, and formatters.
   - If checks cannot run, report the exact blocker.
9. Commit and push:
   - Commit only intended changes.
   - Run `git push -u origin "${BranchName}-comments-fix"`.
10. Open or reuse the follow-up MR:
   - Run `gitlab-proxy create-mr --host-name <host> --repo <repo> --source-branch "${BranchName}-comments-fix" --target-branch "<BranchName>" --title "Fix review comments for <BranchName>" --description "<summary>"`.
   - Report the returned `merge_request.web_url`.

## Rules

- Do not resolve GitLab discussions automatically.
- Do not create duplicate MRs; `create-mr` is idempotent for the same source and target branches.
- Ask the user before choosing among ambiguous repositories, hosts, branches, or MR candidates.
- Treat `gitlab-proxy` stdout as JSON. Treat JSON on stderr as structured failure details.
- Do not expose tokens from `gitlab-proxy export --include-secrets` unless the user explicitly asks.

## Useful Commands

```bash
gitlab-proxy help
gitlab-proxy help mr-context
gitlab-proxy config
gitlab-proxy mr-context --host-name Main --repo group/project --branch feature
gitlab-proxy create-mr --host-name Main --repo group/project --source-branch feature-comments-fix --target-branch feature --title "Fix review comments for feature"
```
