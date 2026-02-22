import { useState, useEffect, useCallback, useRef } from 'react';
import {
  LayoutDashboard,
  RefreshCw,
  ChevronDown,
  ImageIcon,
  X,
  Pencil,
  Trash2,
} from 'lucide-react';
import { Header } from '../components/Header';
import { EmptyState } from '../components/EmptyState';
import { DashboardGrid } from '../components/DashboardGrid';
import { DashboardBackground } from '../components/BackgroundImage';
import { Modal } from '../components/Modal';
import { Button } from '../components/Button';
import { useDashboardRefresh } from '../hooks/useDashboardRefresh';
import { useToast } from '../components/Toast';
import { api, getCSRFToken, type Dashboard, type DashboardWidgetConfig, type DashboardLayout } from '../lib/api';

const LAST_DASHBOARD_KEY = 'openpaw_last_dashboard';

const THEME_VARS = [
  'op-surface-0', 'op-surface-1', 'op-surface-2', 'op-surface-3',
  'op-border-0', 'op-border-1',
  'op-text-0', 'op-text-1', 'op-text-2', 'op-text-3',
  'op-accent', 'op-accent-hover', 'op-accent-muted', 'op-accent-text',
  'op-danger', 'op-danger-hover',
  'op-font-family',
  'op-font-size-xs', 'op-font-size-sm', 'op-font-size-base',
  'op-font-size-lg', 'op-font-size-xl', 'op-font-size-2xl', 'op-font-size-3xl',
  'op-radius-sm', 'op-radius-md', 'op-radius-lg', 'op-radius-xl', 'op-radius-full',
  'op-space-1', 'op-space-2', 'op-space-3', 'op-space-4',
  'op-space-5', 'op-space-6', 'op-space-8',
];

const BG_PRESETS = [
  { url: '/preset-bg/bg-1.webp', name: 'Cyber Cat' },
  { url: '/preset-bg/bg-2.webp', name: 'Digital Garden' },
  { url: '/preset-bg/bg-3.webp', name: 'Peeking Cat' },
  { url: '/preset-bg/bg-4.webp', name: 'Cat & Robot' },
  { url: '/preset-bg/bg-5.webp', name: 'Garden Gate' },
  { url: '/preset-bg/bg-6.webp', name: 'Crystal Path' },
  { url: '/preset-bg/bg-7.webp', name: 'Garden Friends' },
  { url: '/preset-bg/bg-8.webp', name: 'Shoggoth City' },
  { url: '/preset-bg/bg-9.webp', name: 'AI Garden' },
];

