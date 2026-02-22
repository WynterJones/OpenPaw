---
name: page-builder
description: Create new React pages following OpenPaw's frontend patterns and design system
tools: Read, Write, Edit, Grep, Glob
model: sonnet
---

# Page Builder Agent

Creates new React pages for OpenPaw's frontend, following established patterns.

## Tech Stack
- React 19 + TypeScript 5.9
- React Router v7
- Tailwind CSS v4 + design tokens via CSS custom properties
- Lucide React (only icon library)
- No state management library — React Context only

## Design System

All colors use CSS custom properties (never hardcode colors):
- Surfaces: `var(--op-surface-0)` through `var(--op-surface-3)`
- Text: `var(--op-text)`, `var(--op-text-muted)`, `var(--op-text-inverted)`
- Borders: `var(--op-border)`, `var(--op-border-strong)`
- Accent: `var(--op-accent)`, `var(--op-accent-hover)`, `var(--op-accent-text)`
- Danger: `var(--op-danger)`, `var(--op-danger-hover)`
- Typography: `var(--op-font-family)`, `var(--op-font-size-*)`, `var(--op-font-weight-*)`
- Spacing: `var(--op-radius)`, `var(--op-radius-lg)`

## Key Patterns

### Page Structure
```tsx
export default function MyPage() {
  const [items, setItems] = useState<Item[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.get<Item[]>('/my-endpoint').then(data => {
      setItems(data);
      setLoading(false);
    });
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 style={{ fontSize: 'var(--op-font-size-xl)', fontWeight: 'var(--op-font-weight-bold)' }}>
          Page Title
        </h1>
        <button>Action</button>
      </div>
      {/* Content */}
    </div>
  );
}
```

### API Calls
Use `api` from `src/lib/api.ts`: `api.get<T>()`, `api.post<T>()`, `api.put<T>()`, `api.delete<T>()`

### Components Available
`Layout`, `Sidebar`, `Header`, `Modal`, `DataTable`, `StatusBadge`, `EmptyState`, `LoadingSpinner`, `Toast` (via `useToast()` context)

## Instructions

1. **Read existing pages** in `web/frontend/src/pages/` for patterns
2. **Read components** in `web/frontend/src/components/` for available building blocks
3. **Read api.ts** for type definitions and API wrapper usage
4. **Create page** in `web/frontend/src/pages/`
5. **Add route** in `web/frontend/src/App.tsx`
6. **Add navigation** in `Sidebar.tsx` and `BottomNav.tsx` if needed

## Rules
- Never hardcode colors — always use `var(--op-*)` tokens
- Use Lucide icons only (import from `lucide-react`)
- Type everything — no `any`
- Use the existing `api` wrapper, never raw `fetch`
- Follow existing page patterns for consistency
- Mobile-responsive — use Tailwind responsive prefixes
