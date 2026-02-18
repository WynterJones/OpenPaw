# Tool Library Catalog Expansion

## Current Catalog (7 tools)

Already built: `weather`, `claude-code`, `github`, `gmail`, `slack`, `currency`, `google-sheets`

---

## Proposed Tools (37 new tools across 10 categories)

### Communication & Messaging

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 1 | `discord` | Discord | Send messages, manage channels, read history in Discord servers | `DISCORD_BOT_TOKEN` | Yes |
| 2 | `telegram` | Telegram | Send/receive messages, manage groups, send files via Telegram Bot API | `TELEGRAM_BOT_TOKEN` | Yes |
| 3 | `microsoft-teams` | Microsoft Teams | Post messages, create channels, manage team conversations | `TEAMS_WEBHOOK_URL` | Yes |
| 4 | `twilio` | Twilio | Send SMS/MMS, make voice calls, check message status | `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_PHONE_NUMBER` | Trial |
| 5 | `sendgrid` | SendGrid | Send transactional and marketing emails with templates | `SENDGRID_API_KEY` | Free tier |

### Developer & DevOps

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 6 | `gitlab` | GitLab | Manage repos, issues, merge requests, pipelines on GitLab | `GITLAB_TOKEN` | Yes |
| 7 | `jira` | Jira | Create/update issues, search with JQL, manage sprints and boards | `JIRA_URL`, `JIRA_EMAIL`, `JIRA_API_TOKEN` | Free tier |
| 8 | `linear` | Linear | Create/update issues, manage projects and cycles in Linear | `LINEAR_API_KEY` | Free tier |
| 9 | `sentry` | Sentry | Query errors, resolve issues, get crash analytics | `SENTRY_AUTH_TOKEN`, `SENTRY_ORG` | Free tier |
| 10 | `pagerduty` | PagerDuty | Create/acknowledge/resolve incidents, manage on-call schedules | `PAGERDUTY_API_KEY` | Trial |
| 11 | `vercel` | Vercel | Manage deployments, check build status, list projects | `VERCEL_TOKEN` | Free tier |
| 12 | `cloudflare` | Cloudflare | Manage DNS records, purge cache, check analytics | `CLOUDFLARE_API_TOKEN` | Free tier |
| 13 | `docker-hub` | Docker Hub | Search images, list tags, check vulnerability scans | `DOCKER_HUB_TOKEN` | Yes |

### Search & Knowledge

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 14 | `brave-search` | Brave Search | Web search, news search, image search via Brave Search API | `BRAVE_API_KEY` | Free tier |
| 15 | `serp-api` | Google Search | Search Google, Google Maps, Google News, Google Shopping | `SERPAPI_KEY` | Free tier |
| 16 | `wikipedia` | Wikipedia | Search articles, get summaries, retrieve page content | None | Yes |
| 17 | `wolfram-alpha` | Wolfram Alpha | Computational knowledge — math, science, data analysis, conversions | `WOLFRAM_APP_ID` | Free tier |
| 18 | `arxiv` | arXiv | Search and retrieve academic papers, abstracts, and citations | None | Yes |

### Productivity & Project Management

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 19 | `notion` | Notion | Create/update pages, query databases, manage workspaces | `NOTION_API_KEY` | Free tier |
| 20 | `todoist` | Todoist | Create/complete tasks, manage projects and labels | `TODOIST_API_TOKEN` | Free tier |
| 21 | `trello` | Trello | Create cards, manage boards and lists, add comments | `TRELLO_API_KEY`, `TRELLO_TOKEN` | Free tier |
| 22 | `airtable` | Airtable | CRUD on bases and tables, query records with formulas | `AIRTABLE_API_KEY` | Free tier |
| 23 | `google-calendar` | Google Calendar | Create/update events, check availability, manage calendars | `GOOGLE_CREDENTIALS` | Yes |
| 24 | `google-drive` | Google Drive | Upload/download files, search, manage permissions | `GOOGLE_CREDENTIALS` | Yes |

### AI & Machine Learning

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 25 | `openai` | OpenAI | Chat completions, image generation (DALL-E), embeddings, TTS | `OPENAI_API_KEY` | Paid |
| 26 | `replicate` | Replicate | Run open-source ML models — image gen, audio, video, LLMs | `REPLICATE_API_TOKEN` | Pay-per-use |
| 27 | `hugging-face` | Hugging Face | Run inference on 200k+ models — NLP, vision, audio | `HF_API_TOKEN` | Free tier |
| 28 | `stability-ai` | Stability AI | Generate and edit images with Stable Diffusion models | `STABILITY_API_KEY` | Free tier |
| 29 | `elevenlabs` | ElevenLabs | Text-to-speech with realistic AI voices, voice cloning | `ELEVENLABS_API_KEY` | Free tier |

