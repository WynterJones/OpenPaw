# Test Coverage Audit

> Playwright E2E test coverage and gap analysis

**Last Updated:** 2026-02-21
**Score:** 10/10
**Status:** Comprehensive coverage -- all pages, API endpoints, CRUD flows, library pages, settings tabs, and mobile interactions tested
**Tests:** ~214 (13 spec files + 1 setup)
**Runtime:** ~150s (estimated)

---

## Current Coverage

### Setup & Auth (auth.setup.ts, auth.spec.ts) -- 9 tests, COMPREHENSIVE
| Test | Status |
|------|--------|
| Fresh DB setup wizard (4-step flow) | PASS |
| Storage state persists after setup | PASS |
| Redirect unauthenticated to /login | PASS |
| Login page renders all elements | PASS |
| Wrong credentials stay on login | PASS |
| Valid credentials redirect to /chat | PASS |
| Logout flow invalidates session | PASS |
| Password change and revert | PASS |
| Update password button disabled until all fields filled | PASS |

### Navigation (navigation.spec.ts) -- 4 tests, GOOD
| Test | Status |
|------|--------|
| Sidebar shows all 12 nav items | PASS |
| Navigate to all 12 routes without errors | PASS |
| Sidebar link clicks navigate correctly | PASS |
| Unknown routes redirect to /chat | PASS |

### Chat (chat.spec.ts) -- 10 tests, COMPREHENSIVE
| Test | Status |
|------|--------|
| Empty state when no chat selected | PASS |
| Create new chat thread | PASS |
| Type in message input | PASS |
| Send button disabled/enabled state | PASS |
| Send message appears in chat | PASS |
| Chat panel has search bar | PASS |
| Long message renders without overflow | PASS |
| Rename a chat thread | PASS |
| Delete a chat thread | PASS |

### Pages (pages.spec.ts) -- 21 tests, COMPREHENSIVE
| Test | Status |
|------|--------|
| Tools -- loads heading | PASS |
| Tools -- has search input | PASS |
| Tools -- search filters the list | PASS |
| Agents -- loads heading + Add Agent button | PASS |
| Secrets -- loads heading | PASS |
| Dashboards -- loads heading | PASS |
| Scheduler -- loads heading | PASS |
| Logs -- loads heading | PASS |
| Logs -- search filters log entries | PASS |
| Settings -- loads heading | PASS |
| Settings -- app name persists after change | PASS |
| Settings -- accent color persists after save | PASS |
| Skills -- loads heading | PASS |
| Context -- loads heading | PASS |
| Browser -- loads heading | PASS |
| Heartbeat -- loads heading | PASS |
| Heartbeat -- empty execution state | PASS |
| Notification bell -- empty state | PASS |
| Double-click new chat guard | PASS |
| No console errors on any page load | PASS |

### Context CRUD (context.spec.ts) -- 7 tests, COMPREHENSIVE
| Test | Status |
|------|--------|
| Create a folder | PASS |
| Upload a file | PASS |
| Select and view a file | PASS |
| Rename a file (inline edit) | PASS |
| Delete a file (modal confirm) | PASS |
| Delete a folder (native confirm) | PASS |
| About You editor persistence | PASS |

### CRUD (crud.spec.ts) -- 9 tests, SOLID
| Test | Status |
|------|--------|
| Secrets -- create a secret | PASS |
| Secrets -- delete a secret | PASS |
| Skills -- create a skill | PASS |
| Skills -- edit a skill (inline editor) | PASS |
| Skills -- delete a skill (native confirm) | PASS |
| Schedules -- create a schedule (AI prompt) | PASS |
| Schedules -- toggle a schedule | PASS |
| Schedules -- delete a schedule (no confirm) | PASS |

### Agent CRUD (agents-crud.spec.ts) -- 6 tests, SOLID
| Test | Status |
|------|--------|
| Create an agent via modal | PASS |
| Create button disabled with empty name | PASS |
| Navigate to agent edit page | PASS |
| Edit agent details and save | PASS |
| Toggle agent enabled state | PASS |
| Delete agent from edit page | PASS |

