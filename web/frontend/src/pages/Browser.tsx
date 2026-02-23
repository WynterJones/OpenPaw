import { useState, useEffect, useCallback, useRef } from "react";
import {
  Monitor,
  ArrowLeft,
  Play,
  Square,
  Trash2,
  Plus,
  Globe,
  User,
  X,
  Check,
  Lock,
  RefreshCw,
} from "lucide-react";
import { Header } from "../components/Header";
import { Button } from "../components/Button";
import { Card } from "../components/Card";
import { Toggle } from "../components/Toggle";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { SearchBar } from "../components/SearchBar";
import { Pagination } from "../components/Pagination";
import { ViewToggle, type ViewMode } from "../components/ViewToggle";
import { LoadingSpinner } from "../components/LoadingSpinner";
import { BrowserViewer } from "../components/BrowserViewer";
import { BrowserActionBar } from "../components/BrowserActionBar";
import { useToast } from "../components/Toast";
import { useWebSocket } from "../lib/useWebSocket";
import { browserApi, type BrowserSession } from "../lib/api";

const PAGE_SIZE = 12;

function statusColor(status: string) {
  const colors: Record<string, string> = {
    active: "bg-emerald-400",
    busy: "bg-blue-400",
    human: "bg-amber-400",
    idle: "bg-text-3",
    stopped: "bg-text-3",
    error: "bg-red-400",
  };
  return colors[status] ?? "bg-text-3";
}

function truncateUrl(url: string) {
  try {
    const u = new URL(url);
    const path = u.pathname === "/" ? "" : u.pathname;
    const display = u.hostname + path;
    return display.length > 40 ? display.slice(0, 40) + "..." : display;
  } catch (e) {
    console.warn("formatUrl: failed to parse URL:", e);
    return url.length > 40 ? url.slice(0, 40) + "..." : url;
  }
}

// ─── Mini browser card (grid view) ──────────────────────────────────────────

function BrowserCard({
  session,
  onClick,
  onStart,
  onStop,
  onDelete,
}: {
  session: BrowserSession;
  onClick: () => void;
  onStart: (e: React.MouseEvent) => void;
  onStop: (e: React.MouseEvent) => void;
  onDelete: (e: React.MouseEvent) => void;
}) {
  const isStopped = session.status === "stopped" || session.status === "idle";
  const isRunning = ["active", "busy", "human"].includes(session.status);

  return (
    <div
      onClick={onClick}
      tabIndex={0}
      role="button"
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick();
        }
      }}
      className="group rounded-xl border border-border-0 bg-surface-1 shadow-sm hover:border-border-1 hover:shadow-md focus:border-border-1 focus:shadow-md focus-visible:ring-2 focus-visible:ring-accent-primary transition-all duration-150 cursor-pointer overflow-hidden"
    >
      {/* Title bar */}
      <div className="flex items-center gap-2 px-3 py-2 bg-surface-2/60 border-b border-border-0">
        <div className="flex items-center gap-1.5">
          <span className="w-2.5 h-2.5 rounded-full bg-red-400/70" />
          <span className="w-2.5 h-2.5 rounded-full bg-amber-400/70" />
          <span className="w-2.5 h-2.5 rounded-full bg-emerald-400/70" />
        </div>
        <span className="flex-1 text-xs font-medium text-text-1 truncate text-center">
          {session.name}
        </span>
        <span
          className={`w-2 h-2 rounded-full flex-shrink-0 ${statusColor(session.status)}`}
        />
      </div>

      {/* URL bar */}
      <div className="mx-2.5 mt-2 flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-surface-0 border border-border-0">
        {session.current_url ? (
          <>
            <Lock className="w-3 h-3 text-text-3 flex-shrink-0" />
            <span className="text-[11px] text-text-2 font-mono truncate flex-1">
              {truncateUrl(session.current_url)}
            </span>
          </>
        ) : (
          <>
            <Globe className="w-3 h-3 text-text-3 flex-shrink-0" />
            <span className="text-[11px] text-text-3 italic flex-1">
              No URL loaded
            </span>
          </>
        )}
        <RefreshCw className="w-3 h-3 text-text-3 flex-shrink-0" />
      </div>

      {/* Content area */}
      <div className="mx-2.5 mt-2 mb-2.5 rounded-lg bg-surface-0 border border-border-0 h-24 flex items-center justify-center">
        <div className="text-center">
          <Monitor className="w-6 h-6 text-text-3/40 mx-auto mb-1" />
          {session.current_title ? (
            <p className="text-[10px] text-text-3 px-3 truncate max-w-[180px]">
              {session.current_title}
            </p>
          ) : (
            <p className="text-[10px] text-text-3">Browser session</p>
          )}
        </div>
      </div>

      {/* Footer */}
      <div className="flex items-center justify-between px-3 py-2 border-t border-border-0 bg-surface-2/30">
        <div className="flex items-center gap-2 min-w-0">
          {session.owner_agent_slug ? (
            <span className="flex items-center gap-1 text-[11px] text-text-3 truncate">
              <User className="w-3 h-3 flex-shrink-0" />
              {session.owner_agent_slug}
            </span>
          ) : (
            <StatusBadge status={session.status} />
          )}
        </div>
        <div
          className="flex items-center gap-0.5"
          onClick={(e) => e.stopPropagation()}
        >
          {isStopped && (
            <button
              onClick={onStart}
              title="Start"
              aria-label="Start session"
              className="p-1.5 rounded-lg text-emerald-400 hover:bg-emerald-500/10 transition-colors cursor-pointer opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
            >
              <Play className="w-3.5 h-3.5" />
            </button>
          )}
          {isRunning && (
            <button
              onClick={onStop}
              title="Stop"
              aria-label="Stop session"
              className="p-1.5 rounded-lg text-text-2 hover:bg-surface-3 transition-colors cursor-pointer opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
            >
              <Square className="w-3.5 h-3.5" />
            </button>
          )}
          <button
            onClick={onDelete}
            title="Delete"
            aria-label="Delete session"
            className="p-1.5 rounded-lg text-text-3 hover:text-red-400 hover:bg-red-400/10 transition-colors cursor-pointer opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
}

