import { useState, useRef, useEffect } from 'react';
import { Bell, Check, X } from 'lucide-react';
import { useNotifications } from '../hooks/useNotifications';
import { useNavigate } from 'react-router';

function timeAgo(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diff = Math.floor((now - then) / 1000);
  if (diff < 60) return 'just now';
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

const priorityColors: Record<string, string> = {
  high: 'bg-red-500',
  normal: 'bg-accent-primary',
  low: 'bg-text-3',
};

export function NotificationBell() {
  const { notifications, unreadCount, markRead, markAllRead, dismiss } = useNotifications();
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  const handleNotificationClick = (id: string, link: string, read: boolean) => {
    if (!read) markRead(id);
    if (link) {
      navigate(link);
      setOpen(false);
    }
  };

  return (
    <div className="relative" ref={panelRef}>
      <button
        onClick={() => setOpen(!open)}
        aria-label={`Notifications${unreadCount > 0 ? `, ${unreadCount} unread` : ''}`}
        aria-expanded={open}
        aria-haspopup="true"
        className="relative flex items-center justify-center w-8 h-8 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2/50 transition-colors cursor-pointer"
      >
        <Bell className="w-[18px] h-[18px]" aria-hidden="true" />
        {unreadCount > 0 && (
          <span className="absolute -top-0.5 -right-0.5 min-w-[16px] h-4 px-1 rounded-full bg-red-500 text-white text-[10px] font-bold flex items-center justify-center leading-none animate-pulse" aria-hidden="true">
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 w-[420px] max-h-[520px] rounded-lg border border-border-0 bg-surface-1 shadow-xl z-50 flex flex-col overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-border-0">
            <span className="text-sm font-semibold text-text-0">Notifications</span>
            {unreadCount > 0 && (
              <button
                onClick={() => markAllRead()}
                className="flex items-center gap-1 text-[10px] text-accent-primary hover:text-accent-hover transition-colors cursor-pointer"
              >
                <Check className="w-3.5 h-3.5" />
                Mark all read
              </button>
            )}
          </div>

          <div className="overflow-y-auto flex-1">
            {notifications.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 text-text-2">
                <Bell className="w-10 h-10 mb-3 opacity-30" />
                <span className="text-sm">No notifications</span>
              </div>
            ) : (
              notifications.map(n => (
                <div
                  key={n.id}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleNotificationClick(n.id, n.link, n.read); } }}
                  className={`group flex items-start gap-3 px-4 py-3.5 border-b border-border-0 last:border-0 cursor-pointer transition-colors ${
                    n.read ? 'bg-transparent hover:bg-surface-2/30' : 'bg-accent-primary/5 hover:bg-accent-primary/10'
                  }`}
                  onClick={() => handleNotificationClick(n.id, n.link, n.read)}
                >
                  <span className={`mt-1.5 w-2 h-2 rounded-full flex-shrink-0 ${n.read ? 'opacity-0' : priorityColors[n.priority] || priorityColors.normal}`} aria-hidden="true" />
                  <span className="sr-only">{n.priority || 'normal'} priority</span>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-start justify-between gap-1">
                      <p className={`text-sm leading-snug truncate ${n.read ? 'text-text-1' : 'text-text-0 font-medium'}`} title={n.title}>
                        {n.title}
                      </p>
                      <button
                        onClick={(e) => { e.stopPropagation(); dismiss(n.id); }}
                        aria-label="Dismiss notification"
                        className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100 flex-shrink-0 p-0.5 text-text-2 hover:text-text-0 transition-all cursor-pointer"
                      >
                        <X className="w-3.5 h-3.5" aria-hidden="true" />
                      </button>
                    </div>
                    {n.body && (
                      <p className="text-xs text-text-2 mt-1 line-clamp-2 leading-relaxed">{n.body}</p>
                    )}
                    <div className="flex items-center gap-2 mt-1.5">
                      {n.source_agent_slug && (
                        <span className="text-[11px] text-text-2 bg-surface-2 px-1.5 py-0.5 rounded">{n.source_agent_slug}</span>
                      )}
                      <span className="text-[11px] text-text-2">{timeAgo(n.created_at)}</span>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
