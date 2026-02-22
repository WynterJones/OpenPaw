---
description: Clean, stage, lint, test, build, commit, push, and update metrics
argument-hint: [optional: commit message override]
allowed-tools: Bash(find:*), Bash(git:*), Bash(npm:*), Bash(npx:*), Task
---

# Push Command

Run code cleanup, all quality gates, commit changes, and push to remote.

## Workflow

Execute these steps in order. **Stop immediately if any step fails.**

### Step 1: Clean Up System Files
Remove any .DS_Store files from the repository:
```bash
find . -name '.DS_Store' -type f -delete
```

### Step 2: Sync Packages
First check if package.json exists in the project root.

**If no package.json exists:**
- Output in gray text: "(skipped - no package.json)"
- Continue to Step 3

**If package.json exists:**
Clean and reinstall node_modules to ensure package-lock.json stays in sync:
```bash
rm -rf node_modules && npm install
```
This prevents `npm ci` failures in CI/CD due to lock file drift.

If package-lock.json was modified, it will be staged in the next step.

### Step 3: Stage All Changes
```bash
git add -A
```

### Step 4: Check for Changes
Run `git status` to verify there are staged changes. If nothing to commit, inform the user and stop.

### Step 5: Clean Code

Run the code-cleaner agent on staged TypeScript files to remove comments and console.logs.

This removes:
- Line comments (`//`) and block comments (`/* */`)
- `console.log` statements

It preserves:
- JSDoc comments (`/** */`)
- ESLint directives (`// eslint-disable`, `// eslint-disable-next-line`, etc.)
- TypeScript directives (`// @ts-ignore`, `// @ts-expect-error`, etc.)
- `console.error`, `console.warn`, `console.info`

After cleaning, re-stage the modified files:
```bash
git add -A
```

### Step 6: Run Quality Gates (in order)

First, read package.json to check which scripts exist. For each quality gate:

1. **Lint**:
   - Check if "lint" script exists in package.json
   - If exists: Run `npm run lint`
   - If not: Output in gray text "(skipped - no lint script)"

2. **Unit Tests**:
   - Check if "test" script exists in package.json
   - If exists: Run `npm run test`
   - If not: Output in gray text "(skipped - no test script)"

3. **Build**:
   - Check if "build" script exists in package.json
   - If exists: Run `npm run build`
   - If not: Output in gray text "(skipped - no build script)"

If any existing check fails, stop and report which check failed.

### Step 7: Generate Commit Message

If `$ARGUMENTS` is provided, use it as the commit message.

Otherwise, analyze the staged changes:
1. Run `git diff --cached --stat` to see changed files
2. Run `git diff --cached` to see actual changes
3. Run `git log -5 --oneline` to match the repository's commit style
4. Generate a concise, descriptive commit message that:
   - Starts with a type (feat, fix, refactor, docs, style, test, chore)
   - Summarizes the "why" not the "what"
   - Is 1-2 sentences maximum

### Step 8: Commit and Push

Create the commit with the message, including the standard footer:

```
🌽 Generated with FARMWORK
```

Then push to remote:
```bash
git push
```

### Step 9: Update Farmhouse Metrics

Run the-farmer agent to update `_AUDIT/FARMHOUSE.md` with current metrics:
- Commands and agents inventory
- Test counts (unit, e2e)
- Completed issues count

This keeps the harness documentation in sync with the codebase.

### Step 10: Report Success

Show a summary:
- Files changed
- Commit hash
- Push status
- Harness metrics updated
