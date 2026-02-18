import { test, expect } from '@playwright/test';
import { getAuthToken } from './helpers';
import * as fs from 'fs';
import * as path from 'path';

function getAuthCookies(): string {
  const stateFile = path.resolve('tests/.auth/user.json');
  try {
    const state = JSON.parse(fs.readFileSync(stateFile, 'utf8'));
    return (state.cookies || [])
      .map((c: { name: string; value: string }) => `${c.name}=${c.value}`)
      .join('; ');
  } catch {
    return '';
  }
}

function getCsrfToken(): string {
  const stateFile = path.resolve('tests/.auth/user.json');
  try {
    const state = JSON.parse(fs.readFileSync(stateFile, 'utf8'));
    const csrf = state.cookies?.find((c: { name: string }) => c.name === 'openpaw_csrf');
    return csrf?.value || '';
  } catch {
    return '';
  }
}

function authHeaders(): Record<string, string> {
  const token = getAuthToken();
  if (token) {
    return { Authorization: `Bearer ${token}` };
  }
  // Fall back to cookie auth
  const cookies = getAuthCookies();
  return cookies ? { Cookie: cookies } : {};
}

function postHeaders(): Record<string, string> {
  return {
    ...authHeaders(),
    'Content-Type': 'application/json',
    'X-CSRF-Token': getCsrfToken(),
    Cookie: getAuthCookies(),
  };
}

