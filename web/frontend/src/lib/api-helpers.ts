import { api, getCSRFToken, ApiError } from './api';
import type {
  ContextTree,
  ContextFile,
  ContextFolder,
  ChatAttachment,
  MemoryFile,
  Skill,
  LibrarySkill,
  ThreadMember,
  Tool,
  LibraryTool,
  LibraryAgent,
  AgentRole,
  ToolIntegrityInfo,
  BrowserSession,
  BrowserActionRequest,
  BrowserActionResult,
  BrowserTask,
  AppNotification,
  HeartbeatConfig,
  HeartbeatExecutionPage,
  ConfirmationCard,
  ToolSummaryCard,
  WidgetPayload,
} from './types';

const BASE_URL = '/api/v1';

// Parse helpers
export function parseConfirmation(content: string): ConfirmationCard | null {
  try {
    const parsed = JSON.parse(content);
    if (parsed?.__type === 'confirmation') return parsed;
  } catch (e) { console.warn('parseConfirmation: failed to parse JSON:', e); }
  return null;
}

export function parseToolSummary(content: string): ToolSummaryCard | null {
  try {
    const parsed = JSON.parse(content);
    if (parsed?.__type === 'tool_summary') return parsed;
  } catch (e) { console.warn('parseToolSummary: failed to parse JSON:', e); }
  return null;
}

export function parseWidgets(widgetData?: string | null): WidgetPayload[] | null {
  if (!widgetData) return null;
  try {
    const parsed = JSON.parse(widgetData);
    if (Array.isArray(parsed) && parsed.length > 0) return parsed;
  } catch (e) { console.warn('parseWidgets: failed to parse widget JSON:', e); }
  return null;
}

// Context API helpers
export const contextApi = {
  tree: () => api.get<ContextTree>('/context/tree'),
  listFiles: () => api.get<ContextFile[]>('/context/files'),
  getFile: (id: string) => api.get<{ file: ContextFile; content?: string }>(`/context/files/${id}`),
  updateFile: (id: string, data: { name?: string; content?: string }) => api.put(`/context/files/${id}`, data),
  deleteFile: (id: string) => api.delete(`/context/files/${id}`),
  moveFile: (id: string, folderId: string | null) => api.put(`/context/files/${id}/move`, { folder_id: folderId }),
  createFolder: (name: string, parentId?: string) => api.post<ContextFolder>('/context/folders', { name, parent_id: parentId }),
  updateFolder: (id: string, data: { name?: string; parent_id?: string }) => api.put(`/context/folders/${id}`, data),
  deleteFolder: (id: string) => api.delete(`/context/folders/${id}`),
  getAboutYou: () => api.get<{ content: string }>('/context/about-you'),
  updateAboutYou: (content: string) => api.put<{ status: string }>('/context/about-you', { content }),
  uploadFile: async (file: File, folderId?: string): Promise<ContextFile> => {
    const formData = new FormData();
    formData.append('file', file);
    if (folderId) formData.append('folder_id', folderId);
    const headers: Record<string, string> = {};
    const csrf = getCSRFToken();
    if (csrf) headers['X-CSRF-Token'] = csrf;
    const res = await fetch(`${BASE_URL}/context/files`, {
      method: 'POST',
      headers,
      body: formData,
      credentials: 'same-origin',
    });
    if (!res.ok) {
      const body = await res.text();
      let message = `Upload failed: ${res.status}`;
      try { const json = JSON.parse(body); message = json.error || message; } catch (e) { console.warn('contextApi.uploadFile: failed to parse error response:', e); }
      throw new ApiError(res.status, message);
    }
    return res.json();
  },
  uploadChatAttachment: async (file: File, messageId: string): Promise<ChatAttachment> => {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('message_id', messageId);
    const headers: Record<string, string> = {};
    const csrf = getCSRFToken();
    if (csrf) headers['X-CSRF-Token'] = csrf;
    const res = await fetch(`${BASE_URL}/chat/attachments`, {
      method: 'POST',
      headers,
      body: formData,
      credentials: 'same-origin',
    });
    if (!res.ok) {
      const body = await res.text();
      let message = `Upload failed: ${res.status}`;
      try { const json = JSON.parse(body); message = json.error || message; } catch (e) { console.warn('contextApi.uploadChatAttachment: failed to parse error response:', e); }
      throw new ApiError(res.status, message);
    }
    return res.json();
  },
  rawFileUrl: (id: string) => `${BASE_URL}/context/files/${id}/raw`,
  attachmentUrl: (id: string) => `${BASE_URL}/chat/attachments/${id}`,
};

