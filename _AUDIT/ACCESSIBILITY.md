# Accessibility Audit

> WCAG 2.1 Level AA compliance audit of `web/frontend/src/`

**Last Updated:** 2026-02-21
**Score:** 10 / 10
**Status:** All 65 issue types fixed across 88+ instances
**Auditor:** Manual code review of entire frontend

---

## Scope of This Audit

This is a comprehensive audit covering every page and component in the frontend.

### Pages (16 total)

| Page | File | Role |
|------|------|------|
| Chat | `pages/Chat.tsx` | Primary user interface |
| Agents | `pages/Agents.tsx` | Agent listing |
| AgentEdit | `pages/AgentEdit.tsx` | Agent configuration |
| Settings | `pages/Settings.tsx` | Application settings |
| Login | `pages/Login.tsx` | Authentication |
| Setup | `pages/Setup.tsx` | First-run wizard |
| Logs | `pages/Logs.tsx` | Activity log |
| Skills | `pages/Skills.tsx` | Skill management |
| Dashboards | `pages/Dashboards.tsx` | Dashboard viewer |
| Browser | `pages/Browser.tsx` | Browser session management |
| HeartbeatMonitor | `pages/HeartbeatMonitor.tsx` | Agent heartbeat monitoring |
| GatewayEdit | `pages/GatewayEdit.tsx` | Gateway agent configuration |
| Tools | `pages/Tools.tsx` | Tool management |
| Context | `pages/Context.tsx` | File tree and context editor |
| Secrets | `pages/Secrets.tsx` | Secret/credential management |
| Scheduler | `pages/Scheduler.tsx` | Cron schedule management |

### Components (20+ total)

| Component | File | Role |
|-----------|------|------|
| Header | `components/Header.tsx` | Global header bar |
| Sidebar | `components/Sidebar.tsx` | Desktop navigation |
| BottomNav | `components/BottomNav.tsx` | Mobile navigation |
| Layout | `components/Layout.tsx` | App shell |
| Modal | `components/Modal.tsx` | Dialog system |
| Button | `components/Button.tsx` | Primary action element |
| Input / Select / Textarea | `components/Input.tsx` | Form fields |
| Toast | `components/Toast.tsx` | Notification system |
| SearchBar | `components/SearchBar.tsx` | Search input |
| Pagination | `components/Pagination.tsx` | Page navigation |
| DataTable | `components/DataTable.tsx` | Tabular data |
| Card | `components/Card.tsx` | Content container |
| ViewToggle | `components/ViewToggle.tsx` | Grid/list switcher |
| LoadingSpinner | `components/LoadingSpinner.tsx` | Loading indicator |
| NotificationBell | `components/NotificationBell.tsx` | Notification dropdown |
| EmptyState | `components/EmptyState.tsx` | Empty content placeholder |
| StatusBadge | `components/StatusBadge.tsx` | Status indicator |
| PagePanel | `components/PagePanel.tsx` | Side panel |
| BrowserViewer | `components/BrowserViewer.tsx` | Browser screenshot viewer |
| BrowserActionBar | `components/BrowserActionBar.tsx` | Browser controls |

All file paths below are relative to `web/frontend/src/`.

---

## Summary

All 65 accessibility issue types identified across 88+ instances have been fully remediated. The OpenPaw React frontend now meets WCAG 2.1 Level AA compliance. Structural foundations remain solid (semantic `<nav>`, `<aside>`, `<main>`, `<header>` usage; proper `htmlFor`/`id` label associations on the `Input` component), and all systemic gaps have been closed.

**All previously critical systemic problems are resolved:**
1. Focus trap implemented in all modals (8+ modal instances)
2. All clickable elements are keyboard-reachable with proper roles (12+ pages)
3. Shared Toggle component with `role="switch"` and `aria-checked` (20+ instances)
4. Toast notifications use live regions for screen reader announcements
5. Comprehensive `aria-label`, `aria-labelledby`, and `aria-describedby` usage throughout
6. `prefers-reduced-motion` media query applied globally
7. Visible focus indicators on all interactive elements

**Breakdown by severity (all resolved):**
- CRITICAL: 7 findings -- 0 open
- HIGH: 17 findings -- 0 open
- MEDIUM: 21 findings -- 0 open
- LOW: 20 findings -- 0 open

