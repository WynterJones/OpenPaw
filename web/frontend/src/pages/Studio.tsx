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
import { useToast } from '../components/Toast';
import type {
  StudioAsset,
  StudioFolder,
  StudioKind,
  StudioModel,
  StudioPreset,
  StudioProvider,
} from '../lib/types';

const DEFAULT_STATE: EditorState = {
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

  const patch = useCallback((p: Partial<EditorState>) => setState(s => ({ ...s, ...p })), []);

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

  const handleDeletePreset = async (p: StudioPreset) => {
    if (!window.confirm(`Delete saved setup "${p.name}"?`)) return;
    try {
      await studio.deletePreset(p.id);
      setPresets(prev => prev.filter(x => x.id !== p.id));
    } catch {
      toast('error', 'Could not delete');
    }
  };

  const handleNewFolder = async () => {
    const name = window.prompt('Folder name');
    if (!name?.trim()) return;
    try {
      const folder = await studio.createFolder(name.trim());
      await loadFolders();
      patch({ folderId: folder.id });
      toast('success', `Created "${folder.name}"`);
    } catch (e) {
      toast('error', e instanceof Error ? e.message : 'Could not create folder');
    }
  };

  const handleRenameFolder = async (f: StudioFolder) => {
    const name = window.prompt('Rename folder', f.name);
    if (!name?.trim() || name === f.name) return;
    try {
      await studio.renameFolder(f.id, name.trim());
      await loadFolders();
    } catch {
      toast('error', 'Could not rename');
    }
  };

  const handleDeleteFolder = async (f: StudioFolder) => {
    if (!window.confirm(`Delete folder "${f.name}"? Its ${f.count} item(s) stay, unfiled.`)) return;
    try {
      await studio.deleteFolder(f.id);
      if (activeFolder === f.id) setActiveFolder('');
      if (state.folderId === f.id) patch({ folderId: '' });
      await loadFolders();
      await loadAssets(activeFolder === f.id ? '' : activeFolder);
    } catch {
      toast('error', 'Could not delete folder');
    }
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

  const handleDelete = async (asset: StudioAsset) => {
    if (!window.confirm('Delete this permanently?')) return;
    try {
      await api.delete(`/media/${asset.id}`);
      setAssets(prev => prev.filter(a => a.id !== asset.id));
      loadFolders();
    } catch {
      toast('error', 'Could not delete');
    }
  };

  return (
    <div className="flex h-full min-h-0">
      {/* Left rail */}
      <div className="w-80 flex-shrink-0 flex flex-col border-r border-border-0 bg-surface-1 min-h-0">
        <div className="flex items-center gap-2 px-4 pt-4 pb-2 flex-shrink-0">
          <Sparkles className="w-4 h-4 text-accent-text" aria-hidden="true" />
          <h1 className="text-sm font-semibold text-text-0">Studio</h1>
        </div>

        {/* Thin tab strip */}
        <div className="flex px-3 gap-1 border-b border-border-0 flex-shrink-0" role="tablist">
          {(['editor', 'saved'] as const).map(t => (
            <button
              key={t}
              role="tab"
              aria-selected={tab === t}
              onClick={() => setTab(t)}
              className={`relative px-3 py-2 text-xs font-medium capitalize transition-colors cursor-pointer ${
                tab === t ? 'text-accent-text' : 'text-text-3 hover:text-text-1'
              }`}
            >
              {t}
              {tab === t && (
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
    </div>
  );
}
