/**
 * Studio — generate images, video and audio.
 *
 * Two columns: a narrow settings rail on the left with Editor/Saved tabs, and
 * the canvas on the right showing everything made in this workspace.
 *
 * Generation is a paid, slow, foreground operation. There is deliberately no
 * background queue and no auto-retry — a run belongs to the page you started
 * it on, and navigating away cancels it rather than silently spending money.
 */

import { useCallback, useEffect, useState } from 'react';
import { Sparkles } from 'lucide-react';
import { studio } from '../lib/api-helpers';
import { api } from '../lib/api';
import { StudioEditor, type EditorState } from '../components/studio/StudioEditor';
import { StudioSaved } from '../components/studio/StudioSaved';
import { StudioCanvas } from '../components/studio/StudioCanvas';
import { SplitDivider } from '../components/workbench/SplitDivider';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { PromptDialog } from '../components/PromptDialog';
import { useToast } from '../components/Toast';
import type {
  StudioAsset,
  StudioFolder,
  StudioKind,
  StudioModel,
  StudioPreset,
  StudioProvider,
} from '../lib/types';

// The rail holds selects and a prompt box, so it stops well short of unusable
// on the narrow end and never grows enough to squeeze the canvas out.
const RAIL_MIN = 260;
const RAIL_MAX = 620;
const RAIL_DEFAULT = 320;
const RAIL_WIDTH_KEY = 'openpaw.studio.railWidth';

function loadRailWidth() {
  const stored = Number(localStorage.getItem(RAIL_WIDTH_KEY));
  if (!Number.isFinite(stored) || stored <= 0) return RAIL_DEFAULT;
  return Math.min(RAIL_MAX, Math.max(RAIL_MIN, stored));
}

// Labels are explicit rather than a capitalised key: "Saved Prompts" reads as
// what the tab holds, where a bare "Saved" could be saved output.
const TABS = [
  { id: 'editor', label: 'Editor' },
  { id: 'saved', label: 'Saved Prompts' },
] as const;

const DEFAULT_STATE: EditorState = {
  refImages: [],
  type: 'image',
  provider: '',
  model: '',
  prompt: '',
  count: 1,
  size: '1024x1024',
  duration: 0,
  folderId: '',
};

