import { useState, useEffect, useCallback, useMemo } from 'react';
import { Wrench, Bot, Sparkles, Package, Check, Download, WandSparkles, ScanSearch, FilePenLine, BarChart3, BookOpenCheck, TestTubes, ImageIcon, Film, FileText, Plug } from 'lucide-react';
import { Header } from '../components/Header';
import { Card } from '../components/Card';
import { Modal } from '../components/Modal';
import { Button } from '../components/Button';
import { EmptyState } from '../components/EmptyState';
import { SearchBar } from '../components/SearchBar';
import { useToast } from '../components/Toast';
import { FilterDropdown } from '../components/FilterDropdown';
import { toolLibrary, agentLibrary, skillLibrary, type LibraryTool, type LibraryAgent, type LibrarySkill } from '../lib/api';

type LibraryTab = 'tools' | 'agents' | 'skills';
const libraryTabs: { key: LibraryTab; label: string; icon: typeof Wrench }[] = [
  { key: 'tools', label: 'Tools', icon: Wrench },
  { key: 'agents', label: 'Agents', icon: Bot },
  { key: 'skills', label: 'Skills', icon: Sparkles },
];

function CategoryBadge({ category }: { category: string }) {
  return (
    <span className="px-2 py-0.5 rounded text-[10px] font-semibold uppercase bg-surface-3 text-text-2 border border-border-0">
      {category}
    </span>
  );
}

function ModelBadge({ model }: { model: string }) {
  const colors: Record<string, string> = {
    sonnet: 'bg-blue-500/15 text-blue-400 border-blue-500/20',
    haiku: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/20',
    opus: 'bg-purple-500/15 text-purple-400 border-purple-500/20',
  };
  return (
    <span className={`px-2 py-0.5 rounded text-[10px] font-semibold border ${colors[model] || 'bg-surface-3 text-text-2 border-border-0'}`}>
      {model}
    </span>
  );
}

const skillIconMap: Record<string, React.ComponentType<{ className?: string }>> = {
  'wand-sparkles': WandSparkles,
  'scan-search': ScanSearch,
  'file-pen-line': FilePenLine,
  'bar-chart-3': BarChart3,
  'book-open-check': BookOpenCheck,
  'test-tubes': TestTubes,
  'image': ImageIcon,
  'film': Film,
  'file-text': FileText,
  'plug': Plug,
};

function ToolCard({ tool, onClick }: { tool: LibraryTool; onClick: () => void }) {
  return (
    <Card hover onClick={onClick}>
      <div className="flex items-center gap-2 mb-2">
        <CategoryBadge category={tool.category} />
        {tool.installed && (
          <span className="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/20">
            Installed
          </span>
        )}
      </div>
      <h3 className="text-base font-semibold text-text-0 mb-0.5">{tool.name}</h3>
      <p className="text-sm text-text-2 line-clamp-2 leading-snug">{tool.description}</p>
    </Card>
  );
}

function ToolModal({ tool, open, onClose, onInstall, installing }: { tool: LibraryTool | null; open: boolean; onClose: () => void; onInstall: (slug: string) => void; installing: boolean }) {
  if (!tool) return null;
  return (
    <Modal open={open} onClose={onClose} title={tool.name} size="md">
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <CategoryBadge category={tool.category} />
          <span className="text-xs text-text-3">v{tool.version}</span>
        </div>
        <p className="text-sm text-text-1 leading-relaxed">{tool.description}</p>
        {tool.tags.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {tool.tags.map(tag => (
              <span key={tag} className="px-2 py-0.5 rounded text-[11px] bg-surface-2 text-text-3">{tag}</span>
            ))}
          </div>
        )}
        {tool.env && tool.env.length > 0 && (
          <div className="rounded-lg bg-surface-2 p-3">
            <p className="text-[10px] font-semibold uppercase tracking-wider text-text-3 mb-1.5">Required Environment</p>
            <div className="flex flex-wrap gap-1.5">
              {tool.env.map(e => (
                <code key={e} className="px-2 py-0.5 rounded text-xs font-mono bg-surface-3 text-text-1 border border-border-0">{e}</code>
              ))}
            </div>
          </div>
        )}
        <div className="pt-2 border-t border-border-0">
          {tool.installed ? (
            <span className="text-sm text-emerald-400 flex items-center gap-1.5">
              <Check className="w-4 h-4" /> Already installed
            </span>
          ) : (
            <Button onClick={() => onInstall(tool.slug)} loading={installing} icon={<Download className="w-4 h-4" />} className="w-full">
              Install Tool
            </Button>
          )}
        </div>
      </div>
    </Modal>
  );
}

