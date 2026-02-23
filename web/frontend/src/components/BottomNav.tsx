import { useState, useRef, useEffect } from 'react';
import { NavLink, useLocation } from 'react-router';
import {
  MessageSquare,
  Wrench,
  Bot,
  Sparkles,
  KeyRound,
  LayoutDashboard,
  Clock,
  FileText,
  Settings,
  BookOpen,
  Monitor,
  Heart,
  TerminalSquare,
  MoreHorizontal,
} from 'lucide-react';

const primaryItems = [
  { to: '/dashboards', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/chat', icon: MessageSquare, label: 'Chats' },
  { to: '/agents', icon: Bot, label: 'Agents' },
  { to: '/scheduler', icon: Clock, label: 'Scheduler' },
];

const moreItems = [
  { to: '/workbench', icon: TerminalSquare, label: 'Workbench' },
  { to: '/browser', icon: Monitor, label: 'Browser' },
  { to: '/context', icon: BookOpen, label: 'Context' },
  { to: '/heartbeat', icon: Heart, label: 'Heartbeat' },
  { to: '/tools', icon: Wrench, label: 'Tools' },
  { to: '/skills', icon: Sparkles, label: 'Skills' },
  { to: '/library', icon: BookOpen, label: 'Library' },
  { to: '/secrets', icon: KeyRound, label: 'Secrets' },
  { to: '/logs', icon: FileText, label: 'Logs' },
  { to: '/settings', icon: Settings, label: 'Settings' },
];

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
      <div className="flex items-center justify-around h-14 px-1">
        {primaryItems.map(item => (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) =>
              `flex flex-col items-center justify-center gap-0.5 min-w-0 px-2 py-1 rounded-lg transition-colors ${
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

        <div className="relative" ref={moreRef}>
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
              <div className="absolute bottom-full right-0 mb-2 w-48 rounded-xl border border-border-0 bg-surface-1 shadow-2xl py-1.5 z-50" role="menu">
                {moreItems.map(item => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    role="menuitem"
                    className={({ isActive }) =>
                      `flex items-center gap-3 px-4 py-2.5 text-sm font-medium transition-colors ${
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