test.describe('API endpoints', () => {
  test('GET /api/v1/setup/status returns needs_setup false', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/setup/status`);
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.needs_setup).toBe(false);
  });

  test('GET /api/v1/auth/me returns current user', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/auth/me`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.user).toBeDefined();
    expect(body.user.username).toBe('testadmin');
  });

  test('GET /api/v1/chat/threads returns array', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/chat/threads`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });

  test('POST /api/v1/chat/threads creates a thread', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/chat/threads`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ title: 'API Test Thread' }),
    });
    expect(res.ok).toBeTruthy();
    const thread = await res.json();
    expect(thread.id).toBeDefined();
  });

  test('GET /api/v1/tools returns list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/tools`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
  });

  test('GET /api/v1/agent-roles returns roles', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/agent-roles`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const roles = await res.json();
    expect(Array.isArray(roles)).toBeTruthy();
    expect(roles.length).toBeGreaterThan(0);
  });

  test('GET /api/v1/system/info returns system info', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/system/info`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const info = await res.json();
    expect(info.version).toBeDefined();
  });

  test('GET /api/v1/logs returns audit logs', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/logs`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    // Logs endpoint returns paginated: { logs: [...], total: N }
    expect(body.logs).toBeDefined();
    expect(Array.isArray(body.logs)).toBeTruthy();
  });

  test('GET /api/v1/skills returns list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/skills`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
  });

  test('GET /api/v1/schedules returns list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/schedules`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const schedules = await res.json();
    expect(Array.isArray(schedules)).toBeTruthy();
  });

  test('GET /api/v1/secrets returns list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/secrets`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
  });

  test('GET /api/v1/system/health returns ok', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/system/health`);
    expect(res.ok).toBeTruthy();
  });

  test('unauthenticated requests get 401', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/auth/me`);
    expect(res.status).toBe(401);
  });

  // --- Secrets CRUD ---

  test('POST /api/v1/secrets creates a secret', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/secrets`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'API_E2E_SECRET', value: 'testvalue123' }),
    });
    expect(res.ok).toBeTruthy();
    const secret = await res.json();
    expect(secret.id).toBeDefined();
    expect(secret.name).toBe('API_E2E_SECRET');

    // Cleanup
    await fetch(`${baseURL}/api/v1/secrets/${secret.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('DELETE /api/v1/secrets/:id deletes a secret', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/secrets`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'API_DELETE_SECRET', value: 'todelete' }),
    });
    const secret = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/secrets/${secret.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();
  });

  test('POST /api/v1/secrets with empty body returns 400', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/secrets`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({}),
    });
    expect(res.status).toBe(400);
  });

  test('DELETE /api/v1/secrets/nonexistent-uuid returns 404', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/secrets/00000000-0000-0000-0000-000000000000`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(res.status).toBe(404);
  });

  // --- Skills CRUD ---

  test('POST /api/v1/skills creates a skill', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/skills`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'api-e2e-skill', content: '# Test skill' }),
    });
    expect(res.ok).toBeTruthy();
    const skill = await res.json();
    expect(skill.name).toBe('api-e2e-skill');

    // Cleanup
    await fetch(`${baseURL}/api/v1/skills/api-e2e-skill`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/skills/:name returns skill content', async ({ baseURL }) => {
    // Create first
    await fetch(`${baseURL}/api/v1/skills`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'api-e2e-skill', content: '# Test skill' }),
    });

    const res = await fetch(`${baseURL}/api/v1/skills/api-e2e-skill`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const skill = await res.json();
    expect(skill.name).toBe('api-e2e-skill');
    expect(skill.content).toContain('# Test skill');

    // Cleanup
    await fetch(`${baseURL}/api/v1/skills/api-e2e-skill`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('PUT /api/v1/skills/:name updates skill content', async ({ baseURL }) => {
    // Create first
    await fetch(`${baseURL}/api/v1/skills`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'api-e2e-skill', content: '# Test skill' }),
    });

    const res = await fetch(`${baseURL}/api/v1/skills/api-e2e-skill`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ content: '# Updated' }),
    });
    expect(res.ok).toBeTruthy();
    const skill = await res.json();
    expect(skill.content).toContain('# Updated');

    // Cleanup
    await fetch(`${baseURL}/api/v1/skills/api-e2e-skill`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('DELETE /api/v1/skills/:name deletes a skill', async ({ baseURL }) => {
    // Create first
    await fetch(`${baseURL}/api/v1/skills`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'api-e2e-skill', content: '# Test skill' }),
    });

    const res = await fetch(`${baseURL}/api/v1/skills/api-e2e-skill`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();
  });

  test('POST /api/v1/skills with empty name returns 400', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/skills`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: '', content: '# Test' }),
    });
    expect(res.status).toBe(400);
  });

  // --- Schedules CRUD ---

  test('POST /api/v1/schedules creates a prompt schedule', async ({ baseURL }) => {
    // First ensure we have an enabled agent role to reference
    const rolesRes = await fetch(`${baseURL}/api/v1/agent-roles?enabled=true`, {
      headers: authHeaders(),
    });
    const roles = await rolesRes.json();
    // Skip schedule creation if no enabled agent roles exist
    if (!Array.isArray(roles) || roles.length === 0) {
      test.skip();
      return;
    }
    const agentSlug = roles[0].slug;

    const res = await fetch(`${baseURL}/api/v1/schedules`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'API E2E Schedule',
        cron_expr: '0 * * * *',
        type: 'prompt',
        agent_role_slug: agentSlug,
        prompt_content: 'Hello from E2E test',
      }),
    });
    expect(res.ok).toBeTruthy();
    const schedule = await res.json();
    expect(schedule.id).toBeDefined();
    expect(schedule.name).toBe('API E2E Schedule');
    expect(schedule.enabled).toBe(true);

    // Cleanup
    await fetch(`${baseURL}/api/v1/schedules/${schedule.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('POST /api/v1/schedules/:id/toggle toggles a schedule', async ({ baseURL }) => {
    const rolesRes = await fetch(`${baseURL}/api/v1/agent-roles?enabled=true`, {
      headers: authHeaders(),
    });
    const roles = await rolesRes.json();
    if (!Array.isArray(roles) || roles.length === 0) {
      test.skip();
      return;
    }
    const agentSlug = roles[0].slug;

    const createRes = await fetch(`${baseURL}/api/v1/schedules`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'API E2E Toggle Schedule',
        cron_expr: '0 * * * *',
        type: 'prompt',
        agent_role_slug: agentSlug,
        prompt_content: 'Toggle test',
      }),
    });
    const schedule = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/schedules/${schedule.id}/toggle`, {
      method: 'POST',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    // enabled starts true, toggle should make it false
    expect(body.enabled).toBe(false);

    // Cleanup
    await fetch(`${baseURL}/api/v1/schedules/${schedule.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('DELETE /api/v1/schedules/:id deletes a schedule', async ({ baseURL }) => {
    const rolesRes = await fetch(`${baseURL}/api/v1/agent-roles?enabled=true`, {
      headers: authHeaders(),
    });
    const roles = await rolesRes.json();
    if (!Array.isArray(roles) || roles.length === 0) {
      test.skip();
      return;
    }
    const agentSlug = roles[0].slug;

    const createRes = await fetch(`${baseURL}/api/v1/schedules`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'API E2E Delete Schedule',
        cron_expr: '0 * * * *',
        type: 'prompt',
        agent_role_slug: agentSlug,
        prompt_content: 'Delete test',
      }),
    });
    const schedule = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/schedules/${schedule.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();
  });

  // --- Agent Roles CRUD ---

  test('POST /api/v1/agent-roles creates an agent role', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/agent-roles`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'API E2E Agent', description: 'test' }),
    });
    expect(res.ok).toBeTruthy();
    const role = await res.json();
    expect(role.slug).toBeDefined();
    expect(role.name).toBe('API E2E Agent');

    // Cleanup
    await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('POST /api/v1/agent-roles/:slug/toggle toggles an agent role', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/agent-roles`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'API E2E Toggle Agent', description: 'toggle test' }),
    });
    const role = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}/toggle`, {
      method: 'POST',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    // role starts enabled=true, toggle should set it to false
    expect(body.enabled).toBe(false);

    // Cleanup
    await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('DELETE /api/v1/agent-roles/:slug deletes an agent role', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/agent-roles`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'API E2E Delete Agent', description: 'delete test' }),
    });
    const role = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();
  });

  // --- Chat Thread CRUD ---

  test('PUT /api/v1/chat/threads/:id renames a thread', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/chat/threads`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ title: 'Original Thread Title' }),
    });
    const thread = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ title: 'Renamed Thread' }),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.title).toBe('Renamed Thread');

    // Cleanup
    await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('DELETE /api/v1/chat/threads/:id deletes a thread', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/chat/threads`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ title: 'Thread To Delete' }),
    });
    const thread = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(res.status).toBe(204);
  });

  // --- Auth ---

  test('POST /api/v1/auth/logout returns ok', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/auth/logout`, {
      method: 'POST',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.message).toBe('logged out');
  });

  // --- Notifications API ---

  test('GET /api/v1/notifications returns list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/notifications`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });

  test('GET /api/v1/notifications/count returns count', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/notifications/count`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body.count).toBe('number');
  });

  test('PUT /api/v1/notifications/read-all marks all read', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/notifications/read-all`, {
      method: 'PUT',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();
  });

  test('DELETE /api/v1/notifications dismisses all', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/notifications`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();
  });

  // --- Heartbeat API ---

  test('GET /api/v1/heartbeat/config returns config', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/heartbeat/config`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');
  });

  test('PUT /api/v1/heartbeat/config updates config', async ({ baseURL }) => {
    // Get current config first to restore after
    const getRes = await fetch(`${baseURL}/api/v1/heartbeat/config`, { headers: authHeaders() });
    const original = await getRes.json();

    const res = await fetch(`${baseURL}/api/v1/heartbeat/config`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ heartbeat_enabled: 'false' }),
    });
    expect(res.ok).toBeTruthy();

    // Restore original
    await fetch(`${baseURL}/api/v1/heartbeat/config`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify(original),
    });
  });

  test('GET /api/v1/heartbeat/history returns paginated results', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/heartbeat/history?limit=10`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.items).toBeDefined();
    expect(typeof body.total).toBe('number');
  });

  test('POST /api/v1/heartbeat/run-now responds', async ({ baseURL }) => {
    // May return error if no agents configured, just verify it doesn't 500
    const res = await fetch(`${baseURL}/api/v1/heartbeat/run-now`, {
      method: 'POST',
      headers: postHeaders(),
    });
    expect(res.status).toBeLessThan(500);
  });

  // --- Context API ---

  test('GET /api/v1/context/about-you returns content', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/context/about-you`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
  });

  test('PUT /api/v1/context/about-you updates content', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/context/about-you`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ content: 'API test about content' }),
    });
    expect(res.ok).toBeTruthy();
  });

  test('POST /api/v1/context/folders creates and DELETE removes', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/context/folders`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'API Test Folder' }),
    });
    expect(createRes.ok).toBeTruthy();
    const folder = await createRes.json();
    expect(folder.id).toBeDefined();

    const delRes = await fetch(`${baseURL}/api/v1/context/folders/${folder.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(delRes.ok).toBeTruthy();
  });

  test('GET /api/v1/context/tree returns tree structure', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/context/tree`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
  });
});