function AgentCard({ agent, onClick }: { agent: LibraryAgent; onClick: () => void }) {
  return (
    <Card hover onClick={onClick}>
      <div className="flex items-center gap-2 mb-2">
        <CategoryBadge category={agent.category} />
        <ModelBadge model={agent.model} />
        {agent.installed && (
          <span className="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/20">
            Installed
          </span>
        )}
      </div>
      <div className="flex items-center gap-3 mb-1.5">
        <img src={agent.avatar_path} alt="" className="w-10 h-10 rounded-xl flex-shrink-0" />
        <h3 className="text-base font-semibold text-text-0">{agent.name}</h3>
      </div>
      <p className="text-sm text-text-2 line-clamp-2 leading-snug">{agent.description}</p>
    </Card>
  );
}

function AgentModal({ agent, open, onClose, onInstall, installing }: { agent: LibraryAgent | null; open: boolean; onClose: () => void; onInstall: (slug: string) => void; installing: boolean }) {
  if (!agent) return null;
  return (
    <Modal open={open} onClose={onClose} title={agent.name} size="md">
      <div className="space-y-3">
          <div className="flex items-center gap-3">
            <img src={agent.avatar_path} alt={agent.name} className="w-14 h-14 rounded-xl" />
            <div className="flex items-center gap-2">
              <CategoryBadge category={agent.category} />
              <ModelBadge model={agent.model} />
              <span className="text-xs text-text-3">v{agent.version}</span>
            </div>
          </div>
          <p className="text-sm text-text-1 leading-relaxed">{agent.description}</p>
          {agent.tags.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {agent.tags.map(tag => (
                <span key={tag} className="px-2 py-0.5 rounded text-[11px] bg-surface-2 text-text-3">{tag}</span>
              ))}
            </div>
          )}
          <div className="rounded-lg bg-surface-2 p-3">
            <p className="text-[10px] font-semibold uppercase tracking-wider text-text-3 mb-1.5">Includes</p>
            <div className="flex flex-wrap gap-1.5">
              {['SOUL.md', 'AGENTS.md', 'BOOT.md'].map(f => (
                <code key={f} className="px-2 py-0.5 rounded text-xs font-mono bg-surface-3 text-text-1 border border-border-0">{f}</code>
              ))}
            </div>
          </div>
          <div className="pt-2 border-t border-border-0">
            {agent.installed ? (
              <span className="text-sm text-emerald-400 flex items-center gap-1.5">
                <Check className="w-4 h-4" /> Already installed
              </span>
            ) : (
              <Button onClick={() => onInstall(agent.slug)} loading={installing} icon={<Download className="w-4 h-4" />} className="w-full">
                Install Agent
              </Button>
            )}
          </div>
        </div>
    </Modal>
  );
}

function SkillCard({ skill, onClick }: { skill: LibrarySkill; onClick: () => void }) {
  const IconComp = skillIconMap[skill.icon] || Sparkles;
  return (
    <Card hover onClick={onClick}>
      <div className="flex items-center gap-2 mb-2">
        <CategoryBadge category={skill.category} />
        {skill.installed && (
          <span className="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/20">
            Installed
          </span>
        )}
      </div>
      <div className="flex items-center gap-2 mb-0.5">
        <IconComp className="w-5 h-5 text-accent-text flex-shrink-0" />
        <h3 className="text-base font-semibold text-text-0">{skill.name}</h3>
      </div>
      <p className="text-sm text-text-2 line-clamp-2 leading-snug">{skill.description}</p>
    </Card>
  );
}

