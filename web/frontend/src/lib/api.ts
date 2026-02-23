const BASE_URL = '/api/v1';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

export function getCSRFToken(): string | null {
  const match = document.cookie.match(/(?:^|;\s*)openpaw_csrf=([^;]*)/);
  return match ? decodeURIComponent(match[1]) : null;
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> || {}),
  };

  // Add CSRF token for state-changing requests
  const method = (options.method || 'GET').toUpperCase();
  if (method !== 'GET' && method !== 'HEAD') {
    const csrf = getCSRFToken();
    if (csrf) {
      headers['X-CSRF-Token'] = csrf;
    }
  }

  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
    credentials: 'same-origin',
  });

  if (res.status === 401) {
    const currentPath = window.location.pathname;
    if (currentPath !== '/login' && currentPath !== '/setup') {
      window.location.href = '/login';
    }
    throw new ApiError(401, 'Unauthorized');
  }

  if (res.status === 409 && res.headers.get('X-Setup-Required') === 'true') {
    window.location.href = '/setup';
    throw new ApiError(409, 'Setup required');
  }

  if (!res.ok) {
    const body = await res.text();
    let message = `Request failed: ${res.status}`;
    try {
      const json = JSON.parse(body);
      message = json.error || json.message || message;
    } catch (e) {
      console.warn('request: failed to parse error response JSON:', e);
      if (body) message = body;
    }
    throw new ApiError(res.status, message);
  }

  if (res.status === 204) return undefined as T;

  return res.json();
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: body ? JSON.stringify(body) : undefined }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
};

// Re-export types and helpers for backwards compatibility
export * from './types';
export { contextApi, gatewayFiles, agentFiles, agentMemories, skills, threadMembers, agentSkills, browserApi, notificationsApi, heartbeatApi, agentLibrary, toolLibrary, toolExtra, skillLibrary, skillsSh, secretsApi, terminalApi, projectsApi, parseConfirmation, parseToolSummary, parseWidgets } from './api-helpers';
export type { SecretCheckResult } from './api-helpers';
