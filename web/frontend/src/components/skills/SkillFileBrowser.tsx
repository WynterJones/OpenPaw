/**
 * SkillFileBrowser
 *
 * A skill is a directory, not a document. Its SKILL.md can say "run
 * scripts/deploy.sh" or "read references/api.md" and that works — the agent is
 * handed the folder's real path and has file and shell tools. But nothing in
 * the app could ever put a second file there, so every skill was a lone
 * SKILL.md and the instructions had nothing to point at.
 *
 * This is the sidebar that makes the rest of the folder real: list what is
 * there, open it, add to it, remove from it.
 */

import { useState } from 'react';
import { FileText, FileCode, Image, Plus, Trash2, Lock } from 'lucide-react';
import type { SkillFile } from '../../lib/types';

/** Groups files by their top-level directory so the shape of the skill reads. */
function groupByDir(files: SkillFile[]): { dir: string; files: SkillFile[] }[] {
  const groups = new Map<string, SkillFile[]>();
  for (const f of files) {
    const slash = f.path.indexOf('/');
    const dir = slash === -1 ? '' : f.path.slice(0, slash);
    const list = groups.get(dir);
    if (list) list.push(f);
    else groups.set(dir, [f]);
  }
  // Root files first — SKILL.md lives there and is the entry point.
  return [...groups.entries()]
    .sort((a, b) => (a[0] === '' ? -1 : b[0] === '' ? 1 : a[0].localeCompare(b[0])))
    .map(([dir, files]) => ({ dir, files }));
}

function iconFor(file: SkillFile) {
  if (!file.editable) return Image;
  if (/\.(sh|bash|zsh|py|rb|pl|js|ts|go)$/i.test(file.path)) return FileCode;
  return FileText;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function SkillFileBrowser({
  files,
  activePath,
  onSelect,
  onCreate,
  onDelete,
}: {
  files: SkillFile[];
  activePath: string;
  onSelect: (path: string) => void;
  onCreate: (path: string) => void;
  onDelete: (path: string) => void;
}) {
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState('');

  const submit = () => {
    const path = draft.trim().replace(/^\/+/, '');
    if (!path) return;
    onCreate(path);
    setDraft('');
    setAdding(false);
  };

  return (
    <div className="w-full md:w-56 flex-shrink-0 rounded-xl border border-border-0 bg-surface-1 overflow-hidden">
      <div className="px-3 py-2 border-b border-border-0 flex items-center justify-between gap-2">
        <span className="text-xs font-semibold text-text-1">Files</span>
        <button
          onClick={() => setAdding(v => !v)}
          className="p-1 rounded text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
          title="Add a file"
          aria-label="Add a file"
        >
          <Plus className="w-3.5 h-3.5" />
        </button>
      </div>

      {adding && (
        <div className="p-2 border-b border-border-0">
          <input
            value={draft}
            onChange={e => setDraft(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter') { e.preventDefault(); submit(); }
              if (e.key === 'Escape') { setAdding(false); setDraft(''); }
            }}
            // The placeholder teaches the convention: a path, not just a name.
            // scripts/ and references/ are what SKILL.md files refer to.
            placeholder="scripts/deploy.sh"
            autoFocus
            spellCheck={false}
            className="w-full px-2 py-1.5 rounded-lg bg-surface-0 border border-border-1 text-text-1 text-xs font-mono focus:outline-none focus:ring-1 focus:ring-accent-primary"
          />
          <p className="text-[10px] text-text-3 mt-1">Enter to create. Folders are made as needed.</p>
        </div>
      )}

      <div className="max-h-[440px] overflow-y-auto">
        {groupByDir(files).map(({ dir, files: group }) => (
          <div key={dir || '__root'}>
            {dir && (
              <div className="px-3 pt-2 pb-1 text-[10px] font-semibold uppercase tracking-wide text-text-3">
                {dir}/
              </div>
            )}
            {group.map(file => {
              const Icon = iconFor(file);
              const isActive = file.path === activePath;
              const leaf = dir ? file.path.slice(dir.length + 1) : file.path;
              return (
                <div
                  key={file.path}
                  className={`group flex items-center gap-2 px-3 py-1.5 transition-colors ${
                    isActive ? 'bg-surface-2' : 'hover:bg-surface-2'
                  }`}
                >
                  <button
                    onClick={() => file.editable && onSelect(file.path)}
                    disabled={!file.editable}
                    className={`flex items-center gap-2 flex-1 min-w-0 text-left ${
                      file.editable ? 'cursor-pointer' : 'cursor-default'
                    }`}
                    title={file.editable ? file.path : `${file.path} — not a text file`}
                  >
                    <Icon
                      className={`w-3.5 h-3.5 flex-shrink-0 ${isActive ? 'text-accent-primary' : 'text-text-3'}`}
                      aria-hidden="true"
                    />
                    <span className={`flex-1 min-w-0 truncate text-xs font-mono ${
                      isActive ? 'text-text-0 font-semibold' : file.editable ? 'text-text-1' : 'text-text-3'
                    }`}>
                      {leaf}
                    </span>
                    <span className="text-[10px] text-text-3 flex-shrink-0">{formatSize(file.size)}</span>
                  </button>
                  {file.path === 'SKILL.md' ? (
                    // Deleting it would make the folder stop being a skill —
                    // it disappears from the list and from the agent's prompt.
                    <Lock className="w-3 h-3 text-text-3/50 flex-shrink-0" aria-label="SKILL.md cannot be deleted" />
                  ) : (
                    <button
                      onClick={() => onDelete(file.path)}
                      className="p-0.5 rounded flex-shrink-0 text-text-3 opacity-0 group-hover:opacity-100 hover:text-danger transition-all cursor-pointer"
                      title={`Delete ${file.path}`}
                      aria-label={`Delete ${file.path}`}
                    >
                      <Trash2 className="w-3 h-3" />
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        ))}
      </div>
    </div>
  );
}