### API (api.spec.ts) -- 43 tests, COMPREHENSIVE
| Test | Status |
|------|--------|
| GET /setup/status | PASS |
| GET /auth/me | PASS |
| GET /chat/threads | PASS |
| POST /chat/threads (create) | PASS |
| GET /tools | PASS |
| GET /agent-roles | PASS |
| GET /system/info | PASS |
| GET /logs | PASS |
| GET /skills | PASS |
| GET /schedules | PASS |
| GET /secrets | PASS |
| GET /system/health | PASS |
| Unauthenticated -> 401 | PASS |
| POST /secrets (create) | PASS |
| DELETE /secrets/:id | PASS |
| POST /secrets empty body -> 400 | PASS |
| DELETE /secrets/nonexistent -> 404 | PASS |
| POST /skills (create) | PASS |
| GET /skills/:name (with content) | PASS |
| PUT /skills/:name (update) | PASS |
| DELETE /skills/:name | PASS |
| POST /skills empty name -> 400 | PASS |
| POST /schedules (create prompt) | PASS |
| POST /schedules/:id/toggle | PASS |
| DELETE /schedules/:id | PASS |
| POST /agent-roles (create) | PASS |
| POST /agent-roles/:slug/toggle | PASS |
| DELETE /agent-roles/:slug | PASS |
| PUT /chat/threads/:id (rename) | PASS |
| DELETE /chat/threads/:id | PASS |
| POST /auth/logout | PASS |
| GET /notifications | PASS |
| GET /notifications/count | PASS |
| PUT /notifications/read-all | PASS |
| DELETE /notifications (dismiss all) | PASS |
| GET /heartbeat/config | PASS |
| PUT /heartbeat/config | PASS |
| GET /heartbeat/history | PASS |
| POST /heartbeat/run-now | PASS |
| GET /context/about-you | PASS |
| PUT /context/about-you | PASS |
| POST + DELETE /context/folders | PASS |
| GET /context/tree | PASS |

### API Extended (api-extended.spec.ts) -- ~57 tests, NEW
| Test | Status |
|------|--------|
| **Tools CRUD** | |
| POST /tools (create) | NEW |
| GET /tools/{id} (get specific) | NEW |
| PUT /tools/{id} (update) | NEW |
| DELETE /tools/{id} (delete + verify 404) | NEW |
| GET /tools/nonexistent -> 404 | NEW |
| POST /tools/{id}/enable | NEW |
| POST /tools/{id}/disable | NEW |
| GET /tools/{id}/export | NEW |
| GET /tools/{id}/integrity | NEW |
| **Tool Library** | |
| GET /tool-library (catalog list) | NEW |
| GET /tool-library/{slug} | NEW |
| **Agent Library** | |
| GET /agent-library (catalog list) | NEW |
| GET /agent-library/{slug} | NEW |
| **Skill Library** | |
| GET /skill-library (catalog list) | NEW |
| GET /skill-library/{slug} | NEW |
| **Dashboards CRUD** | |
| GET /dashboards (list) | NEW |
| POST /dashboards (create, 201) | NEW |
| GET /dashboards/{id} | NEW |
| PUT /dashboards/{id} (update) | NEW |
| DELETE /dashboards/{id} (delete + verify 404) | NEW |
| POST /dashboards empty name -> 400 | NEW |
| **Settings** | |
| GET /settings (full) | NEW |
| PUT /settings (update + restore) | NEW |
| GET /settings/design (public) | NEW |
| PUT /settings/design | NEW |
| GET /settings/models | NEW |
| GET /settings/api-key | NEW |
| GET /settings/available-models | NEW |
| **System** | |
| GET /system/balance | NEW |
| GET /system/prerequisites (public) | NEW |
| **Logs** | |
| GET /logs/stats | NEW |
| GET /logs/tools/{id} | NEW |
| **Gateway Memories** | |
| GET /gateway/memories | NEW |
| GET /gateway/memories/stats | NEW |
| **Agent Roles Extended** | |
| GET /agent-roles/{slug} (single) | NEW |
| PUT /agent-roles/{slug} (update) | NEW |
| GET /agent-roles/{slug}/tools | NEW |
| GET /agent-roles/{slug}/skills | NEW |
| GET /agent-roles/{slug}/memories | NEW |
| GET /agent-roles/{slug}/memories/stats | NEW |
| GET /agent-roles/nonexistent -> 404 | NEW |
| GET /agent-roles/{slug}/memory | NEW |
| GET /agent-roles/{slug}/files | NEW |
| GET /agent-roles/gateway/files | NEW |
| GET /agent-roles/gateway/memory | NEW |
| **Chat Extended** | |
| GET /chat/threads/active | NEW |
| GET /chat/threads/{id}/messages | NEW |
| GET /chat/threads/{id}/status | NEW |
| GET /chat/threads/{id}/stats | NEW |
| GET /chat/threads/{id}/members | NEW |
| GET /chat/threads/nonexistent/messages | NEW |
| **Auth Extended** | |
| PUT /auth/profile (display name) | NEW |
| POST /auth/change-password (wrong password) | NEW |
| **Agents** | |
| GET /agents (list) | NEW |
| **Browser** | |
| GET /browser/sessions (list) | NEW |
| GET /browser/tasks (list) | NEW |
| **Schedules Extended** | |
| GET /schedules/{id}/executions | NEW |
| PUT /schedules/{id} (update) | NEW |
| **Secrets Extended** | |
| POST /secrets/{id}/rotate | NEW |
| **Context Extended** | |
| GET /context/files (list) | NEW |
| PUT /context/folders/{id} (rename) | NEW |
| **Unauthenticated Access** | |
| GET /tools without auth -> 401 | NEW |
| GET /dashboards without auth -> 401 | NEW |
| GET /settings without auth -> 401 | NEW |
| GET /agents without auth -> 401 | NEW |
| POST /tools without auth -> 401 | NEW |
| Public endpoints work without auth | NEW |
| **Notifications Extended** | |
| PUT /notifications/{id}/read (single) | NEW |

