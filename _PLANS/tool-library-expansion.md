# Tool Library Expansion Backlog

Scope: add new catalog tools under `internal/toollibrary/catalog/` and register them in `internal/toollibrary/catalog/registry.json`.

## Rollout Rules
- Keep each tool as a standalone Go HTTP server template set: `main.go.tmpl`, `handlers.go`, `{slug}.go`, `manifest.json.tmpl`, `go.mod.tmpl`, optional `widget.js`.
- Only add registry entries when corresponding catalog folder exists.
- For API-key tools, expose required env vars in registry and manifest.
- Pass `go test ./internal/toollibrary` after each batch.

## Phase 1: Search & Knowledge (Foundation)
- [x] wikipedia
- [x] news-api
- [x] arxiv
- [x] openlibrary
- [x] brave-search
- [x] bing-search
- [x] semantic-scholar
- [x] crossref
- [x] openalex

## Phase 2: Developer & DevOps
- [x] gitlab
- [ ] bitbucket
- [ ] azure-devops
- [ ] circleci
- [ ] buildkite
- [ ] jenkins
- [x] vercel
- [x] netlify
- [ ] cloudflare
- [ ] snyk
- [ ] dependabot
- [x] docker-hub
- [ ] terraform-cloud

## Phase 3: Productivity & Project Management
- [ ] jira
- [ ] asana
- [ ] trello
- [ ] monday
- [ ] clickup
- [ ] todoist
- [ ] basecamp
- [ ] wrike

## Phase 4: Docs, Files, and Storage
- [x] google-drive
- [ ] onedrive
- [x] dropbox
- [ ] box
- [ ] sharepoint
- [ ] confluence
- [ ] coda
- [x] airtable

## Phase 5: Communication
- [ ] microsoft-teams
- [ ] telegram
- [ ] twilio
- [ ] whatsapp
- [ ] zoom
- [ ] google-calendar
- [ ] outlook

## Phase 6: CRM & Customer Support
- [ ] salesforce
- [ ] hubspot
- [ ] pipedrive
- [ ] zendesk
- [ ] freshdesk
- [ ] intercom

## Phase 7: Marketing & Email
- [ ] sendgrid
- [ ] mailchimp
- [ ] resend
- [ ] klaviyo
- [ ] postmark

## Phase 8: Finance & Payments
- [x] paypal
- [x] square
- [x] plaid
- [x] coinbase
- [x] alpha-vantage
- [x] polygon

## Phase 9: Data, Analytics, and BI
- [ ] mixpanel
- [ ] amplitude
- [ ] ga4
- [ ] posthog
- [ ] segment
- [ ] metabase

## Phase 10: Cloud & Infrastructure
- [ ] aws-s3
- [ ] aws-cloudwatch
- [ ] gcp-storage
- [ ] azure-blob
- [ ] supabase
- [ ] upstash-redis
- [ ] redis

## Phase 11: HR & Recruiting
- [ ] greenhouse
- [ ] lever
- [ ] workday
- [ ] bamboohr

## Phase 12: Security & Identity
- [ ] okta
- [ ] auth0
- [ ] 1password
- [ ] vault

## Phase 13: Geo & Maps
- [ ] google-maps
- [ ] mapbox
- [ ] openstreetmap

## Phase 14: Media
- [ ] giphy
- [ ] pexels
- [ ] vimeo
- [ ] twitch

## Phase 15: Automation
- [ ] zapier
- [ ] make
- [ ] n8n

## Definition of Done Per Tool
- [ ] Tool appears in `registry.json` with accurate `env` list and tags.
- [ ] Tool installs successfully from library UI/API.
- [ ] Binary starts and `/health` returns `200`.
- [ ] At least two functional API endpoints (or one if API only supports one core workflow).
- [ ] Manifest endpoint docs match actual handlers.
- [ ] Tool returns clear error messages for missing config and upstream API failures.
