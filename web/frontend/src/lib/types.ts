// API types

export interface User {
  id: string;
  username: string;
  avatar_path: string;
  created_at: string;
}

export interface ToolEndpointParam {
  name: string;
  type: string;
  required: boolean;
  description: string;
}

export interface ToolEndpoint {
  method: string;
  path: string;
  description: string;
  path_params?: ToolEndpointParam[];
  query_params?: ToolEndpointParam[];
  body?: Record<string, string>;
  response?: Record<string, string>;
}

export interface ToolManifest {
  id: string;
  name: string;
  description: string;
  version: string;
  health_check: string;
  endpoints: ToolEndpoint[];
  env?: { name: string; required: boolean; default?: string; description: string }[];
}

export interface Tool {
  id: string;
  name: string;
  description: string;
  status: string;
  enabled: boolean;
  port: number;
  pid: number;
  capabilities: string;
  version: string;
  last_run: string | null;
  owner_agent_slug: string;
  library_slug: string;
  library_version: string;
  source_hash: string;
  binary_hash: string;
  folder: string;
  created_at: string;
  updated_at: string;
  manifest?: ToolManifest;
}

export interface LibraryTool {
  slug: string;
  name: string;
  description: string;
  version: string;
  category: string;
  icon: string;
  tags: string[];
  env?: string[];
  installed: boolean;
  installed_id?: string;
}

export interface ToolIntegrityInfo {
  source_hash: string;
  binary_hash: string;
  verified: boolean;
  files: { filename: string; hash: string; size: number }[];
}

export interface Secret {
  id: string;
  name: string;
  description: string;
  tool_id: string;
  tool_name: string;
  created_at: string;
  updated_at: string;
}

export interface ChatThread {
  id: string;
  title: string;
  agent: string;
  total_cost_usd: number;
  created_at: string;
  updated_at: string;
  message_count: number;
}

export interface ChatMessage {
  id: string;
  thread_id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  agent_role_slug: string;
  cost_usd: number;
  input_tokens: number;
  output_tokens: number;
  widget_data?: string;
  image_url?: string;
  tool_calls?: ToolCallResult[];
  created_at: string;
}

export interface ThreadMember {
  thread_id: string;
  agent_role_slug: string;
  name: string;
  description: string;
  avatar_path: string;
  joined_at: string;
}

export interface ThreadStats {
  total_cost_usd: number;
  total_input_tokens: number;
  total_output_tokens: number;
  message_count: number;
  context_used_tokens: number;
  context_limit_tokens: number;
  auto_compact_enabled: boolean;
  auto_compact_threshold: number;
}

export interface AgentRole {
  id: string;
  slug: string;
  name: string;
  description: string;
  system_prompt: string;
  model: string;
  avatar_path: string;
  enabled: boolean;
  sort_order: number;
  is_preset: boolean;
  identity_initialized: boolean;
  heartbeat_enabled: boolean;
  library_slug: string;
  library_version: string;
  folder: string;
  created_at: string;
  updated_at: string;
}

export interface LibraryAgent {
  slug: string;
  name: string;
  description: string;
  version: string;
  category: string;
  icon: string;
  tags: string[];
  model: string;
  avatar_path: string;
  installed: boolean;
  installed_slug?: string;
}

export interface Skill {
  name: string;
  content?: string;
  summary?: string;
  description?: string;
  allowed_tools?: string;
  folder?: string;
}

export interface LibrarySkill {
  slug: string;
  name: string;
  description: string;
  version: string;
  category: string;
  icon: string;
  tags: string[];
  uses_tools: boolean;
  required_tools?: string[];
  installed: boolean;
}

export interface MemoryFile {
  name: string;
  size: number;
  updated_at: string;
}

export interface MemoryItem {
  id: string;
  content: string;
  summary: string;
  category: string;
  importance: number;
  source: string;
  tags: string;
  access_count: number;
  archived: boolean;
  created_at: string;
  updated_at: string;
}

export interface ToolCallResult {
  tool_name: string;
  input: Record<string, unknown>;
  output: string;
  status: 'success' | 'error';
}

