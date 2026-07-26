/**
 * AgentSkillFiles
 *
 * The contents of one skill as the agent has it.
 *
 * An agent's skill is a copy, and after copying it is edited in place — by the
 * user, or by the agent's own file tools. Nothing in the UI could see past
 * SKILL.md, so a skill the agent had filled out with sub-skills and references
 * showed here as a single short document, and the agent describing its real
 * contents read as if it were making things up.
 */

import { useCallback, useEffect, useState } from 'react';
import { Loader2, Save } from 'lucide-react';
import { agentSkills } from '../../lib/api-helpers';
import type { SkillFile } from '../../lib/types';
import { SkillFileBrowser } from './SkillFileBrowser';
import { Button } from '../Button';
import { useToast } from '../Toast';

export function AgentSkillFiles({ slug, skillName }: { slug: string; skillName: string }) {
  const { toast } = useToast();
  const [files, setFiles] = useState<SkillFile[]>([]);
  const [activePath, setActivePath] = useState('SKILL.md');
  const [content, setContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const loadFiles = useCallback(async () => {
    try {
      const list = await agentSkills.files(slug, skillName);
      setFiles(list);
      return list;
    } catch {
      setFiles([]);
      return [];
    }
  }, [slug, skillName]);

  const openFile = useCallback(async (path: string) => {
    setActivePath(path);
    try {
      const file = await agentSkills.readFile(slug, skillName, path);
      setContent(file.content);
    } catch {
      setContent('');
    }
  }, [slug, skillName]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    (async () => {
      const list = await loadFiles();
      if (cancelled) return;
      const first = list.find(f => f.path === 'SKILL.md') ?? list.find(f => f.editable);
      if (first) await openFile(first.path);
      if (!cancelled) setLoading(false);
    })();
    return () => { cancelled = true; };
  }, [loadFiles, openFile]);

  const save = async () => {
    setSaving(true);
    try {
      await agentSkills.writeFile(slug, skillName, activePath, content);
      await loadFiles();
      toast('success', `Saved ${activePath}`);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const create = async (path: string) => {
    try {
      await agentSkills.writeFile(slug, skillName, path, '');
      await loadFiles();
      await openFile(path);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to create file');
    }
  };

  const remove = async (path: string) => {
    try {
      await agentSkills.deleteFile(slug, skillName, path);
      const list = await loadFiles();
      if (path === activePath) {
        const next = list.find(f => f.editable);
        if (next) await openFile(next.path);
        else { setActivePath(''); setContent(''); }
      }
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to delete file');
    }
  };

  if (loading) {
    return (
      <div className="flex items-center gap-2 px-3 py-6 text-xs text-text-3">
        <Loader2 className="w-3.5 h-3.5 animate-spin" aria-hidden="true" />
        Loading files...
      </div>
    );
  }

  return (
    <div className="flex flex-col md:flex-row gap-3 p-3 border-t border-border-0">
      <SkillFileBrowser
        files={files}
        activePath={activePath}
        onSelect={openFile}
        onCreate={create}
        onDelete={remove}
      />
      <div className="flex-1 min-w-0 flex flex-col gap-2">
        <div className="flex items-center justify-between gap-2">
          <span className="text-[11px] font-mono text-text-3 truncate">{activePath || 'No file selected'}</span>
          <Button size="sm" onClick={save} loading={saving} disabled={!activePath} icon={<Save className="w-3.5 h-3.5" />}>
            Save
          </Button>
        </div>
        <textarea
          value={content}
          onChange={e => setContent(e.target.value)}
          spellCheck={false}
          className="w-full h-72 px-3 py-2 rounded-lg bg-surface-0 border border-border-1 text-text-1 text-xs font-mono leading-relaxed focus:outline-none focus:ring-1 focus:ring-accent-primary resize-y"
        />
      </div>
    </div>
  );
}
