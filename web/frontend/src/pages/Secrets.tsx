import { useState, useEffect } from 'react';
import { KeyRound, Plus, RotateCcw, Trash2, Shield, Activity } from 'lucide-react';
import { Header } from '../components/Header';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { Modal } from '../components/Modal';
import { Input, Select } from '../components/Input';
import { EmptyState } from '../components/EmptyState';
import { Pagination } from '../components/Pagination';
import { DataTable } from '../components/DataTable';
import { SearchBar } from '../components/SearchBar';
import { ViewToggle, type ViewMode } from '../components/ViewToggle';
import { api, type Secret, type Tool } from '../lib/api';
import { useToast } from '../components/Toast';

const PAGE_SIZE = 12;

export function Secrets() {
  const { toast } = useToast();
  const [secrets, setSecrets] = useState<Secret[]>([]);
  const [tools, setTools] = useState<Tool[]>([]);
  const [search, setSearch] = useState('');
  const [view, setView] = useState<ViewMode>('list');
  const [page, setPage] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [value, setValue] = useState('');
  const [toolId, setToolId] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => { loadData(); }, []);

  const loadData = async () => {
    try {
      const [secretsData, toolsData] = await Promise.all([api.get<Secret[]>('/secrets'), api.get<Tool[]>('/tools')]);
      setSecrets(secretsData || []); setTools(toolsData || []);
    } catch (e) { console.warn('loadSecrets failed:', e); setSecrets([]); setTools([]); } finally { setLoaded(true); }
  };

  const addSecret = async () => {
    setSaving(true);
    try { await api.post('/secrets', { name, value, tool_id: toolId || null }); toast('success', 'Secret added'); setAddOpen(false); setName(''); setValue(''); setToolId(''); loadData(); }
    catch (err) { toast('error', err instanceof Error ? err.message : 'Failed to add secret'); } finally { setSaving(false); }
  };

  const rotateSecret = async (id: string) => { try { await api.post(`/secrets/${id}/rotate`); toast('success', 'Secret rotated'); loadData(); } catch (err) { toast('error', err instanceof Error ? err.message : 'Failed to rotate secret'); } };
  const deleteSecret = async () => { if (!deleteId) return; try { await api.delete(`/secrets/${deleteId}`); toast('success', 'Secret deleted'); setDeleteId(null); loadData(); } catch (err) { toast('error', err instanceof Error ? err.message : 'Failed to delete secret'); } };
  const testConnection = async (id: string) => { try { const data = await api.post<{ status: string }>(`/secrets/${id}/test`); toast(data.status === 'ok' ? 'success' : 'warning', data.status === 'ok' ? 'Connection successful' : 'Connection test returned warnings'); } catch (err) { toast('error', err instanceof Error ? err.message : 'Connection test failed'); } };

  const handleSearch = (val: string) => { setSearch(val); setPage(0); };

  const filtered = secrets.filter(s => s.name.toLowerCase().includes(search.toLowerCase()));
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const paginated = filtered.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
  const toolOptions = [{ value: '', label: 'No association' }, ...tools.map(t => ({ value: t.id, label: t.name }))];

  return (
    <div className="flex flex-col h-full">
      <Header title="Secrets" />
      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="flex items-center gap-3 mb-4">
          <SearchBar value={search} onChange={handleSearch} placeholder="Search secrets..." className="flex-1" />
          <ViewToggle view={view} onViewChange={setView} />
          <Button onClick={() => setAddOpen(true)} icon={<Plus className="w-4 h-4" />}>Add Secret</Button>
        </div>
        {!loaded ? (
          <div className="flex items-center justify-center py-16"><div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" /></div>
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={<KeyRound className="w-8 h-8" />}
            title={search ? 'No secrets found' : 'No secrets yet'}
            description={search ? 'Try a different search term.' : 'Add secrets for your tools to use API keys, passwords, and other credentials securely.'}
          />
        ) : view === 'grid' ? (
          <>
            <Pagination page={page} totalPages={totalPages} total={filtered.length} onPageChange={setPage} label="secrets" />
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
              {paginated.map(s => (
                <Card key={s.id}>
                  <div className="flex flex-col gap-3">
                    <div className="flex items-start justify-between">
                      <div className="w-10 h-10 rounded-xl bg-accent-muted flex items-center justify-center flex-shrink-0">
                        <Shield className="w-5 h-5 text-accent-primary" />
                      </div>
                      <div className="flex items-center gap-1">
                        <button onClick={() => testConnection(s.id)} className="p-1.5 rounded-lg text-text-3 hover:text-accent-text hover:bg-surface-2 transition-colors cursor-pointer" title="Test connection" aria-label="Test connection"><Activity className="w-4 h-4" /></button>
                        <button onClick={() => rotateSecret(s.id)} className="p-1.5 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer" title="Rotate" aria-label="Rotate secret"><RotateCcw className="w-4 h-4" /></button>
                        <button onClick={() => setDeleteId(s.id)} className="p-1.5 rounded-lg text-text-3 hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer" title="Delete" aria-label="Delete secret"><Trash2 className="w-4 h-4" /></button>
                      </div>
                    </div>
                    <div>
                      <p className="text-sm font-semibold text-text-0 font-mono truncate">{s.name}</p>
                      <p className="text-xs text-text-3 mt-1">{s.tool_name || 'Global'} &middot; {s.scope}</p>
                    </div>
                    <div className="text-xs text-text-3">
                      Rotated: {s.last_rotated ? new Date(s.last_rotated).toLocaleDateString() : 'Never'}
                    </div>
                  </div>
                </Card>
              ))}
            </div>
          </>
        ) : (
          <>
            <Pagination page={page} totalPages={totalPages} total={filtered.length} onPageChange={setPage} label="secrets" />
            <Card padding={false}>
              <DataTable
                columns={[
                  { key: 'name', header: 'Name', render: (s: Secret) => (<div className="flex items-center gap-2"><Shield className="w-4 h-4 text-accent-primary flex-shrink-0" /><div className="min-w-0"><span className="font-medium text-text-0 block truncate">{s.name}</span><span className="text-xs text-text-3 md:hidden">{s.tool_name || 'Global'}</span></div></div>) },
                  { key: 'value', header: 'Value', hideOnMobile: true, render: () => (<span className="text-sm text-text-3 font-mono tracking-wider">&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;&#x2022;</span>) },
                  { key: 'tool', header: 'Tool', hideOnMobile: true, render: (s: Secret) => (<span className="text-sm text-text-2">{s.tool_name || 'Global'}</span>) },
                  { key: 'scope', header: 'Scope', hideOnMobile: true, render: (s: Secret) => (<span className="text-xs px-2 py-0.5 rounded-full bg-surface-2 text-text-1">{s.scope}</span>) },
                  { key: 'rotated', header: 'Last Rotated', hideOnMobile: true, render: (s: Secret) => (<span className="text-sm text-text-2">{s.last_rotated ? new Date(s.last_rotated).toLocaleDateString() : 'Never'}</span>) },
                  { key: 'actions', header: '', className: 'text-right', render: (s: Secret) => (
                    <div className="flex items-center justify-end gap-1">
                      <button onClick={(e) => { e.stopPropagation(); testConnection(s.id); }} className="p-1.5 rounded-lg text-text-2 hover:text-accent-text hover:bg-surface-2 transition-colors cursor-pointer" title="Test connection" aria-label="Test connection"><Activity className="w-4 h-4" /></button>
                      <button onClick={(e) => { e.stopPropagation(); rotateSecret(s.id); }} className="p-1.5 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer" title="Rotate secret" aria-label="Rotate secret"><RotateCcw className="w-4 h-4" /></button>
                      <button onClick={(e) => { e.stopPropagation(); setDeleteId(s.id); }} className="p-1.5 rounded-lg text-text-2 hover:text-red-400 hover:bg-surface-2 transition-colors cursor-pointer" title="Delete secret" aria-label="Delete secret"><Trash2 className="w-4 h-4" /></button>
                    </div>
                  )},
                ]}
                data={paginated} keyExtractor={s => s.id}
                emptyState={<EmptyState icon={<KeyRound className="w-8 h-8" />} title={search ? 'No secrets found' : 'No secrets yet'}
                  description={search ? 'Try a different search term.' : 'Add secrets for your tools to use API keys, passwords, and other credentials securely.'} />}
              />
            </Card>
          </>
        )}
      </div>
      <Modal open={addOpen} onClose={() => setAddOpen(false)} title="Add Secret">
        <div className="space-y-4">
          <Input label="Name" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. SLACK_API_TOKEN" />
          <Input label="Value" type="password" value={value} onChange={e => setValue(e.target.value)} placeholder="Enter secret value" />
          <Select label="Associated Tool" value={toolId} onChange={e => setToolId(e.target.value)} options={toolOptions} />
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setAddOpen(false)}>Cancel</Button>
            <Button onClick={addSecret} loading={saving} disabled={!name.trim() || !value.trim()}>Add Secret</Button>
          </div>
        </div>
      </Modal>
      <Modal open={!!deleteId} onClose={() => setDeleteId(null)} title="Delete Secret" size="sm">
        <div className="space-y-4">
          <p className="text-sm text-text-1">Are you sure you want to delete this secret? This action cannot be undone. Any tools using this secret will lose access.</p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setDeleteId(null)}>Cancel</Button>
            <Button variant="danger" onClick={deleteSecret}>Delete</Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
