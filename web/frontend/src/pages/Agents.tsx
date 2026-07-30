import { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router";
import { Bot, Plus, Upload, X, Shield, Cpu, ClipboardList } from "lucide-react";
import { Header } from "../components/Header";
import { Button } from "../components/Button";
import { Card } from "../components/Card";
import { Input, Textarea } from "../components/Input";
import { Modal } from "../components/Modal";
import { EmptyState } from "../components/EmptyState";
import { LoadingSpinner } from "../components/LoadingSpinner";
import { Pagination } from "../components/Pagination";
import { SearchBar } from "../components/SearchBar";
import { ViewToggle } from "../components/ViewToggle";
import { usePersistentViewMode } from "../components/viewMode";
import { FolderFilter } from "../components/FolderFilter";
import { FolderSection } from "../components/FolderSection";
import { useFolderGrouping } from "../hooks/useFolderGrouping";
import { useToast } from "../components/Toast";
import { Toggle } from "../components/Toggle";
import { api, agentTasks, type AgentRole } from "../lib/api";
import { workspaces } from "../lib/api-helpers";
import { PRESET_AVATARS } from "../lib/avatars";


const MODEL_OPTIONS = [
  { id: "anthropic/claude-haiku-4.5", label: "Haiku 4.5" },
  { id: "anthropic/claude-sonnet-5", label: "Sonnet 5" },
  { id: "anthropic/claude-opus-5", label: "Opus 5" },
  { id: "anthropic/claude-fable-5", label: "Fable 5" },
];

const PROVIDER_LABELS: Record<string, string> = {
  "": "Chat default",
  openrouter: "OpenRouter",
  "claude-code": "Claude Code",
  codex: "Codex",
};

interface ProviderStatus {
  configured?: boolean;
  available?: boolean;
}

interface ProviderInfo {
  active: string;
  providers: Record<string, ProviderStatus>;
}

function providerUsable(name: string, status?: ProviderStatus) {
  if (name === "") return true;
  if (name === "openrouter") return Boolean(status?.configured);
  return Boolean(status?.available ?? status?.configured);
}

function formatProviderName(provider: string): string {
  return PROVIDER_LABELS[provider] || provider;
}

function formatModelName(model: string): string {
  const known: Record<string, string> = {
    "anthropic/claude-haiku-4.5": "Haiku 4.5",
    "anthropic/claude-sonnet-5": "Sonnet 5",
    "anthropic/claude-opus-5": "Opus 5",
    "anthropic/claude-fable-5": "Fable 5",
    // Legacy IDs kept so existing agents still render a friendly name.
    "anthropic/claude-haiku-4-5": "Haiku 4.5",
    "anthropic/claude-sonnet-4-6": "Sonnet 4.6",
    "anthropic/claude-opus-4-6": "Opus 4.6",
    haiku: "Haiku 4.5",
    sonnet: "Sonnet 5",
    opus: "Opus 5",
    fable: "Fable 5",
  };
  if (known[model]) return known[model];
  const parts = model.split("/");
  return parts[parts.length - 1];
}

function CreateAgentModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: (role: AgentRole) => void;
}) {
  const { toast } = useToast();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [model, setModel] = useState("anthropic/claude-sonnet-5");
  const [provider, setProvider] = useState("");
  const [providerInfo, setProviderInfo] = useState<ProviderInfo | null>(null);
  const [avatarPath, setAvatarPath] = useState(PRESET_AVATARS[0]);
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open || providerInfo) return;
    api.get<ProviderInfo>("/settings/llm-provider")
      .then(setProviderInfo)
      .catch((error) => console.warn("loadProviderInfo failed:", error));
  }, [open, providerInfo]);

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!["image/png", "image/jpeg", "image/webp"].includes(file.type)) {
      toast("error", "Please upload a PNG, JPEG, or WebP image");
      return;
    }
    setUploading(true);
    try {
      const formData = new FormData();
      formData.append("avatar", file);
      const csrfHeaders: Record<string, string> = {};
      const csrf = (await import("../lib/api")).getCSRFToken();
      if (csrf) csrfHeaders["X-CSRF-Token"] = csrf;
      const res = await fetch("/api/v1/agent-roles/upload-avatar", {
        method: "POST",
        headers: csrfHeaders,
        body: formData,
        credentials: "same-origin",
      });
      if (!res.ok) throw new Error("Upload failed");
      const data = await res.json();
      setAvatarPath(data.avatar_path);
      toast("success", "Avatar uploaded");
    } catch (e) {
      console.warn("uploadAvatar failed:", e);
      toast("error", "Failed to upload avatar");
    } finally {
      setUploading(false);
    }
  };

  const handleCreate = async () => {
    if (!name.trim()) {
      toast("error", "Name is required");
      return;
    }
    setSaving(true);
    try {
      const role = await api.post<AgentRole>("/agent-roles", {
        name: name.trim(),
        description: description.trim(),
        system_prompt: systemPrompt.trim(),
        model,
        provider,
        avatar_path: avatarPath,
      });
      onCreated(role);
      onClose();
      setName("");
      setDescription("");
      setSystemPrompt("");
      setModel("anthropic/claude-sonnet-5");
      setProvider("");
      setAvatarPath(PRESET_AVATARS[0]);
      toast("success", "Agent created");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to create agent",
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="Create Agent" size="md">
      <div className="space-y-5">
        <div>
          <label className="block text-xs font-medium text-text-1 mb-2">
            Avatar
          </label>
          {/* Full preset set in a scrolling grid — the same control the edit page
              uses. A flex-wrap row of every avatar would overflow the modal. */}
          <div className="w-full max-h-64 overflow-y-auto rounded-lg border border-border-1 bg-surface-0 p-2.5 mb-3">
            <div className="grid grid-cols-4 md:grid-cols-6 gap-2">
              {PRESET_AVATARS.map((path, i) => (
                <button
                  key={path}
                  onClick={() => setAvatarPath(path)}
                  aria-label={`Select avatar ${i + 1}`}
                  aria-pressed={avatarPath === path}
                  className={`aspect-square rounded-lg overflow-hidden border-2 transition-all cursor-pointer ${
                    avatarPath === path
                      ? "border-accent-primary ring-2 ring-accent-primary/20 scale-105"
                      : "border-transparent hover:border-border-0 opacity-70 hover:opacity-100"
                  }`}
                >
                  <img src={path} alt="" className="w-full h-full object-cover" />
                </button>
              ))}
            </div>
          </div>
          <div className="flex items-center gap-3 flex-wrap">
            <label
              className={`w-14 h-14 rounded-xl border-2 border-dashed flex items-center justify-center cursor-pointer transition-all ${
                !PRESET_AVATARS.includes(avatarPath)
                  ? "border-accent-primary bg-accent-muted"
                  : "border-border-1 hover:border-border-0"
              }`}
            >
              {uploading ? (
                <div className="w-5 h-5 border-2 border-text-3 border-t-transparent rounded-full animate-spin" />
              ) : !PRESET_AVATARS.includes(avatarPath) ? (
                <img
                  src={avatarPath}
                  alt="Custom"
                  className="w-full h-full rounded-xl"
                />
              ) : (
                <Upload className="w-4 h-4 text-text-3" />
              )}
              <input
                type="file"
                accept="image/png,image/jpeg,image/webp"
                onChange={handleUpload}
                className="hidden"
              />
            </label>
          </div>
        </div>

        <Input
          label="Name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. Atlas"
        />
        <Input
          label="Description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="e.g. Project Manager"
        />
        <Textarea
          label="System Prompt"
          value={systemPrompt}
          onChange={(e) => setSystemPrompt(e.target.value)}
          placeholder="You are a helpful AI assistant that..."
          rows={4}
        />

        <div>
          <label htmlFor="new-agent-provider" className="mb-2 flex items-center gap-1.5 text-xs font-medium text-text-1">
            <Cpu className="h-3.5 w-3.5" aria-hidden="true" />
            AI provider
          </label>
          <select
            id="new-agent-provider"
            value={provider}
            onChange={(event) => setProvider(event.target.value)}
            className="h-10 w-full rounded-lg border border-border-1 bg-surface-0 px-3 text-sm text-text-1 outline-none transition-colors focus:border-accent-primary focus:ring-2 focus:ring-accent-primary/20"
          >
            {["", "openrouter", "claude-code", "codex"].map((name) => {
              const usable = providerUsable(name, providerInfo?.providers?.[name]);
              const activeLabel = formatProviderName(providerInfo?.active || "openrouter");
              return (
                <option key={name || "default"} value={name} disabled={!usable}>
                  {formatProviderName(name)}
                  {name === "" ? ` (${activeLabel})` : usable ? "" : " — unavailable"}
                </option>
              );
            })}
          </select>
          <p className="mt-1.5 text-[11px] leading-relaxed text-text-3">
            Pinned agents keep this engine even when the chat default changes.
          </p>
        </div>

        <div>
          <label className="block text-xs font-medium text-text-1 mb-2">
            Model tier
          </label>
          <div className="flex gap-2">
            {MODEL_OPTIONS.map((opt) => (
              <button
                key={opt.id}
                type="button"
                onClick={() => setModel(opt.id)}
                className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-all cursor-pointer ${
                  model === opt.id
                    ? "bg-accent-primary/15 text-accent-primary border border-accent-primary/30"
                    : "bg-surface-2 text-text-3 border border-border-1 hover:border-border-0"
                }`}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={handleCreate}
            loading={saving}
            disabled={!name.trim()}
            icon={<Plus className="w-4 h-4" />}
          >
            Create Agent
          </Button>
        </div>
      </div>
    </Modal>
  );
}

const PAGE_SIZE = 12;

export function Agents() {
  const navigate = useNavigate();
  const { toast } = useToast();
  const [roles, setRoles] = useState<AgentRole[]>([]);
  const [loading, setLoading] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [view, setView] = usePersistentViewMode("agents", "grid");
  const [page, setPage] = useState(0);
  const [customFolders, setCustomFolders] = useState<string[]>([]);
  const [taskCounts, setTaskCounts] = useState<Record<string, number>>({});
  const [workspaceNames, setWorkspaceNames] = useState<Record<string, string>>({});

  const loadRoles = useCallback(() => {
    api
      .get<AgentRole[]>("/agent-roles")
      .then((data) => setRoles(data || []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    loadRoles();
    agentTasks.allCounts().then(setTaskCounts).catch(() => {});
    workspaces.list().then(list => {
      setWorkspaceNames(Object.fromEntries(list.map(w => [w.id, w.name])));
    }).catch((e) => { console.warn('load workspaces failed:', e); });
  }, [loadRoles]);

  const handleSearch = (val: string) => {
    setSearch(val);
    setPage(0);
  };

  const builderRole = roles.find((r) => r.slug === "builder");
  const gatewayName = builderRole?.name || "Gateway";

  const nonBuilderRoles = roles.filter((r) => r.slug !== "builder");
  const getFolder = useCallback((r: AgentRole) => r.folder || '', []);
  const folderGrouping = useFolderGrouping(nonBuilderRoles, getFolder);

  const searchFiltered = folderGrouping.filtered.filter((role) => {
    if (!search.trim()) return true;
    const term = search.toLowerCase();
    return (
      role.name.toLowerCase().includes(term) ||
      (role.description?.toLowerCase().includes(term) ?? false)
    );
  });

  const showFolderSections = folderGrouping.selectedFolder === null && folderGrouping.folders.length > 0;
  const totalPages = showFolderSections ? 1 : Math.max(1, Math.ceil(searchFiltered.length / PAGE_SIZE));
  const paginatedRoles = showFolderSections ? searchFiltered : searchFiltered.slice(
    page * PAGE_SIZE,
    (page + 1) * PAGE_SIZE,
  );

  const renderGatewayCard = () => (
    <div className="mb-4">
      <Card hover onClick={() => navigate("/agents/gateway")}>
        <div className="flex items-center gap-3 md:gap-4">
          <div className="relative flex-shrink-0">
            <img src={builderRole?.avatar_path || "/gateway-avatar.png"} alt={gatewayName} className="w-10 h-10 md:w-12 md:h-12 rounded-xl" />
            <div className="absolute -bottom-0.5 -right-0.5 w-4 h-4 rounded-full bg-accent-primary flex items-center justify-center ring-2 ring-surface-1">
              <Shield className="w-2.5 h-2.5 text-white" />
            </div>
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <p className="text-base font-bold text-text-0 truncate">{gatewayName}</p>
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-accent-primary/20 text-accent-primary font-medium">Gateway</span>
            </div>
            <p className="text-xs text-text-3 truncate">Routes conversations, builds services, dashboards, and agents</p>
          </div>
          {/* Both badges are detail the card's own text already implies, so on
              a phone they give way rather than squeezing the name. */}
          <span className="hidden sm:inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-surface-2 text-text-3 text-[10px] font-medium flex-shrink-0">
            <Cpu className="w-2.5 h-2.5" />{builderRole?.model ? formatModelName(builderRole.model) : "—"}
          </span>
          <span className="hidden sm:inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-accent-primary/10 text-accent-primary text-[10px] font-medium flex-shrink-0">
            <span className="w-1.5 h-1.5 rounded-full bg-accent-primary" />Always active
          </span>
        </div>
      </Card>
    </div>
  );

  const renderAgentContent = (items: AgentRole[]) => view === "grid" ? (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 md:gap-4">
      {items.map((role) => (
        <Card key={role.slug} hover onClick={() => navigate(`/agents/${role.slug}`)}>
          <div className="relative">
            <div className="absolute top-0 right-0" onClick={(e) => e.stopPropagation()}>
              <Toggle enabled={role.enabled} onChange={() => toggleRole(role.slug, { stopPropagation: () => {} } as React.MouseEvent)} label="Enable agent" />
            </div>
          </div>
          <div className="flex items-center gap-3 md:gap-4 mb-3">
            <img src={role.avatar_path} alt={role.name} className="w-12 h-12 md:w-14 md:h-14 rounded-2xl shadow-lg flex-shrink-0" />
            <h3 className="text-lg md:text-xl font-bold text-text-0 pr-14 flex-1 min-w-0 truncate">{role.name}</h3>
          </div>
          <p className="text-sm text-text-2 line-clamp-1 mb-3 leading-snug">{role.description}</p>
          <div className="flex items-center gap-2 flex-wrap">
            <span className="inline-flex items-center gap-1 text-[10px] text-text-3 font-medium">
              <Cpu className="w-3 h-3" />
              {formatProviderName(role.provider || "")} · {formatModelName(role.model)}
            </span>
            {role.workspace_id && (
              <span className="px-1.5 py-0.5 rounded text-[10px] font-medium bg-surface-2 text-text-3" title="Only available in this workspace">
                {workspaceNames[role.workspace_id] || "Workspace-only"}
              </span>
            )}
            {(taskCounts[role.slug] || 0) > 0 && (
              <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-full bg-blue-500/15 text-blue-400 text-[10px] font-medium">
                <ClipboardList className="w-2.5 h-2.5" />
                {taskCounts[role.slug]} {taskCounts[role.slug] === 1 ? 'task' : 'tasks'}
              </span>
            )}
          </div>
        </Card>
      ))}
    </div>
  ) : (
    <div className="space-y-3">
      {items.map((role) => (
        <Card key={role.slug} hover onClick={() => navigate(`/agents/${role.slug}`)}>
          <div className="flex items-center gap-3 md:gap-4">
            <img src={role.avatar_path} alt={role.name} className="w-10 h-10 md:w-12 md:h-12 rounded-xl flex-shrink-0" />
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <p className="text-base font-bold pt-2 text-text-0 truncate">{role.name}</p>
                {role.workspace_id && (
                  <span className="px-1.5 py-0.5 rounded text-[10px] font-medium bg-surface-2 text-text-3 flex-shrink-0" title="Only available in this workspace">
                    {workspaceNames[role.workspace_id] || "Workspace-only"}
                  </span>
                )}
              </div>
              <p className="text-xs text-text-3 truncate">{role.description}</p>
            </div>
            <span className="hidden sm:inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-surface-2 text-text-3 text-[10px] font-medium flex-shrink-0">
              <Cpu className="w-2.5 h-2.5" />{formatProviderName(role.provider || "")} · {formatModelName(role.model)}
            </span>
            {(taskCounts[role.slug] || 0) > 0 && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-blue-500/15 text-blue-400 text-[10px] font-medium flex-shrink-0">
                <ClipboardList className="w-2.5 h-2.5" />{taskCounts[role.slug]}
              </span>
            )}
            <div className="flex items-center gap-1.5 md:gap-2 flex-shrink-0">
              {!role.is_preset && (
                <button onClick={(e) => deleteRole(role.slug, role.name, e)} className="p-1.5 rounded-lg text-text-3 hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer" title="Delete agent">
                  <X className="w-4 h-4" />
                </button>
              )}
              <div onClick={(e) => e.stopPropagation()}>
                <Toggle enabled={role.enabled} onChange={() => toggleRole(role.slug, { stopPropagation: () => {} } as React.MouseEvent)} label="Enable agent" />
              </div>
            </div>
          </div>
        </Card>
      ))}
    </div>
  );

  const toggleRole = async (slug: string, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      const result = await api.put<{ slug: string; enabled: boolean }>(
        `/agent-roles/${slug}/toggle`,
      );
      setRoles((prev) =>
        prev.map((r) =>
          r.slug === slug ? { ...r, enabled: result.enabled } : r,
        ),
      );
      toast("success", `Agent ${result.enabled ? "enabled" : "disabled"}`);
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to toggle agent",
      );
    }
  };

  const deleteRole = async (
    slug: string,
    name: string,
    e: React.MouseEvent,
  ) => {
    e.stopPropagation();
    if (!confirm(`Delete agent "${name}"? This cannot be undone.`)) return;
    try {
      await api.delete(`/agent-roles/${slug}`);
      setRoles((prev) => prev.filter((r) => r.slug !== slug));
      toast("success", "Agent deleted");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to delete agent",
      );
    }
  };

  return (
    <div className="flex flex-col h-full">
      <Header title="Agents" />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        {loading ? (
          <LoadingSpinner message="Loading agents..." />
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2 md:gap-3 mb-4">
              <SearchBar
                value={search}
                onChange={handleSearch}
                placeholder="Search agents..."
                className="flex-1 min-w-[10rem]"
              />
              <ViewToggle view={view} onViewChange={setView} />
              <Button
                onClick={() => setCreateOpen(true)}
                icon={<Plus className="w-4 h-4" />}
              >
                Add Agent
              </Button>
            </div>
            <FolderFilter
              folders={[...folderGrouping.folders, ...customFolders.filter(f => !folderGrouping.folders.includes(f))].sort((a, b) => a.localeCompare(b))}
              folderCounts={folderGrouping.folderCounts}
              unfiledCount={folderGrouping.unfiledCount}
              totalCount={folderGrouping.totalCount}
              selectedFolder={folderGrouping.selectedFolder}
              onSelect={(f) => { folderGrouping.setSelectedFolder(f); setPage(0); }}
              onAddFolder={(name) => {
                if (!customFolders.includes(name)) {
                  setCustomFolders(prev => [...prev, name]);
                }
                folderGrouping.setSelectedFolder(name);
                setPage(0);
                toast("success", `Folder "${name}" created — assign agents to it from their detail page`);
              }}
            />

            {searchFiltered.length === 0 ? (
              <EmptyState
                icon={<Bot className="w-8 h-8" />}
                title={
                  search.trim()
                    ? "No agents match your search"
                    : "No agents yet"
                }
                description={
                  search.trim()
                    ? "Try a different search term."
                    : "Install from Templates or create a custom agent."
                }
              />
            ) : showFolderSections ? (
              <div className="space-y-4">
                {renderGatewayCard()}
                {folderGrouping.folders.map((folder) => {
                  const items = (folderGrouping.grouped.get(folder) || []).filter(role => {
                    if (!search.trim()) return true;
                    const term = search.toLowerCase();
                    return role.name.toLowerCase().includes(term) || (role.description?.toLowerCase().includes(term) ?? false);
                  });
                  if (items.length === 0) return null;
                  return (
                    <FolderSection key={folder} name={folder} count={items.length}>
                      {renderAgentContent(items)}
                    </FolderSection>
                  );
                })}
                {(() => {
                  const unfiled = (folderGrouping.grouped.get('') || []).filter(role => {
                    if (!search.trim()) return true;
                    const term = search.toLowerCase();
                    return role.name.toLowerCase().includes(term) || (role.description?.toLowerCase().includes(term) ?? false);
                  });
                  if (unfiled.length === 0) return null;
                  return (
                    <FolderSection name="" count={unfiled.length}>
                      {renderAgentContent(unfiled)}
                    </FolderSection>
                  );
                })()}
              </div>
            ) : (
              <>
                <Pagination
                  page={page}
                  totalPages={totalPages}
                  total={searchFiltered.length}
                  onPageChange={setPage}
                  label="agents"
                />
                {folderGrouping.selectedFolder === null && renderGatewayCard()}
                {renderAgentContent(paginatedRoles)}
              </>
            )}
          </>
        )}
      </div>

      <CreateAgentModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(role) => {
          setRoles((prev) => [...prev, role]);
        }}
      />
    </div>
  );
}
