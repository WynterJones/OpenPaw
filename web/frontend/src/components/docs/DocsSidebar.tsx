import { NavLink, useLocation } from 'react-router';
import { X } from 'lucide-react';
import { docsNav } from './DocsNav';

interface DocsSidebarProps {
  mobileOpen: boolean;
  onClose: () => void;
}

export function DocsSidebar({ mobileOpen, onClose }: DocsSidebarProps) {
  const location = useLocation();

  const nav = (
    <nav aria-label="Documentation" className="py-4 px-3 space-y-5">
      {docsNav.map((group) => (
        <div key={group.title}>
          <p className="px-3 mb-1.5 text-xs font-semibold text-text-3 uppercase tracking-wider">
            {group.title}
          </p>
          <ul className="space-y-0.5">
            {group.items.map((item) => (
              <li key={item.href}>
                <NavLink
                  to={item.href}
                  end={item.href === '/docs'}
                  onClick={onClose}
                  className={() => {
                    const isActive = item.href === '/docs'
                      ? location.pathname === '/docs'
                      : location.pathname.startsWith(item.href);
                    return `flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                      isActive
                        ? 'bg-accent-muted text-accent-text'
                        : 'text-text-2 hover:text-text-1 hover:bg-surface-2'
                    }`;
                  }}
                >
                  <item.icon className="w-4 h-4 shrink-0" />
                  {item.label}
                </NavLink>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </nav>
  );

  return (
    <>
      {/* Desktop sidebar */}
      <aside className="hidden lg:block w-72 shrink-0 border-r border-border-0 overflow-y-auto">
        <div className="sticky top-0">
          {nav}
        </div>
      </aside>

      {/* Mobile drawer overlay */}
      {mobileOpen && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div className="absolute inset-0 bg-black/50" onClick={onClose} />
          <aside className="absolute left-0 top-0 bottom-0 w-72 bg-surface-1 border-r border-border-0 overflow-y-auto shadow-xl">
            <div className="flex items-center justify-between px-4 py-3 border-b border-border-0">
              <span className="text-sm font-semibold text-text-0">Navigation</span>
              <button
                onClick={onClose}
                className="p-1.5 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
                aria-label="Close navigation"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            {nav}
          </aside>
        </div>
      )}
    </>
  );
}
