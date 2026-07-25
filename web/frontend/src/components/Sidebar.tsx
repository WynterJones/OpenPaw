import { useState, useEffect, useRef } from "react";
import { NavLink, Link, useLocation } from "react-router";
import {
  MessageSquare,
  Wrench,
  TerminalSquare,
  Bot,
  Sparkles,
  KeyRound,
  LayoutDashboard,
  Clock,
  FileText,
  Settings,
  Store,
  Database,
  Heart,
  ListTodo,
  ChevronDown,
  ChevronsUpDown,
  Check,
  Plus,
  MoreHorizontal,
  ImagePlus,
  Settings2,
  Trash2,
  Brain,
} from "lucide-react";
import { api, type Dashboard } from "../lib/api";
import { workspaces } from "../lib/api-helpers";
import type { Workspace } from "../lib/types";
import { startWindowDrag } from "../lib/tauri";
import { Button } from "./Button";
import { useOpenRouterBalance } from "../hooks/useOpenRouterBalance";
import { providerName } from "../lib/provider";

type NavItem = {
  to: string;
  icon: typeof LayoutDashboard;
  label: string;
  featured?: boolean;
};
type NavGroup = { items: NavItem[] };

const navGroups: NavGroup[] = [
  {
    items: [
      { to: "/chat", icon: MessageSquare, label: "Chats" },
      { to: "/knowledge-base", icon: Database, label: "Context" },
      { to: "/todo-lists", icon: ListTodo, label: "Tasks" },
    ],
  },
];

const moreItems: NavItem[] = [
  { to: "/agents", icon: Bot, label: "Agents" },
  { to: "/tools", icon: Wrench, label: "Tools" },
  { to: "/workbench", icon: TerminalSquare, label: "Workbench" },
  { to: "/skills", icon: Sparkles, label: "Skills" },
  { to: "/library", icon: Store, label: "Templates" },
  { to: "/scheduler", icon: Clock, label: "Scheduler" },
  { to: "/heartbeat", icon: Heart, label: "Heartbeat" },
  { to: "/secrets", icon: KeyRound, label: "Secrets" },
  { to: "/logs", icon: FileText, label: "Logs" },
  { to: "/settings", icon: Settings, label: "Settings" },
];