function SkillModal({ skill, open, onClose, onInstall, installing }: { skill: LibrarySkill | null; open: boolean; onClose: () => void; onInstall: (slug: string) => void; installing: boolean }) {
  if (!skill) return null;
  return (
    <Modal open={open} onClose={onClose} title={skill.name} size="md">
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <CategoryBadge category={skill.category} />
          <span className="text-xs text-text-3">v{skill.version}</span>
        </div>
        <p className="text-sm text-text-1 leading-relaxed">{skill.description}</p>
        {skill.tags.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {skill.tags.map(tag => (
              <span key={tag} className="px-2 py-0.5 rounded text-[11px] bg-surface-2 text-text-3">{tag}</span>
            ))}
          </div>
        )}
        {skill.uses_tools && (
          <div className="flex items-center gap-1.5 text-sm text-text-2">
            <Wrench className="w-3.5 h-3.5 text-text-3" />
            <span>Uses tools</span>
          </div>
        )}
        {skill.required_tools && skill.required_tools.length > 0 && (
          <div className="rounded-lg bg-surface-2 p-3">
            <p className="text-[10px] font-semibold uppercase tracking-wider text-text-3 mb-1.5">Required Tools</p>
            <div className="flex flex-wrap gap-1.5">
              {skill.required_tools.map(t => (
                <code key={t} className="px-2 py-0.5 rounded text-xs font-mono bg-surface-3 text-text-1 border border-border-0">{t}</code>
              ))}
            </div>
          </div>
        )}
        <div className="pt-2 border-t border-border-0">
          {skill.installed ? (
            <span className="text-sm text-emerald-400 flex items-center gap-1.5">
              <Check className="w-4 h-4" /> Already installed
            </span>
          ) : (
            <Button onClick={() => onInstall(skill.slug)} loading={installing} icon={<Download className="w-4 h-4" />} className="w-full">
              Install Skill
            </Button>
          )}
        </div>
      </div>
    </Modal>
  );
}

function ToolsPanel() {
  const { toast } = useToast();
  const [catalog, setCatalog] = useState<LibraryTool[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const [installing, setInstalling] = useState<string | null>(null);
  const [selected, setSelected] = useState<LibraryTool | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await toolLibrary.list({ q: search || undefined, category: category || undefined });
      setCatalog(Array.isArray(data) ? data : []);
    } catch { setCatalog([]); }
    finally { setLoading(false); }
  }, [search, category]);

  useEffect(() => { load(); }, [load]);

  const handleInstall = async (slug: string) => {
    setInstalling(slug);
    try {
      await toolLibrary.install(slug);
      toast('success', 'Tool installed');
      setSelected(null);
      load();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Install failed');
    } finally { setInstalling(null); }
  };

  const categories = [...new Set(catalog.map(t => t.category))].sort();

  return (
    <>
      <div className="flex items-center gap-3 mb-3">
        <SearchBar value={search} onChange={setSearch} placeholder="Search tools..." className="flex-1" />
        <FilterDropdown value={category} onChange={setCategory} options={categories} allLabel="All Categories" placeholder="Filter tools by category" />
      </div>
      {loading ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
        </div>
      ) : catalog.length === 0 ? (
        <EmptyState icon={<Package className="w-8 h-8" />} title="No tools found" description="Try a different search or category filter." />
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
          {catalog.map(tool => <ToolCard key={tool.slug} tool={tool} onClick={() => setSelected(tool)} />)}
        </div>
      )}
      <ToolModal tool={selected} open={!!selected} onClose={() => setSelected(null)} onInstall={handleInstall} installing={installing === selected?.slug} />
    </>
  );
}

function AgentsPanel() {
  const { toast } = useToast();
  const [catalog, setCatalog] = useState<LibraryAgent[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const [installing, setInstalling] = useState<string | null>(null);
  const [selected, setSelected] = useState<LibraryAgent | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await agentLibrary.list({ q: search || undefined, category: category || undefined });
      setCatalog(Array.isArray(data) ? data : []);
    } catch { setCatalog([]); }
    finally { setLoading(false); }
  }, [search, category]);

  useEffect(() => { load(); }, [load]);

  const handleInstall = async (slug: string) => {
    setInstalling(slug);
    try {
      await agentLibrary.install(slug);
      toast('success', 'Agent installed');
      setSelected(null);
      load();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Install failed');
    } finally { setInstalling(null); }
  };

  const categories = [...new Set(catalog.map(a => a.category))].sort();

  return (
    <>
      <div className="flex items-center gap-3 mb-3">
        <SearchBar value={search} onChange={setSearch} placeholder="Search agents..." className="flex-1" />
        <FilterDropdown value={category} onChange={setCategory} options={categories} allLabel="All Categories" placeholder="Filter agents by category" />
      </div>
      {loading ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
        </div>
      ) : catalog.length === 0 ? (
        <EmptyState icon={<Package className="w-8 h-8" />} title="No agents found" description="Try a different search or category filter." />
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
          {catalog.map(agent => <AgentCard key={agent.slug} agent={agent} onClick={() => setSelected(agent)} />)}
        </div>
      )}
      <AgentModal agent={selected} open={!!selected} onClose={() => setSelected(null)} onInstall={handleInstall} installing={installing === selected?.slug} />
    </>
  );
}

