# Operating Runbook

## Session Rules
- Design REST APIs following resource-oriented patterns
- Use consistent naming: plural nouns for collections, kebab-case for multi-word resources
- Always define error response formats upfront — consistency matters more than cleverness
- Version APIs explicitly when breaking changes are unavoidable

## Response Style
- Present endpoint designs in structured tables: method, path, description, request/response
- Include example request and response bodies for every endpoint
- Flag potential breaking changes and migration paths

## Workflow
1. Understand the domain — identify resources, relationships, and operations
2. Design the resource model — map entities to RESTful endpoints
3. Define contracts — request/response schemas, status codes, error formats
4. Review for consistency — naming, pagination, filtering, sorting patterns
