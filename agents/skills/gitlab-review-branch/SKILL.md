---
name: gitlab-review-branch
description: "Use this skill when Codex must review a GitLab merge request or branch: accept an MR URL, MR IID, or branch name, check out the source branch, inspect project Markdown rules and code changes, produce numbered review comments for the user to choose from, and publish only the selected comments back to the merge request."
---

# GitLab Review Branch

Use this skill to review a GitLab MR or branch in two phases:

1. Draft review comments with code positions and show them to the user as a numbered list.
2. After the user replies with comment numbers, publish only those selected comments to the MR: use review threads for comments with a safe code position and general MR comments for the rest.

Do not publish comments during phase 1.

## Inputs

The user may provide:

- A GitLab MR URL.
- A project path plus MR IID.
- A branch name.

If the repository or MR cannot be inferred safely, ask for the missing `--repo`, branch, MR IID, or host.

## Phase 1: Prepare Review

1. Inspect the local repository:
   - Run `git status --short` and preserve unrelated user changes.
   - Run `git remote get-url origin`.
   - Parse the GitLab project path from the remote or MR URL.
2. Inspect project guidance:
   - Find Markdown files with `rg --files -g '*.md'`.
   - Read project-level guidance before reviewing code, prioritizing files such as `README.md`, `CONTRIBUTING.md`, `AGENTS.md`, `CODE_REVIEW.md`, architecture docs, and docs near changed files.
   - Treat these files as project-specific review rules when they apply.
3. Resolve the target:
   - For an MR URL or IID, run `gitlab-proxy mr-context --repo <repo> --mr-iid <iid>`.
   - For a branch, run `gitlab-proxy mr-context --repo <repo> --branch <branch>`.
   - If no `default_host` is configured, add `--host-name <host>`.
   - If GitLab returns `ambiguous_mr`, show candidates and ask the user which MR IID to use.
4. Check out the source branch:
   - Use the MR `source_branch` when reviewing an MR.
   - Use the provided branch when reviewing a branch.
   - Fetch the branch if needed.
   - Do not discard local changes; ask before any operation that would overwrite work.
5. Analyze the change:
   - Compare source branch to the MR target branch when available.
   - Review changed code against project Markdown guidance, correctness, tests, security, performance, maintainability, and user-visible behavior.
   - Prefer concrete, actionable comments tied to files or code regions.
6. Return only a numbered list of proposed comments:
   - Include severity, file path, target line, and the exact comment text that would be posted.
   - Prefer a `new_line` for comments on added or unchanged new-side lines.
   - Use `old_line` for comments on removed old-side lines.
   - Keep enough metadata internally to publish the selected comment later: file path, optional old file path for renames, old line, new line, and body.
   - Keep each item independently selectable.
   - Do not post anything yet.

## Phase 2: Publish Selected Comments

When the user replies with numbers:

1. Map the numbers to the exact proposed comments from phase 1.
2. If any number is unclear or out of range, ask for clarification.
3. Publish each selected item with a safe code position as a separate code-position review thread:

```bash
gitlab-proxy add-mr-thread --repo <repo> --mr-iid <iid> --file <path> --new-line <line> --body "**Severity:** <level>\n\n<comment text>"
```

Format `--body` as inline GitLab Flavored Markdown. For removed lines, use `--old-line <line>`. For renamed files, include `--old-file <old-path>` when it differs from `--file`. `add-mr-thread` does not support `--body-file`.

Add `--host-name <host>` only when no default host is configured.

4. Publish every selected comment that cannot be safely tied to a changed line as a general MR comment:

```bash
gitlab-proxy add-mr-comment --repo <repo> --mr-iid <iid> --body "## Review\n\n<comment text>"
```

Format the body as GitLab Flavored Markdown. For multi-line content, write a `.md` file and use `--body-file <path>`. Do not drop a selected comment merely because it has no code position. Add `--host-name <host>` only when no default host is configured.

5. Report which selected comment numbers were posted and include the MR URL when available.

## Reply to Existing Discussions

When the user asks to answer an existing review discussion, use its `discussion_id` from `mr-context` or `comments`:

```bash
gitlab-proxy reply-mr-discussion --repo <repo> --mr-iid <iid> --discussion-id <discussion-id> --body "**Resolved:** <reply text>"
```

Do not create a general MR comment when the user asked for a reply to a specific discussion.

When the user asks to revise or remove a previously posted reply, use its `note_id` and `discussion_id`:

```bash
gitlab-proxy edit-mr-comment --repo <repo> --mr-iid <iid> --discussion-id <discussion-id> --note-id <note-id> --body "**Resolved:** <updated reply>"
gitlab-proxy delete-mr-comment --repo <repo> --mr-iid <iid> --discussion-id <discussion-id> --note-id <note-id>
```

Only omit `--discussion-id` when editing or deleting a general MR comment.

## Rules

- Never publish unselected comments.
- Never publish comments before showing the numbered list and receiving the user's selected numbers.
- Prefer code-position threads over general MR comments when a safe changed-line position is available.
- Format every text value sent to GitLab as GitLab Flavored Markdown, including thread bodies and discussion replies.
- Preserve unrelated local changes.
- Treat `gitlab-proxy` stdout as JSON and stderr as structured JSON errors.
- Do not expose tokens from `gitlab-proxy export --include-secrets`.