**Total: 65 unique issue types, all fixed.**

---

## 10/10 Criteria (All Met)

All criteria for a perfect accessibility score are satisfied:
1. Every interactive element reachable and operable via keyboard alone
2. All custom widgets use correct ARIA roles, states, and properties
3. Focus is trapped inside modals and restored on close
4. All dynamic content changes announced via live regions
5. Color contrast meets 4.5:1 for normal text, 3:1 for large text
6. All images have appropriate alt text (or `aria-hidden` for decorative)
7. All forms have programmatically associated labels and error messages
8. Skip navigation link present
9. `prefers-reduced-motion` respected for all animations
10. Touch targets meet 44x44px minimum on mobile

---

## Constraints

| Constraint | Reason | Impact |
|------------|--------|--------|
| OKLCH theming via CSS custom properties | Colors are dynamic, set from API | Contrast ratios depend on the active theme; defaults audited here |
| Dark-only default theme | No light mode shipped | Contrast issues specific to dark backgrounds |
| Tailwind v4 utility classes | No component-level ARIA by default | Every interactive pattern must add ARIA manually |

---

## Previously Fixed (65 issues)

All 65 issue types have been remediated. The table below summarizes each finding and the fix applied.

| ID | Severity | Issue | Fix Applied |
|----|----------|-------|-------------|
| C1 | CRITICAL | Modal missing dialog role and focus trap | Added `role="dialog"`, `aria-modal="true"`, `aria-labelledby`, focus trap with focus restore, `aria-label="Close dialog"` on close button |
| C2 | CRITICAL | Browser NewSessionModal no accessibility | Refactored to use shared Modal component with inherited dialog semantics and focus trap |
| C3 | CRITICAL | Toast notifications invisible to assistive technology | Added `role="status"` and `aria-live="polite"` to container; `role="alert"` and `aria-live="assertive"` for error toasts; `aria-label="Dismiss notification"` on dismiss button |
| C4 | CRITICAL | DataTable clickable rows not keyboard accessible | Added `tabIndex={0}`, `onKeyDown` handler for Enter/Space on clickable rows; added `scope="col"` to `<th>` elements |
| C5 | CRITICAL | Toggle switches missing role and state (20+ instances) | Extracted shared Toggle component with `role="switch"`, `aria-checked`, and `aria-label`; applied across all pages |
| C6 | CRITICAL | Card component clickable without keyboard support | Added conditional `tabIndex={0}`, `role="button"`, and `onKeyDown` for Enter/Space when `hover` and `onClick` are present |
| C7 | CRITICAL | Button component removes focus outline with no replacement | Replaced `focus:outline-none` with `focus-visible:ring-2 focus-visible:ring-accent-primary focus-visible:ring-offset-2 focus-visible:outline-none` |
| H1 | HIGH | No skip-to-content link | Added visually hidden skip link as first focusable element in Layout; added `id="main-content"` to `<main>` |
| H2 | HIGH | SearchBar input has no accessible name | Added `aria-label` derived from placeholder; added `aria-hidden="true"` to decorative Search icon |
| H3 | HIGH | HeartbeatMonitor search input has no accessible name | Added `aria-label="Search executions"`; added `aria-hidden="true"` to Search icon |
| H4 | HIGH | ViewToggle icon-only buttons missing accessible names | Added `aria-label="Grid view"` / `aria-label="List view"` and `aria-pressed` for active state |
| H5 | HIGH | Pagination buttons have no accessible names | Added `aria-label="Previous page"` / `aria-label="Next page"`; wrapped in `<nav aria-label="Pagination">` |
| H6 | HIGH | NotificationBell button and panel lack ARIA attributes | Added `aria-label="Notifications"`, `aria-expanded`, `aria-haspopup="true"`; added `aria-label="Dismiss"` to dismiss buttons; made notification items keyboard focusable |
| H7 | HIGH | Header user menu dropdown lacks ARIA menu pattern | Added `aria-label="User menu"`, `aria-expanded`, `aria-haspopup="true"`, `role="menu"`, `role="menuitem"`, and Escape key handler |
| H8 | HIGH | BottomNav More button missing ARIA attributes | Added `aria-expanded`, `aria-haspopup="true"`, `role="menu"`, `role="menuitem"`, and Escape key handler |
| H9 | HIGH | LoadingSpinner has no live region or status role | Added `role="status"` and `aria-live="polite"` to LoadingSpinner; wrapped inline spinners with `role="status"` and sr-only text |
| H10 | HIGH | Form error messages not programmatically associated | Added `aria-describedby` linking input to error message, `aria-invalid` on error state, and `id` on error `<p>` element |
| H11 | HIGH | Dropdown menus have no arrow key navigation | Implemented WAI-ARIA Menu pattern with arrow key navigation, Escape to close, Home/End support across all dropdowns |
| H12 | HIGH | Sidebar collapse button has incomplete accessible name | Added `aria-label` reflecting collapsed/expanded state; added `aria-hidden="true"` to decorative icons |
| H13 | HIGH | Chat textarea and action buttons lack accessible names | Added `aria-label="Type a message"` to textarea; `aria-label="Send message"` / `aria-label="Stop generation"` / `aria-label="Attach file"` to action buttons |
| H14 | HIGH | Chat autocomplete dropdowns lack combobox ARIA pattern | Implemented WAI-ARIA Combobox pattern with `role="combobox"`, `aria-expanded`, `aria-controls`, `role="listbox"`, `role="option"`, and `aria-selected` |
| H15 | HIGH | Chat thread edit/delete buttons hidden from keyboard users | Added `aria-label="Rename thread"` / `aria-label="Delete thread"`; added `focus-within:opacity-100` for keyboard visibility |
| H16 | HIGH | BrowserCard and ToolRow clickable without keyboard support | Added `tabIndex={0}`, `role="button"`, and `onKeyDown` for Enter/Space to BrowserCard and ToolRow |
| H17 | HIGH | Action buttons hidden on hover only (Browser, Context, Tools) | Added `group-focus-within:opacity-100 focus:opacity-100` alongside existing hover visibility |
| M1 | MEDIUM | Color contrast: text-3 on surface-0 fails AA | Updated `--op-text-3` default to meet 4.5:1 contrast ratio on dark backgrounds |
| M2 | MEDIUM | Color contrast: text-3/50 copyright extremely low contrast | Removed `/50` opacity modifier; uses full `text-text-3` for sufficient contrast |
| M3 | MEDIUM | Color contrast: text-text-3/50 on Browser session cards | Removed `/50` opacity modifier from fallback text |
| M4 | MEDIUM | No prefers-reduced-motion media query | Added global `@media (prefers-reduced-motion: reduce)` rule to disable animations and transitions |
| M5 | MEDIUM | Tab panels lack ARIA tab pattern (AgentEdit, Settings, GatewayEdit) | Added `role="tablist"`, `role="tab"`, `role="tabpanel"`, `aria-selected`, `aria-controls`, `aria-labelledby`, and arrow key navigation |
| M6 | MEDIUM | Model picker dropdowns lack ARIA attributes | Added `aria-expanded`, `aria-haspopup="listbox"`, `role="listbox"`, `role="option"`, and `aria-selected` |
| M7 | MEDIUM | Dashboards page switcher dropdown lacks ARIA attributes | Added `aria-expanded`, `aria-haspopup="listbox"`, `aria-label="Select dashboard"`, `role="listbox"`, and `role="option"` |
| M8 | MEDIUM | Balance badge tooltip is hover-only | Made badge focusable with `tabIndex={0}`; shows tooltip on focus; dismissable with Escape |
| M9 | MEDIUM | Hidden file inputs lack accessible labeling | Added `aria-label="Upload photo"` and `tabIndex={-1}` to all hidden file inputs |
| M10 | MEDIUM | Heading hierarchy issues across all pages | Corrected heading levels to logical hierarchy: h1 in Header, h2 for page sections, h3 for subsections |
| M11 | MEDIUM | Auto-dismissing toasts cannot be paused | Timer pauses on hover and focus; resumes on mouse leave and blur |
| M12 | MEDIUM | Chat thread selection state not announced | Added `aria-current="true"` to active thread |
| M13 | MEDIUM | Chat context bar progress indicator has no text alternative | Added `role="progressbar"`, `aria-valuenow`, `aria-valuemin`, `aria-valuemax`, and `aria-label="Context window usage"` |
| M14 | MEDIUM | Settings color preset buttons have no accessible names | Added `aria-label` with color name and `aria-pressed` for selected state |
| M15 | MEDIUM | Settings font selection buttons lack accessible state | Added `aria-pressed` to indicate selected font |
| M16 | MEDIUM | AgentEdit avatar preset buttons have no accessible names | Added `aria-label="Select preset avatar"` and `aria-pressed` for selected state |
| M17 | MEDIUM | Login form error not associated with inputs | Added `role="alert"` and `aria-live="assertive"` to error container |
| M18 | MEDIUM | Setup wizard step indicators are visual-only | Added `aria-hidden="true"` to decorative step bars (text heading provides equivalent information) |
| M19 | MEDIUM | Scheduler type selection buttons lack aria-pressed | Added `aria-pressed` reflecting selected schedule type |
| M20 | MEDIUM | Context tree view lacks ARIA tree pattern | Implemented WAI-ARIA Tree pattern with `role="tree"`, `role="treeitem"`, `aria-expanded`, `aria-level`, arrow key navigation, and keyboard alternatives for context menu |
| M21 | MEDIUM | Agents page avatar images have empty or generic alt text | Added `aria-hidden="true"` to decorative blurred background images |
| L1 | LOW | Decorative icons lack explicit aria-hidden | Verified Lucide defaults and added explicit `aria-hidden="true"` to decorative icons throughout |
| L2 | LOW | Touch targets below 44x44px minimum | Increased padding and added minimum dimensions to meet 44x44px target size |
| L3 | LOW | HTML lang attribute | Already present: `<html lang="en">` (no action needed) |
| L4 | LOW | Color-only status indicators | Added visually hidden text and `aria-label` for notification priority indicators |
| L5 | LOW | Table scope attributes missing | Added `scope="col"` to all `<th>` elements in DataTable and Tools inline table |
| L6 | LOW | Focus order with z-index stacking | Implemented `inert` on background content and focus trapping for all overlays |
| L7 | LOW | focus:outline-none on custom form inputs | Replaced with `focus-visible:ring-*` across all inputs; ensured visible focus indicators everywhere |
| L8 | LOW | Text truncation without full text available | Added `title` attributes to all elements using `truncate` class |
| L9 | LOW | Icon-only buttons throughout all pages | Added `aria-label` to all icon-only buttons across every page and component |
| L10 | LOW | Custom scrollbar may have insufficient contrast | Increased scrollbar thumb contrast to meet minimum ratio |
| L11 | LOW | Keyboard shortcut discoverability in Chat | Added `aria-keyshortcuts` attributes and keyboard help documentation |
| L12 | LOW | Empty state messaging lacks status role | Added `role="status"` to EmptyState container |
| L13 | LOW | Numeric badge in Header lacks context | Added `aria-label` with count and title context to badge |
| L14 | LOW | Agents page card selection state not announced | Added `aria-current` to focused/active agent card |
| L15 | LOW | AgentEdit/GatewayEdit unsaved changes not announced | Added `aria-label` with "unsaved" text when dirty; live region for change announcements |
| L16 | LOW | Responsive hiding removes content from screen readers | Replaced `hidden` with `sr-only` where content is informational; ensured mobile equivalents |
| L17 | LOW | Dashboards refresh button has title but no aria-label | Added `aria-label="Refresh data"` |
| L18 | LOW | Logs auto-refresh toggle without aria-pressed | Added `aria-pressed` reflecting auto-refresh state |
| L19 | LOW | BrowserActionBar inputs lack accessible names | Added `aria-label="Navigate to URL"` and `aria-label="Type text to browser"` |
| L20 | LOW | HeartbeatMonitor/Browser/GatewayEdit time inputs lack accessible names | Added matching `id`/`htmlFor` pairs and `aria-label` to all time inputs |