### Finance & Payments

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 30 | `stripe` | Stripe | Create charges, manage customers, list invoices, handle subscriptions | `STRIPE_SECRET_KEY` | Test mode free |
| 31 | `stock-data` | Stock Market | Real-time and historical stock prices, company fundamentals | `ALPHA_VANTAGE_KEY` | Free tier |
| 32 | `crypto` | Cryptocurrency | Live crypto prices, market data, historical charts via CoinGecko | None | Yes |

### Content & Media

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 33 | `youtube` | YouTube | Search videos, get video details, list channel content, get transcripts | `YOUTUBE_API_KEY` | Free tier |
| 34 | `unsplash` | Unsplash | Search and download free high-resolution photos | `UNSPLASH_ACCESS_KEY` | Free tier |
| 35 | `rss-reader` | RSS Reader | Fetch, parse, and filter RSS/Atom feeds from any URL | None | Yes |

### Data & Analytics

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 36 | `news-api` | News | Search headlines and articles from 80,000+ news sources worldwide | `NEWS_API_KEY` | Free tier |
| 37 | `open-meteo-air` | Air Quality | Air quality index, pollutant levels, pollen forecasts worldwide | None | Yes |

### CRM & Marketing

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 38 | `hubspot` | HubSpot | Manage contacts, deals, companies, and tickets in HubSpot CRM | `HUBSPOT_API_KEY` | Free tier |
| 39 | `mailchimp` | Mailchimp | Manage audiences, create campaigns, check analytics | `MAILCHIMP_API_KEY` | Free tier |

### Infrastructure & Cloud

| # | Slug | Name | Description | Env Vars | Free? |
|---|------|------|-------------|----------|-------|
| 40 | `aws-s3` | AWS S3 | Upload, download, list, and manage objects in S3 buckets | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` | Free tier |
| 41 | `supabase` | Supabase | Query Postgres, manage auth users, interact with storage | `SUPABASE_URL`, `SUPABASE_KEY` | Free tier |
| 42 | `upstash-redis` | Upstash Redis | Key-value get/set, lists, pub/sub via serverless Redis | `UPSTASH_URL`, `UPSTASH_TOKEN` | Free tier |
| 43 | `resend` | Resend | Modern email API — send transactional emails with React templates | `RESEND_API_KEY` | Free tier |

---

## Priority Tiers

### Tier 1 — High Impact, Build First
Tools with broad appeal, simple APIs, and high utility for agents:

1. **brave-search** — Every agent needs web search. Free tier. Simple API.
2. **wikipedia** — No API key. Instant knowledge retrieval.
3. **notion** — Most popular workspace tool. Agents managing knowledge bases.
4. **discord** — Huge community. Agents in Discord servers.
5. **news-api** — Agents that stay current. Simple REST API.
6. **rss-reader** — No API key. Universal feed reading.
7. **crypto** — No API key (CoinGecko). Very popular data source.
8. **youtube** — Search and metadata. Generous free tier.
9. **openai** — Cross-model agent orchestration. Everyone has a key.
10. **jira** — Enterprise must-have. Huge user base.

### Tier 2 — Strong Demand
11. telegram
12. linear
13. todoist
14. stripe
15. stock-data
16. sendgrid
17. gitlab
18. sentry
19. wolfram-alpha
20. hugging-face

### Tier 3 — Niche but Valuable
21. twilio
22. elevenlabs
23. replicate
24. stability-ai
25. trello
26. airtable
27. google-calendar
28. google-drive
29. pagerduty
30. vercel

### Tier 4 — Specialized
31. cloudflare
32. docker-hub
33. unsplash
34. hubspot
35. mailchimp
36. microsoft-teams
37. aws-s3
38. supabase
39. upstash-redis
40. resend
41. arxiv
42. open-meteo-air

---

## Implementation Notes

Each catalog tool follows the existing pattern:
```
internal/toollibrary/catalog/{slug}/
  manifest.json.tmpl    — Endpoint definitions + env vars
  main.go.tmpl          — HTTP server boilerplate (templated)
  go.mod.tmpl           — Go module definition
  handlers.go           — HTTP handlers calling the API
  {slug}.go             — Core API client logic
  widget.js             — Optional dashboard widget
```

### Category Icons (Lucide)
| Category | Icon |
|----------|------|
| Communication | `message-circle` |
| Developer | `code` |
| Search | `search` |
| Productivity | `check-square` |
| AI | `brain` |
| Finance | `credit-card` |
| Content | `film` |
| Data | `bar-chart-3` |
| CRM | `users` |
| Infrastructure | `server` |

### No-Key Tools (Zero Friction)
These tools require no API key — prioritize for first-run experience:
- `wikipedia`
- `rss-reader`
- `crypto` (CoinGecko)
- `arxiv`
- `open-meteo-air`