// ─── List row ────────────────────────────────────────────────────────────────

function BrowserRow({
  session,
  onClick,
  onStart,
  onStop,
  onDelete,
}: {
  session: BrowserSession;
  onClick: () => void;
  onStart: (e: React.MouseEvent) => void;
  onStop: (e: React.MouseEvent) => void;
  onDelete: (e: React.MouseEvent) => void;
}) {
  const isStopped = session.status === "stopped" || session.status === "idle";
  const isRunning = ["active", "busy", "human"].includes(session.status);

  return (
    <Card hover onClick={onClick}>
      <div className="flex items-center gap-3 md:gap-4">
        <div className="w-10 h-10 md:w-12 md:h-12 rounded-xl bg-surface-2 flex items-center justify-center flex-shrink-0 border border-border-0">
          <Monitor className="w-5 h-5 text-text-2" />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-semibold text-text-0 truncate">
            {session.name}
          </p>
          {session.current_url ? (
            <p className="text-xs text-text-3 font-mono truncate flex items-center gap-1">
              <Lock className="w-3 h-3 flex-shrink-0 inline" />
              {truncateUrl(session.current_url)}
            </p>
          ) : (
            <p className="text-xs text-text-3 italic">No URL loaded</p>
          )}
        </div>
        {session.owner_agent_slug && (
          <span className="hidden md:flex items-center gap-1 text-xs text-text-3 flex-shrink-0">
            <User className="w-3 h-3" />
            {session.owner_agent_slug}
          </span>
        )}
        <StatusBadge status={session.status} />
        <div
          className="flex items-center gap-0.5 flex-shrink-0"
          onClick={(e) => e.stopPropagation()}
        >
          {isStopped && (
            <button
              onClick={onStart}
              title="Start"
              aria-label="Start session"
              className="p-1.5 rounded-lg text-emerald-400 hover:bg-emerald-500/10 transition-colors cursor-pointer"
            >
              <Play className="w-3.5 h-3.5" />
            </button>
          )}
          {isRunning && (
            <button
              onClick={onStop}
              title="Stop"
              aria-label="Stop session"
              className="p-1.5 rounded-lg text-text-2 hover:bg-surface-3 transition-colors cursor-pointer"
            >
              <Square className="w-3.5 h-3.5" />
            </button>
          )}
          <button
            onClick={onDelete}
            title="Delete"
            aria-label="Delete session"
            className="p-1.5 rounded-lg text-text-3 hover:text-red-400 hover:bg-red-400/10 transition-colors cursor-pointer"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </Card>
  );
}

// ─── New Session modal ──────────────────────────────────────────────────────

