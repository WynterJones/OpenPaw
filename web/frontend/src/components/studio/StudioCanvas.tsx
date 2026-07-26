/**
 * StudioCanvas — the right column.
 *
 * A folder rail across the top, then a grid of everything in the current view.
 * Images render as thumbnails; video and audio get real players, since judging
 * a generation without being able to play it is useless.
 */

import { useState } from 'react';
import {
  Download,
  Trash2,
  FolderInput,
  Loader2,
  Sparkles,
  Image as ImageIcon,
  Film,
  Music,
  Pencil,
  X,
} from 'lucide-react';
import { Button } from '../Button';
import { studio } from '../../lib/api-helpers';
import type { StudioAsset, StudioFolder, StudioKind } from '../../lib/types';

interface Props {
  assets: StudioAsset[];
  loading: boolean;
  generating: boolean;
  generatingCount: number;
  folders: StudioFolder[];
  activeFolder: string;
  onSelectFolder: (id: string) => void;
  onMove: (asset: StudioAsset, folderId: string) => void;
  onDelete: (asset: StudioAsset) => void;
  onRenameFolder: (folder: StudioFolder) => void;
  onDeleteFolder: (folder: StudioFolder) => void;
  onUsePrompt: (prompt: string) => void;
}

const KIND_ICON: Record<StudioKind, typeof ImageIcon> = {
  image: ImageIcon,
  video: Film,
  audio: Music,
};

