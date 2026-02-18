import { useState, useEffect, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router';
import { ArrowLeft, Save, Upload, BookOpen, Shield } from 'lucide-react';
import { Button } from '../components/Button';
import { Card } from '../components/Card';
import { LoadingSpinner } from '../components/LoadingSpinner';
import { useToast } from '../components/Toast';
import { api, gatewayFiles, type AgentRole, type MemoryFile } from '../lib/api';

interface FileTab {
  key: string;
  label: string;
  filename: string;
  description: string;
}

const FILE_TABS: FileTab[] = [
  { key: 'soul', label: 'Soul', filename: 'SOUL.md', description: 'Personality, name, tone, values — the core of who the gateway is.' },
  { key: 'user', label: 'User', filename: 'USER.md', description: 'What the gateway knows about you, learned from conversations.' },
  { key: 'heartbeat', label: 'Heartbeat', filename: 'HEARTBEAT.md', description: 'Periodic self-check instructions. Empty = skip.' },
];

export function GatewayEdit() {
  const navigate = useNavigate();
  const { toast } = useToast();
  const promptRef = useRef<HTMLTextAreaElement>(null);

  const [loading, setLoading] = useState(true);
  const [gatewayRole, setGatewayRole] = useState<AgentRole | null>(null);
  const [avatarPath, setAvatarPath] = useState('/gateway-avatar.png');
  const [uploading, setUploading] = useState(false);

  const [activeTab, setActiveTab] = useState('soul');
  const [files, setFiles] = useState<Record<string, string>>({});
  const [fileDirty, setFileDirty] = useState<Record<string, boolean>>({});
  const [fileSaving, setFileSaving] = useState(false);

  const [memoryFiles, setMemoryFiles] = useState<MemoryFile[]>([]);
  const [memoryContent, setMemoryContent] = useState('');
  const [memoryDirty, setMemoryDirty] = useState(false);

  const loadFiles = useCallback(async () => {
    try {
      const data = await gatewayFiles.getAll();
      setFiles(data);
      setFileDirty({});
    } catch (e) { console.warn('loadGatewayFiles failed:', e); }
  }, []);

  const loadMemory = useCallback(async () => {
    try {
      const mem = await gatewayFiles.listMemory();
      setMemoryFiles(mem);
      const result = await gatewayFiles.get('memory/memory.md');
      setMemoryContent(result.content);
      setMemoryDirty(false);
    } catch (e) { console.warn('loadGatewayMemory failed:', e); }
  }, []);

  useEffect(() => {
    Promise.all([
      api.get<AgentRole[]>('/agent-roles').then(roles => {
        const builder = roles?.find(r => r.slug === 'builder');
        if (builder) {
          setGatewayRole(builder);
          setAvatarPath(builder.avatar_path || '/gateway-avatar.png');
        }
      }),
      loadFiles(),
      loadMemory(),
    ]).finally(() => setLoading(false));
  }, [loadFiles, loadMemory]);

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
      toast('error', 'Please upload a PNG, JPEG, or WebP image');
      return;
    }
    setUploading(true);
    try {
      const formData = new FormData();
      formData.append('avatar', file);
      const csrfHeaders: Record<string, string> = {};
      const csrf = (await import('../lib/api')).getCSRFToken();
      if (csrf) csrfHeaders['X-CSRF-Token'] = csrf;
      const res = await fetch('/api/v1/agent-roles/upload-avatar', {
        method: 'POST',
        headers: csrfHeaders,
        body: formData,
        credentials: 'same-origin',
      });
      if (!res.ok) throw new Error('Upload failed');
      const data = await res.json();
      setAvatarPath(data.avatar_path);
      if (gatewayRole) {
        await api.put(`/agent-roles/${gatewayRole.slug}`, { avatar_path: data.avatar_path });
      }
      toast('success', 'Avatar uploaded');
    } catch (e) {
      console.warn('uploadGatewayAvatar failed:', e);
      toast('error', 'Failed to upload avatar');
    } finally {
      setUploading(false);
    }
  };

  const handleFileChange = (filename: string, content: string) => {
    setFiles(prev => ({ ...prev, [filename]: content }));
    setFileDirty(prev => ({ ...prev, [filename]: true }));
  };

  const handleFileSave = async (filename: string) => {
    setFileSaving(true);
    try {
      await gatewayFiles.update(filename, files[filename] || '');
      setFileDirty(prev => ({ ...prev, [filename]: false }));
      toast('success', `${filename} saved`);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to save file');
    } finally {
      setFileSaving(false);
    }
  };

  const handleMemorySave = async () => {
    setFileSaving(true);
    try {
      await gatewayFiles.update('memory/memory.md', memoryContent);
      setMemoryDirty(false);
      toast('success', 'Memory saved');
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Failed to save memory');
    } finally {
      setFileSaving(false);
    }
  };

  const handlePromptKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Tab') {
      e.preventDefault();
      const ta = promptRef.current;
      if (!ta) return;
      const start = ta.selectionStart;
      const end = ta.selectionEnd;
      const tab = FILE_TABS.find(t => t.key === activeTab);
      if (!tab) return;
      const currentVal = files[tab.filename] || '';
      handleFileChange(tab.filename, currentVal.substring(0, start) + '  ' + currentVal.substring(end));
      requestAnimationFrame(() => {
        ta.selectionStart = ta.selectionEnd = start + 2;
      });
    }
  };

  if (loading) {
    return (
      <div className="flex flex-col h-full items-center justify-center">
        <LoadingSpinner message="Loading gateway..." />
      </div>
    );
  }

  const gatewayName = files['SOUL.md']?.match(/^#\s+(.+)/m)?.[1]?.replace('Soul', '').trim() || 'Pounce';
  const currentTab = FILE_TABS.find(t => t.key === activeTab);
  const currentFileContent = currentTab ? (files[currentTab.filename] || '') : '';
  const currentLineCount = currentFileContent.split('\n').length;

  return (
    <div className="flex flex-col h-full">
      <div className="h-14 md:h-16 flex items-center justify-between px-4 md:px-6 border-b border-border-0 bg-surface-1/50 backdrop-blur-sm flex-shrink-0">
        <div className="flex items-center gap-3 min-w-0">
          <button
            onClick={() => navigate('/agents')}
            aria-label="Back to agents"
            className="p-2 -ml-2 rounded-lg text-text-3 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer"
          >
            <ArrowLeft className="w-5 h-5" />
          </button>
          <div className="flex items-center gap-2.5 min-w-0">
            <div className="relative">
              <img src={avatarPath} alt="" className="w-7 h-7 rounded-lg flex-shrink-0" />
              <div className="absolute -bottom-0.5 -right-0.5 w-3.5 h-3.5 rounded-full bg-accent-primary flex items-center justify-center ring-1 ring-surface-1">
                <Shield className="w-2 h-2 text-white" />
              </div>
            </div>
            <div className="min-w-0">
              <h1 className="text-sm md:text-base font-semibold text-text-0 truncate">{gatewayName || 'Gateway'}</h1>
              <p className="text-[11px] text-text-3 truncate">Edit Gateway</p>
            </div>
          </div>
          <span className="text-[10px] px-2 py-0.5 rounded-full bg-accent-primary/15 text-accent-primary flex-shrink-0">
            Gateway
          </span>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto">
        <div className="max-w-5xl mx-auto p-4 md:p-6">
          <div className="grid grid-cols-1 lg:grid-cols-[320px_1fr] gap-5 md:gap-6">

            <div className="space-y-5">
              <Card>
                <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-4">Avatar</h3>
                <div className="flex flex-col items-center gap-4">
                  <div className="relative group">
                    <img src={avatarPath} alt={gatewayName} className="w-24 h-24 rounded-2xl shadow-lg shadow-black/20" />
                    <label className="absolute inset-0 rounded-2xl bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center cursor-pointer">
                      {uploading ? (
                        <div className="w-6 h-6 border-2 border-white border-t-transparent rounded-full animate-spin" />
                      ) : (
                        <Upload className="w-5 h-5 text-white" />
                      )}
                      <input type="file" accept="image/png,image/jpeg,image/webp" onChange={handleUpload} className="hidden" aria-label="Upload avatar" tabIndex={-1} />
                    </label>
                  </div>
                </div>
              </Card>

              <Card>
                <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-4">About</h3>
                <div className="space-y-2">
                  <div className="flex justify-between text-[11px]">
                    <span className="text-text-3">Role</span>
                    <span className="text-text-2">Gateway / Router</span>
                  </div>
                  <div className="flex justify-between text-[11px]">
                    <span className="text-text-3">Status</span>
                    <span className="text-emerald-400">Always active</span>
                  </div>
                  <div className="flex justify-between text-[11px]">
                    <span className="text-text-3">Model</span>
                    <span className="text-text-2">Configured in Settings</span>
                  </div>
                </div>
              </Card>

              <Card>
                <p className="text-xs text-text-3 leading-relaxed">
                  The gateway is the first agent that processes every message. It routes to specialists, builds tools and dashboards, and guides users directly. Edit its soul to change its personality.
                </p>
              </Card>
            </div>

            <Card className="flex flex-col flex-1">
              <div role="tablist" className="flex items-center gap-1 mb-4 overflow-x-auto pb-1 -mx-1 px-1">
                {FILE_TABS.map(tab => (
                  <button
                    key={tab.key}
                    role="tab"
                    id={`tab-${tab.key}`}
                    aria-selected={activeTab === tab.key}
                    aria-controls={`tabpanel-${tab.key}`}
                    onClick={() => setActiveTab(tab.key)}
                    className={`px-3 py-1.5 text-xs font-medium rounded-lg whitespace-nowrap transition-colors cursor-pointer ${
                      activeTab === tab.key
                        ? 'bg-accent-muted text-accent-text'
                        : 'text-text-3 hover:text-text-1 hover:bg-surface-2'
                    }`}
                  >
                    {tab.label}
                    {fileDirty[tab.filename] && <span className="ml-1 text-accent-primary">*</span>}
                  </button>
                ))}
                <button
                  role="tab"
                  id="tab-memory"
                  aria-selected={activeTab === 'memory'}
                  aria-controls="tabpanel-memory"
                  onClick={() => setActiveTab('memory')}
                  className={`px-3 py-1.5 text-xs font-medium rounded-lg whitespace-nowrap transition-colors cursor-pointer ${
                    activeTab === 'memory'
                      ? 'bg-accent-muted text-accent-text'
                      : 'text-text-3 hover:text-text-1 hover:bg-surface-2'
                  }`}
                >
                  Memory
                  {memoryDirty && <span className="ml-1 text-accent-primary">*</span>}
                </button>
              </div>

              {activeTab === 'memory' ? (
                <div role="tabpanel" id="tabpanel-memory" aria-labelledby="tab-memory" className="flex flex-col flex-1">
                  <div className="flex items-center justify-between mb-3">
                    <div className="flex items-center gap-2">
                      <BookOpen className="w-4 h-4 text-accent-primary" />
                      <h3 className="text-xs font-semibold uppercase tracking-wider text-text-3">Gateway Memory</h3>
                    </div>
                    <Button size="sm" onClick={handleMemorySave} loading={fileSaving} disabled={!memoryDirty} icon={<Save className="w-3.5 h-3.5" />} aria-label={memoryDirty ? 'Save (unsaved changes)' : 'Save'}>
                      Save
                    </Button>
                  </div>
                  <p className="text-[11px] text-text-3 mb-3">Notes the gateway automatically saves from conversations. You can also edit them manually.</p>
                  <div className="relative flex-1 min-h-[300px]">
                    <textarea
                      value={memoryContent}
                      onChange={e => { setMemoryContent(e.target.value); setMemoryDirty(true); }}
                      placeholder="Gateway memory notes..."
                      className="absolute inset-0 w-full h-full rounded-lg border border-border-1 bg-surface-0 text-text-1 px-4 py-3 text-[13px] leading-relaxed font-mono placeholder:text-text-3/50 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors resize-none"
                      spellCheck={false}
                    />
                  </div>
                  {memoryFiles.length > 0 && (
                    <div className="mt-4">
                      <h4 className="text-[11px] font-medium text-text-3 uppercase tracking-wider mb-2">Daily Logs</h4>
                      <div className="space-y-1">
                        {memoryFiles.filter(f => f.name !== 'memory.md').map(f => (
                          <div key={f.name} className="flex items-center justify-between text-xs px-2 py-1.5 rounded-lg bg-surface-0">
                            <span className="text-text-2 font-mono">{f.name}</span>
                            <span className="text-text-3">{(f.size / 1024).toFixed(1)}KB</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              ) : currentTab ? (
                <div role="tabpanel" id={`tabpanel-${currentTab.key}`} aria-labelledby={`tab-${currentTab.key}`} className="flex flex-col flex-1">
                  <div className="flex items-center justify-between mb-3">
                    <div>
                      <p className="text-[11px] text-text-3">{currentTab.description}</p>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="text-[11px] text-text-3 font-mono tabular-nums">
                        {currentLineCount} lines · {currentFileContent.length} chars
                      </span>
                      <Button
                        size="sm"
                        onClick={() => handleFileSave(currentTab.filename)}
                        loading={fileSaving}
                        disabled={!fileDirty[currentTab.filename]}
                        icon={<Save className="w-3.5 h-3.5" />}
                        aria-label={fileDirty[currentTab.filename] ? 'Save (unsaved changes)' : 'Save'}
                      >
                        Save
                      </Button>
                    </div>
                  </div>
                  <div className="relative flex-1 min-h-[400px]">
                    <textarea
                      ref={promptRef}
                      value={currentFileContent}
                      onChange={e => handleFileChange(currentTab.filename, e.target.value)}
                      onKeyDown={handlePromptKeyDown}
                      placeholder={`Edit ${currentTab.filename}...`}
                      className="absolute inset-0 w-full h-full rounded-lg border border-border-1 bg-surface-0 text-text-1 px-4 py-3 text-[13px] leading-relaxed font-mono placeholder:text-text-3/50 focus:border-accent-primary focus:ring-1 focus:ring-accent-primary transition-colors resize-none"
                      spellCheck={false}
                    />
                  </div>
                </div>
              ) : null}
            </Card>
          </div>
        </div>
      </div>
    </div>
  );
}