### Library Pages (library-pages.spec.ts) -- ~20 tests, NEW
| Test | Status |
|------|--------|
| **Tool Library** | |
| Tool Library -- loads heading | NEW |
| Tool Library -- displays catalog items | NEW |
| Tool Library -- category filter narrows results | NEW |
| Tool Library -- has cross-links to other libraries | NEW |
| **Agent Library** | |
| Agent Library -- loads heading | NEW |
| Agent Library -- displays catalog items | NEW |
| Agent Library -- displays model badges | NEW |
| Agent Library -- category filter narrows results | NEW |
| Agent Library -- has cross-links | NEW |
| **Skill Library** | |
| Skill Library -- loads heading | NEW |
| Skill Library -- displays catalog items | NEW |
| Skill Library -- displays "Uses tools" badges | NEW |
| Skill Library -- displays required tools info | NEW |
| Skill Library -- category filter narrows results | NEW |
| Skill Library -- has cross-links | NEW |
| **Cross-navigation** | |
| Navigate Tool Library -> Agent Library | NEW |
| Navigate Agent Library -> Skill Library | NEW |
| Navigate Skill Library -> Tool Library | NEW |
| **Dashboards** | |
| Dashboards -- loads heading | NEW |
| Dashboards -- shows empty state or list | NEW |
| **Console errors** | |
| No console errors on library pages | NEW |

### Settings Extended (settings-extended.spec.ts) -- ~26 tests, NEW
| Test | Status |
|------|--------|
| **Design tab** | |
| Accent color picker with preset swatches | NEW |
| Background image section with presets | NEW |
| Font picker section | NEW |
| Save Design and Reset buttons | NEW |
| **AI Models tab** | |
| API key status section | NEW |
| Gateway and builder model pickers | NEW |
| Agent max turns and timeout settings | NEW |
| **About tab** | |
| Version and branding info | NEW |
| **Security tab** | |
| Session timeout and IP allowlist controls | NEW |
| **System tab** | |
| System information grid | NEW |
| Data management buttons | NEW |
| **Notifications tab** | |
| Notification sound toggle | NEW |
| **Network tab** | |
| Local network and Tailscale sections | NEW |
| **Danger tab** | |
| Warning and danger actions visible | NEW |
| Delete data modal requires typing DELETE | NEW |
| **Profile tab** | |
| Account info and username field | NEW |
| Update Username button disabled when unchanged | NEW |
| Password change section shows fields | NEW |
| Password mismatch shows error toast | NEW |
| **Tab navigation** | |
| All tabs activate without console errors | NEW |
| **Agent edit** | |
| Gateway agent edit page loads | NEW |
| Gateway agent has system prompt or identity | NEW |
| Gateway agent has tools/skills tabs | NEW |

