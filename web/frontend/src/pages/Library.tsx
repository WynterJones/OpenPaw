import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { Wrench, Bot, Sparkles, Package, Check, Download, WandSparkles, ScanSearch, FilePenLine, BarChart3, BookOpenCheck, TestTubes, ImageIcon, Film, FileText, Plug, ExternalLink, TrendingUp, AlertTriangle, KeyRound } from 'lucide-react';
import { Header } from '../components/Header';
import { Card } from '../components/Card';
import { Modal } from '../components/Modal';
import { Button } from '../components/Button';
import { EmptyState } from '../components/EmptyState';
import { SearchBar } from '../components/SearchBar';
import { useToast } from '../components/Toast';
import { FilterDropdown } from '../components/FilterDropdown';
import { toolLibrary, agentLibrary, skillLibrary, skillsSh, secretsApi, type LibraryTool, type LibraryAgent, type LibrarySkill, type SkillsShSkill, type SkillsShDetail, type SecretCheckResult } from '../lib/api';

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

function ToolCard({ tool, onClick, needsSecrets }: { tool: LibraryTool; onClick: () => void; needsSecrets?: boolean }) {
  return (
    <Card hover onClick={onClick}>
      <div className="flex items-center gap-2 mb-2">
        <CategoryBadge category={tool.category} />
        {tool.installed && (
          <span className="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/20">
            Installed
          </span>
        )}
        {tool.installed && needsSecrets && (
          <span className="px-2 py-0.5 rounded text-[10px] font-semibold bg-amber-500/15 text-amber-400 border border-amber-500/20 flex items-center gap-1">
            <AlertTriangle className="w-2.5 h-2.5" />
            Needs secrets
          </span>
        )}
      </div>
      <h3 className="text-base font-semibold text-text-0 mb-0.5">{tool.name}</h3>
      <p className="text-sm text-text-2 line-clamp-2 leading-snug">{tool.description}</p>
    </Card>
  );
}

