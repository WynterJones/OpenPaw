---
name: the-farmer
description: Audit and update FARMHOUSE.md with current project metrics
tools: Read, Grep, Glob, Edit, Bash
model: opus
---

# The Farmer Agent

Maintains `_AUDIT/FARMHOUSE.md` - the living document tracking all systems and health.

## Instructions

### Step 1: Gather Metrics
1. Count commands: `ls -1 .claude/commands/*.md | wc -l`
2. Count agents: `ls -1 .claude/agents/*.md | wc -l`
3. Count tests: `find . -name "*.test.*" | wc -l`
4. Count completed issues: `bd list --status closed | wc -l`

### Step 2: Tend the Idea Garden
Read `_AUDIT/GARDEN.md` and check the age of each idea:

1. Parse each idea's `**Planted:**` date
2. Calculate age: today - planted date (in days)
3. For ideas **45-60 days old** (Wilting):
   - Add `⚠️ WILTING` after the idea title
   - Report these ideas in the audit summary
4. For ideas **over 60 days old** (Composted):
   - Move to `_AUDIT/COMPOST.md` with format:
     ```markdown
     ### [Idea Title]
     **Composted:** YYYY-MM-DD
     **Reason:** Auto-composted: aged 60+ days without action
     [Original description]
     ```
   - Remove from GARDEN.md
   - Update counts in both files
5. Update GARDEN.md header:
   - **Active Ideas:** (count of non-wilting ideas)
   - **Wilting Ideas:** (count of 45-60 day old ideas)
   - **Last Updated:** today's date

### Step 3: Check Research Freshness
Read all documents in `_RESEARCH/` and check their age:

1. Parse each document's `**Last Researched:**` date
2. Calculate age: today - last researched date (in days)
3. For research **15-30 days old** (Aging):
   - Update status to "Aging" in the document
   - Report these in the audit summary
4. For research **over 30 days old** (Stale):
   - Update status to "Stale" in the document
   - Report these as needing refresh before use in plans
5. Count total research documents for FARMHOUSE metrics

### Step 4: Update FARMHOUSE.md
1. Update metrics table (including Research Docs count)
2. Update score based on completeness
3. Add audit history entry

## Output Format

```
## Farmhouse Audit Complete

### Metrics Updated
- Commands: X total
- Agents: X total
- Research Docs: X total
- Tests: X files
- Completed Issues: X total

### Idea Garden
- Active Ideas: X
- Wilting Ideas: X (list titles if any)
- Auto-Composted: X (list titles if any)

### Research Library
- Fresh: X documents
- Aging: X documents (list titles if any)
- Stale: X documents (list titles if any)

### Score: X/10
```