// Gateway identity file helpers
export const gatewayFiles = {
  getAll: () => api.get<Record<string, string>>('/agent-roles/gateway/files'),
  get: (filename: string) => api.get<{ filename: string; content: string }>(`/agent-roles/gateway/files/${filename}`),
  update: (filename: string, content: string) => api.put<{ filename: string; status: string }>(`/agent-roles/gateway/files/${filename}`, { content }),
  listMemory: () => api.get<MemoryFile[]>('/agent-roles/gateway/memory'),
};

// Agent identity file helpers
export const agentFiles = {
  getAll: (slug: string) => api.get<Record<string, string>>(`/agent-roles/${slug}/files`),
  get: (slug: string, filename: string) => api.get<{ filename: string; content: string }>(`/agent-roles/${slug}/files/${filename}`),
  update: (slug: string, filename: string, content: string) => api.put<{ filename: string; status: string }>(`/agent-roles/${slug}/files/${filename}`, { content }),
  init: (slug: string) => api.post<{ status: string }>(`/agent-roles/${slug}/files/init`),
  listMemory: (slug: string) => api.get<MemoryFile[]>(`/agent-roles/${slug}/memory`),
};

// Global skills helpers
export const skills = {
  list: () => api.get<Skill[]>('/skills'),
  get: (name: string) => api.get<Skill>(`/skills/${name}`),
  create: (name: string, content: string) => api.post<Skill>('/skills', { name, content }),
  update: (name: string, content: string) => api.put<Skill>(`/skills/${name}`, { content }),
  delete: (name: string) => api.delete(`/skills/${name}`),
};

// Thread members helpers
export const threadMembers = {
  list: (threadId: string) => api.get<ThreadMember[]>(`/chat/threads/${threadId}/members`),
  remove: (threadId: string, slug: string) => api.delete(`/chat/threads/${threadId}/members/${slug}`),
};

// Agent skills helpers
export const agentSkills = {
  list: (slug: string) => api.get<Skill[]>(`/agent-roles/${slug}/skills`),
  add: (slug: string, skillName: string) => api.post(`/agent-roles/${slug}/skills/add`, { name: skillName }),
  update: (slug: string, skillName: string, content: string) => api.put(`/agent-roles/${slug}/skills/${skillName}`, { content }),
  remove: (slug: string, skillName: string) => api.delete(`/agent-roles/${slug}/skills/${skillName}`),
  publish: (slug: string, skillName: string) => api.post(`/agent-roles/${slug}/skills/${skillName}/publish`),
};

// Browser automation API helpers
export const browserApi = {
  listSessions: () => api.get<BrowserSession[]>('/browser/sessions'),
  createSession: (data: { name: string; headless?: boolean; owner_agent_slug?: string }) =>
    api.post<BrowserSession>('/browser/sessions', data),
  getSession: (id: string) => api.get<BrowserSession>(`/browser/sessions/${id}`),
  updateSession: (id: string, data: { name?: string; owner_agent_slug?: string }) =>
    api.put<BrowserSession>(`/browser/sessions/${id}`, data),
  deleteSession: (id: string) => api.delete(`/browser/sessions/${id}`),
  startSession: (id: string) => api.post(`/browser/sessions/${id}/start`),
  stopSession: (id: string) => api.post(`/browser/sessions/${id}/stop`),
  executeAction: (id: string, action: BrowserActionRequest) =>
    api.post<BrowserActionResult>(`/browser/sessions/${id}/action`, action),
  getScreenshot: (id: string) => api.get<{ image: string }>(`/browser/sessions/${id}/screenshot`),
  takeControl: (id: string) => api.post(`/browser/sessions/${id}/control`),
  releaseControl: (id: string) => api.post(`/browser/sessions/${id}/release`),
  listSessionTasks: (id: string) => api.get<BrowserTask[]>(`/browser/sessions/${id}/tasks`),
  listAllTasks: () => api.get<BrowserTask[]>('/browser/tasks'),
};

