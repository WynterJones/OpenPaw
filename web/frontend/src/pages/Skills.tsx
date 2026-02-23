import { useState, useEffect, useCallback } from 'react';
import { Plus, Trash2, X, Save, Wrench } from 'lucide-react';
import { Header } from '../components/Header';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Input } from '../components/Input';
import { Modal } from '../components/Modal';
import { EmptyState } from '../components/EmptyState';
import { LoadingSpinner } from '../components/LoadingSpinner';
import { Pagination } from '../components/Pagination';
import { SearchBar } from '../components/SearchBar';
import { ViewToggle, type ViewMode } from '../components/ViewToggle';
import { FolderFilter } from '../components/FolderFilter';
import { FolderSection } from '../components/FolderSection';
import { FolderAssign } from '../components/FolderAssign';
import { useFolderGrouping } from '../hooks/useFolderGrouping';
import { useToast } from '../components/Toast';
import { skills as skillsApi, type Skill } from '../lib/api';

function CreateSkillModal({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (skill: Skill) => void }) {
  const { toast } = useToast();
  const [name, setName] = useState('');
  const [content, setContent] = useState('---\nname: skill-name\ndescription: What this skill does\n---\n\n# Skill Name\n\nDescribe what this skill does and how to use it.\n');
  const [saving, setSaving] = useState(false);

  const handleCreate = async () => {
    const slug = name.trim().toLowerCase().replace(/\s+/g, '-').replace(/[^a-z0-9-]/g, '');
    if (!slug) {
      toast('error', 'Name is required');
      return;
    }
    setSaving(true);
    try {
      const skill = await skillsApi.create(slug, content);
      onCreated(skill);
      onClose();
      setName('');
      setContent('---\nname: skill-name\ndescription: What this skill does\n---\n\n# Skill Name\n\nDescribe what this skill does and how to use it.\n');
      toast('success', 'Skill created');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to create skill');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose} title="Create Skill" size="md">
      <div className="space-y-4">
        <Input label="Name" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. code-review" />
        <div>
          <label className="block text-xs font-medium text-text-1 mb-1.5">Content (Markdown)</label>
          <textarea
            value={content}
            onChange={e => setContent(e.target.value)}
            rows={12}
            className="w-full rounded-lg border border-border-1 bg-surface-0 text-text-1 px-3 py-2 text-[13px] font-mono placeholder:text-text-3/50 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors resize-none"
            spellCheck={false}
          />
        </div>
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button onClick={handleCreate} loading={saving} disabled={!name.trim()} icon={<Plus className="w-4 h-4" />}>Create</Button>
        </div>
      </div>
    </Modal>
  );
}

const PAGE_SIZE = 12;

export function Skills() {
  const { toast } = useToast();
  const [skillList, setSkillList] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<Skill | null>(null);
  const [editContent, setEditContent] = useState('');
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState('');
  const [view, setView] = useState<ViewMode>('list');
  const [page, setPage] = useState(0);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const loadSkills = useCallback(() => {
    skillsApi.list()
      .then(data => setSkillList(data || []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    loadSkills();
  }, [loadSkills]);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleteLoading(true);
    try {
      await skillsApi.delete(deleteTarget);
      setSkillList(prev => prev.filter(s => s.name !== deleteTarget));
      if (editing?.name === deleteTarget) setEditing(null);
      setDeleteTarget(null);
      toast('success', 'Skill deleted');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to delete skill');
    } finally {
      setDeleteLoading(false);
    }
  };

  const handleEdit = async (skill: Skill) => {
    try {
      const full = await skillsApi.get(skill.name);
      setEditing(full);
      setEditContent(full.content || '');
    } catch (e) {
      console.warn('loadSkillDetail failed:', e);
      setEditing(skill);
      setEditContent(skill.content || '');
    }
  };

  const handleSaveEdit = async () => {
    if (!editing) return;
    setSaving(true);
    try {
      await skillsApi.update(editing.name, { content: editContent });
      setSkillList(prev => prev.map(s => s.name === editing.name ? { ...s, content: editContent, summary: editContent.split('\n').find(l => l.trim())?.replace(/^#+\s*/, '') || '' } : s));
      setEditing(null);
      toast('success', 'Skill saved');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to save skill');
    } finally {
      setSaving(false);
    }
  };

  const handleFolderChange = async (skill: Skill, folder: string) => {
    try {
      await skillsApi.update(skill.name, { folder });
      setSkillList(prev => prev.map(s => s.name === skill.name ? { ...s, folder } : s));
      toast('success', 'Folder updated');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to update folder');
    }
  };

  const handleSearch = (val: string) => { setSearch(val); setPage(0); };

  const getFolder = useCallback((s: Skill) => s.folder || '', []);
  const folderGrouping = useFolderGrouping(skillList, getFolder);

  const searchFiltered = folderGrouping.filtered.filter(skill => {
    if (!search) return true;
    const term = search.toLowerCase();
    return skill.name.toLowerCase().includes(term) || (skill.description || skill.summary || '').toLowerCase().includes(term);
  });

  const showFolderSections = folderGrouping.selectedFolder === null && folderGrouping.folders.length > 0;
  const totalPages = showFolderSections ? 1 : Math.max(1, Math.ceil(searchFiltered.length / PAGE_SIZE));
  const paginatedSkills = showFolderSections ? searchFiltered : searchFiltered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  const renderSkillContent = (items: Skill[]) => view === 'grid' ? (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
      {items.map(skill => (
        <Card key={skill.name} hover onClick={() => handleEdit(skill)}>
          <div className="min-w-0">
            <p className="text-sm font-semibold text-text-0 font-mono truncate">{skill.name}</p>
            <p className="text-xs text-text-3 line-clamp-2 mt-1">{skill.description || skill.summary || 'No description'}</p>
          </div>
        </Card>
      ))}
    </div>
  ) : (
    <div className="space-y-3">
      {items.map(skill => (
        <Card key={skill.name} hover onClick={() => handleEdit(skill)}>
          <div className="flex items-center gap-3">
            <div className="flex-1 min-w-0">
              <p className="text-sm font-semibold text-text-0 font-mono">{skill.name}</p>
              <p className="text-xs text-text-3 truncate">{skill.description || skill.summary || 'No description'}</p>
            </div>
          </div>
        </Card>
      ))}
    </div>
  );

  return (
    <div className="flex flex-col h-full">
      <Header title="Skills" />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        {editing ? (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <button onClick={() => setEditing(null)} aria-label="Close editor" className="p-1.5 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer">
                  <X className="w-5 h-5" />
                </button>
                <h2 className="text-base font-semibold text-text-0 font-mono">{editing.name}</h2>
                <div className="w-[160px]">
                  <FolderAssign
                    value={editing.folder || ''}
                    folders={folderGrouping.folders}
                    onChange={(f) => handleFolderChange(editing, f)}
                  />
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button size="sm" variant="danger" onClick={() => setDeleteTarget(editing.name)} icon={<Trash2 className="w-3.5 h-3.5" />}>
                  Delete
                </Button>
                <Button size="sm" onClick={handleSaveEdit} loading={saving} icon={<Save className="w-3.5 h-3.5" />}>
                  Save
                </Button>
              </div>
            </div>
            <Card>
              <textarea
                value={editContent}
                onChange={e => setEditContent(e.target.value)}
                className="w-full min-h-[500px] rounded-lg border border-border-1 bg-surface-0 text-text-1 px-4 py-3 text-[13px] font-mono placeholder:text-text-3/50 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors resize-none"
                spellCheck={false}
              />
            </Card>
          </div>
        ) : loading ? (
          <LoadingSpinner message="Loading skills..." />
        ) : (
          <>
                <div className="flex items-center gap-3 mb-4">
                  <SearchBar value={search} onChange={handleSearch} placeholder="Search skills..." className="flex-1" />
                  <ViewToggle view={view} onViewChange={setView} />
                  <Button onClick={() => setCreateOpen(true)} icon={<Plus className="w-4 h-4" />} className="flex-shrink-0">Add Skill</Button>
                </div>
                <FolderFilter
                  folders={folderGrouping.folders}
                  folderCounts={folderGrouping.folderCounts}
                  unfiledCount={folderGrouping.unfiledCount}
                  totalCount={folderGrouping.totalCount}
                  selectedFolder={folderGrouping.selectedFolder}
                  onSelect={(f) => { folderGrouping.setSelectedFolder(f); setPage(0); }}
                  onAddFolder={() => {}}
                />

                {searchFiltered.length === 0 ? (
                  <EmptyState
                    icon={<Wrench className="w-8 h-8" />}
                    title={search ? 'No skills found' : 'No skills yet'}
                    description={search ? 'Try a different search term.' : 'Create a skill or install one from the Library page.'}
                  />
                ) : showFolderSections ? (
                  <div className="space-y-4">
                    {folderGrouping.folders.map((folder) => {
                      const items = (folderGrouping.grouped.get(folder) || []).filter(s => {
                        if (!search) return true;
                        const term = search.toLowerCase();
                        return s.name.toLowerCase().includes(term) || (s.description || s.summary || '').toLowerCase().includes(term);
                      });
                      if (items.length === 0) return null;
                      return (
                        <FolderSection key={folder} name={folder} count={items.length}>
                          {renderSkillContent(items)}
                        </FolderSection>
                      );
                    })}
                    {(() => {
                      const unfiled = (folderGrouping.grouped.get('') || []).filter(s => {
                        if (!search) return true;
                        const term = search.toLowerCase();
                        return s.name.toLowerCase().includes(term) || (s.description || s.summary || '').toLowerCase().includes(term);
                      });
                      if (unfiled.length === 0) return null;
                      return (
                        <FolderSection name="" count={unfiled.length}>
                          {renderSkillContent(unfiled)}
                        </FolderSection>
                      );
                    })()}
                  </div>
                ) : (
                  <>
                    <Pagination page={page} totalPages={totalPages} total={searchFiltered.length} onPageChange={setPage} label="skills" />
                    {renderSkillContent(paginatedSkills)}
                  </>
                )}
          </>
        )}
      </div>

      <CreateSkillModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(skill) => setSkillList(prev => [...prev, skill])}
      />

      <Modal open={!!deleteTarget} onClose={() => setDeleteTarget(null)} title="Delete Skill" size="sm">
        <div className="space-y-4">
          <p className="text-sm text-text-1">
            Are you sure you want to delete <strong className="text-text-0">{deleteTarget}</strong>? This action cannot be undone.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setDeleteTarget(null)}>Cancel</Button>
            <Button variant="danger" onClick={handleDelete} loading={deleteLoading}>Delete</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