export interface Schedule {
  id: string;
  name: string;
  description: string;
  cron_expr: string;
  agent_role_slug: string;
  prompt_content: string;
  thread_id: string;
  enabled: boolean;
  last_run_at: string | null;
  next_run_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface ScheduleExecution {
  id: string;
  schedule_id: string;
  status: 'success' | 'error' | 'running';
  output: string;
  error: string;
  started_at: string;
  finished_at: string | null;
}

export interface DashboardLayout {
  columns: number;
  gap: 'sm' | 'md' | 'lg';
}

export interface DashboardWidgetConfig {
  id: string;
  type: string;
  title: string;
  position: { col: number; row: number; colSpan: number; rowSpan: number };
  dataSource?: {
    type: 'tool' | 'static';
    toolId: string;
    endpoint: string;
    method?: string;
    refreshInterval?: number;
    dataPath?: string;
  };
  data?: Record<string, unknown>;
  config: Record<string, unknown>;
}

export interface Dashboard {
  id: string;
  name: string;
  description: string;
  layout: DashboardLayout;
  widgets: DashboardWidgetConfig[];
  dashboard_type: 'config' | 'custom';
  owner_agent_slug: string;
  bg_image: string;
  created_at: string;
  updated_at: string;
}

export interface LogStats {
  total_cost_usd: number;
  total_tokens: number;
  total_activity: number;
}

export interface LogEntry {
  id: string;
  user_id: string;
  username: string;
  action: string;
  category: string;
  target: string;
  target_id: string;
  details: string;
  created_at: string;
}

// Streaming types (from Go StreamEvent)
export interface StreamEvent {
  type: 'text_delta' | 'tool_start' | 'tool_delta' | 'tool_end' | 'turn_complete' | 'result' | 'error' | 'init';
  text?: string;
  tool_name?: string;
  tool_id?: string;
  tool_input?: Record<string, unknown>;
  tool_output?: string;
  total_cost_usd?: number;
  usage?: { input_tokens?: number; output_tokens?: number; cache_read_input_tokens?: number; cache_creation_input_tokens?: number };
  result?: string;
  error?: string;
  session_id?: string;
  num_turns?: number;
}

// WebSocket message wrapper
export interface WSMessage {
  type: string;
  payload: {
    agent_id?: string;
    work_order_id?: string;
    thread_id?: string;
    event?: StreamEvent;
    status?: string;
    output?: string;
    [key: string]: unknown;
  };
}

export interface SystemInfo {
  version: string;
  go_version: string;
  os: string;
  arch: string;
  uptime: string;
  db_size: string;
  tool_count: number;
  secret_count: number;
  schedule_count: number;
  api_key_configured: boolean;
  api_key_source: string;
  lan_ip: string;
  tailscale_ip: string;
  port: number;
  tailscale_enabled: boolean;
  bind_address: string;
}

export interface DesignConfig {
  surface_0: string;
  surface_1: string;
  surface_2: string;
  surface_3: string;
  border_0: string;
  border_1: string;
  text_0: string;
  text_1: string;
  text_2: string;
  text_3: string;
  accent: string;
  accent_hover: string;
  accent_muted: string;
  accent_text: string;
  accent_btn_text: string;
  danger: string;
  danger_hover: string;
  font_family: string;
  font_scale: string;
  bg_image: string;
}

// Confirmation card types
export interface ConfirmationCard {
  __type: 'confirmation';
  action: string;
  action_label: string;
  title: string;
  description: string;
  work_order_id: string;
  message_id: string;
  status: 'pending' | 'confirmed' | 'rejected';
}

export interface ToolSummaryCard {
  __type: 'tool_summary';
  tool_id: string;
  tool_name: string;
  port: number;
  status: string;
  healthy: boolean;
  endpoints: { method: string; path: string; description: string }[];
}

export interface WidgetPayload {
  type: string;
  title?: string;
  tool_id?: string;
  tool_name?: string;
  endpoint?: string;
  data: Record<string, unknown>;
  config?: Record<string, unknown>;
}

// Context types
export interface ContextFolder {
  id: string;
  parent_id?: string;
  name: string;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export interface ContextFile {
  id: string;
  folder_id?: string;
  name: string;
  filename: string;
  mime_type: string;
  size_bytes: number;
  is_about_you: boolean;
  created_at: string;
  updated_at: string;
}

export interface ContextTreeNode extends ContextFolder {
  children: ContextTreeNode[];
  files: ContextFile[];
}

export interface ContextTree {
  folders: ContextTreeNode[];
  files: ContextFile[];
}

export interface ChatAttachment {
  id: string;
  message_id: string;
  filename: string;
  original_name: string;
  mime_type: string;
  size_bytes: number;
  created_at: string;
}

// Browser automation types
export interface BrowserSession {
  id: string;
  name: string;
  status: 'idle' | 'active' | 'busy' | 'human' | 'stopped' | 'error';
  headless: boolean;
  current_url: string;
  current_title: string;
  owner_agent_slug: string;
  created_at: string;
  updated_at: string;
}

export interface BrowserActionRequest {
  action: string;
  selector?: string;
  value?: string;
  x?: number;
  y?: number;
  timeout?: number;
}

export interface BrowserActionResult {
  success: boolean;
  data?: string;
  url?: string;
  title?: string;
  screenshot?: string;
  error?: string;
}

export interface BrowserTask {
  id: string;
  session_id: string;
  thread_id: string;
  agent_role_slug: string;
  title: string;
  status: string;
  instructions: string;
  result: string;
  extracted_data: string;
  error: string;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
}

// Notification types
export interface AppNotification {
  id: string;
  title: string;
  body: string;
  priority: 'low' | 'normal' | 'high';
  source_agent_slug: string;
  source_type: string;
  link: string;
  read: boolean;
  dismissed: boolean;
  created_at: string;
}

export interface HeartbeatExecution {
  id: string;
  agent_role_slug: string;
  status: 'running' | 'completed' | 'failed';
  actions_taken: string;
  output: string;
  error: string;
  cost_usd: number;
  input_tokens: number;
  output_tokens: number;
  started_at: string;
  finished_at: string | null;
}

export interface SkillsShSkill {
  id: string;
  skill_id: string;
  name: string;
  installs: number;
  source: string;
  description?: string;
  installed: boolean;
}

export interface SkillsShDetail {
  skill_id: string;
  name: string;
  source: string;
  description: string;
  content: string;
  body: string;
  installed: boolean;
}

export interface HeartbeatConfig {
  heartbeat_enabled: string;
  heartbeat_interval_sec: string;
  heartbeat_timezone: string;
  heartbeat_active_start: string;
  heartbeat_active_end: string;
}

export interface HeartbeatExecutionPage {
  items: HeartbeatExecution[];
  total: number;
}

export interface TerminalSession {
  id: string;
  title: string;
  shell: string;
  cols: number;
  rows: number;
  color: string;
  workbench_id: string;
  created_at: string;
}

export interface Workbench {
  id: string;
  name: string;
  color: string;
  sort_order: number;
  created_at: string;
}

export interface Project {
  id: string;
  name: string;
  color: string;
  repos: ProjectRepo[];
  created_at: string;
}

export interface ProjectRepo {
  id: string;
  project_id: string;
  name: string;
  folder_path: string;
  command: string;
  sort_order: number;
}

export type AgentTaskStatus = 'backlog' | 'doing' | 'blocked' | 'done';

export interface AgentTask {
  id: string;
  agent_role_slug: string;
  title: string;
  description: string;
  status: AgentTaskStatus;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export interface AgentTaskCounts {
  backlog: number;
  doing: number;
  blocked: number;
  done: number;
}

export interface SubAgentTask {
  subagent_id: string;
  agent_slug: string;
  agent_name: string;
  task_summary: string;
  status: 'started' | 'completed' | 'failed';
  result_preview?: string;
  cost_usd?: number;
  streaming_text?: string;
}

export interface TodoList {
  id: string;
  name: string;
  description: string;
  color: string;
  sort_order: number;
  total_items: number;
  completed_items: number;
  created_at: string;
  updated_at: string;
}

export interface TodoItem {
  id: string;
  list_id: string;
  title: string;
  notes: string;
  completed: boolean;
  sort_order: number;
  due_date: string | null;
  last_actor_agent_slug: string | null;
  last_actor_agent_name: string | null;
  last_actor_avatar: string | null;
  last_actor_note: string;
  created_at: string;
  updated_at: string;
  completed_at: string | null;
}

export interface MediaItem {
  id: string;
  thread_id: string;
  message_id: string;
  source: 'fal' | 'dalle' | 'tool' | 'upload';
  source_model: string;
  media_type: 'image' | 'audio' | 'video';
  url: string;
  filename: string;
  mime_type: string;
  width: number;
  height: number;
  size_bytes: number;
  prompt: string;
  metadata: string;
  created_at: string;
}

export interface MediaListResponse {
  items: MediaItem[];
  total: number;
}

export interface FalStatus {
  configured: boolean;
  source: 'settings' | 'env' | 'none';
}

export interface FalModel {
  id: string;
  name: string;
  description: string;
}

export interface FalGenerateRequest {
  model: string;
  prompt: string;
  image_size?: string;
  num_inference_steps?: number;
  guidance_scale?: number;
  seed?: number;
}

export interface FalGenerateResult {
  id: string;
  url: string;
  width: number;
  height: number;
  prompt: string;
  seed: number;
  local_url: string;
}
