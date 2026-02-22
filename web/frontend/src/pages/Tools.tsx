import { useState, useEffect, useRef, useCallback } from "react";
import {
  Wrench,
  Clock,
  ArrowLeft,
  Play,
  Square,
  Power,
  RefreshCw,
  Hammer,
  Trash2,
  Pencil,
  Check,
  X,
  Copy,
  Globe,
  User,
  Info,
  Library,
  Download,
  Upload,
  Shield,
  ShieldCheck,
  ShieldAlert,
  AlertTriangle,
  KeyRound,
} from "lucide-react";
import { Header } from "../components/Header";
import { Button } from "../components/Button";
import { Card } from "../components/Card";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { Pagination } from "../components/Pagination";
import { SearchBar } from "../components/SearchBar";
import { ViewToggle, type ViewMode } from "../components/ViewToggle";
import { api, toolExtra, toolLibrary, secretsApi, type Tool, type LibraryTool, type ToolEndpoint, type ToolIntegrityInfo, type SecretCheckResult } from "../lib/api";
import { useToast } from "../components/Toast";
import { useWebSocket } from "../lib/useWebSocket";

function statusDot(status: string) {
  const colors: Record<string, string> = {
    running: "bg-emerald-400",
    starting: "bg-yellow-400",
    stopped: "bg-text-3",
    error: "bg-red-400",
    building: "bg-blue-400",
    compiling: "bg-blue-400",
    active: "bg-emerald-400",
  };
  return (
    <span
      className={`inline-block w-2 h-2 rounded-full ${colors[status] || "bg-text-3"}`}
    />
  );
}

function ToolCard({ tool, onClick, needsSecrets }: { tool: Tool; onClick: () => void; needsSecrets?: boolean }) {
  return (
    <Card hover onClick={onClick}>
      <div className="flex items-center gap-2 mb-3">
        {statusDot(tool.status)}
        <StatusBadge status={tool.status} />
        {tool.library_slug && (
          <span className="px-1.5 py-0.5 rounded text-[10px] bg-purple-500/15 text-purple-400 border border-purple-500/20 flex items-center gap-1">
            <Library className="w-2.5 h-2.5" />
            {tool.library_slug}
          </span>
        )}
        {needsSecrets && (
          <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-amber-500/15 text-amber-400 border border-amber-500/20 flex items-center gap-1">
            <AlertTriangle className="w-2.5 h-2.5" />
            Needs secrets
          </span>
        )}
      </div>
      <h3 className="text-base font-semibold text-text-0 mb-1">{tool.name}</h3>
      <p className="text-sm text-text-2 line-clamp-2 mb-3 leading-snug">
        {tool.description}
      </p>
      <div className="flex items-center justify-between text-xs text-text-3">
        {tool.port > 0 ? (
          <span>:{tool.port}</span>
        ) : (
          <span>&nbsp;</span>
        )}
        <span className="flex items-center gap-1">
          <Clock className="w-3 h-3" />
          {new Date(tool.created_at).toLocaleDateString()}
        </span>
      </div>
    </Card>
  );
}

function ToolRow({ tool, onClick }: { tool: Tool; onClick: () => void }) {
  return (
    <tr
      onClick={onClick}
      tabIndex={0}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick(); } }}
      className="border-b border-border-0/50 transition-colors hover:bg-surface-2/50 cursor-pointer focus:bg-surface-2/50 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-primary"
    >
      <td className="px-3 md:px-4 py-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <p className="text-sm font-medium text-text-0 truncate">{tool.name}</p>
            {tool.library_slug && (
              <span className="px-1.5 py-0.5 rounded text-[9px] bg-purple-500/15 text-purple-400 border border-purple-500/20 flex-shrink-0">
                <Library className="w-2.5 h-2.5 inline" />
              </span>
            )}
          </div>
          <p className="text-xs text-text-3 truncate max-w-xs">
            {tool.description}
          </p>
        </div>
      </td>
      <td className="px-3 md:px-4 py-3">
        <div className="flex items-center gap-2">
          {statusDot(tool.status)}
          <StatusBadge status={tool.status} />
        </div>
      </td>
      <td className="px-3 md:px-4 py-3 text-sm text-text-2 hidden md:table-cell">
        {tool.port > 0 ? `:${tool.port}` : "--"}
      </td>
      <td className="px-3 md:px-4 py-3 text-sm text-text-2 hidden md:table-cell">
        {tool.pid > 0 ? tool.pid : "--"}
      </td>
    </tr>
  );
}

