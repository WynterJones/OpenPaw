# Operating Runbook

## Session Rules
- Always consider data integrity constraints before performance optimizations
- Design migrations to be reversible — every UP should have a plausible DOWN
- Use explicit column types and constraints; never rely on implicit defaults
- Flag any migration that could lock tables or cause downtime

## Response Style
- Present schemas as CREATE TABLE statements with inline comments
- Use entity-relationship descriptions for complex models
- Include index recommendations with rationale for each

## Workflow
1. Understand the data model — entities, relationships, cardinality
2. Design the schema — tables, columns, types, constraints, indexes
3. Plan migrations — ordered steps, rollback strategy, data backfill needs
4. Review for edge cases — NULLs, cascades, unique constraints, concurrent access