### Mobile (mobile.spec.ts) -- 5 tests, COMPREHENSIVE
| Test | Status |
|------|--------|
| Sidebar hidden on mobile viewport | PASS |
| Pages load without horizontal overflow | PASS |
| Chat usable on mobile (New Chat + textarea) | PASS |
| Bottom nav visible on mobile | PASS |
| More menu opens and navigates | PASS |

---

## Intentionally Not Tested

| Item | Reason |
|------|--------|
| WebSocket connection indicator | No visible indicator exists in the UI |
| Dark/light theme toggle | App is dark-only, no toggle exists |
| Session expiry redirect | Requires JWT time manipulation, fragile |
| Browser headless sessions | Requires external browser runtime |
| Full AI chat response loop | Requires mocking Claude CLI |

---

## Constraints

| Constraint | Reason | Impact |
|------------|--------|--------|
| No unit tests | Frontend has no vitest/jest setup | All testing is E2E only |
| Serial execution | `workers: 1` in config | ~150s runtime |
| No AI response tests | Would require mocking Claude CLI | Can't test full chat loop |
| Browser sessions | Requires headless browser runtime | Can't fully E2E test browser page |

---

## Test File Inventory

| File | Tests | Category |
|------|-------|----------|
| `auth.setup.ts` | 4 | Setup wizard |
| `auth.spec.ts` | 5 | Auth flows |
| `navigation.spec.ts` | 4 | Sidebar nav |
| `chat.spec.ts` | 10 | Chat CRUD |
| `pages.spec.ts` | 21 | Page load + features |
| `context.spec.ts` | 7 | Context CRUD |
| `crud.spec.ts` | 9 | Secrets/Skills/Schedules CRUD |
| `agents-crud.spec.ts` | 6 | Agent lifecycle |
| `api.spec.ts` | 43 | API endpoint tests |
| `api-extended.spec.ts` | ~57 | Extended API coverage |
| `library-pages.spec.ts` | ~20 | Library page UI |
| `settings-extended.spec.ts` | ~26 | Settings tabs + agent edit |
| `mobile.spec.ts` | 5 | Mobile responsive |

---

## Audit History

| Date | Changes |
|------|---------|
| 2026-02-18 | Initial test coverage audit setup via Farmwork CLI |
| 2026-02-20 | Fixed all 45 tests (was 0 passing). Fixed api.ts 401 redirect bug. Updated all selectors for current UI. Full gap analysis written. |
| 2026-02-20 | Major expansion: 45 -> 84 tests. Added logout flow, console error interception, settings persistence, search/filter tests. Added chat rename/delete. Created crud.spec.ts (secrets/skills/schedules CRUD). Created agents-crud.spec.ts (full agent lifecycle). Expanded api.spec.ts with 17 POST/PUT/DELETE tests + validation. Score 5.5 -> 8/10. |
| 2026-02-20 | Final push: 84 -> 111 tests. Created context.spec.ts (7 file/folder CRUD tests). Added password change + accent color persistence. Added 13 API tests (notifications, heartbeat, context). Added mobile More menu, empty states, long content, rapid action tests. Score 8 -> 10/10. |
| 2026-02-21 | Major expansion: 111 -> ~214 tests. Created api-extended.spec.ts (57 tests covering tools CRUD, all 3 library catalogs, dashboards CRUD, settings endpoints, system, logs stats, gateway memories, agent role details/tools/skills/memories/files, chat extended, auth profile, agents list, browser sessions, schedules extended, secrets rotate, context extended, unauthenticated access control). Created library-pages.spec.ts (20 tests for tool/agent/skill library page UI, category filtering, cross-navigation, dashboards). Created settings-extended.spec.ts (26 tests for all 10 settings tabs: Profile, General, Notifications, Network, AI Models, Design, Security, System, About, Danger + agent edit page). |
