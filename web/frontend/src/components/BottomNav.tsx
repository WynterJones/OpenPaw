import { useState, useRef, useEffect } from 'react';
import { NavLink, useLocation } from 'react-router';
import { MoreHorizontal } from 'lucide-react';
import { APP_NAV_ITEMS } from '../lib/app-navigation';

const primaryItems = [
  { ...APP_NAV_ITEMS.find((item) => item.id === 'dashboards')!, label: 'Dashboard' },
  { ...APP_NAV_ITEMS.find((item) => item.id === 'chat')!, label: 'Chat' },
  { ...APP_NAV_ITEMS.find((item) => item.id === 'inbox')!, label: 'Inbox' },
];

const primaryIds = new Set(primaryItems.map((item) => item.id));
const moreItems = APP_NAV_ITEMS.filter((item) => !primaryIds.has(item.id));

export function BottomNav() {
  const [moreOpen, setMoreOpen] = useState(false);
  const moreRef = useRef<HTMLDivElement>(null);
  const location = useLocation();
  const { pathname } = location;

  const isMoreActive = moreItems.some(item => pathname.startsWith(item.to));

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (moreRef.current && !moreRef.current.contains(e.target as Node)) {
        setMoreOpen(false);
      }
    }
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setMoreOpen(false);
    }
    document.addEventListener('mousedown', handleClick);
    if (moreOpen) document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [moreOpen]);

  useEffect(() => {
    return () => setMoreOpen(false);
  }, [pathname]);

  return (
    <nav className="md:hidden fixed bottom-0 left-0 right-0 bg-surface-1/95 backdrop-blur-sm border-t border-border-0 z-40 safe-bottom">
      <div className="grid h-14 grid-cols-4 items-center px-1">
        {primaryItems.map(item => (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) =>
              `flex min-w-0 flex-col items-center justify-center gap-0.5 rounded-lg px-2 py-1 transition-colors ${
                isActive
                  ? 'text-accent-primary'
                  : 'text-text-3'
              }`
            }
          >
            <item.icon className="w-5 h-5" />
            <span className="text-[10px] font-medium leading-tight truncate">{item.label}</span>
          </NavLink>
        ))}

        <div className="relative flex justify-center" ref={moreRef}>
          <button
            onClick={() => setMoreOpen(!moreOpen)}
            aria-expanded={moreOpen}
            aria-haspopup="true"
            aria-label="More navigation options"
            className={`flex flex-col items-center justify-center gap-0.5 min-w-0 px-2 py-1 rounded-lg transition-colors cursor-pointer ${
              isMoreActive || moreOpen
                ? 'text-accent-primary'
                : 'text-text-3'
            }`}
          >
            <MoreHorizontal className="w-5 h-5" aria-hidden="true" />
            <span className="text-[10px] font-medium leading-tight">More</span>
          </button>

          {moreOpen && (
            <>
              <div className="fixed inset-0 z-40" />
              <div
                className="absolute bottom-full right-1 mb-2 max-h-[min(70dvh,30rem)] w-52 overflow-y-auto overscroll-contain rounded-xl border border-border-0 bg-surface-1 py-1.5 shadow-2xl z-50"
                role="menu"
              >
                {moreItems.map(item => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    role="menuitem"
                    className={({ isActive }) =>
                      `flex min-h-11 items-center gap-3 px-4 py-2.5 text-sm font-medium transition-colors ${
                        isActive
                          ? 'text-accent-primary bg-accent-muted'
                          : 'text-text-1 hover:bg-surface-2'
                      }`
                    }
                  >
                    <item.icon className="w-4 h-4 flex-shrink-0" />
                    {item.label}
                  </NavLink>
                ))}
              </div>
            </>
          )}
        </div>
      </div>
    </nav>
  );
}
