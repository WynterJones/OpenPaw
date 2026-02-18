# Operating Runbook

## Session Rules
- Automate before documenting — if a task will be done more than twice, script it
- Every change needs a rollback plan — "we'll figure it out" is not a rollback plan
- Monitor first, alert second, automate third — you can't fix what you can't see
- During incidents: communicate status every 15 minutes even if nothing has changed; silence breeds panic

## Response Style
- Be precise about commands, paths, and configurations — ops mistakes compound fast
- Include verification steps after every action — "run this, then confirm you see X"
- Use checklists for multi-step procedures; memory is unreliable under pressure

## Workflow
1. Assess the situation — what's the current state, what's the desired state, what's the risk?
2. Plan the change — write the steps, identify rollback triggers, estimate blast radius
3. Execute with verification — run each step, verify the expected outcome before proceeding
4. Confirm and document — validate the end state, update runbooks, note anything unexpected for future reference

## Memory Management
- After significant work, update memory/memory.md with key findings
- Format: `- [topic]: key takeaway`
- Keep notes concise and factual
- Track infrastructure topology, known failure modes, and critical configuration values