function SkillsPanel() {
  const { toast } = useToast();
  const [catalog, setCatalog] = useState<LibrarySkill[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const [installing, setInstalling] = useState<string | null>(null);
  const [selected, setSelected] = useState<LibrarySkill | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await skillLibrary.list({ q: search || undefined, category: category || undefined });
      setCatalog(Array.isArray(data) ? data : []);
    } catch { setCatalog([]); }
    finally { setLoading(false); }
  }, [search, category]);

  useEffect(() => { load(); }, [load]);

  const handleInstall = async (slug: string) => {
    setInstalling(slug);
    try {
      await skillLibrary.install(slug);
      toast('success', 'Skill installed');
      setSelected(null);
      load();
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Install failed');
    } finally { setInstalling(null); }
  };

  const categories = [...new Set(catalog.map(s => s.category))].sort();

  return (
    <>
      <div className="flex items-center gap-3 mb-3">
        <SearchBar value={search} onChange={setSearch} placeholder="Search skills..." className="flex-1" />
        <FilterDropdown value={category} onChange={setCategory} options={categories} allLabel="All Categories" placeholder="Filter skills by category" />
      </div>
      {loading ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
        </div>
      ) : catalog.length === 0 ? (
        <EmptyState icon={<Package className="w-8 h-8" />} title="No skills found" description="Try a different search or category filter." />
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
          {catalog.map(skill => <SkillCard key={skill.slug} skill={skill} onClick={() => setSelected(skill)} />)}
        </div>
      )}
      <SkillModal skill={selected} open={!!selected} onClose={() => setSelected(null)} onInstall={handleInstall} installing={installing === selected?.slug} />
    </>
  );
}

export function Library() {
  const [tab, setTab] = useState<LibraryTab>('tools');
  const [counts, setCounts] = useState<Record<LibraryTab, number | null>>({ tools: null, agents: null, skills: null });

  useEffect(() => {
    Promise.all([
      toolLibrary.list({}).then(d => Array.isArray(d) ? d.length : 0).catch(() => 0),
      agentLibrary.list({}).then(d => Array.isArray(d) ? d.length : 0).catch(() => 0),
      skillLibrary.list({}).then(d => Array.isArray(d) ? d.length : 0).catch(() => 0),
    ]).then(([tools, agents, skills]) => setCounts({ tools, agents, skills }));
  }, []);

  const tabsWithCounts = useMemo(() => libraryTabs.map(t => ({
    ...t,
    count: counts[t.key],
  })), [counts]);

  return (
    <div className="flex flex-col h-full">
      <Header title="Library" />
      <div className="flex-1 overflow-y-auto px-4 md:px-6 py-4">
        <div className="flex items-center gap-2 mb-3 border-b border-border-0">
          {tabsWithCounts.map(t => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={`px-4 py-2 text-sm font-medium transition-colors relative cursor-pointer ${
                tab === t.key ? 'text-text-0' : 'text-text-3 hover:text-text-1'
              }`}
            >
              <span className="flex items-center gap-2">
                <t.icon className="w-4 h-4" />
                {t.label}
                {t.count !== null && (
                  <span className={`px-1.5 py-0.5 rounded-full text-[11px] font-semibold leading-none ${
                    tab === t.key
                      ? 'bg-accent-primary/15 text-accent-primary'
                      : 'bg-surface-3 text-text-3'
                  }`}>
                    {t.count}
                  </span>
                )}
              </span>
              {tab === t.key && (
                <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-accent-primary rounded-t" />
              )}
            </button>
          ))}
        </div>

        {tab === 'tools' && <ToolsPanel />}
        {tab === 'agents' && <AgentsPanel />}
        {tab === 'skills' && <SkillsPanel />}
      </div>
    </div>
  );
}
