---
name: commit
description: Create well-formatted conventional git commits. Use when the user wants to commit changes or asks for a commit message.
when-to-use: git commit, stage and commit, conventional commits
user-invocable: true
---

# Git commit skill

Review the working tree and create a clear conventional commit.

## Steps

1. Run `git status` and `git diff` (and `git diff --staged` if anything is staged)
2. Summarize what changed and why
3. Stage only relevant files (do not add secrets, `.env`, or build artifacts)
4. Create a conventional commit message: `type(scope): summary`
   - Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`
5. Run `git commit -m "..."` (use a HEREDOC if the body needs multiple lines)
6. Show the resulting `git log -1 --oneline`

## Rules

- Never commit secrets or credentials
- Prefer small, focused commits over dumping the entire workspace
- If nothing meaningful changed, say so instead of creating an empty commit