const methodColors: Record<string, string> = {
  GET: "bg-emerald-500/15 text-emerald-400 border-emerald-500/20",
  POST: "bg-blue-500/15 text-blue-400 border-blue-500/20",
  PUT: "bg-amber-500/15 text-amber-400 border-amber-500/20",
  PATCH: "bg-orange-500/15 text-orange-400 border-orange-500/20",
  DELETE: "bg-red-500/15 text-red-400 border-red-500/20",
};

function EndpointCard({ endpoint }: { endpoint: ToolEndpoint }) {
  const color = methodColors[endpoint.method] ?? "bg-text-3/15 text-text-2 border-text-3/20";
  const params = [...(endpoint.path_params ?? []), ...(endpoint.query_params ?? [])];

  return (
    <div className="rounded-lg bg-surface-2 p-3 space-y-2">
      <div className="flex items-center gap-2">
        <span className={`px-2 py-0.5 rounded text-[10px] font-bold uppercase border ${color}`}>
          {endpoint.method}
        </span>
        <code className="text-sm font-mono text-text-0">{endpoint.path}</code>
      </div>
      {endpoint.description && (
        <p className="text-xs text-text-2">{endpoint.description}</p>
      )}
      {params.length > 0 && (
        <div className="space-y-1">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-text-3">Parameters</p>
          {params.map((p) => (
            <div key={p.name} className="flex items-baseline gap-2 text-xs pl-2">
              <code className="text-accent-primary font-mono">{p.name}</code>
              <span className="text-text-3">{p.type}</span>
              {p.required && <span className="text-red-400 text-[10px]">required</span>}
              {p.description && <span className="text-text-2 truncate">{p.description}</span>}
            </div>
          ))}
        </div>
      )}
      {endpoint.response && Object.keys(endpoint.response).length > 0 && (
        <div className="space-y-1">
          <p className="text-[10px] font-semibold uppercase tracking-wider text-text-3">Response</p>
          {Object.entries(endpoint.response).map(([key, desc]) => (
            <div key={key} className="flex items-baseline gap-2 text-xs pl-2">
              <code className="text-accent-primary font-mono">{key}</code>
              <span className="text-text-2 truncate">{desc}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function EditableField({ value, onSave, multiline }: { value: string; onSave: (v: string) => Promise<void>; multiline?: boolean }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const inputRef = useRef<HTMLInputElement | HTMLTextAreaElement>(null);

  useEffect(() => { setDraft(value); }, [value]);
  useEffect(() => { if (editing) inputRef.current?.focus(); }, [editing]);

  const save = async () => {
    if (draft.trim() === value) { setEditing(false); return; }
    setSaving(true);
    try {
      await onSave(draft.trim());
      setEditing(false);
    } finally {
      setSaving(false);
    }
  };

  const cancel = () => { setDraft(value); setEditing(false); };

  if (!editing) {
    return (
      <button
        onClick={() => setEditing(true)}
        className="group flex items-center gap-1.5 text-left cursor-pointer hover:bg-surface-2/50 rounded-md -ml-1.5 px-1.5 py-0.5 transition-colors w-full"
      >
        <span className="flex-1 min-w-0">{value || <span className="text-text-3 italic">None</span>}</span>
        <Pencil className="w-3 h-3 text-text-3 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100 transition-opacity flex-shrink-0" />
      </button>
    );
  }

  if (multiline) {
    return (
      <div className="flex gap-1.5 items-start">
        <textarea
          ref={inputRef as React.RefObject<HTMLTextAreaElement>}
          value={draft}
          onChange={e => setDraft(e.target.value)}
          onKeyDown={e => { if (e.key === 'Escape') cancel(); }}
          rows={3}
          className="flex-1 px-2.5 py-1.5 rounded-lg text-sm bg-surface-2 border border-border-1 text-text-0 focus:outline-none focus:ring-1 focus:ring-accent-primary resize-none"
        />
        <button onClick={save} disabled={saving} aria-label="Save" className="p-1.5 rounded-md text-emerald-400 hover:bg-emerald-500/10 cursor-pointer disabled:opacity-50"><Check className="w-3.5 h-3.5" /></button>
        <button onClick={cancel} aria-label="Cancel" className="p-1.5 rounded-md text-text-3 hover:bg-surface-3 cursor-pointer"><X className="w-3.5 h-3.5" /></button>
      </div>
    );
  }

  return (
    <div className="flex gap-1.5 items-center">
      <input
        ref={inputRef as React.RefObject<HTMLInputElement>}
        value={draft}
        onChange={e => setDraft(e.target.value)}
        onKeyDown={e => { if (e.key === 'Enter') save(); if (e.key === 'Escape') cancel(); }}
        className="flex-1 px-2.5 py-1 rounded-lg text-sm bg-surface-2 border border-border-1 text-text-0 focus:outline-none focus:ring-1 focus:ring-accent-primary"
      />
      <button onClick={save} disabled={saving} aria-label="Save" className="p-1.5 rounded-md text-emerald-400 hover:bg-emerald-500/10 cursor-pointer disabled:opacity-50"><Check className="w-3.5 h-3.5" /></button>
      <button onClick={cancel} aria-label="Cancel" className="p-1.5 rounded-md text-text-3 hover:bg-surface-3 cursor-pointer"><X className="w-3.5 h-3.5" /></button>
    </div>
  );
}

function IntegrityPanel({ toolId }: { toolId: string }) {
  const [integrity, setIntegrity] = useState<ToolIntegrityInfo | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    toolExtra.integrity(toolId)
      .then(setIntegrity)
      .catch(() => setIntegrity(null))
      .finally(() => setLoading(false));
  }, [toolId]);

  if (loading) return <p className="text-xs text-text-3">Loading integrity...</p>;
  if (!integrity || (!integrity.source_hash && integrity.files.length === 0)) return null;

  return (
    <Card>
      <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3 flex items-center gap-2">
        <Shield className="w-3.5 h-3.5" />
        Integrity
      </h4>
      <div className="space-y-2">
        <div className="flex items-center gap-2 text-sm">
          {integrity.verified ? (
            <ShieldCheck className="w-4 h-4 text-emerald-400 flex-shrink-0" />
          ) : (
            <ShieldAlert className="w-4 h-4 text-red-400 flex-shrink-0" />
          )}
          <span className={integrity.verified ? "text-emerald-400" : "text-red-400"}>
            {integrity.verified ? "Source verified" : "Source modified since last compile"}
          </span>
        </div>
        {integrity.source_hash && (
          <div className="flex items-center gap-2 text-xs">
            <span className="text-text-3 w-20 flex-shrink-0">Source</span>
            <code className="text-text-2 font-mono truncate">{integrity.source_hash.slice(0, 16)}...</code>
          </div>
        )}
        {integrity.binary_hash && (
          <div className="flex items-center gap-2 text-xs">
            <span className="text-text-3 w-20 flex-shrink-0">Binary</span>
            <code className="text-text-2 font-mono truncate">{integrity.binary_hash.slice(0, 16)}...</code>
          </div>
        )}
        {integrity.files.length > 0 && (
          <details className="mt-2">
            <summary className="text-[10px] font-semibold uppercase tracking-wider text-text-3 cursor-pointer hover:text-text-2">
              {integrity.files.length} tracked files
            </summary>
            <div className="mt-1 space-y-0.5 max-h-32 overflow-y-auto">
              {integrity.files.map((f) => (
                <div key={f.filename} className="flex items-center gap-2 text-[11px]">
                  <code className="text-text-2 font-mono truncate flex-1">{f.filename}</code>
                  <span className="text-text-3">{(f.size / 1024).toFixed(1)}KB</span>
                </div>
              ))}
            </div>
          </details>
        )}
      </div>
    </Card>
  );
}

function SecretsPanel({ tool }: { tool: Tool }) {
  const [statuses, setStatuses] = useState<SecretCheckResult[]>([]);
  const [loading, setLoading] = useState(true);
  const envVars = tool.manifest?.env;
  const hasEnv = envVars && envVars.length > 0;

  useEffect(() => {
    if (!hasEnv) return;
    const names = envVars!.map(e => typeof e === 'string' ? e : e.name);
    secretsApi.checkNames(names)
      .then(setStatuses)
      .catch(() => setStatuses([]))
      .finally(() => setLoading(false));
  }, [tool.id, hasEnv, envVars]);

  if (!hasEnv) return null;
  if (loading) return <p className="text-xs text-text-3">Checking secrets...</p>;

  const names = envVars.map(e => typeof e === 'string' ? e : e.name);
  const missingOrPlaceholder = names.filter(name => {
    const s = statuses.find(st => st.name === name);
    return !s || !s.exists || s.placeholder;
  });
  const allConfigured = missingOrPlaceholder.length === 0;

  return (
    <Card>
      <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3 flex items-center gap-2">
        <KeyRound className="w-3.5 h-3.5" />
        Secrets
      </h4>
      <div className="space-y-2">
        {names.map(name => {
          const s = statuses.find(st => st.name === name);
          const configured = s && s.exists && !s.placeholder;
          return (
            <div key={name} className="flex items-center gap-2 text-sm">
              {configured ? (
                <Check className="w-4 h-4 text-emerald-400 flex-shrink-0" />
              ) : (
                <AlertTriangle className="w-4 h-4 text-amber-400 flex-shrink-0" />
              )}
              <code className="font-mono text-text-1">{name}</code>
              <span className={`text-xs ${configured ? 'text-emerald-400' : 'text-amber-400'}`}>
                {configured ? 'Configured' : s?.placeholder ? 'Placeholder — needs real value' : 'Missing'}
              </span>
            </div>
          );
        })}
      </div>
      {!allConfigured && (
        <a href="/secrets" className="mt-3 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-amber-500/15 text-amber-400 hover:bg-amber-500/25 transition-colors">
          <KeyRound className="w-4 h-4" />
          Configure Secrets
        </a>
      )}
    </Card>
  );
}

function ToolDetail({ tool, onBack, onRefresh, onDelete }: { tool: Tool; onBack: () => void; onRefresh: () => void; onDelete: () => void }) {
  const { toast } = useToast();
  const [loading, setLoading] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const isTransient = ["building", "compiling", "starting"].includes(tool.status);
  useEffect(() => {
    if (!isTransient) return;
    const interval = setInterval(onRefresh, 3000);
    return () => clearInterval(interval);
  }, [isTransient, onRefresh]);

  const doAction = async (action: string, label: string) => {
    setLoading(action);
    try {
      await api.post(`/tools/${tool.id}/${action}`);
      toast("success", `Tool ${label}`);
      onRefresh();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : `Failed to ${label} tool`);
    } finally {
      setLoading(null);
    }
  };

  const updateField = async (field: string, value: string) => {
    try {
      await api.put(`/tools/${tool.id}`, { [field]: value });
      toast("success", `Tool ${field} updated`);
      onRefresh();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : `Failed to update ${field}`);
      throw err;
    }
  };

  const toggleTool = async () => {
    const action = tool.enabled ? "disable" : "enable";
    setLoading("toggle");
    try {
      await api.post(`/tools/${tool.id}/${action}`);
      toast("success", `Tool ${action}d`);
      onRefresh();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to update tool");
    } finally {
      setLoading(null);
    }
  };

  const handleDelete = async () => {
    setLoading("delete");
    try {
      await api.delete(`/tools/${tool.id}`);
      toast("success", "Tool deleted");
      onDelete();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to delete tool");
    } finally {
      setLoading(null);
      setConfirmDelete(false);
    }
  };

  const handleExport = () => {
    window.open(toolExtra.exportUrl(tool.id), '_blank');
    toast("success", "Export started");
  };

  const copyId = () => {
    navigator.clipboard.writeText(tool.id);
    toast("success", "Tool ID copied");
  };

  const endpoints = (tool.manifest?.endpoints ?? []).filter(
    (ep) => ep.path !== "/health",
  );

  const isRunning = tool.status === "running";
  const isStopped = tool.status === "stopped" || tool.status === "active";

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <button
          onClick={onBack}
          aria-label="Back"
          className="p-2 rounded-lg text-text-2 hover:bg-surface-2 transition-colors cursor-pointer"
        >
          <ArrowLeft className="w-5 h-5" />
        </button>
        <div className="flex-1 min-w-0">
          <div className="text-lg font-semibold text-text-0">
            <EditableField value={tool.name} onSave={v => updateField("name", v)} />
          </div>
          <div className="text-sm text-text-2 mt-0.5">
            <EditableField value={tool.description} onSave={v => updateField("description", v)} multiline />
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          {statusDot(tool.status)}
          <StatusBadge status={tool.status} />
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-2">Status</h4>
          <div className="flex items-center gap-2">
            {statusDot(tool.status)}
            <span className="text-sm font-medium text-text-1 capitalize">{tool.status}</span>
          </div>
        </Card>
        <Card>
          <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-2">Port</h4>
          <p className="text-sm font-mono text-text-1">{tool.port > 0 ? `:${tool.port}` : "--"}</p>
        </Card>
        <Card>
          <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-2">PID</h4>
          <p className="text-sm font-mono text-text-1">{tool.pid > 0 ? tool.pid : "--"}</p>
        </Card>
        <Card>
          <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-2">Created</h4>
          <p className="text-sm text-text-1">{new Date(tool.created_at).toLocaleDateString()}</p>
        </Card>
      </div>

      <Card>
        <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">Tool Info</h4>
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm">
            <Info className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
            <span className="text-text-2 w-24 flex-shrink-0">ID</span>
            <code className="text-xs font-mono text-text-1 truncate flex-1">{tool.id}</code>
            <button onClick={copyId} aria-label="Copy ID" className="p-1 rounded text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer" title="Copy ID">
              <Copy className="w-3.5 h-3.5" />
            </button>
          </div>
          {tool.owner_agent_slug && (
            <div className="flex items-center gap-2 text-sm">
              <User className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
              <span className="text-text-2 w-24 flex-shrink-0">Owner</span>
              <span className="text-text-1">{tool.owner_agent_slug}</span>
            </div>
          )}
          {tool.enabled !== undefined && (
            <div className="flex items-center gap-2 text-sm">
              <Power className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
              <span className="text-text-2 w-24 flex-shrink-0">Enabled</span>
              <span className={tool.enabled ? "text-emerald-400" : "text-text-3"}>{tool.enabled ? "Yes" : "No"}</span>
            </div>
          )}
          {tool.manifest?.version && (
            <div className="flex items-center gap-2 text-sm">
              <Globe className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
              <span className="text-text-2 w-24 flex-shrink-0">Version</span>
              <span className="text-text-1">{tool.manifest.version}</span>
            </div>
          )}
          {tool.library_slug && (
            <div className="flex items-center gap-2 text-sm">
              <Library className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
              <span className="text-text-2 w-24 flex-shrink-0">Library</span>
              <span className="text-purple-400">{tool.library_slug} v{tool.library_version}</span>
            </div>
          )}
        </div>
      </Card>

      <IntegrityPanel toolId={tool.id} />
      <SecretsPanel tool={tool} />

      <Card>
        <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-4">
          Endpoints
          <span className="ml-2 text-text-3 normal-case font-normal">
            ({endpoints.length})
          </span>
        </h4>
        {endpoints.length > 0 ? (
          <div className="space-y-3">
            {endpoints.map((ep, i) => (
              <EndpointCard key={i} endpoint={ep} />
            ))}
          </div>
        ) : (
          <p className="text-sm text-text-3 italic">No endpoints registered. Endpoints are defined in the tool's manifest.json.</p>
        )}
      </Card>

      {tool.capabilities && (
        <Card>
          <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">Capabilities</h4>
          <pre className="text-sm text-text-1 whitespace-pre-wrap font-mono bg-surface-2 rounded-lg p-3">{tool.capabilities}</pre>
        </Card>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          variant="secondary"
          onClick={() => doAction("compile", "compiled")}
          loading={loading === "compile"}
          icon={<Hammer className="w-4 h-4" />}
        >
          Compile
        </Button>
        {isStopped && (
          <Button
            onClick={() => doAction("start", "started")}
            loading={loading === "start"}
            icon={<Play className="w-4 h-4" />}
          >
            Start
          </Button>
        )}
        {isRunning && (
          <Button
            variant="secondary"
            onClick={() => doAction("stop", "stopped")}
            loading={loading === "stop"}
            icon={<Square className="w-4 h-4" />}
          >
            Stop
          </Button>
        )}
        <Button
          variant="secondary"
          onClick={() => doAction("restart", "restarted")}
          loading={loading === "restart"}
          icon={<RefreshCw className="w-4 h-4" />}
        >
          Restart
        </Button>
        <Button
          variant="secondary"
          onClick={toggleTool}
          loading={loading === "toggle"}
          icon={<Power className="w-4 h-4" />}
        >
          {tool.enabled ? "Disable" : "Enable"}
        </Button>
        <Button
          variant="secondary"
          onClick={handleExport}
          icon={<Download className="w-4 h-4" />}
        >
          Export
        </Button>
        <div className="flex-1" />
        {confirmDelete ? (
          <div className="flex items-center gap-2">
            <span className="text-sm text-red-400">Delete this tool?</span>
            <Button
              variant="danger"
              size="sm"
              onClick={handleDelete}
              loading={loading === "delete"}
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
    </div>
  );
}

const PAGE_SIZE = 12;

export function Tools() {
  const { toast } = useToast();
  const [tools, setTools] = useState<Tool[]>([]);
  const [loading, setLoading] = useState(true);
  const [view, setView] = useState<ViewMode>("grid");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [selectedTool, setSelectedTool] = useState<Tool | null>(null);
  const selectedToolRef = useRef<Tool | null>(null);
  const importRef = useRef<HTMLInputElement>(null);
  const [toolsMissingSecrets, setToolsMissingSecrets] = useState<Set<string>>(new Set());

  useEffect(() => {
    selectedToolRef.current = selectedTool;
  }, [selectedTool]);

  useEffect(() => {
    loadTools();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const checkToolSecrets = useCallback(async (toolList: Tool[], catalogTools: LibraryTool[]) => {
    // Match installed tools to catalog entries to get env requirements
    const toolEnvMap = new Map<string, string[]>();
    for (const tool of toolList) {
      if (!tool.library_slug) continue;
      const catalog = catalogTools.find(c => c.slug === tool.library_slug);
      if (catalog?.env && catalog.env.length > 0) {
        toolEnvMap.set(tool.id, catalog.env);
      }
    }
    if (toolEnvMap.size === 0) { setToolsMissingSecrets(new Set()); return; }
    const allEnvNames = [...new Set([...toolEnvMap.values()].flat())];
    try {
      const statuses = await secretsApi.checkNames(allEnvNames);
      const missing = new Set<string>();
      for (const [toolId, envNames] of toolEnvMap) {
        const hasMissing = envNames.some(name => {
          const s = statuses.find(st => st.name === name);
          return !s || !s.exists || s.placeholder;
        });
        if (hasMissing) missing.add(toolId);
      }
      setToolsMissingSecrets(missing);
    } catch { /* ignore */ }
  }, []);

  const loadTools = async () => {
    try {
      const [data, catalog] = await Promise.all([
        api.get<Tool[]>("/tools"),
        toolLibrary.list({}).catch(() => [] as LibraryTool[]),
      ]);
      const toolList = Array.isArray(data) ? data : [];
      setTools(toolList);
      checkToolSecrets(toolList, Array.isArray(catalog) ? catalog : []);
    } catch (e) {
      void e;
      setTools([]);
    } finally {
      setLoading(false);
    }
  };

  const fetchToolDetail = useCallback(async (toolId: string) => {
    try {
      const data = await api.get<Tool>(`/tools/${toolId}`);
      setSelectedTool(data);
    } catch (e) {
      void e;
    }
  }, []);

  const selectTool = useCallback(async (tool: Tool) => {
    setSelectedTool(tool);
    fetchToolDetail(tool.id);
  }, [fetchToolDetail]);

  const refreshSelected = async () => {
    if (!selectedTool) return;
    try {
      const data = await api.get<Tool>(`/tools/${selectedTool.id}`);
      setSelectedTool(data);
      loadTools();
    } catch (e) {
      void e;
    }
  };

  useWebSocket({
    onMessage: (msg) => {
      if (msg.type === "tool_status") {
        loadTools();
        const current = selectedToolRef.current;
        if (current && msg.payload?.tool_id === current.id) {
          fetchToolDetail(current.id);
        }
      }
    },
  });

  const handleImport = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      await toolExtra.importTool(file);
      toast("success", "Tool imported and compiled");
      loadTools();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Import failed");
    }
    if (importRef.current) importRef.current.value = "";
  };

  const handleSearch = (val: string) => { setSearch(val); setPage(0); };

  const filtered = tools.filter(
    (t) =>
      t.name.toLowerCase().includes(search.toLowerCase()) ||
      t.description.toLowerCase().includes(search.toLowerCase()),
  );
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const paginated = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  if (selectedTool)
    return (
      <div className="flex flex-col h-full">
        <Header title="Tools" />
        <div className="flex-1 overflow-y-auto p-4 md:p-6">
          <ToolDetail
            tool={selectedTool}
            onBack={() => setSelectedTool(null)}
            onRefresh={refreshSelected}
            onDelete={() => { setSelectedTool(null); loadTools(); }}
          />
        </div>
      </div>
    );

  return (
    <div className="flex flex-col h-full">
      <Header title="Tools" />
      <div className="flex-1 overflow-y-auto p-4 md:p-6">
            <div className="flex items-center gap-3 mb-4">
              <SearchBar value={search} onChange={handleSearch} placeholder="Search tools..." className="flex-1" />
              <input
                ref={importRef}
                type="file"
                accept=".zip"
                onChange={handleImport}
                className="hidden"
              />
              <Button
                variant="secondary"
                onClick={() => importRef.current?.click()}
                icon={<Upload className="w-4 h-4" />}
              >
                Import
              </Button>
              <ViewToggle view={view} onViewChange={setView} />
            </div>
            {loading ? (
              <div className="flex items-center justify-center py-16">
                <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
              </div>
            ) : filtered.length === 0 ? (
              <EmptyState
                icon={<Wrench className="w-8 h-8" />}
                title={search ? "No tools found" : "No tools yet"}
                description={
                  search
                    ? "Try a different search term"
                    : "Install from the Library page or chat with Pounce to build a custom tool."
                }
              />
            ) : view === "grid" ? (
              <>
                <Pagination page={page} totalPages={totalPages} total={filtered.length} onPageChange={setPage} label="tools" />
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                  {paginated.map((tool) => (
                    <ToolCard
                      key={tool.id}
                      tool={tool}
                      onClick={() => selectTool(tool)}
                      needsSecrets={toolsMissingSecrets.has(tool.id)}
                    />
                  ))}
                </div>
              </>
            ) : (
              <>
                <Pagination page={page} totalPages={totalPages} total={filtered.length} onPageChange={setPage} label="tools" />
                <Card padding={false}>
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-border-0">
                        {[
                          { label: "Tool", hideOnMobile: false },
                          { label: "Status", hideOnMobile: false },
                          { label: "Port", hideOnMobile: true },
                          { label: "PID", hideOnMobile: true },
                        ].map((h) => (
                          <th
                            key={h.label}
                            scope="col"
                            className={`text-left px-3 md:px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-3 ${h.hideOnMobile ? 'hidden md:table-cell' : ''}`}
                          >
                            {h.label}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {paginated.map((tool) => (
                        <ToolRow
                          key={tool.id}
                          tool={tool}
                          onClick={() => selectTool(tool)}
                        />
                      ))}
                    </tbody>
                  </table>
                </Card>
              </>
            )}
      </div>
    </div>
  );
}