---

## Positive Findings

The following accessibility practices are already in place and should be maintained:

1. **Semantic HTML structure:** `<aside>` (Sidebar.tsx), `<nav>` (Sidebar.tsx line 63, BottomNav.tsx line 60), `<main>` (Layout.tsx line 12), `<header>` (Header.tsx line 149) used correctly.

2. **Input label associations:** The `Input`, `Select`, and `Textarea` components (Input.tsx) properly generate `id` attributes from label text and associate `<label>` elements via `htmlFor`.

3. **Escape key handling in Modal:** The Modal component listens for Escape to close (Modal.tsx line 23). Also present in Context.tsx ContextMenu (line 80).

4. **Body scroll lock in Modal:** The Modal prevents background scrolling when open (Modal.tsx line 22).

5. **Browser.tsx toggle switch:** The headless toggle in `NewSessionModal` correctly uses `role="switch"` and `aria-checked` (Browser.tsx lines 320-321) -- this is the model for all other toggles.

6. **Disabled state handling:** Buttons properly set `disabled` attribute with `disabled:opacity-40 disabled:cursor-not-allowed`.

7. **Safe area inset support:** CSS includes `safe-bottom` class for notched mobile devices.

8. **Focus ring on form inputs:** Input, Select, Textarea, and many custom inputs include `focus:ring-1 focus:ring-accent-primary`.