function NewSessionModal({
  onClose,
  onCreate,
}: {
  onClose: () => void;
  onCreate: (session: BrowserSession) => void;
}) {
  const { toast } = useToast();
  const [name, setName] = useState("");
  const [ownerSlug, setOwnerSlug] = useState("");
  const [headless, setHeadless] = useState(true);
  const [saving, setSaving] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setSaving(true);
    try {
      const session = await browserApi.createSession({
        name: name.trim(),
        headless,
        owner_agent_slug: ownerSlug.trim() || undefined,
      });
      toast("success", "Session created");
      onCreate(session);
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to create session",
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="New browser session"
        className="relative w-full max-w-md rounded-2xl border border-border-0 bg-surface-1 shadow-2xl p-6"
      >
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-lg font-semibold text-text-0">
            New Browser Session
          </h2>
          <button
            onClick={onClose}
            aria-label="Close"
            className="p-1.5 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold uppercase tracking-wider text-text-3 mb-1.5">
              Session Name
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My Browser Session"
              autoFocus
              required
              className="w-full px-3 py-2 rounded-lg text-sm bg-surface-2 border border-border-0 text-text-0 placeholder:text-text-3 focus:outline-none focus:ring-1 focus:ring-accent-primary"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold uppercase tracking-wider text-text-3 mb-1.5">
              Owner Agent (optional)
            </label>
            <input
              type="text"
              value={ownerSlug}
              onChange={(e) => setOwnerSlug(e.target.value)}
              placeholder="agent-slug"
              className="w-full px-3 py-2 rounded-lg text-sm bg-surface-2 border border-border-0 text-text-0 placeholder:text-text-3 focus:outline-none focus:ring-1 focus:ring-accent-primary"
            />
          </div>

          <div className="flex items-center gap-3">
            <Toggle
              enabled={headless}
              onChange={(v) => setHeadless(v)}
              label="Headless mode"
            />
            <span className="text-sm text-text-1">Headless mode</span>
          </div>

          <div className="flex gap-2 pt-1">
            <Button
              type="submit"
              loading={saving}
              icon={<Check className="w-4 h-4" />}
            >
              Create Session
            </Button>
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}

// ─── Session detail ─────────────────────────────────────────────────────────

interface ActionLogEntry {
  id: string;
  timestamp: Date;
  action: string;
  detail: string;
  success: boolean;
}

function SessionDetail({
  session,
  onBack,
  onRefresh,
}: {
  session: BrowserSession;
  onBack: () => void;
  onRefresh: () => void;
}) {
  const { toast } = useToast();
  const [humanControl, setHumanControl] = useState(session.status === "human");
  const [actionLog, setActionLog] = useState<ActionLogEntry[]>([]);
  const [loadingAction, setLoadingAction] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const logEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [actionLog]);

  const addLog = (action: string, detail: string, success: boolean) => {
    setActionLog((prev) => [
      ...prev,
      {
        id: `${Date.now()}-${Math.random()}`,
        timestamp: new Date(),
        action,
        detail,
        success,
      },
    ]);
  };

  useWebSocket({
    onMessage: (msg) => {
      if (
        msg.type === "browser_action" &&
        (msg.payload as { session_id?: string }).session_id === session.id
      ) {
        const p = msg.payload as {
          action?: string;
          detail?: string;
          success?: boolean;
        };
        addLog(p.action ?? "action", p.detail ?? "", p.success !== false);
      }
      if (
        msg.type === "browser_status" &&
        (msg.payload as { session_id?: string }).session_id === session.id
      ) {
        onRefresh();
      }
    },
  });

  const isStopped = session.status === "stopped" || session.status === "idle";
  const isRunning = ["active", "busy", "human"].includes(session.status);

  const doSessionAction = async (action: "start" | "stop") => {
    setLoadingAction(action);
    try {
      if (action === "start") {
        await browserApi.startSession(session.id);
        toast("success", "Session started");
      } else {
        await browserApi.stopSession(session.id);
        toast("success", "Session stopped");
      }
      onRefresh();
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : `Failed to ${action} session`,
      );
    } finally {
      setLoadingAction(null);
    }
  };

  const handleDelete = async () => {
    setLoadingAction("delete");
    try {
      await browserApi.deleteSession(session.id);
      toast("success", "Session deleted");
      onBack();
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to delete session",
      );
      setLoadingAction(null);
      setConfirmDelete(false);
    }
  };

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center gap-3">
        <button
          onClick={onBack}
          aria-label="Back"
          className="p-2 rounded-lg text-text-2 hover:bg-surface-2 transition-colors cursor-pointer flex-shrink-0"
        >
          <ArrowLeft className="w-5 h-5" />
        </button>
        <div className="flex-1 min-w-0">
          <h2 className="text-lg font-semibold text-text-0 truncate">
            {session.name}
          </h2>
          {session.current_url && (
            <p className="text-xs text-text-3 font-mono truncate">
              {session.current_url}
            </p>
          )}
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <span
            className={`w-2 h-2 rounded-full ${statusColor(session.status)}`}
          />
          <StatusBadge status={session.status} />
        </div>
      </div>

      {/* Two-column layout */}
      <div className="flex flex-col lg:flex-row gap-4">
        {/* Left column: controls */}
        <div className="flex-1 min-w-0 space-y-4">
          <div className="flex flex-wrap gap-2">
            {isStopped && (
              <Button
                onClick={() => doSessionAction("start")}
                loading={loadingAction === "start"}
                icon={<Play className="w-4 h-4" />}
              >
                Start
              </Button>
            )}
            {isRunning && (
              <Button
                variant="secondary"
                onClick={() => doSessionAction("stop")}
                loading={loadingAction === "stop"}
                icon={<Square className="w-4 h-4" />}
              >
                Stop
              </Button>
            )}

            <div className="flex-1" />

            {confirmDelete ? (
              <div className="flex items-center gap-2">
                <span className="text-sm text-red-400">
                  Delete this session?
                </span>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={handleDelete}
                  loading={loadingAction === "delete"}
                >
                  Confirm
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setConfirmDelete(false)}
                >
                  Cancel
                </Button>
              </div>
            ) : (
              <Button
                variant="ghost"
                onClick={() => setConfirmDelete(true)}
                icon={<Trash2 className="w-4 h-4" />}
                className="text-red-400 hover:text-red-300 hover:bg-red-400/10"
              >
                Delete
              </Button>
            )}
          </div>

          <BrowserActionBar
            sessionId={session.id}
            currentUrl={session.current_url}
            humanControl={humanControl}
            onTakeControl={() => {
              setHumanControl(true);
              addLog("control", "Human control activated", true);
              onRefresh();
            }}
            onReleaseControl={() => {
              setHumanControl(false);
              addLog("release", "Human control released", true);
              onRefresh();
            }}
          />

          <Card>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
              Action Log
            </h4>
            {actionLog.length === 0 ? (
              <p className="text-sm text-text-3 italic">
                No actions recorded yet.
              </p>
            ) : (
              <div className="space-y-1.5 max-h-56 overflow-y-auto">
                {actionLog.map((entry) => (
                  <div
                    key={entry.id}
                    className="flex items-start gap-2 text-xs font-mono"
                  >
                    <span className="text-text-3 flex-shrink-0 tabular-nums">
                      {entry.timestamp.toLocaleTimeString()}
                    </span>
                    <span
                      className={`flex-shrink-0 font-semibold ${
                        entry.success ? "text-emerald-400" : "text-red-400"
                      }`}
                    >
                      [{entry.action}]
                    </span>
                    <span className="text-text-2 break-all">
                      {entry.detail}
                    </span>
                  </div>
                ))}
                <div ref={logEndRef} />
              </div>
            )}
          </Card>
        </div>

        {/* Right column: screenshot viewer */}
        <div className="lg:w-[500px] lg:flex-shrink-0 lg:max-h-[calc(100vh-12rem)] lg:overflow-y-auto">
          <BrowserViewer sessionId={session.id} humanControl={humanControl} />
        </div>
      </div>
    </div>
  );
}