function ToolModal({ tool, open, onClose, onInstall, installing, secretStatuses }: { tool: LibraryTool | null; open: boolean; onClose: () => void; onInstall: (slug: string) => void; installing: boolean; secretStatuses?: SecretCheckResult[] }) {
  if (!tool) return null;
  const missingOrPlaceholder = secretStatuses?.filter(s => !s.exists || s.placeholder) ?? [];
  const hasSecretIssues = tool.installed && tool.env && tool.env.length > 0 && missingOrPlaceholder.length > 0;
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
              {tool.env.map(e => {
                const status = secretStatuses?.find(s => s.name === e);
                const configured = status && status.exists && !status.placeholder;
                return (
                  <code key={e} className={`px-2 py-0.5 rounded text-xs font-mono border ${configured ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-surface-3 text-text-1 border-border-0'}`}>{e}{configured ? ' \u2713' : ''}</code>
                );
              })}
            </div>
          </div>
        )}
        {hasSecretIssues && (
          <div className="rounded-lg bg-amber-500/10 border border-amber-500/20 p-3 flex items-start gap-2">
            <AlertTriangle className="w-4 h-4 text-amber-400 flex-shrink-0 mt-0.5" />
            <div>
              <p className="text-sm font-medium text-amber-400">Secrets need configuration</p>
              <p className="text-xs text-text-2 mt-0.5">
                {missingOrPlaceholder.map(s => s.name).join(', ')} {missingOrPlaceholder.length === 1 ? 'needs' : 'need'} a real value. Go to <a href="/secrets" className="text-accent-text underline">Secrets</a> to update.
              </p>
            </div>
          </div>
        )}
        <div className="pt-2 border-t border-border-0">
          {tool.installed ? (
            <div className="flex items-center justify-between">
              <span className="text-sm text-emerald-400 flex items-center gap-1.5">
                <Check className="w-4 h-4" /> Already installed
              </span>
              {hasSecretIssues && (
                <a href="/secrets" className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium bg-amber-500/15 text-amber-400 hover:bg-amber-500/25 transition-colors">
                  <KeyRound className="w-4 h-4" />
                  Configure Secrets
                </a>
              )}
            </div>
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
  const [secretStatuses, setSecretStatuses] = useState<SecretCheckResult[]>([]);
  const [toolsMissingSecrets, setToolsMissingSecrets] = useState<Set<string>>(new Set());

  const load = useCallback(async () => {
    try {
      const data = await toolLibrary.list({ q: search || undefined, category: category || undefined });
      const tools = Array.isArray(data) ? data : [];
      setCatalog(tools);

      // Check secrets for installed tools that have env requirements
      const installedWithEnv = tools.filter(t => t.installed && t.env && t.env.length > 0);
      const allEnvNames = [...new Set(installedWithEnv.flatMap(t => t.env || []))];
      if (allEnvNames.length > 0) {
        try {
          const statuses = await secretsApi.checkNames(allEnvNames);
          setSecretStatuses(statuses);
          const missing = new Set<string>();
          for (const tool of installedWithEnv) {
            const hasMissing = (tool.env || []).some(envName => {
              const s = statuses.find(st => st.name === envName);
              return !s || !s.exists || s.placeholder;
            });
            if (hasMissing) missing.add(tool.slug);
          }
          setToolsMissingSecrets(missing);
        } catch { /* ignore secrets check failure */ }
      } else {
        setToolsMissingSecrets(new Set());
      }
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

  const handleSelect = async (tool: LibraryTool) => {
    setSelected(tool);
    if (tool.installed && tool.env && tool.env.length > 0) {
      try {
        const statuses = await secretsApi.checkNames(tool.env);
        setSecretStatuses(statuses);
      } catch { /* ignore */ }
    }
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
          {catalog.map(tool => <ToolCard key={tool.slug} tool={tool} onClick={() => handleSelect(tool)} needsSecrets={toolsMissingSecrets.has(tool.slug)} />)}
        </div>
      )}
      <ToolModal tool={selected} open={!!selected} onClose={() => setSelected(null)} onInstall={handleInstall} installing={installing === selected?.slug} secretStatuses={secretStatuses} />
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

function SkillsCatalogPanel() {
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

function SkillsShCard({ skill, onClick }: { skill: SkillsShSkill; onClick: () => void }) {
  return (
    <Card hover onClick={onClick}>
      <div className="flex items-center gap-2 mb-2">
        <span className="px-2 py-0.5 rounded text-[10px] font-semibold uppercase bg-surface-3 text-text-2 border border-border-0">
          skills.sh
        </span>
        {skill.installed && (
          <span className="px-2 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/20">
            Installed
          </span>
        )}
      </div>
      <h3 className="text-base font-semibold text-text-0 mb-0.5">{skill.name || skill.skill_id}</h3>
      <div className="flex items-center gap-3 text-xs text-text-3">
        <span className="flex items-center gap-1">
          <TrendingUp className="w-3 h-3" />
          {skill.installs.toLocaleString()} installs
        </span>
        <span className="truncate">{skill.source}</span>
      </div>
    </Card>
  );
}

function SkillsShModal({ skill, open, onClose, onInstall, installing }: {
  skill: SkillsShDetail | null;
  open: boolean;
  onClose: () => void;
  onInstall: () => void;
  installing: boolean;
}) {
  if (!skill) return null;
  return (
    <Modal open={open} onClose={onClose} title={skill.name || skill.skill_id} size="md">
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <span className="px-2 py-0.5 rounded text-[10px] font-semibold uppercase bg-surface-3 text-text-2 border border-border-0">
            skills.sh
          </span>
          <span className="text-xs text-text-3">{skill.source}</span>
        </div>
        {skill.description && (
          <p className="text-sm text-text-1 leading-relaxed">{skill.description}</p>
        )}
        {skill.body && (
          <div className="rounded-lg bg-surface-2 p-3 max-h-48 overflow-y-auto">
            <pre className="text-xs text-text-2 whitespace-pre-wrap font-mono">{skill.body}</pre>
          </div>
        )}
        <a
          href={`https://github.com/${skill.source}/tree/main/skills/${skill.skill_id}`}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1.5 text-xs text-accent-text hover:underline"
        >
          <ExternalLink className="w-3 h-3" />
          View on GitHub
        </a>
        <div className="pt-2 border-t border-border-0">
          {skill.installed ? (
            <span className="text-sm text-emerald-400 flex items-center gap-1.5">
              <Check className="w-4 h-4" /> Already installed
            </span>
          ) : (
            <Button onClick={onInstall} loading={installing} icon={<Download className="w-4 h-4" />} className="w-full">
              Install Skill
            </Button>
          )}
        </div>
      </div>
    </Modal>
  );
}

function SkillsShPanel() {
  const { toast } = useToast();
  const [results, setResults] = useState<SkillsShSkill[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [installing, setInstalling] = useState(false);
  const [selectedSkill, setSelectedSkill] = useState<SkillsShSkill | null>(null);
  const [detail, setDetail] = useState<SkillsShDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const load = useCallback(async (q?: string) => {
    setLoading(true);
    try {
      const data = await skillsSh.search(q);
      setResults(Array.isArray(data) ? data : []);
    } catch { setResults([]); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      load(search || undefined);
    }, 300);
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current); };
  }, [search, load]);

  const handleCardClick = async (skill: SkillsShSkill) => {
    setSelectedSkill(skill);
    setDetail(null);
    setDetailLoading(true);
    try {
      const d = await skillsSh.detail(skill.source, skill.skill_id);
      setDetail(d);
    } catch {
      toast('error', 'Failed to load skill details');
      setSelectedSkill(null);
    } finally { setDetailLoading(false); }
  };

  const handleInstall = async () => {
    if (!selectedSkill) return;
    setInstalling(true);
    try {
      await skillsSh.install(selectedSkill.source, selectedSkill.skill_id);
      toast('success', 'Skill installed from skills.sh');
      setSelectedSkill(null);
      setDetail(null);
      load(search || undefined);
    } catch (err) {
      toast('error', err instanceof Error ? err.message : 'Install failed');
    } finally { setInstalling(false); }
  };

  return (
    <>
      <div className="flex items-center gap-3 mb-3">
        <SearchBar value={search} onChange={setSearch} placeholder="Search skills.sh..." className="flex-1" />
      </div>
      {loading ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
        </div>
      ) : results.length === 0 ? (
        <EmptyState icon={<Package className="w-8 h-8" />} title="No skills found" description="Try a different search term." />
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
          {results.map(skill => (
            <SkillsShCard key={skill.id || skill.skill_id} skill={skill} onClick={() => handleCardClick(skill)} />
          ))}
        </div>
      )}
      {detailLoading && selectedSkill && (
        <Modal open={true} onClose={() => { setSelectedSkill(null); setDetailLoading(false); }} title="Loading..." size="md">
          <div className="flex items-center justify-center py-8">
            <div className="w-8 h-8 border-2 border-accent-primary border-t-transparent rounded-full animate-spin" />
          </div>
        </Modal>
      )}
      <SkillsShModal
        skill={detail}
        open={!!detail && !!selectedSkill}
        onClose={() => { setSelectedSkill(null); setDetail(null); }}
        onInstall={handleInstall}
        installing={installing}
      />
    </>
  );
}

type SkillsSubTab = 'catalog' | 'skillssh';

function SkillsPanel() {
  const [subTab, setSubTab] = useState<SkillsSubTab>('catalog');
  return (
    <>
      <div className="flex items-center gap-1.5 mb-4">
        <button
          onClick={() => setSubTab('catalog')}
          className={`px-3 py-1.5 text-xs font-medium rounded-full transition-colors cursor-pointer ${
            subTab === 'catalog'
              ? 'bg-accent-primary text-accent-btn-text'
              : 'bg-surface-2 text-text-2 hover:text-text-0 hover:bg-surface-3'
          }`}
        >
          Our Skills
        </button>
        <button
          onClick={() => setSubTab('skillssh')}
          className={`px-3 py-1.5 text-xs font-medium rounded-full transition-colors cursor-pointer ${
            subTab === 'skillssh'
              ? 'bg-accent-primary text-accent-btn-text'
              : 'bg-surface-2 text-text-2 hover:text-text-0 hover:bg-surface-3'
          }`}
        >
          Skills.sh
        </button>
      </div>
      {subTab === 'catalog' && <SkillsCatalogPanel />}
      {subTab === 'skillssh' && <SkillsShPanel />}
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