// Notification API helpers
export const notificationsApi = {
  list: (unread?: boolean) => api.get<AppNotification[]>(`/notifications${unread ? '?unread=true' : ''}`),
  unreadCount: () => api.get<{ count: number }>('/notifications/count'),
  markRead: (id: string) => api.put(`/notifications/${id}/read`),
  markAllRead: () => api.put('/notifications/read-all'),
  dismiss: (id: string) => api.delete(`/notifications/${id}`),
  dismissAll: () => api.delete('/notifications'),
};

// Agent Library API helpers
export const agentLibrary = {
  list: (params?: { category?: string; q?: string }) => {
    const p = new URLSearchParams();
    if (params?.category) p.set('category', params.category);
    if (params?.q) p.set('q', params.q);
    const qs = p.toString();
    return api.get<LibraryAgent[]>(`/agent-library${qs ? '?' + qs : ''}`);
  },
  get: (slug: string) => api.get<LibraryAgent & { installed_slug?: string }>(`/agent-library/${slug}`),
  install: (slug: string) => api.post<AgentRole>(`/agent-library/${slug}/install`),
};

// Tool Library API helpers
export const toolLibrary = {
  list: (params?: { category?: string; q?: string }) => {
    const p = new URLSearchParams();
    if (params?.category) p.set('category', params.category);
    if (params?.q) p.set('q', params.q);
    const qs = p.toString();
    return api.get<LibraryTool[]>(`/tool-library${qs ? '?' + qs : ''}`);
  },
  get: (slug: string) => api.get<LibraryTool & { installed_id?: string }>(`/tool-library/${slug}`),
  install: (slug: string) => api.post<Tool>(`/tool-library/${slug}/install`),
};

// Skill Library API helpers
export const skillLibrary = {
  list: (params?: { category?: string; q?: string }) => {
    const p = new URLSearchParams();
    if (params?.category) p.set('category', params.category);
    if (params?.q) p.set('q', params.q);
    const qs = p.toString();
    return api.get<LibrarySkill[]>(`/skill-library${qs ? '?' + qs : ''}`);
  },
  get: (slug: string) => api.get<LibrarySkill>(`/skill-library/${slug}`),
  install: (slug: string) => api.post<Skill>(`/skill-library/${slug}/install`),
};

// Tool Import/Export/Integrity helpers
export const toolExtra = {
  exportUrl: (id: string) => `${BASE_URL}/tools/${id}/export`,
  importTool: async (file: File): Promise<Tool> => {
    const formData = new FormData();
    formData.append('file', file);
    const headers: Record<string, string> = {};
    const csrf = getCSRFToken();
    if (csrf) headers['X-CSRF-Token'] = csrf;
    const res = await fetch(`${BASE_URL}/tools/import`, {
      method: 'POST',
      headers,
      body: formData,
      credentials: 'same-origin',
    });
    if (!res.ok) {
      const body = await res.text();
      let message = `Import failed: ${res.status}`;
      try { const json = JSON.parse(body); message = json.error || message; } catch (e) { void e; }
      throw new ApiError(res.status, message);
    }
    return res.json();
  },
  integrity: (id: string) => api.get<ToolIntegrityInfo>(`/tools/${id}/integrity`),
};

// Heartbeat API helpers
export const heartbeatApi = {
  getConfig: () => api.get<HeartbeatConfig>('/heartbeat/config'),
  updateConfig: (cfg: Partial<HeartbeatConfig>) => api.put<HeartbeatConfig>('/heartbeat/config', cfg),
  listExecutions: (params?: { limit?: number; offset?: number; q?: string; status?: string; agent?: string }) => {
    const p = new URLSearchParams();
    if (params?.limit) p.set('limit', String(params.limit));
    if (params?.offset) p.set('offset', String(params.offset));
    if (params?.q) p.set('q', params.q);
    if (params?.status) p.set('status', params.status);
    if (params?.agent) p.set('agent', params.agent);
    const qs = p.toString();
    return api.get<HeartbeatExecutionPage>(`/heartbeat/history${qs ? '?' + qs : ''}`);
  },
  runNow: () => api.post<{ status: string }>('/heartbeat/run-now'),
};
