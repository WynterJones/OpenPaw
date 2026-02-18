import { useState, useRef, useEffect } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { useNavigate } from 'react-router';
import { User, LogOut, Camera } from 'lucide-react';

import { useConnectionStatus } from '../hooks/useConnectionStatus';
import { useOpenRouterBalance, type BalanceData } from '../hooks/useOpenRouterBalance';
import { useDesign } from '../contexts/DesignContext';
import { NotificationBell } from './NotificationBell';

function fmt(n: number): string {
  if (n < 0.01 && n > 0) return `$${n.toFixed(4)}`;
  return `$${n.toFixed(2)}`;
}

function BalanceBadge({ balance }: { balance: BalanceData }) {
  const [hover, setHover] = useState(false);

  const hasCredits = balance.totalCredits !== null;
  const creditBalance = hasCredits ? balance.totalCredits! - (balance.totalUsage ?? 0) : null;

  const hasData = creditBalance !== null || balance.limitRemaining !== null || balance.usage !== null;
  if (!hasData) return null;

  const badgeLabel = creditBalance !== null
    ? fmt(creditBalance)
    : balance.limitRemaining !== null
      ? fmt(balance.limitRemaining)
      : `${fmt(balance.usage!)} used`;

  const isLow = creditBalance !== null
    ? creditBalance < (balance.totalCredits ?? 0) * 0.1
    : balance.limitRemaining !== null && balance.limit !== null && balance.limitRemaining < balance.limit * 0.1;

  return (
    <div className="relative hidden sm:block" onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}>
      <span
        tabIndex={0}
        onFocus={() => setHover(true)}
        onBlur={() => setHover(false)}
        className={`inline-flex items-center px-2.5 py-1 rounded-full text-[11px] font-medium cursor-default ${
        isLow
          ? 'bg-red-500/10 text-red-400'
          : creditBalance !== null || balance.limitRemaining !== null
            ? 'bg-accent-primary/10 text-accent-primary'
            : 'bg-surface-2 text-text-2'
      }`}>
        {badgeLabel}
      </span>

      {hover && (
        <div className="absolute right-0 top-full mt-2 w-56 rounded-lg border border-border-0 bg-surface-1 shadow-xl p-3 z-50">
          <p className="text-[11px] font-semibold text-text-0 mb-2 pb-1.5 border-b border-border-0">
            OpenRouter{balance.label ? ` \u2014 ${balance.label}` : ''}
          </p>
          <div className="space-y-1.5 text-[11px]">
            {creditBalance !== null && (
              <Row label="Credits" value={fmt(creditBalance)} className="text-accent-primary font-semibold" />
            )}
            {balance.totalCredits !== null && (
              <Row label="Total Purchased" value={fmt(balance.totalCredits)} />
            )}
            {balance.limitRemaining !== null && (
              <Row label="Key Limit Left" value={fmt(balance.limitRemaining)} />
            )}
            {balance.usage !== null && (
              <Row label="Total Spent" value={fmt(balance.usage)} />
            )}
            {balance.usageMonthly !== null && (
              <Row label="This Month" value={fmt(balance.usageMonthly)} />
            )}
            {balance.rateLimit && (
              <Row label="Rate Limit" value={`${balance.rateLimit.requests}/${balance.rateLimit.interval}`} />
            )}
            {balance.isFreeTier && (
              <p className="text-[10px] text-amber-400 mt-1.5 pt-1.5 border-t border-border-0">Free tier</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function Row({ label, value, className }: { label: string; value: string; className?: string }) {
  return (
    <div className="flex justify-between items-center">
      <span className="text-text-3">{label}</span>
      <span className={className || 'text-text-1'}>{value}</span>
    </div>
  );
}

interface HeaderProps {
  title: string;
  count?: number;
  actions?: React.ReactNode;
}

export function Header({ title, count, actions }: HeaderProps) {
  const connected = useConnectionStatus();
  const balance = useOpenRouterBalance();
  const { showMascot } = useDesign();
  const { user, logout, refreshUser } = useAuth();
  const navigate = useNavigate();
  const [menuOpen, setMenuOpen] = useState(false);
  const [uploading, setUploading] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    }
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setMenuOpen(false);
    }
    document.addEventListener('mousedown', handleClick);
    if (menuOpen) document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [menuOpen]);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) return;
    setUploading(true);
    try {
      const formData = new FormData();
      formData.append('avatar', file);
      const csrfHeaders: Record<string, string> = {};
      const csrf = (await import('../lib/api')).getCSRFToken();
      if (csrf) csrfHeaders['X-CSRF-Token'] = csrf;
      const res = await fetch('/api/v1/auth/avatar', {
        method: 'POST',
        headers: csrfHeaders,
        body: formData,
        credentials: 'same-origin',
      });
      if (!res.ok) throw new Error('Upload failed');
      await refreshUser();
      setMenuOpen(false);
    } catch (e) { console.warn('avatarUpload failed:', e); } finally {
      setUploading(false);
    }
    e.target.value = '';
  };

  const profilePic = user?.avatar_path;

  return (
    <header className="relative z-30 h-14 md:h-16 flex items-center justify-between px-4 md:px-6 border-b border-border-0 bg-surface-1/50 backdrop-blur-sm flex-shrink-0">
      <div className="relative min-w-0 flex-1 mr-2 flex items-center gap-2.5">
        {showMascot && (
          <img
            src="/cat-toolbar.webp"
            alt=""
            className="h-full w-auto object-contain pointer-events-none select-none md:hidden"
          />
        )}
        <h1 className="hidden md:block text-lg md:text-xl font-bold text-text-0 truncate" title={title}>{title}</h1>
        {count !== undefined && count > 0 && (
          <span className="inline-flex items-center justify-center min-w-[22px] h-[22px] px-1.5 rounded-full bg-accent-primary text-white text-xs font-bold leading-none" aria-label={`${count} ${title}`}>
            {count}
          </span>
        )}
      </div>

      <div className="relative flex items-center gap-[20px] self-stretch flex-shrink-0">
        {showMascot && (
          <img
            src="/cat-toolbar.webp"
            alt=""
            className="h-full w-auto object-contain pointer-events-none select-none hidden md:block"
          />
        )}
        <div className="flex items-center gap-1.5 md:gap-2">
        {actions}

        <span className={`hidden sm:inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium ${
          connected
            ? 'bg-accent-primary/10 text-accent-primary'
            : 'bg-red-500/10 text-red-400'
        }`}>
          <span className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-accent-primary' : 'bg-red-400 animate-pulse'}`} />
          {connected ? 'Connected' : 'Disconnected'}
        </span>

        <BalanceBadge balance={balance} />

        <NotificationBell />

        <div className="relative" ref={menuRef}>
          <button
            onClick={() => setMenuOpen(!menuOpen)}
            aria-label="User menu"
            aria-expanded={menuOpen}
            aria-haspopup="true"
            className="flex items-center gap-2 p-1.5 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2/50 transition-colors cursor-pointer"
          >
            <div className="w-8 h-8 rounded-full ring-2 ring-accent-primary/30 overflow-hidden flex items-center justify-center bg-accent-muted flex-shrink-0">
              {profilePic ? (
                <img src={profilePic} alt="Profile" className="w-8 h-8 rounded-full object-cover" />
              ) : (
                <User className="w-4 h-4 text-accent-primary" />
              )}
            </div>
            <span className="text-sm font-bold hidden sm:inline text-text-0">{user?.username}</span>
          </button>
          {menuOpen && (
            <div className="absolute right-0 top-full mt-1 w-52 rounded-lg border border-border-0 bg-surface-1 shadow-xl py-1 z-50" role="menu">
              <button
                onClick={() => fileRef.current?.click()}
                disabled={uploading}
                role="menuitem"
                className="w-full flex items-center gap-2 px-4 py-2 text-sm text-text-1 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-50"
              >
                <Camera className="w-4 h-4" aria-hidden="true" />
                {uploading ? 'Uploading...' : profilePic ? 'Change photo' : 'Add photo'}
              </button>
              <button
                onClick={handleLogout}
                role="menuitem"
                className="w-full flex items-center gap-2 px-4 py-2 text-sm text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
              >
                <LogOut className="w-4 h-4" aria-hidden="true" />
                Sign out
              </button>
            </div>
          )}
        </div>
        </div>
      </div>

      <input ref={fileRef} type="file" accept="image/png,image/jpeg,image/webp" className="hidden" onChange={handleFileChange} aria-label="Upload profile photo" tabIndex={-1} />
    </header>
  );
}