function WorkspaceSwitcher({ collapsed }: { collapsed: boolean }) {
  const [open, setOpen] = useState(false);
  const [list, setList] = useState<Workspace[]>([]);
  const [active, setActive] = useState<Workspace | null>(null);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [uploading, setUploading] = useState(false);
  const [genOpen, setGenOpen] = useState(false);
  const [genPrompt, setGenPrompt] = useState("");
  const [generating, setGenerating] = useState(false);
  const [genError, setGenError] = useState<string | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const ref = useRef<HTMLDivElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const load = async () => {
    try {
      const [all, act] = await Promise.all([
        workspaces.list(),
        workspaces.getActive(),
      ]);
      setList(Array.isArray(all) ? all : []);
      setActive(act ?? null);
    } catch {
      /* ignore — leave the switcher in its default state */
    }
  };

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load();
  }, []);

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, []);

  // Switching the active workspace re-scopes everything server-side, so the
  // simplest reliable refresh is a full reload after the setActive call.
  const switchTo = async (id: string) => {
    setOpen(false);
    if (active && id === active.id) return;
    try {
      await workspaces.setActive(id);
      window.location.reload();
    } catch {
      /* ignore */
    }
  };

  const submitCreate = async () => {
    const name = newName.trim();
    if (!name) return;
    try {
      const ws = await workspaces.create(name);
      await workspaces.setActive(ws.id);
      window.location.reload();
    } catch {
      /* ignore */
    }
  };

  const startEdit = (ws: Workspace) => {
    setEditingId(ws.id);
    setEditName(ws.name);
  };

  const saveEdit = async (id: string) => {
    const name = editName.trim();
    if (!name) return;
    try {
      await workspaces.rename(id, name);
      setEditingId(null);
      await load();
    } catch {
      /* ignore */
    }
  };

  const deleteWorkspace = async (ws: Workspace) => {
    if (!window.confirm(`Delete workspace "${ws.name}"? Its chats, dashboards, context and tasks move to Default.`)) return;
    try {
      await workspaces.remove(ws.id);
      // If the active workspace was deleted the server switched to Default — reload to re-scope.
      if (active?.id === ws.id) {
        window.location.reload();
        return;
      }
      setEditingId(null);
      await load();
    } catch {
      /* ignore */
    }
  };

  const uploadImage = async (file: File) => {
    if (!active) return;
    setUploading(true);
    try {
      const { image_url } = await workspaces.uploadImage(file);
      await workspaces.setImage(active.id, image_url);
      await load();
    } catch {
      /* ignore */
    } finally {
      setUploading(false);
    }
  };

  const generateImage = async () => {
    if (!active || !genPrompt.trim()) return;
    setGenerating(true);
    setGenError(null);
    try {
      await workspaces.generateImage(active.id, genPrompt.trim());
      await load();
      setGenOpen(false);
      setGenPrompt("");
    } catch (e) {
      setGenError(e instanceof Error ? e.message : "Generation failed");
    } finally {
      setGenerating(false);
    }
  };

  const activeName = active?.name ?? "Workspace";

  const badge = (ws: Workspace | null, size: string) =>
    ws?.image_url ? (
      <img
        src={ws.image_url}
        alt=""
        className={`flex-shrink-0 ${size} rounded-md object-cover`}
      />
    ) : (
      <span
        className={`flex-shrink-0 ${size} rounded-md bg-accent-primary/15 text-accent-text text-xs font-bold flex items-center justify-center`}
      >
        {(ws?.name ?? activeName).charAt(0).toUpperCase() || "W"}
      </span>
    );

  return (
    <div ref={ref} className="relative px-2 pt-1 pb-1">
      <button
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-haspopup="menu"
        title={collapsed ? activeName : "Switch workspace"}
        className={`w-full flex items-center gap-2 rounded-lg border border-border-0 bg-surface-2 hover:bg-surface-3 transition-colors cursor-pointer ${collapsed ? "justify-center p-2" : "px-2.5 py-2"}`}
      >
        {badge(active, "w-6 h-6")}
        {!collapsed && (
          <>
            <span className="flex-1 min-w-0 text-left text-sm font-medium text-text-1 truncate">
              {activeName}
            </span>
            <ChevronsUpDown className="w-4 h-4 text-text-3 flex-shrink-0" aria-hidden="true" />
          </>
        )}
      </button>

      {open && (
        <div
          className={`absolute z-50 rounded-lg border border-border-0 bg-surface-1 shadow-xl py-1 ${collapsed ? "left-full top-1 ml-2 w-52" : "left-2 right-2 top-full mt-1"}`}
          role="menu"
        >
          <p className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-text-3">
            Workspaces
          </p>
          <div className="max-h-64 overflow-y-auto">
            {list.length === 0 ? (
              <p className="px-3 py-1.5 text-xs text-text-3">No workspaces</p>
            ) : (
              list.map((ws) => (
                <div key={ws.id}>
                  <div className="group w-full flex items-center gap-2 pl-3 pr-1.5 py-1.5 text-sm text-text-1 hover:bg-surface-2 transition-colors">
                    <button
                      role="menuitem"
                      onClick={() => switchTo(ws.id)}
                      className="flex-1 min-w-0 flex items-center gap-2 cursor-pointer"
                    >
                      {badge(ws, "w-5 h-5")}
                      <span className="flex-1 min-w-0 text-left truncate">{ws.name}</span>
                    </button>
                    {active?.id === ws.id && (
                      <Check className="w-3.5 h-3.5 text-accent-text flex-shrink-0" aria-hidden="true" />
                    )}
                    <button
                      onClick={(e) => { e.stopPropagation(); if (editingId === ws.id) { setEditingId(null); } else { startEdit(ws); } }}
                      title="Rename or delete workspace"
                      aria-label={`Edit ${ws.name}`}
                      className={`p-1 rounded-md flex-shrink-0 transition-colors cursor-pointer ${editingId === ws.id ? "text-accent-text bg-surface-3" : "text-text-3 opacity-0 group-hover:opacity-100 hover:text-text-1 hover:bg-surface-3"}`}
                    >
                      <Settings2 className="w-3.5 h-3.5" aria-hidden="true" />
                    </button>
                  </div>
                  {editingId === ws.id && (
                    <div className="px-2 py-1.5 space-y-1.5 bg-surface-2/50">
                      <input
                        autoFocus
                        value={editName}
                        onChange={(e) => setEditName(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") saveEdit(ws.id);
                          if (e.key === "Escape") setEditingId(null);
                        }}
                        className="w-full rounded-md border border-border-1 bg-surface-0 text-text-1 px-2.5 py-1.5 text-sm focus:border-accent-primary focus:ring-1 focus:ring-accent-primary outline-none"
                      />
                      <div className="flex gap-1.5">
                        <Button onClick={() => saveEdit(ws.id)} disabled={!editName.trim()} size="sm" className="flex-1">
                          Save
                        </Button>
                        <Button variant="secondary" size="sm" onClick={() => setEditingId(null)}>
                          Cancel
                        </Button>
                        {!ws.is_default && (
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => deleteWorkspace(ws)}
                            className="!text-danger hover:!bg-danger/10"
                            aria-label="Delete workspace"
                          >
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
          {active && (
            <>
              <div className="my-1 border-t border-border-0" />
              <input
                ref={fileRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) uploadImage(f);
                  e.target.value = "";
                }}
              />
              <div className="flex items-center px-1">
                <button
                  role="menuitem"
                  disabled={uploading || generating}
                  onClick={() => fileRef.current?.click()}
                  className="flex-1 flex items-center gap-2 px-2 py-1.5 rounded-md text-sm text-text-2 hover:bg-surface-2 hover:text-text-1 transition-colors cursor-pointer disabled:opacity-50"
                >
                  <ImagePlus className="w-4 h-4 flex-shrink-0" aria-hidden="true" />
                  <span className="flex-1 text-left">
                    {uploading ? "Uploading…" : active.image_url ? "Change image" : "Add image"}
                  </span>
                </button>
                <button
                  role="menuitem"
                  title="Generate an image with AI"
                  aria-label="Generate image with AI"
                  disabled={uploading || generating}
                  onClick={() => { setGenOpen((o) => !o); setGenError(null); }}
                  className={`p-1.5 rounded-md transition-colors cursor-pointer disabled:opacity-50 ${genOpen ? "bg-accent-primary/15 text-accent-text" : "text-text-3 hover:bg-surface-2 hover:text-accent-text"}`}
                >
                  <Sparkles className="w-4 h-4" aria-hidden="true" />
                </button>
              </div>
              {genOpen && (
                <div className="px-2 pt-1 pb-1.5 space-y-1.5">
                  <textarea
                    autoFocus
                    value={genPrompt}
                    onChange={(e) => setGenPrompt(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) generateImage();
                      if (e.key === "Escape") setGenOpen(false);
                    }}
                    placeholder="Describe the workspace image…"
                    rows={2}
                    className="w-full resize-none rounded-md border border-border-1 bg-surface-0 text-text-1 px-2.5 py-1.5 text-sm placeholder:text-text-3/60 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary outline-none"
                  />
                  {genError && (
                    <p className="text-[11px] text-danger leading-snug">{genError}</p>
                  )}
                  <Button
                    onClick={generateImage}
                    disabled={!genPrompt.trim() || generating}
                    icon={<Sparkles className="w-4 h-4" />}
                    size="sm"
                    className="w-full"
                  >
                    {generating ? "Generating…" : "Generate"}
                  </Button>
                </div>
              )}
            </>
          )}
          <div className="my-1 border-t border-border-0" />
          <div className="px-2 py-1">
            {creating ? (
              <div className="space-y-1.5">
                <input
                  autoFocus
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") submitCreate();
                    if (e.key === "Escape") { setCreating(false); setNewName(""); }
                  }}
                  placeholder="Workspace name"
                  className="w-full rounded-md border border-border-1 bg-surface-0 text-text-1 px-2.5 py-1.5 text-sm placeholder:text-text-3/60 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary outline-none"
                />
                <div className="flex gap-1.5">
                  <Button
                    onClick={submitCreate}
                    disabled={!newName.trim()}
                    icon={<Plus className="w-4 h-4" />}
                    size="sm"
                    className="flex-1"
                  >
                    Create
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => { setCreating(false); setNewName(""); }}
                  >
                    Cancel
                  </Button>
                </div>
              </div>
            ) : (
              <Button
                onClick={() => setCreating(true)}
                icon={<Plus className="w-4 h-4" />}
                size="sm"
                className="w-full"
              >
                New workspace
              </Button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function DashboardsNav({ collapsed }: { collapsed: boolean }) {
  const [open, setOpen] = useState(false);
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const ref = useRef<HTMLDivElement>(null);
  const location = useLocation();
  const activeId = new URLSearchParams(location.search).get("id");
  const activeDashboard =
    location.pathname === "/dashboards"
      ? dashboards.find((d) => d.id === activeId) ?? null
      : null;

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, []);

  const loadDashboards = async () => {
    try {
      const data = await api.get<Dashboard[]>("/dashboards");
      setDashboards(Array.isArray(data) ? data : []);
    } catch {
      setDashboards([]);
    }
  };

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    loadDashboards();
  }, []);

  if (collapsed) {
    return (
      <NavLink
        to="/dashboards"
        title="Dashboards"
        className={({ isActive }) =>
          `flex items-center justify-center px-3 py-3 rounded-lg text-sm font-medium transition-all duration-150 ${
            isActive
              ? "bg-accent-primary/15 text-accent-text"
              : "text-text-2 hover:text-text-1 hover:bg-surface-2"
          }`
        }
      >
        <LayoutDashboard className="flex-shrink-0 w-5 h-5" />
      </NavLink>
    );
  }

  return (
    <div ref={ref} className="relative">
      {/* Presented like the workspace switcher: one control showing the current
          dashboard, opening a floating list — rather than an inline accordion
          that pushed the rest of the nav down. */}
      <button
        onClick={() => {
          setOpen((o) => !o);
          if (!open) loadDashboards();
        }}
        aria-expanded={open}
        aria-haspopup="menu"
        title="Switch dashboard"
        className="w-full flex items-center gap-3 rounded-lg border border-border-0 bg-surface-2 hover:bg-surface-3 transition-colors cursor-pointer px-2.5 py-2"
      >
        <LayoutDashboard className="flex-shrink-0 w-5 h-5 text-accent-text" aria-hidden="true" />
        <span className="flex-1 min-w-0 text-left text-sm font-medium text-text-1 truncate">
          {activeDashboard ? activeDashboard.name : "Dashboards"}
        </span>
        <ChevronsUpDown className="w-4 h-4 text-text-3 flex-shrink-0" aria-hidden="true" />
      </button>

      {open && (
        <div
          className="absolute z-50 left-0 right-0 top-full mt-1 rounded-lg border border-border-0 bg-surface-1 shadow-xl py-1"
          role="menu"
        >
          <p className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-text-3">
            Dashboards
          </p>
          <div className="max-h-64 overflow-y-auto">
            {dashboards.length === 0 ? (
              <p className="px-3 py-2 text-xs text-text-3">No dashboards yet</p>
            ) : (
              dashboards.map((d) => {
                const isActive =
                  location.pathname === "/dashboards" && activeId === d.id;
                return (
                  <Link
                    key={d.id}
                    to={`/dashboards?id=${d.id}`}
                    title={d.name}
                    onClick={() => setOpen(false)}
                    role="menuitem"
                    className={`flex items-center gap-2 px-3 py-2 text-sm truncate transition-colors ${
                      isActive
                        ? "bg-accent-muted text-accent-text"
                        : "text-text-2 hover:text-text-1 hover:bg-surface-2"
                    }`}
                  >
                    <span className="w-4 flex-shrink-0">
                      {isActive && <Check className="w-3.5 h-3.5" aria-hidden="true" />}
                    </span>
                    <span className="truncate">{d.name}</span>
                  </Link>
                );
              })
            )}
          </div>
          <div className="border-t border-border-0 mt-1 pt-1">
            <Link
              to="/dashboards"
              onClick={() => setOpen(false)}
              role="menuitem"
              className="flex items-center gap-2 px-3 py-2 text-sm text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors"
            >
              <span className="w-4 flex-shrink-0" />
              All dashboards
            </Link>
          </div>
        </div>
      )}
    </div>
  );
}

function MoreNav({ collapsed }: { collapsed: boolean }) {
  const location = useLocation();
  const hasActive = moreItems.some((i) => location.pathname === i.to);
  const [open, setOpen] = useState(hasActive);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (hasActive) setOpen(true);
  }, [hasActive]);

  if (collapsed) {
    return (
      <div className="space-y-0.5">
        {moreItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            title={item.label}
            className={({ isActive }) =>
              `flex items-center justify-center px-3 py-2.5 rounded-lg text-sm font-medium transition-all duration-150 ${
                isActive
                  ? "bg-accent-muted text-accent-text"
                  : "text-text-2 hover:text-text-1 hover:bg-surface-2"
              }`
            }
          >
            <item.icon className="flex-shrink-0 w-5 h-5" />
          </NavLink>
        ))}
      </div>
    );
  }

  return (
    <div>
      <button
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-all duration-150 ${
          open
            ? "text-text-1"
            : "text-text-2 hover:text-text-1 hover:bg-surface-2"
        }`}
      >
        <MoreHorizontal className="flex-shrink-0 w-5 h-5" />
        <span className="flex-1 text-left">More</span>
        <ChevronDown
          className={`w-4 h-4 text-text-3 transition-transform ${open ? "rotate-180" : ""}`}
          aria-hidden="true"
        />
      </button>

      {open && (
        <div className="mt-0.5 ml-4 pl-2 border-l border-border-0 space-y-0.5">
          {moreItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-all duration-150 ${
                  isActive
                    ? "bg-accent-muted text-accent-text"
                    : "text-text-2 hover:text-text-1 hover:bg-surface-2"
                }`
              }
            >
              <item.icon className="flex-shrink-0 w-4 h-4" />
              <span>{item.label}</span>
            </NavLink>
          ))}
        </div>
      )}
    </div>
  );
}

interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
}

export function Sidebar({ collapsed }: SidebarProps) {
  const balance = useOpenRouterBalance();
  const [version, setVersion] = useState("");
  useEffect(() => {
    api.get<{ version: string }>("/system/info").then((d) => setVersion(d.version || "")).catch(() => {});
  }, []);
  const openWynter = (e: React.MouseEvent) => {
    e.preventDefault();
    api.post("/system/open-external", { url: "https://wynter.ai" }).catch(() => {});
  };
  return (
    <aside
      data-tauri-drag-region
      onMouseDown={startWindowDrag}
      className={`hidden md:flex flex-col bg-surface-1 border-r border-border-0 transition-all duration-200 relative z-[1] ${collapsed ? "w-16" : "w-56"}`}
    >
      <div
        data-tauri-drag-region
        className={`flex flex-col flex-shrink-0 pt-9 pb-2.5 ${collapsed ? "items-center px-2.5" : "items-start px-4"}`}
      >
        <img
          src="/logo-transparent.png"
          alt="OpenPaw"
          className={`object-contain pointer-events-none select-none ${collapsed ? "h-4 w-auto" : "w-full h-auto"}`}
        />
      </div>
      <WorkspaceSwitcher collapsed={collapsed} />
      <nav data-tauri-drag-region className="op-sidebar-nav flex-1 px-2 pb-3 overflow-y-auto">
        <DashboardsNav collapsed={collapsed} />
        {navGroups.map((group, gi) => (
          <div key={gi}>
            <div className="mx-3 my-2 border-b border-border-0" />
            <div className="space-y-0.5">
              {group.items.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    `flex items-center gap-3 px-3 rounded-lg text-sm font-medium transition-all duration-150 ${
                      item.featured ? "py-3" : "py-2.5"
                    } ${
                      isActive
                        ? item.featured
                          ? "bg-accent-primary/15 text-accent-text"
                          : "bg-accent-muted text-accent-text"
                        : item.featured
                          ? "text-text-2 hover:text-text-1 hover:bg-surface-2"
                          : "text-text-2 hover:text-text-1 hover:bg-surface-2"
                    } ${collapsed ? "justify-center" : ""}`
                  }
                  title={collapsed ? item.label : undefined}
                >
                  <item.icon className="flex-shrink-0 w-5 h-5" />
                  {!collapsed && (
                    <span className={item.featured ? "font-semibold" : ""}>
                      {item.label}
                    </span>
                  )}
                </NavLink>
              ))}
            </div>
          </div>
        ))}
        <div className="mx-3 my-2 border-b border-border-0" />
        <MoreNav collapsed={collapsed} />
      </nav>

      {/* The show/hide control lives in the header's view-options menu, so the
          sidebar no longer carries its own toggle button. */}
      <div className="border-t border-border-0">
        {!collapsed && (
          <div className="px-4 pb-4 pt-3">
            <p className="text-[11px] font-medium text-text-3 mb-1.5 flex items-center gap-1.5">
              <Brain className="w-3.5 h-3.5 flex-shrink-0" aria-hidden="true" />
              {providerName(balance)}
            </p>
            <p className="text-[10px] text-text-3 mb-2" aria-hidden="true">
              &copy; OpenPaw{version ? ` - v${version}` : ""}
            </p>
            <a
              href="https://wynter.ai"
              onClick={openWynter}
              title="Made by Wynter — visit wynter.ai"
              className="block w-[40%] opacity-50 hover:opacity-90 transition-opacity cursor-pointer"
            >
              <img
                src="/wynter-logo.png"
                alt="Wynter"
                className="w-full h-auto object-contain select-none"
              />
            </a>
          </div>
        )}
      </div>
    </aside>
  );
}
