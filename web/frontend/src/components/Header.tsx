import { useState, useRef, useEffect } from "react";
import { useAuth } from "../contexts/AuthContext";
import { useNavigate } from "react-router";
import { User, LogOut, Camera, Settings2, Check } from "lucide-react";

import { useConnectionStatus } from "../hooks/useConnectionStatus";
import {
  useOpenRouterBalance,
  type BalanceData,
} from "../hooks/useOpenRouterBalance";
import { useDesign } from "../contexts/DesignContext";
import { NotificationBell } from "./NotificationBell";
import { startWindowDrag } from "../lib/tauri";
import { useViewToggles, type ViewToggleKey } from "../contexts/viewToggles";

function fmt(n: number): string {
  if (n < 0.01 && n > 0) return `$${n.toFixed(4)}`;
  return `$${n.toFixed(2)}`;
}

function BalanceBadge({ balance }: { balance: BalanceData }) {
  const [hover, setHover] = useState(false);

  // Cost/credit UI is OpenRouter-only. CLI subscription providers (Claude Code /
  // Codex) have no per-token billing, so render nothing here.
  if (balance.subscription) return null;

  const hasCredits = balance.totalCredits !== null;
  const creditBalance = hasCredits
    ? balance.totalCredits! - (balance.totalUsage ?? 0)
    : null;

  const hasData =
    creditBalance !== null ||
    balance.limitRemaining !== null ||
    balance.usage !== null;
  if (!hasData) return null;

  const badgeLabel =
    creditBalance !== null
      ? fmt(creditBalance)
      : balance.limitRemaining !== null
        ? fmt(balance.limitRemaining)
        : `${fmt(balance.usage!)} used`;

  const isLow =
    creditBalance !== null
      ? creditBalance < (balance.totalCredits ?? 0) * 0.1
      : balance.limitRemaining !== null &&
        balance.limit !== null &&
        balance.limitRemaining < balance.limit * 0.1;

  return (
    <div
      className="relative hidden sm:block"
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
    >
      <span
        tabIndex={0}
        onFocus={() => setHover(true)}
        onBlur={() => setHover(false)}
        className={`inline-flex items-center px-2.5 py-1 rounded-full text-[11px] font-medium cursor-default ${
          isLow
            ? "bg-red-500/10 text-red-400"
            : creditBalance !== null || balance.limitRemaining !== null
              ? "bg-accent-primary/10 text-accent-primary"
              : "bg-surface-2 text-text-2"
        }`}
      >
        {badgeLabel}
      </span>

      {hover && (
        <div className="absolute right-0 top-full mt-2 w-56 rounded-lg border border-border-0 bg-surface-1 shadow-xl p-3 z-50">
          <p className="text-[11px] font-semibold text-text-0 mb-2 pb-1.5 border-b border-border-0">
            OpenRouter{balance.label ? ` \u2014 ${balance.label}` : ""}
          </p>
          <div className="space-y-1.5 text-[11px]">
            {creditBalance !== null && (
              <Row
                label="Credits"
                value={fmt(creditBalance)}
                className="text-accent-primary font-semibold"
              />
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
              <Row
                label="Rate Limit"
                value={`${balance.rateLimit.requests}/${balance.rateLimit.interval}`}
              />
            )}
            {balance.isFreeTier && (
              <p className="text-[10px] text-amber-400 mt-1.5 pt-1.5 border-t border-border-0">
                Free tier
              </p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function Row({
  label,
  value,
  className,
}: {
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div className="flex justify-between items-center">
      <span className="text-text-3">{label}</span>
      <span className={className || "text-text-1"}>{value}</span>
    </div>
  );
}

interface HeaderProps {
  title: string;
  count?: number;
  actions?: React.ReactNode;
  hideTitleOnMobile?: boolean;
}

/**
 * One menu for showing/hiding the layout panes. Replaces the three separate
 * toggle buttons that used to sit in the sidebar footer and the chat header,
 * which were easy to miss and gave no hint of what they controlled.
 */
function ViewTogglesMenu() {
  const { sidebar, chatList, chatPanel, canvas, toggle } = useViewToggles();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, [open]);

  // Canvas is listed here as well as in the chat itself, because turning it on
  // hides the chat list its own button lives in.
  const items: { key: ViewToggleKey; label: string; checked: boolean }[] = [
    { key: 'sidebar', label: 'Toggle Sidebar', checked: sidebar },
    { key: 'chatList', label: 'Toggle Chat List', checked: chatList },
    { key: 'chatPanel', label: 'Toggle Chat Panel', checked: chatPanel },
    { key: 'canvas', label: 'Toggle Canvas', checked: canvas },
  ];

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(!open)}
        aria-label="View options"
        aria-expanded={open}
        aria-haspopup="true"
        title="View options"
        className="p-2 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2/50 transition-colors cursor-pointer"
      >
        <Settings2 className="w-4 h-4" aria-hidden="true" />
      </button>
      {open && (
        <div className="absolute right-0 mt-1 w-52 rounded-xl border border-border-1 bg-surface-1 shadow-xl shadow-black/30 overflow-hidden z-50 py-1">
          {items.map(item => (
            <button
              key={item.key}
              onClick={() => toggle(item.key)}
              role="menuitemcheckbox"
              aria-checked={item.checked}
              className="w-full flex items-center gap-2.5 px-3 py-2 text-left text-sm text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
            >
              <span className="w-4 h-4 flex items-center justify-center flex-shrink-0">
                {item.checked && <Check className="w-3.5 h-3.5 text-accent-primary" aria-hidden="true" />}
              </span>
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export function Header({ title, count, actions, hideTitleOnMobile }: HeaderProps) {
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
      if (e.key === "Escape") setMenuOpen(false);
    }
    document.addEventListener("mousedown", handleClick);
    if (menuOpen) document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [menuOpen]);

  const handleLogout = () => {
    logout();
    navigate("/login");
  };

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!["image/png", "image/jpeg", "image/webp"].includes(file.type)) return;
    setUploading(true);
    try {
      const formData = new FormData();
      formData.append("avatar", file);
      const csrfHeaders: Record<string, string> = {};
      const csrf = (await import("../lib/api")).getCSRFToken();
      if (csrf) csrfHeaders["X-CSRF-Token"] = csrf;
      const res = await fetch("/api/v1/auth/avatar", {
        method: "POST",
        headers: csrfHeaders,
        body: formData,
        credentials: "same-origin",
      });
      if (!res.ok) throw new Error("Upload failed");
      await refreshUser();
      setMenuOpen(false);
    } catch (e) {
      console.warn("avatarUpload failed:", e);
    } finally {
      setUploading(false);
    }
    e.target.value = "";
  };

  const profilePic = user?.avatar_path;

  return (
    <header data-tauri-drag-region onMouseDown={startWindowDrag} className="relative z-40 h-14 md:h-16 flex items-center justify-between px-4 md:px-6 border-b border-border-0 bg-surface-1/50 backdrop-blur-sm flex-shrink-0">
      <div data-tauri-drag-region className="relative min-w-0 flex-1 mr-2 flex items-center gap-2.5">
        <h1
          data-tauri-drag-region
          className={`text-lg md:text-xl font-bold text-text-0 truncate ${hideTitleOnMobile ? 'hidden md:block' : ''}`}
          title={title}
        >
          {title}
        </h1>
        {count !== undefined && count > 0 && (
          <span
            className="inline-flex items-center justify-center min-w-[22px] h-[22px] px-1.5 rounded-full bg-accent-primary text-white text-xs font-bold leading-none"
            aria-label={`${count} ${title}`}
          >
            {count}
          </span>
        )}
      </div>

      <div className="relative ml-auto flex items-center gap-2 self-stretch flex-shrink-0">
        {showMascot && (
          <img
            src="/cat-toolbar.webp"
            alt=""
            className="h-full w-auto max-w-[120px] object-contain object-right pointer-events-none select-none hidden md:block"
            style={{ position: "relative", bottom: "-3px" }}
          />
        )}
        <div className="flex items-center gap-1.5 md:gap-2">
          {actions}

          {!connected && (
            <span className="hidden sm:inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium bg-red-500/10 text-red-400">
              <span className="w-1.5 h-1.5 rounded-full bg-red-400 animate-pulse" />
              Disconnected
            </span>
          )}

          <BalanceBadge balance={balance} />

          <NotificationBell />

          <ViewTogglesMenu />

          <div className="relative" ref={menuRef}>
            <button
              onClick={() => setMenuOpen(!menuOpen)}
              aria-label="User menu"
              aria-expanded={menuOpen}
              aria-haspopup="true"
              className="flex items-center gap-2 p-1.5 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2/50 transition-colors cursor-pointer"
            >
              <div className="w-8 h-8 rounded-md border border-border-1 overflow-hidden flex items-center justify-center bg-surface-2 flex-shrink-0">
                {profilePic ? (
                  <img
                    src={profilePic}
                    alt="Profile"
                    className="w-8 h-8 rounded-md object-cover"
                  />
                ) : (
                  <User className="w-4 h-4 text-accent-primary" />
                )}
              </div>
              <span className="text-sm font-bold hidden sm:inline text-text-0">
                {user?.username}
              </span>
            </button>
            {menuOpen && (
              <div
                className="absolute right-0 top-full mt-1 w-52 rounded-lg border border-border-0 bg-surface-1 shadow-xl py-1 z-50"
                role="menu"
              >
                <button
                  onClick={() => fileRef.current?.click()}
                  disabled={uploading}
                  role="menuitem"
                  className="w-full flex items-center gap-2 px-4 py-2 text-sm text-text-1 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-50"
                >
                  <Camera className="w-4 h-4" aria-hidden="true" />
                  {uploading
                    ? "Uploading..."
                    : profilePic
                      ? "Change photo"
                      : "Add photo"}
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

      <input
        ref={fileRef}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        className="hidden"
        onChange={handleFileChange}
        aria-label="Upload profile photo"
        tabIndex={-1}
      />
    </header>
  );
}