export function Studio() {
  const { toast } = useToast();

  const [tab, setTab] = useState<'editor' | 'saved'>('editor');
  const [state, setState] = useState<EditorState>(DEFAULT_STATE);

  const [providers, setProviders] = useState<StudioProvider[]>([]);
  const [supports, setSupports] = useState<Record<StudioKind, boolean>>({
    image: false,
    video: false,
    audio: false,
  });

  const [models, setModels] = useState<StudioModel[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);

  const [folders, setFolders] = useState<StudioFolder[]>([]);
  const [activeFolder, setActiveFolder] = useState('');

  const [assets, setAssets] = useState<StudioAsset[]>([]);
  const [assetsLoading, setAssetsLoading] = useState(true);

  const [presets, setPresets] = useState<StudioPreset[]>([]);
  const [presetsLoading, setPresetsLoading] = useState(true);

  const [generating, setGenerating] = useState(false);
  const [railWidth, setRailWidth] = useState(loadRailWidth);

  // Generated media is permanent until explicitly removed, so every delete
  // goes through a themed confirm rather than a native one.
  const [confirm, setConfirm] = useState<{
    title: string;
    message: React.ReactNode;
    confirmLabel: string;
    run: () => Promise<void>;
  } | null>(null);
  const [confirmBusy, setConfirmBusy] = useState(false);

  // Folder create/rename share one dialog — window.prompt does nothing in the
  // desktop webview.
  const [naming, setNaming] = useState<{ mode: 'create' | 'rename'; folder?: StudioFolder } | null>(
    null,
  );
  const [namingBusy, setNamingBusy] = useState(false);

  const runConfirm = async () => {
    if (!confirm) return;
    setConfirmBusy(true);
    try {
      await confirm.run();
      setConfirm(null);
    } finally {
      setConfirmBusy(false);
    }
  };

  const patch = useCallback((p: Partial<EditorState>) => setState(s => ({ ...s, ...p })), []);

  // Persisted on release rather than on every mousemove — the drag fires
  // continuously and localStorage writes are synchronous.
  const resizeRail = useCallback((delta: number) => {
    setRailWidth(w => Math.min(RAIL_MAX, Math.max(RAIL_MIN, w + delta)));
  }, []);

  useEffect(() => {
    const persist = () => localStorage.setItem(RAIL_WIDTH_KEY, String(railWidth));
    const timer = setTimeout(persist, 400);
    return () => clearTimeout(timer);
  }, [railWidth]);

  // --- loaders ---

  const loadFolders = useCallback(async () => {
    try {
      const { folders } = await studio.folders();
      setFolders(folders || []);
    } catch {
      setFolders([]);
    }
  }, []);

  const loadAssets = useCallback(async (folderId: string) => {
    setAssetsLoading(true);
    try {
      const { items } = await studio.media({ folderId: folderId || undefined, limit: 120 });
      setAssets(items || []);
    } catch {
      setAssets([]);
    } finally {
      setAssetsLoading(false);
    }
  }, []);

  const loadPresets = useCallback(async () => {
    setPresetsLoading(true);
    try {
      const { presets } = await studio.presets();
      setPresets(presets || []);
    } catch {
      setPresets([]);
    } finally {
      setPresetsLoading(false);
    }
  }, []);

  useEffect(() => {
    studio
      .providers()
      .then(d => {
        setProviders(d.providers || []);
        setSupports(d.supports);
      })
      .catch(() => {});
    loadFolders();
    loadPresets();
  }, [loadFolders, loadPresets]);

  useEffect(() => {
    loadAssets(activeFolder);
  }, [activeFolder, loadAssets]);

  // Model list follows the type/provider selection.
  useEffect(() => {
    let cancelled = false;
    setModelsLoading(true);
    studio
      .models(state.type, state.provider || undefined)
      .then(d => {
        if (!cancelled) setModels(d.models || []);
      })
      .catch(() => {
        if (!cancelled) setModels([]);
      })
      .finally(() => {
        if (!cancelled) setModelsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [state.type, state.provider]);

  // --- actions ---

  const handleGenerate = async () => {
    if (!state.prompt.trim() || generating) return;
    setGenerating(true);
    try {
      const res = await studio.generate({
        type: state.type,
        prompt: state.prompt.trim(),
        provider: state.provider || undefined,
        model: state.model || undefined,
        count: state.count,
        size: state.size || undefined,
        duration: state.duration || undefined,
        folder_id: state.folderId || undefined,
        ref_images: state.refImages.length ? state.refImages.map(r => r.src) : undefined,
      });

      // New items are prepended rather than refetched so they appear even when
      // the current folder view wouldn't include them.
      setAssets(prev => [...(res.items || []), ...prev]);
      loadFolders();

      if (res.errors?.length) {
        toast('warning', `Generated ${res.items.length}, ${res.errors.length} failed: ${res.errors[0]}`);
      } else {
        toast('success', `Generated ${res.items.length} ${state.type}${res.items.length > 1 ? 's' : ''}`);
      }
    } catch (e) {
      toast('error', e instanceof Error ? e.message : 'Generation failed');
    } finally {
      setGenerating(false);
    }
  };

  const handleSavePreset = async () => {
    try {
      await studio.savePreset({
        provider: state.provider,
        media_type: state.type,
        model: state.model,
        prompt: state.prompt.trim(),
        count: state.count,
        size: state.size,
        folder_id: state.folderId,
      });
      await loadPresets();
      setTab('saved');
      toast('success', 'Saved');
    } catch (e) {
      toast('error', e instanceof Error ? e.message : 'Could not save');
    }
  };

  const handleLoadPreset = (p: StudioPreset) => {
    setState({
      // References are per-run, not part of a saved setup: they are often
      // one-off pastes and would be surprising to have reappear later.
      refImages: [],
      type: p.media_type,
      provider: p.provider,
      model: p.model,
      prompt: p.prompt,
      count: p.count || 1,
      size: p.size,
      duration: 0,
      folderId: p.folder_id,
    });
    setTab('editor');
  };

  const handleDeletePreset = (p: StudioPreset) => {
    setConfirm({
      title: 'Delete saved setup',
      confirmLabel: 'Delete',
      message: (
        <>
          Delete <span className="text-text-0">{p.name}</span>? This removes the saved settings
          only — anything generated from it is kept.
        </>
      ),
      run: async () => {
        try {
          await studio.deletePreset(p.id);
          setPresets(prev => prev.filter(x => x.id !== p.id));
        } catch {
          toast('error', 'Could not delete');
        }
      },
    });
  };

  const handleNewFolder = () => setNaming({ mode: 'create' });

  const handleRenameFolder = (f: StudioFolder) => setNaming({ mode: 'rename', folder: f });

  const submitName = async (name: string) => {
    if (!naming) return;
    setNamingBusy(true);
    try {
      if (naming.mode === 'create') {
        const folder = await studio.createFolder(name);
        await loadFolders();
        patch({ folderId: folder.id });
        toast('success', `Created "${folder.name}"`);
      } else if (naming.folder) {
        await studio.renameFolder(naming.folder.id, name);
        await loadFolders();
      }
      setNaming(null);
    } catch (e) {
      toast('error', e instanceof Error ? e.message : 'Could not save folder');
    } finally {
      setNamingBusy(false);
    }
  };

  const handleDeleteFolder = (f: StudioFolder) => {
    setConfirm({
      title: 'Delete folder',
      confirmLabel: 'Delete folder',
      message: (
        <>
          Delete <span className="text-text-0">{f.name}</span>? Its {f.count} item
          {f.count === 1 ? '' : 's'} will be kept and moved to Unfiled — nothing is generated
          again and nothing is lost.
        </>
      ),
      run: async () => {
        try {
          await studio.deleteFolder(f.id);
          if (activeFolder === f.id) setActiveFolder('');
          if (state.folderId === f.id) patch({ folderId: '' });
          await loadFolders();
          await loadAssets(activeFolder === f.id ? '' : activeFolder);
        } catch {
          toast('error', 'Could not delete folder');
        }
      },
    });
  };

  const handleMove = async (asset: StudioAsset, folderId: string) => {
    try {
      await studio.move(asset.id, folderId);
      setAssets(prev =>
        activeFolder && folderId !== activeFolder
          ? prev.filter(a => a.id !== asset.id)
          : prev.map(a => (a.id === asset.id ? { ...a, folder_id: folderId } : a)),
      );
      loadFolders();
    } catch {
      toast('error', 'Could not move');
    }
  };

  const handleDelete = (asset: StudioAsset) => {
    setConfirm({
      title: `Delete this ${asset.media_type}?`,
      confirmLabel: 'Delete permanently',
      message: (
        <>
          This deletes the file from disk and removes it from the media library. It cannot be
          undone, and regenerating it costs another API call.
          {asset.prompt && (
            <span className="block mt-2 text-xs text-text-3 italic line-clamp-3">
              “{asset.prompt}”
            </span>
          )}
        </>
      ),
      run: async () => {
        try {
          await api.delete(`/media/${asset.id}`);
          setAssets(prev => prev.filter(a => a.id !== asset.id));
          loadFolders();
        } catch {
          toast('error', 'Could not delete');
        }
      },
    });
  };

  return (
    <div className="flex h-full min-h-0">
      {/* Left rail */}
      <div
        style={{ width: railWidth }}
        className="flex-shrink-0 flex flex-col bg-surface-1 min-h-0"
      >
        <div className="flex items-center gap-2 px-4 pt-4 pb-2 flex-shrink-0">
          <Sparkles className="w-4 h-4 text-accent-text" aria-hidden="true" />
          <h1 className="text-sm font-semibold text-text-0">Studio</h1>
        </div>

        {/* Thin tab strip */}
        <div className="flex px-3 gap-1 border-b border-border-0 flex-shrink-0" role="tablist">
          {TABS.map(({ id, label }) => (
            <button
              key={id}
              role="tab"
              aria-selected={tab === id}
              onClick={() => setTab(id)}
              className={`relative px-3 py-2 text-xs font-medium whitespace-nowrap transition-colors cursor-pointer ${
                tab === id ? 'text-accent-text' : 'text-text-3 hover:text-text-1'
              }`}
            >
              {label}
              {tab === id && (
                <span className="absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-accent-primary" />
              )}
            </button>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto min-h-0">
          {tab === 'editor' ? (
            <StudioEditor
              state={state}
              onChange={patch}
              providers={providers}
              supports={supports}
              models={models}
              modelsLoading={modelsLoading}
              folders={folders}
              generating={generating}
              onGenerate={handleGenerate}
              onSavePreset={handleSavePreset}
              onNewFolder={handleNewFolder}
              onError={msg => toast('error', msg)}
            />
          ) : (
            <StudioSaved
              presets={presets}
              loading={presetsLoading}
              onLoad={handleLoadPreset}
              onDelete={handleDeletePreset}
            />
          )}
        </div>
      </div>

      {/* Drag to rebalance the two columns. Replaces the rail's right border,
          so the seam stays a single line until it is hovered. */}
      <SplitDivider direction="horizontal" onDrag={resizeRail} />

      {/* Canvas */}
      <div className="flex-1 min-w-0 min-h-0">
        <StudioCanvas
          assets={assets}
          loading={assetsLoading}
          generating={generating}
          generatingCount={state.count}
          folders={folders}
          activeFolder={activeFolder}
          onSelectFolder={setActiveFolder}
          onMove={handleMove}
          onDelete={handleDelete}
          onRenameFolder={handleRenameFolder}
          onDeleteFolder={handleDeleteFolder}
          onUsePrompt={prompt => {
            patch({ prompt });
            setTab('editor');
          }}
        />
      </div>

      <PromptDialog
        open={naming !== null}
        title={naming?.mode === 'rename' ? 'Rename folder' : 'New folder'}
        label="Folder name"
        placeholder="e.g. Logos"
        initialValue={naming?.folder?.name ?? ''}
        confirmLabel={naming?.mode === 'rename' ? 'Rename' : 'Create'}
        busy={namingBusy}
        onConfirm={submitName}
        onCancel={() => setNaming(null)}
      />

      <ConfirmDialog
        open={confirm !== null}
        title={confirm?.title ?? ''}
        message={confirm?.message ?? ''}
        confirmLabel={confirm?.confirmLabel}
        busy={confirmBusy}
        onConfirm={runConfirm}
        onCancel={() => setConfirm(null)}
      />
    </div>
  );
}