export function StudioCanvas({
  assets,
  loading,
  generating,
  generatingCount,
  folders,
  activeFolder,
  onSelectFolder,
  onMove,
  onDelete,
  onRenameFolder,
  onDeleteFolder,
  onUsePrompt,
}: Props) {
  const [preview, setPreview] = useState<StudioAsset | null>(null);
  const [movingId, setMovingId] = useState<string | null>(null);

  const tabs = [
    { id: '', name: 'All' },
    { id: 'unfiled', name: 'Unfiled' },
    ...folders.map(f => ({ id: f.id, name: `${f.name} (${f.count})` })),
  ];

  return (
    <div className="flex flex-col h-full min-h-0">
      {/* Folder rail */}
      <div className="flex items-center gap-1 px-4 py-2 border-b border-border-0 overflow-x-auto flex-shrink-0">
        {tabs.map(t => {
          const folder = folders.find(f => f.id === t.id);
          const active = activeFolder === t.id;
          return (
            <div key={t.id} className="group relative flex-shrink-0">
              <button
                onClick={() => onSelectFolder(t.id)}
                className={`rounded-lg px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors cursor-pointer ${
                  active
                    ? 'bg-accent-primary/15 text-accent-text'
                    : 'text-text-2 hover:text-text-1 hover:bg-surface-2'
                }`}
              >
                {t.name}
              </button>
              {folder && (
                <span className="absolute -top-1 -right-1 hidden group-hover:flex items-center gap-0.5 rounded-md bg-surface-3 border border-border-1 px-0.5 py-0.5 shadow-lg">
                  <button
                    onClick={() => onRenameFolder(folder)}
                    aria-label={`Rename ${folder.name}`}
                    className="p-0.5 rounded text-text-3 hover:text-text-0 cursor-pointer"
                  >
                    <Pencil className="w-3 h-3" aria-hidden="true" />
                  </button>
                  <button
                    onClick={() => onDeleteFolder(folder)}
                    aria-label={`Delete ${folder.name}`}
                    className="p-0.5 rounded text-text-3 hover:text-danger cursor-pointer"
                  >
                    <X className="w-3 h-3" aria-hidden="true" />
                  </button>
                </span>
              )}
            </div>
          );
        })}
      </div>

      {/* Grid */}
      <div className="flex-1 overflow-y-auto p-4">
        {generating && (
          <div className="mb-4 flex items-center gap-3 rounded-xl border border-accent-primary/30 bg-accent-primary/5 px-4 py-3">
            <Loader2 className="w-4 h-4 animate-spin text-accent-text" aria-hidden="true" />
            <p className="text-sm text-text-1">
              Generating {generatingCount > 1 ? `${generatingCount} items` : ''}…
              <span className="text-text-3"> results appear here as they finish</span>
            </p>
          </div>
        )}

        {loading ? (
          <div className="flex items-center justify-center py-20">
            <Loader2 className="w-5 h-5 animate-spin text-text-3" aria-hidden="true" />
          </div>
        ) : assets.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <Sparkles className="w-8 h-8 mb-3 text-text-3" aria-hidden="true" />
            <p className="text-sm text-text-1 mb-1">Nothing here yet</p>
            <p className="text-xs text-text-3 max-w-xs leading-relaxed">
              Write a prompt on the left and press Generate. Everything you make lands here and in
              the media library.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-3">
            {assets.map(a => {
              const Icon = KIND_ICON[a.media_type] ?? ImageIcon;
              return (
                <div
                  key={a.id}
                  className="group flex flex-col rounded-xl border border-border-0 bg-surface-1 overflow-hidden hover:border-border-1 transition-colors"
                >
                  <div className="relative bg-surface-0 flex items-center justify-center aspect-square">
                    {a.media_type === 'image' ? (
                      <button
                        onClick={() => setPreview(a)}
                        className="w-full h-full cursor-zoom-in"
                        aria-label="Open preview"
                      >
                        <img
                          src={a.local_url}
                          alt={a.prompt}
                          loading="lazy"
                          className="w-full h-full object-cover"
                        />
                      </button>
                    ) : a.media_type === 'video' ? (
                      <video
                        src={a.local_url}
                        controls
                        preload="metadata"
                        className="w-full h-full object-contain bg-black"
                      />
                    ) : (
                      <div className="flex flex-col items-center justify-center gap-3 w-full h-full p-4">
                        <Icon className="w-8 h-8 text-accent-text" aria-hidden="true" />
                        <audio src={a.local_url} controls className="w-full" />
                      </div>
                    )}
                  </div>

                  <div className="p-2.5 space-y-2">
                    <p className="text-[11px] text-text-2 line-clamp-2 leading-snug" title={a.prompt}>
                      {a.prompt || 'No prompt'}
                    </p>
                    <p className="text-[10px] text-text-3 truncate">
                      {a.provider || 'unknown'} · {shortModel(a.source_model)}
                    </p>

                    <div className="flex items-center gap-1">
                      <a
                        href={studio.downloadUrl(a.id)}
                        download
                        title="Download"
                        aria-label="Download"
                        className="p-1.5 rounded-md text-text-3 hover:text-text-0 hover:bg-surface-2 transition-colors"
                      >
                        <Download className="w-3.5 h-3.5" aria-hidden="true" />
                      </a>
                      <button
                        onClick={() => onUsePrompt(a.prompt)}
                        title="Load this prompt into the editor"
                        aria-label="Reuse prompt"
                        className="p-1.5 rounded-md text-text-3 hover:text-accent-text hover:bg-surface-2 transition-colors cursor-pointer"
                      >
                        <Sparkles className="w-3.5 h-3.5" aria-hidden="true" />
                      </button>
                      <button
                        onClick={() => setMovingId(movingId === a.id ? null : a.id)}
                        title="Move to folder"
                        aria-label="Move to folder"
                        className="p-1.5 rounded-md text-text-3 hover:text-text-0 hover:bg-surface-2 transition-colors cursor-pointer"
                      >
                        <FolderInput className="w-3.5 h-3.5" aria-hidden="true" />
                      </button>
                      <button
                        onClick={() => onDelete(a)}
                        title="Delete"
                        aria-label="Delete"
                        className="ml-auto p-1.5 rounded-md text-text-3 hover:text-danger hover:bg-surface-2 transition-colors cursor-pointer"
                      >
                        <Trash2 className="w-3.5 h-3.5" aria-hidden="true" />
                      </button>
                    </div>

                    {movingId === a.id && (
                      <select
                        autoFocus
                        value={a.folder_id}
                        onChange={e => {
                          onMove(a, e.target.value);
                          setMovingId(null);
                        }}
                        className="w-full rounded-md border border-border-1 bg-surface-2 text-text-1 px-2 py-1 text-[11px] focus:border-accent-primary outline-none"
                      >
                        <option value="">Unfiled</option>
                        {folders.map(f => (
                          <option key={f.id} value={f.id}>
                            {f.name}
                          </option>
                        ))}
                      </select>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Lightbox */}
      {preview && (
        <div
          className="fixed inset-0 z-[100] flex items-center justify-center p-6 bg-black/80 backdrop-blur-sm"
          onClick={e => {
            if (e.target === e.currentTarget) setPreview(null);
          }}
          role="dialog"
          aria-modal="true"
          aria-label="Image preview"
        >
          <div className="max-w-5xl max-h-full flex flex-col gap-3">
            <img
              src={preview.local_url}
              alt={preview.prompt}
              className="max-h-[80vh] w-auto object-contain rounded-xl"
            />
            <div className="flex items-start gap-3">
              <p className="flex-1 text-xs text-white/70 leading-relaxed">{preview.prompt}</p>
              <a href={studio.downloadUrl(preview.id)} download>
                <Button size="sm" variant="secondary" icon={<Download className="w-4 h-4" />}>
                  Download
                </Button>
              </a>
              <Button size="sm" variant="secondary" onClick={() => setPreview(null)}>
                Close
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function shortModel(id: string) {
  if (!id) return 'default';
  const slash = id.lastIndexOf('/');
  return slash >= 0 ? id.slice(slash + 1) : id;
}