// ─── Main page ──────────────────────────────────────────────────────────────

export function Browser() {
  const { toast } = useToast();
  const [sessions, setSessions] = useState<BrowserSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedSession, setSelectedSession] = useState<BrowserSession | null>(
    null,
  );
  const [showNewModal, setShowNewModal] = useState(false);
  const [search, setSearch] = useState("");
  const [view, setView] = useState<ViewMode>("grid");
  const [page, setPage] = useState(0);
  const selectedRef = useRef<BrowserSession | null>(null);

  useEffect(() => {
    selectedRef.current = selectedSession;
  }, [selectedSession]);

  useEffect(() => {
    loadSessions();
  }, []);

  const loadSessions = async () => {
    try {
      const data = await browserApi.listSessions();
      setSessions(Array.isArray(data) ? data : []);
    } catch (e) {
      console.warn("loadBrowserSessions failed:", e);
      setSessions([]);
    } finally {
      setLoading(false);
    }
  };

  const fetchSessionDetail = useCallback(async (id: string) => {
    try {
      const data = await browserApi.getSession(id);
      setSelectedSession(data);
      setSessions((prev) => prev.map((s) => (s.id === id ? data : s)));
    } catch (e) {
      console.warn("fetchSessionDetail failed:", e);
    }
  }, []);

  useWebSocket({
    onMessage: (msg) => {
      if (msg.type === "browser_status") {
        loadSessions();
        const current = selectedRef.current;
        const payload = msg.payload as { session_id?: string };
        if (current && payload.session_id === current.id) {
          fetchSessionDetail(current.id);
        }
      }
    },
  });

  const handleSearch = (val: string) => {
    setSearch(val);
    setPage(0);
  };

  const filteredSessions = sessions.filter((s) => {
    if (!search.trim()) return true;
    const term = search.toLowerCase();
    return (
      s.name.toLowerCase().includes(term) ||
      s.current_url?.toLowerCase().includes(term) ||
      s.owner_agent_slug?.toLowerCase().includes(term)
    );
  });

  const totalPages = Math.max(
    1,
    Math.ceil(filteredSessions.length / PAGE_SIZE),
  );
  const paginatedSessions = filteredSessions.slice(
    page * PAGE_SIZE,
    (page + 1) * PAGE_SIZE,
  );

  const handleStart = async (e: React.MouseEvent, session: BrowserSession) => {
    e.stopPropagation();
    try {
      await browserApi.startSession(session.id);
      toast("success", "Session started");
      loadSessions();
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to start session",
      );
    }
  };

  const handleStop = async (e: React.MouseEvent, session: BrowserSession) => {
    e.stopPropagation();
    try {
      await browserApi.stopSession(session.id);
      toast("success", "Session stopped");
      loadSessions();
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to stop session",
      );
    }
  };

  const handleDelete = async (e: React.MouseEvent, session: BrowserSession) => {
    e.stopPropagation();
    try {
      await browserApi.deleteSession(session.id);
      toast("success", "Session deleted");
      if (selectedSession?.id === session.id) setSelectedSession(null);
      loadSessions();
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to delete session",
      );
    }
  };

  // Detail view
  if (selectedSession) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Browsers" />
        <div className="flex-1 overflow-y-auto p-4 md:p-6">
          <SessionDetail
            session={selectedSession}
            onBack={() => setSelectedSession(null)}
            onRefresh={() => fetchSessionDetail(selectedSession.id)}
          />
        </div>
      </div>
    );
  }

  // List view
  return (
    <div className="flex flex-col h-full">
      <Header title="Browsers" />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        {loading ? (
          <LoadingSpinner message="Loading browser sessions..." />
        ) : (
          <>
            <div className="flex items-center gap-3 mb-4">
              <SearchBar
                value={search}
                onChange={handleSearch}
                placeholder="Search sessions..."
                className="flex-1"
              />
              <ViewToggle view={view} onViewChange={setView} />
              <Button
                onClick={() => setShowNewModal(true)}
                icon={<Plus className="w-4 h-4" />}
              >
                New Session
              </Button>
            </div>

            {filteredSessions.length === 0 ? (
              <EmptyState
                icon={<Monitor className="w-8 h-8" />}
                title={
                  sessions.length === 0
                    ? "No browser sessions"
                    : "No sessions match your search"
                }
                description={
                  sessions.length === 0
                    ? "Create a browser session to start controlling web browsers."
                    : "Try a different search term."
                }
              />
            ) : view === "grid" ? (
              <>
                <Pagination
                  page={page}
                  totalPages={totalPages}
                  total={filteredSessions.length}
                  onPageChange={setPage}
                  label="sessions"
                />
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                  {paginatedSessions.map((session) => (
                    <BrowserCard
                      key={session.id}
                      session={session}
                      onClick={() => {
                        setSelectedSession(session);
                        fetchSessionDetail(session.id);
                      }}
                      onStart={(e) => handleStart(e, session)}
                      onStop={(e) => handleStop(e, session)}
                      onDelete={(e) => handleDelete(e, session)}
                    />
                  ))}
                </div>
              </>
            ) : (
              <>
                <Pagination
                  page={page}
                  totalPages={totalPages}
                  total={filteredSessions.length}
                  onPageChange={setPage}
                  label="sessions"
                />
                <div className="space-y-3">
                  {paginatedSessions.map((session) => (
                    <BrowserRow
                      key={session.id}
                      session={session}
                      onClick={() => {
                        setSelectedSession(session);
                        fetchSessionDetail(session.id);
                      }}
                      onStart={(e) => handleStart(e, session)}
                      onStop={(e) => handleStop(e, session)}
                      onDelete={(e) => handleDelete(e, session)}
                    />
                  ))}
                </div>
              </>
            )}
          </>
        )}
      </div>

      {showNewModal && (
        <NewSessionModal
          onClose={() => setShowNewModal(false)}
          onCreate={(session) => {
            setShowNewModal(false);
            setSessions((prev) => [session, ...prev]);
          }}
        />
      )}
    </div>
  );
}