export function Dashboards() {
  const [dashboards, setDashboards] = useState<Dashboard[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [bgPickerOpen, setBgPickerOpen] = useState(false);
  const [bgUploading, setBgUploading] = useState(false);
  const [renameOpen, setRenameOpen] = useState(false);
  const [renameValue, setRenameValue] = useState('');
  const [renameSaving, setRenameSaving] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const { toast } = useToast();

  const selected = dashboards.find(d => d.id === selectedId) || null;
  const widgets: DashboardWidgetConfig[] = selected?.widgets || [];
  const layout: DashboardLayout = selected?.layout || { columns: 3, gap: 'md' };

  const isCustom = selected?.dashboard_type === 'custom';
  const iframeRef = useRef<HTMLIFrameElement>(null);

  useEffect(() => {
    function handleIframeMessage(e: MessageEvent) {
      const iframe = iframeRef.current;
      if (!iframe?.contentWindow || e.source !== iframe.contentWindow) return;

      const { type, id, action, toolId, endpoint, payload } = e.data || {};

      if (type === 'openpaw_request') {
        const hdrs: Record<string, string> = { 'Content-Type': 'application/json' };
        const csrfMatch = document.cookie.match(/(?:^|;\s*)openpaw_csrf=([^;]*)/);
        if (csrfMatch) hdrs['X-CSRF-Token'] = decodeURIComponent(csrfMatch[1]);

        let promise: Promise<unknown>;

        switch (action) {
          case 'callTool':
            promise = fetch(`/api/v1/tools/${toolId}/call`, {
              method: 'POST',
              headers: hdrs,
              body: JSON.stringify({ endpoint, payload: payload ? JSON.stringify(payload) : undefined }),
              credentials: 'same-origin',
            }).then(r => { if (!r.ok) throw new Error('API error: ' + r.status); return r.json(); });
            break;
          case 'getTools':
            promise = fetch('/api/v1/tools', { headers: hdrs, credentials: 'same-origin' })
              .then(r => { if (!r.ok) throw new Error('API error: ' + r.status); return r.json(); });
            break;
          default:
            iframe.contentWindow?.postMessage({ type: 'openpaw_response', id, error: 'Unknown action: ' + action }, window.location.origin);
            return;
        }

        // Use '*' because the sandboxed iframe has a null origin and cannot receive
        // messages targeted at a specific origin.
        promise
          .then(result => iframe.contentWindow?.postMessage({ type: 'openpaw_response', id, result }, '*'))
          .catch(err => iframe.contentWindow?.postMessage({ type: 'openpaw_response', id, error: err.message }, '*'));
      }

      if (type === 'openpaw_theme_request') {
        const style = getComputedStyle(document.documentElement);
        const vars: Record<string, string> = {};
        for (const v of THEME_VARS) {
          const val = style.getPropertyValue('--' + v).trim();
          if (val) vars['--' + v] = val;
        }
        iframe.contentWindow?.postMessage({ type: 'openpaw_theme', vars }, '*');
      }
    }

    window.addEventListener('message', handleIframeMessage);
    return () => window.removeEventListener('message', handleIframeMessage);
  }, []);

  const refreshDashboard = useCallback(async (id: string) => {
    return api.post<Record<string, unknown>>(`/dashboards/${id}/refresh`);
  }, []);

  const { widgetData, loading: refreshing, refresh } = useDashboardRefresh(
    isCustom ? undefined : (selectedId || undefined),
    isCustom ? [] : widgets,
    refreshDashboard,
  );

  useEffect(() => {
    loadDashboards();
  }, []);

  const loadDashboards = async () => {
    try {
      const data = await api.get<Dashboard[]>('/dashboards');
      const list = Array.isArray(data) ? data : [];
      setDashboards(list);

      // Auto-select: last viewed or first available
      if (list.length > 0) {
        const lastId = localStorage.getItem(LAST_DASHBOARD_KEY);
        const found = list.find(d => d.id === lastId);
        setSelectedId(found ? found.id : list[0].id);
      }
    } catch (e) {
      console.warn('loadDashboards failed:', e);
      setDashboards([]);
    } finally {
      setLoading(false);
    }
  };

  const selectDashboard = (id: string) => {
    setSelectedId(id);
    setDropdownOpen(false);
    setBgPickerOpen(false);
    localStorage.setItem(LAST_DASHBOARD_KEY, id);
  };

  const updateDashboardBg = async (url: string) => {
    if (!selectedId) return;
    try {
      await api.put(`/dashboards/${selectedId}`, { bg_image: url });
      setDashboards(prev => prev.map(d => d.id === selectedId ? { ...d, bg_image: url } : d));
    } catch {
      toast('error', 'Failed to update background');
    }
  };

  const openRename = () => {
    if (!selected) return;
    setRenameValue(selected.name);
    setRenameOpen(true);
  };

  const renameDashboard = async () => {
    if (!selectedId || !renameValue.trim()) return;
    setRenameSaving(true);
    try {
      await api.put(`/dashboards/${selectedId}`, { name: renameValue.trim() });
      setDashboards(prev => prev.map(d => d.id === selectedId ? { ...d, name: renameValue.trim() } : d));
      setRenameOpen(false);
      toast('success', 'Dashboard renamed');
    } catch {
      toast('error', 'Failed to rename dashboard');
    } finally {
      setRenameSaving(false);
    }
  };

  const deleteDashboard = async () => {
    if (!selectedId) return;
    setDeleteLoading(true);
    try {
      await api.delete(`/dashboards/${selectedId}`);
      const remaining = dashboards.filter(d => d.id !== selectedId);
      setDashboards(remaining);
      setDeleteOpen(false);
      if (remaining.length > 0) {
        setSelectedId(remaining[0].id);
        localStorage.setItem(LAST_DASHBOARD_KEY, remaining[0].id);
      } else {
        setSelectedId(null);
        localStorage.removeItem(LAST_DASHBOARD_KEY);
      }
      toast('success', 'Dashboard deleted');
    } catch {
      toast('error', 'Failed to delete dashboard');
    } finally {
      setDeleteLoading(false);
    }
  };

  const handleDashboardBgUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
      toast('error', 'Please upload a PNG, JPEG, or WebP image');
      return;
    }
    if (file.size > 5 * 1024 * 1024) {
      toast('error', 'Image must be under 5MB');
      return;
    }
    setBgUploading(true);
    try {
      const formData = new FormData();
      formData.append('background', file);
      const csrfHeaders: Record<string, string> = {};
      const csrf = getCSRFToken();
      if (csrf) csrfHeaders['X-CSRF-Token'] = csrf;
      const res = await fetch('/api/v1/settings/design/background', {
        method: 'POST',
        headers: csrfHeaders,
        body: formData,
        credentials: 'same-origin',
      });
      if (!res.ok) throw new Error('Upload failed');
      const data = await res.json();
      await updateDashboardBg(data.url);
      toast('success', 'Background uploaded');
    } catch {
      toast('error', 'Failed to upload background');
    } finally {
      setBgUploading(false);
    }
    e.target.value = '';
  };

  if (loading) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Dashboards" />
        <div className="flex-1 flex items-center justify-center">
          <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
        </div>
      </div>
    );
  }

  if (dashboards.length === 0) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Dashboards" />
        <div className="flex-1 overflow-y-auto p-6">
          <EmptyState
            icon={<LayoutDashboard className="w-8 h-8" />}
            title="No dashboards yet"
            description='Create your first dashboard by asking in Chat. Try: "Create a dashboard that monitors my weather tool"'
          />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header title="Dashboards" actions={
        <div className="flex items-center gap-2">
          {/* Dashboard switcher dropdown */}
          <div className="relative">
            <button
              onClick={() => setDropdownOpen(!dropdownOpen)}
              aria-expanded={dropdownOpen}
              aria-haspopup="listbox"
              aria-label="Select dashboard"
              className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-surface-2 border border-border-0 text-sm text-text-1 hover:border-border-1 transition-colors cursor-pointer min-w-[140px]"
            >
              <span className="truncate">{selected?.name || 'Select dashboard'}</span>
              <ChevronDown className={`w-4 h-4 text-text-3 flex-shrink-0 transition-transform ${dropdownOpen ? 'rotate-180' : ''}`} />
            </button>
            {dropdownOpen && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setDropdownOpen(false)} />
                <div className="absolute top-full left-0 mt-1 z-20 w-56 rounded-lg border border-border-0 bg-surface-1 shadow-lg overflow-hidden" role="listbox" aria-label="Dashboards">
                  {dashboards.map(d => (
                    <button
                      key={d.id}
                      role="option"
                      aria-selected={d.id === selectedId}
                      onClick={() => selectDashboard(d.id)}
                      className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer ${
                        d.id === selectedId
                          ? 'bg-accent-primary/10 text-accent-primary'
                          : 'text-text-1 hover:bg-surface-2'
                      }`}
                    >
                      <span className="flex items-center gap-2">
                        <span className="truncate font-medium">{d.name}</span>
                        <span className={`flex-shrink-0 text-[10px] px-1.5 py-0.5 rounded-full ${
                          d.dashboard_type === 'custom'
                            ? 'bg-accent-primary/15 text-accent-text'
                            : 'bg-surface-3 text-text-3'
                        }`}>
                          {d.dashboard_type === 'custom' ? 'Custom' : 'Block'}
                        </span>
                      </span>
                      {d.description && (
                        <span className="block truncate text-xs text-text-3 mt-0.5">{d.description}</span>
                      )}
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>

          {/* Rename button */}
          <button
            onClick={openRename}
            className="p-1.5 rounded-lg text-text-2 hover:bg-surface-2 transition-colors cursor-pointer"
            title="Rename dashboard"
            aria-label="Rename dashboard"
          >
            <Pencil className="w-4 h-4" />
          </button>

          {/* Delete button */}
          <button
            onClick={() => setDeleteOpen(true)}
            className="p-1.5 rounded-lg text-text-2 hover:bg-danger/10 hover:text-danger transition-colors cursor-pointer"
            title="Delete dashboard"
            aria-label="Delete dashboard"
          >
            <Trash2 className="w-4 h-4" />
          </button>

          {/* Background picker button */}
          <div className="relative">
            <button
              onClick={() => setBgPickerOpen(!bgPickerOpen)}
              className={`p-1.5 rounded-lg transition-colors cursor-pointer ${
                selected?.bg_image ? 'text-accent-primary hover:bg-accent-primary/10' : 'text-text-2 hover:bg-surface-2'
              }`}
              title="Dashboard background"
              aria-label="Dashboard background"
            >
              <ImageIcon className="w-4 h-4" />
            </button>
            {bgPickerOpen && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setBgPickerOpen(false)} />
                <div className="absolute top-full right-0 mt-1 z-20 w-72 rounded-lg border border-border-0 bg-surface-1 shadow-lg overflow-hidden p-3">
                  <div className="text-xs font-medium text-text-2 mb-2">Dashboard Background</div>
                  <div className="grid grid-cols-5 gap-1.5 mb-2">
                    <button
                      onClick={() => { updateDashboardBg(''); setBgPickerOpen(false); }}
                      className={`aspect-square rounded-md border-2 flex items-center justify-center text-[10px] text-text-3 cursor-pointer transition-colors ${
                        !selected?.bg_image ? 'border-accent-primary bg-surface-2' : 'border-border-0 bg-surface-2 hover:border-border-1'
                      }`}
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                    {BG_PRESETS.map(p => (
                      <button
                        key={p.url}
                        onClick={() => { updateDashboardBg(p.url); setBgPickerOpen(false); }}
                        className={`aspect-square rounded-md border-2 bg-cover bg-center cursor-pointer transition-colors ${
                          selected?.bg_image === p.url ? 'border-accent-primary' : 'border-border-0 hover:border-border-1'
                        }`}
                        style={{ backgroundImage: `url(${p.url})` }}
                        title={p.name}
                      />
                    ))}
                  </div>
                  <label className={`flex items-center justify-center gap-1.5 px-2 py-1.5 rounded-md text-xs cursor-pointer transition-colors ${
                    bgUploading ? 'opacity-50 pointer-events-none' : ''
                  } bg-surface-2 border border-border-0 text-text-2 hover:border-border-1`}>
                    <ImageIcon className="w-3 h-3" />
                    {bgUploading ? 'Uploading...' : 'Upload custom'}
                    <input type="file" accept="image/png,image/jpeg,image/webp" className="hidden" onChange={handleDashboardBgUpload} />
                  </label>
                </div>
              </>
            )}
          </div>

          {/* Refresh button (hidden for custom dashboards) */}
          {!isCustom && (
            <button
              onClick={() => refresh()}
              disabled={refreshing}
              className="p-1.5 rounded-lg text-text-2 hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-50"
              title="Refresh data"
              aria-label="Refresh data"
            >
              <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
            </button>
          )}
        </div>
      } />

      {isCustom ? (
        <div className="flex-1 overflow-hidden relative">
          {selected?.bg_image && <DashboardBackground bgImage={selected.bg_image} />}
          <iframe
            ref={iframeRef}
            src={`/api/v1/dashboards/${selected!.id}/assets/index.html`}
            sandbox="allow-scripts"
            className="w-full h-full border-0 relative z-[1] bg-transparent"
            title={selected!.name}
          />
        </div>
      ) : (
        <div className="flex-1 overflow-y-auto relative">
          {selected?.bg_image && <DashboardBackground bgImage={selected.bg_image} />}
          <div className="relative z-[1] p-4 md:p-6">
            {selected && widgets.length > 0 ? (
              <>
                {selected.description && (
                  <p className="text-sm text-text-2 mb-4">{selected.description}</p>
                )}
                <DashboardGrid
                  layout={layout}
                  widgets={widgets}
                  widgetData={widgetData}
                  loading={refreshing}
                />
              </>
            ) : selected ? (
              <EmptyState
                icon={<LayoutDashboard className="w-8 h-8" />}
                title="No widgets yet"
                description="This dashboard has no widgets configured. Update it by asking in Chat."
              />
            ) : null}
          </div>
        </div>
      )}

      {/* Rename modal */}
      <Modal open={renameOpen} onClose={() => setRenameOpen(false)} title="Rename Dashboard" size="sm">
        <div className="space-y-4">
          <input
            type="text"
            value={renameValue}
            onChange={e => setRenameValue(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && renameValue.trim()) renameDashboard(); }}
            placeholder="Dashboard name"
            className="w-full px-3 py-2 rounded-lg bg-surface-2 border border-border-0 text-sm text-text-0 focus:outline-none focus:ring-1 focus:ring-accent-primary"
            autoFocus
          />
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setRenameOpen(false)}>Cancel</Button>
            <Button onClick={renameDashboard} loading={renameSaving} disabled={!renameValue.trim()}>Save</Button>
          </div>
        </div>
      </Modal>

      {/* Delete confirmation modal */}
      <Modal open={deleteOpen} onClose={() => setDeleteOpen(false)} title="Delete Dashboard" size="sm">
        <div className="space-y-4">
          <p className="text-sm text-text-1">
            Are you sure you want to delete <strong className="text-text-0">{selected?.name}</strong>? This will remove all widgets and collected data. This action cannot be undone.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setDeleteOpen(false)}>Cancel</Button>
            <Button variant="danger" onClick={deleteDashboard} loading={deleteLoading}>Delete</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