9. **HTML lang attribute:** `<html lang="en">` correctly set in `index.html`.

10. **NavLink aria-current:** React Router's `NavLink` automatically adds `aria-current="page"` to active links (Sidebar.tsx, BottomNav.tsx).

11. **BrowserViewer screenshot alt text:** The screenshot image has `alt="Browser screenshot"` (BrowserViewer.tsx line 101).

12. **HeartbeatMonitor execution items use `<button>`:** Execution history items (HeartbeatMonitor.tsx line 407) use semantic `<button>` elements, making them keyboard accessible by default.

13. **Proper form labeling in Login and Setup:** Login.tsx uses the Input component with proper labels (lines 83-98). Setup.tsx similarly uses Input with labels.

14. **PagePanel uses semantic `<aside>`:** The PagePanel component correctly uses `<aside>` element.

---

## Remediation Status (Complete)

| Priority | Issue Count | Status |
|----------|------------|--------|
| CRITICAL | 7 | All fixed |
| HIGH | 17 | All fixed |
| MEDIUM | 21 | All fixed |
| LOW | 20 | All fixed |
| **Total** | **65** | **All fixed** |

All remediation work is complete. The systemic fixes that addressed multiple findings at once were:

1. **Shared Toggle component** with built-in `role="switch"`, `aria-checked`, and `aria-label` -- fixed C5 across 20+ instances
2. **Modal component overhaul** with `role="dialog"`, `aria-modal`, focus trap, and `aria-labelledby` -- fixed C1 and C2 for all 8+ modal uses
3. **Card component keyboard support** with conditional `tabIndex`, `role="button"`, and `onKeyDown` -- fixed C6 across 5+ pages
4. **Keyboard focus visibility** via `group-focus-within:opacity-100` on all hover-only action buttons -- fixed H17 across 3+ components
5. **Global `prefers-reduced-motion` CSS** -- fixed M4 for all animations

---

## Audit History

| Date | Changes |
|------|---------|
| 2026-02-18 | Initial accessibility audit setup via Farmwork CLI |
| 2026-02-20 | Full WCAG 2.1 AA audit: 52 findings across 6 priority pages + shared components. Score: 3.5/10 |
| 2026-02-20 | Comprehensive audit expanded to ALL 16 pages + ALL 20+ components. 88 findings across 65 unique issue types. Score: 3.0/10. New pages audited: Login, Setup, Logs, Skills, Dashboards, Browser, HeartbeatMonitor, GatewayEdit, Tools, Context, Secrets, Scheduler. New components audited: BrowserActionBar, BrowserViewer, EmptyState, StatusBadge, PagePanel. |
| 2026-02-21 | All 65 issue types remediated across 88+ instances. Score: 10/10. Key systemic fixes: shared Toggle component (C5, 20+ instances), Modal focus trap and dialog semantics (C1/C2, 8+ modals), Card keyboard support (C6, 5+ pages), global prefers-reduced-motion (M4), visible focus indicators on all interactive elements (C7/L7), skip-to-content link (H1), ARIA patterns for tabs/menus/trees/comboboxes (M5/H11/M20/H14). |
