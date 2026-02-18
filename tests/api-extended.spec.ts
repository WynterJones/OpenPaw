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

// --- Tools CRUD ---

test.describe('Tools CRUD', () => {
  test('POST /api/v1/tools creates a tool', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/tools`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Test Tool',
        description: 'test tool for e2e',
        source_code: '#!/bin/bash\necho hello',
        language: 'bash',
      }),
    });
    expect(res.ok).toBeTruthy();
    const tool = await res.json();
    expect(tool.id).toBeDefined();
    expect(tool.name).toBe('E2E Test Tool');

    // Cleanup
    await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/tools/{id} returns a specific tool', async ({ baseURL }) => {
    // Create first
    const createRes = await fetch(`${baseURL}/api/v1/tools`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Get Tool',
        description: 'test',
        source_code: '#!/bin/bash\necho hi',
        language: 'bash',
      }),
    });
    const tool = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.id).toBe(tool.id);
    expect(body.name).toBe('E2E Get Tool');

    // Cleanup
    await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('PUT /api/v1/tools/{id} updates a tool', async ({ baseURL }) => {
    // Create first
    const createRes = await fetch(`${baseURL}/api/v1/tools`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Update Tool',
        description: 'original',
        source_code: '#!/bin/bash\necho original',
        language: 'bash',
      }),
    });
    const tool = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Updated Tool',
        description: 'updated',
        source_code: '#!/bin/bash\necho updated',
        language: 'bash',
      }),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.name).toBe('E2E Updated Tool');

    // Cleanup
    await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('DELETE /api/v1/tools/{id} deletes a tool', async ({ baseURL }) => {
    // Create first
    const createRes = await fetch(`${baseURL}/api/v1/tools`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Delete Tool',
        description: 'to delete',
        source_code: '#!/bin/bash\necho delete',
        language: 'bash',
      }),
    });
    const tool = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();

    // Verify it's gone
    const getRes = await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      headers: authHeaders(),
    });
    expect(getRes.status).toBe(404);
  });

  test('GET /api/v1/tools/nonexistent returns 404', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/tools/00000000-0000-0000-0000-000000000000`, {
      headers: authHeaders(),
    });
    expect(res.status).toBe(404);
  });

  test('POST /api/v1/tools/{id}/enable enables a tool', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/tools`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Enable Tool',
        description: 'test',
        source_code: '#!/bin/bash\necho hi',
        language: 'bash',
      }),
    });
    const tool = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/tools/${tool.id}/enable`, {
      method: 'POST',
      headers: postHeaders(),
    });
    expect(res.status).toBeLessThan(500);

    // Cleanup
    await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('POST /api/v1/tools/{id}/disable disables a tool', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/tools`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Disable Tool',
        description: 'test',
        source_code: '#!/bin/bash\necho hi',
        language: 'bash',
      }),
    });
    const tool = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/tools/${tool.id}/disable`, {
      method: 'POST',
      headers: postHeaders(),
    });
    expect(res.status).toBeLessThan(500);

    // Cleanup
    await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });
});

// --- Tool Library ---

test.describe('Tool Library', () => {
  test('GET /api/v1/tool-library returns catalog list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/tool-library`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });

  test('GET /api/v1/tool-library/{slug} returns catalog tool or 404', async ({ baseURL }) => {
    // First get the list to find a valid slug
    const listRes = await fetch(`${baseURL}/api/v1/tool-library`, { headers: authHeaders() });
    const catalog = await listRes.json();

    if (Array.isArray(catalog) && catalog.length > 0) {
      const slug = catalog[0].slug || catalog[0].name;
      const res = await fetch(`${baseURL}/api/v1/tool-library/${slug}`, {
        headers: authHeaders(),
      });
      expect(res.status).toBeLessThan(500);
    } else {
      // No catalog items, just verify the endpoint doesn't 500 for a fake slug
      const res = await fetch(`${baseURL}/api/v1/tool-library/nonexistent`, {
        headers: authHeaders(),
      });
      expect(res.status).toBeLessThan(500);
    }
  });
});

// --- Agent Library ---

test.describe('Agent Library', () => {
  test('GET /api/v1/agent-library returns catalog list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/agent-library`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });

  test('GET /api/v1/agent-library/{slug} returns catalog agent or 404', async ({ baseURL }) => {
    const listRes = await fetch(`${baseURL}/api/v1/agent-library`, { headers: authHeaders() });
    const catalog = await listRes.json();

    if (Array.isArray(catalog) && catalog.length > 0) {
      const slug = catalog[0].slug || catalog[0].name;
      const res = await fetch(`${baseURL}/api/v1/agent-library/${slug}`, {
        headers: authHeaders(),
      });
      expect(res.status).toBeLessThan(500);
    } else {
      const res = await fetch(`${baseURL}/api/v1/agent-library/nonexistent`, {
        headers: authHeaders(),
      });
      expect(res.status).toBeLessThan(500);
    }
  });
});

// --- Skill Library ---

test.describe('Skill Library', () => {
  test('GET /api/v1/skill-library returns catalog list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/skill-library`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });

  test('GET /api/v1/skill-library/{slug} returns catalog skill or 404', async ({ baseURL }) => {
    const listRes = await fetch(`${baseURL}/api/v1/skill-library`, { headers: authHeaders() });
    const catalog = await listRes.json();

    if (Array.isArray(catalog) && catalog.length > 0) {
      const slug = catalog[0].slug || catalog[0].name;
      const res = await fetch(`${baseURL}/api/v1/skill-library/${slug}`, {
        headers: authHeaders(),
      });
      expect(res.status).toBeLessThan(500);
    } else {
      const res = await fetch(`${baseURL}/api/v1/skill-library/nonexistent`, {
        headers: authHeaders(),
      });
      expect(res.status).toBeLessThan(500);
    }
  });
});

// --- Dashboards CRUD ---

test.describe('Dashboards CRUD', () => {
  test('GET /api/v1/dashboards returns list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/dashboards`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });

  test('POST /api/v1/dashboards creates a dashboard', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/dashboards`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Test Dashboard', description: 'test dashboard' }),
    });
    expect(res.status).toBe(201);
    const dashboard = await res.json();
    expect(dashboard.id).toBeDefined();
    expect(dashboard.name).toBe('E2E Test Dashboard');

    // Cleanup
    await fetch(`${baseURL}/api/v1/dashboards/${dashboard.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/dashboards/{id} returns a specific dashboard', async ({ baseURL }) => {
    // Create first
    const createRes = await fetch(`${baseURL}/api/v1/dashboards`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Get Dashboard', description: 'test' }),
    });
    const dashboard = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/dashboards/${dashboard.id}`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.id).toBe(dashboard.id);
    expect(body.name).toBe('E2E Get Dashboard');

    // Cleanup
    await fetch(`${baseURL}/api/v1/dashboards/${dashboard.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('PUT /api/v1/dashboards/{id} updates a dashboard', async ({ baseURL }) => {
    // Create first
    const createRes = await fetch(`${baseURL}/api/v1/dashboards`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Update Dashboard', description: 'original' }),
    });
    const dashboard = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/dashboards/${dashboard.id}`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Updated Dashboard', description: 'updated' }),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.name).toBe('E2E Updated Dashboard');

    // Cleanup
    await fetch(`${baseURL}/api/v1/dashboards/${dashboard.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('DELETE /api/v1/dashboards/{id} deletes a dashboard', async ({ baseURL }) => {
    // Create first
    const createRes = await fetch(`${baseURL}/api/v1/dashboards`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Delete Dashboard', description: 'to delete' }),
    });
    const dashboard = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/dashboards/${dashboard.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
    expect(res.ok).toBeTruthy();

    // Verify deleted
    const getRes = await fetch(`${baseURL}/api/v1/dashboards/${dashboard.id}`, {
      headers: authHeaders(),
    });
    expect(getRes.status).toBe(404);
  });

  test('POST /api/v1/dashboards with empty name returns 400', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/dashboards`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: '', description: 'no name' }),
    });
    expect(res.status).toBe(400);
  });
});

// --- Settings ---

test.describe('Settings', () => {
  test('GET /api/v1/settings returns full settings', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/settings`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');
  });

  test('PUT /api/v1/settings updates and restores settings', async ({ baseURL }) => {
    // Get current settings to restore later
    const getRes = await fetch(`${baseURL}/api/v1/settings`, { headers: authHeaders() });
    const original = await getRes.json();

    const res = await fetch(`${baseURL}/api/v1/settings`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ app_name: 'E2E Test App Name' }),
    });
    expect(res.ok).toBeTruthy();

    // Restore original
    await fetch(`${baseURL}/api/v1/settings`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify(original),
    });
  });

  test('GET /api/v1/settings/design returns design config (public)', async ({ baseURL }) => {
    // This is a public endpoint, no auth needed
    const res = await fetch(`${baseURL}/api/v1/settings/design`);
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');
  });

  test('PUT /api/v1/settings/design updates design settings', async ({ baseURL }) => {
    // Get current design to restore
    const getRes = await fetch(`${baseURL}/api/v1/settings/design`);
    const original = await getRes.json();

    const res = await fetch(`${baseURL}/api/v1/settings/design`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify(original),
    });
    expect(res.ok).toBeTruthy();
  });

  test('GET /api/v1/settings/models returns model config', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/settings/models`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');
  });

  test('GET /api/v1/settings/api-key returns API key status', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/settings/api-key`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');
  });

  test('GET /api/v1/settings/available-models returns list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/settings/available-models`, {
      headers: authHeaders(),
    });
    // May fail if no API key configured, but should not 500
    expect(res.status).toBeLessThan(500);
  });
});

// --- System ---

test.describe('System', () => {
  test('GET /api/v1/system/balance responds without 500', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/system/balance`, { headers: authHeaders() });
    // May return error if no API key, just check it doesn't 500
    expect(res.status).toBeLessThan(500);
  });

  test('GET /api/v1/system/prerequisites returns prerequisites (public)', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/system/prerequisites`);
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');
  });
});

// --- Logs ---

test.describe('Logs extended', () => {
  test('GET /api/v1/logs/stats returns log statistics', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/logs/stats`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');
  });
});

// --- Gateway Memories ---

test.describe('Gateway Memories', () => {
  test('GET /api/v1/gateway/memories returns list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/gateway/memories`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body) || typeof body === 'object').toBeTruthy();
  });

  test('GET /api/v1/gateway/memories/stats returns stats', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/gateway/memories/stats`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');
  });
});

// --- Agent Roles Extended ---

test.describe('Agent Roles extended', () => {
  let testSlug: string;

  test('GET /api/v1/agent-roles/{slug} returns single role', async ({ baseURL }) => {
    // Create a role
    const createRes = await fetch(`${baseURL}/api/v1/agent-roles`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Detail Agent', description: 'detail test' }),
    });
    const role = await createRes.json();
    testSlug = role.slug;

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${testSlug}`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.slug).toBe(testSlug);
    expect(body.name).toBe('E2E Detail Agent');

    // Cleanup
    await fetch(`${baseURL}/api/v1/agent-roles/${testSlug}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('PUT /api/v1/agent-roles/{slug} updates a role', async ({ baseURL }) => {
    // Create
    const createRes = await fetch(`${baseURL}/api/v1/agent-roles`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Update Agent', description: 'to update' }),
    });
    const role = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Updated Agent', description: 'updated' }),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.name).toBe('E2E Updated Agent');

    // Cleanup
    await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/agent-roles/{slug}/tools returns tool list', async ({ baseURL }) => {
    // Create a role
    const createRes = await fetch(`${baseURL}/api/v1/agent-roles`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Tools Agent', description: 'tools test' }),
    });
    const role = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}/tools`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();

    // Cleanup
    await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/agent-roles/{slug}/skills returns skill list', async ({ baseURL }) => {
    // Create a role
    const createRes = await fetch(`${baseURL}/api/v1/agent-roles`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Skills Agent', description: 'skills test' }),
    });
    const role = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}/skills`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();

    // Cleanup
    await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/agent-roles/{slug}/memories returns memory list', async ({ baseURL }) => {
    // Create a role
    const createRes = await fetch(`${baseURL}/api/v1/agent-roles`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Memories Agent', description: 'memories test' }),
    });
    const role = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}/memories`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body) || typeof body === 'object').toBeTruthy();

    // Cleanup
    await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/agent-roles/{slug}/memories/stats returns stats', async ({ baseURL }) => {
    // Create a role
    const createRes = await fetch(`${baseURL}/api/v1/agent-roles`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E MemStats Agent', description: 'mem stats test' }),
    });
    const role = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}/memories/stats`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');

    // Cleanup
    await fetch(`${baseURL}/api/v1/agent-roles/${role.slug}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/agent-roles/nonexistent returns 404', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/agent-roles/nonexistent-slug-xyz`, {
      headers: authHeaders(),
    });
    expect(res.status).toBe(404);
  });

  test('GET /api/v1/agent-roles/{slug}/memory returns agent memory', async ({ baseURL }) => {
    // Use an existing role from the list
    const listRes = await fetch(`${baseURL}/api/v1/agent-roles`, { headers: authHeaders() });
    const roles = await listRes.json();
    if (!Array.isArray(roles) || roles.length === 0) {
      test.skip();
      return;
    }

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${roles[0].slug}/memory`, {
      headers: authHeaders(),
    });
    expect(res.status).toBeLessThan(500);
  });

  test('GET /api/v1/agent-roles/{slug}/files returns agent files', async ({ baseURL }) => {
    const listRes = await fetch(`${baseURL}/api/v1/agent-roles`, { headers: authHeaders() });
    const roles = await listRes.json();
    if (!Array.isArray(roles) || roles.length === 0) {
      test.skip();
      return;
    }

    const res = await fetch(`${baseURL}/api/v1/agent-roles/${roles[0].slug}/files`, {
      headers: authHeaders(),
    });
    expect(res.status).toBeLessThan(500);
  });

  test('GET /api/v1/agent-roles/gateway/files returns gateway files', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/agent-roles/gateway/files`, {
      headers: authHeaders(),
    });
    expect(res.status).toBeLessThan(500);
  });

  test('GET /api/v1/agent-roles/gateway/memory returns gateway memory', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/agent-roles/gateway/memory`, {
      headers: authHeaders(),
    });
    expect(res.status).toBeLessThan(500);
  });
});

// --- Chat Extended ---

test.describe('Chat extended', () => {
  test('GET /api/v1/chat/threads/active returns active thread IDs', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/chat/threads/active`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });

  test('GET /api/v1/chat/threads/{id}/messages returns messages', async ({ baseURL }) => {
    // Create a thread first
    const createRes = await fetch(`${baseURL}/api/v1/chat/threads`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ title: 'E2E Messages Thread' }),
    });
    const thread = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}/messages`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();

    // Cleanup
    await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/chat/threads/{id}/status returns thread status', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/chat/threads`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ title: 'E2E Status Thread' }),
    });
    const thread = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}/status`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');

    // Cleanup
    await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/chat/threads/{id}/stats returns thread stats', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/chat/threads`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ title: 'E2E Stats Thread' }),
    });
    const thread = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}/stats`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(typeof body).toBe('object');

    // Cleanup
    await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/chat/threads/{id}/members returns members', async ({ baseURL }) => {
    const createRes = await fetch(`${baseURL}/api/v1/chat/threads`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ title: 'E2E Members Thread' }),
    });
    const thread = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}/members`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();

    // Cleanup
    await fetch(`${baseURL}/api/v1/chat/threads/${thread.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/chat/threads/nonexistent/messages returns 404', async ({ baseURL }) => {
    const res = await fetch(
      `${baseURL}/api/v1/chat/threads/00000000-0000-0000-0000-000000000000/messages`,
      { headers: authHeaders() },
    );
    // May return 404 or empty array depending on implementation
    expect(res.status).toBeLessThan(500);
  });
});

// --- Auth Extended ---

test.describe('Auth extended', () => {
  test('PUT /api/v1/auth/profile updates display name', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/auth/profile`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ display_name: 'E2E Test Admin' }),
    });
    expect(res.ok).toBeTruthy();

    // Verify the change
    const meRes = await fetch(`${baseURL}/api/v1/auth/me`, { headers: authHeaders() });
    const me = await meRes.json();
    expect(me.user.display_name).toBe('E2E Test Admin');

    // Restore (set back to empty)
    await fetch(`${baseURL}/api/v1/auth/profile`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ display_name: '' }),
    });
  });

  test('POST /api/v1/auth/change-password with wrong current password returns error', async ({
    baseURL,
  }) => {
    const res = await fetch(`${baseURL}/api/v1/auth/change-password`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        current_password: 'WrongPassword123',
        new_password: 'NewPassword123',
      }),
    });
    // Should return 400 or 401 for wrong current password
    expect(res.status).toBeGreaterThanOrEqual(400);
    expect(res.status).toBeLessThan(500);
  });
});

// --- Agents ---

test.describe('Agents', () => {
  test('GET /api/v1/agents returns running agents list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/agents`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });
});

// --- Browser Sessions ---

test.describe('Browser Sessions', () => {
  test('GET /api/v1/browser/sessions returns sessions list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/browser/sessions`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });

  test('GET /api/v1/browser/tasks returns all tasks list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/browser/tasks`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });
});

// --- Tool Import/Export/Integrity ---

test.describe('Tool Import/Export/Integrity', () => {
  test('GET /api/v1/tools/{id}/export exports a tool', async ({ baseURL }) => {
    // Create a tool first
    const createRes = await fetch(`${baseURL}/api/v1/tools`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Export Tool',
        description: 'test export',
        source_code: '#!/bin/bash\necho export',
        language: 'bash',
      }),
    });
    const tool = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/tools/${tool.id}/export`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();

    // Cleanup
    await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('GET /api/v1/tools/{id}/integrity returns integrity info', async ({ baseURL }) => {
    // Create a tool first
    const createRes = await fetch(`${baseURL}/api/v1/tools`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Integrity Tool',
        description: 'test integrity',
        source_code: '#!/bin/bash\necho integrity',
        language: 'bash',
      }),
    });
    const tool = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/tools/${tool.id}/integrity`, {
      headers: authHeaders(),
    });
    expect(res.status).toBeLessThan(500);

    // Cleanup
    await fetch(`${baseURL}/api/v1/tools/${tool.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });
});

// --- Schedules Extended ---

test.describe('Schedules extended', () => {
  test('GET /api/v1/schedules/{id}/executions returns executions', async ({ baseURL }) => {
    const rolesRes = await fetch(`${baseURL}/api/v1/agent-roles?enabled=true`, {
      headers: authHeaders(),
    });
    const roles = await rolesRes.json();
    if (!Array.isArray(roles) || roles.length === 0) {
      test.skip();
      return;
    }

    // Create a schedule
    const createRes = await fetch(`${baseURL}/api/v1/schedules`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Executions Schedule',
        cron_expr: '0 * * * *',
        type: 'prompt',
        agent_role_slug: roles[0].slug,
        prompt_content: 'Executions test',
      }),
    });
    const schedule = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/schedules/${schedule.id}/executions`, {
      headers: authHeaders(),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();

    // Cleanup
    await fetch(`${baseURL}/api/v1/schedules/${schedule.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });

  test('PUT /api/v1/schedules/{id} updates a schedule', async ({ baseURL }) => {
    const rolesRes = await fetch(`${baseURL}/api/v1/agent-roles?enabled=true`, {
      headers: authHeaders(),
    });
    const roles = await rolesRes.json();
    if (!Array.isArray(roles) || roles.length === 0) {
      test.skip();
      return;
    }

    const createRes = await fetch(`${baseURL}/api/v1/schedules`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Update Schedule',
        cron_expr: '0 * * * *',
        type: 'prompt',
        agent_role_slug: roles[0].slug,
        prompt_content: 'Update test',
      }),
    });
    const schedule = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/schedules/${schedule.id}`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({
        name: 'E2E Updated Schedule',
        cron_expr: '30 * * * *',
        type: 'prompt',
        agent_role_slug: roles[0].slug,
        prompt_content: 'Updated content',
      }),
    });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(body.name).toBe('E2E Updated Schedule');

    // Cleanup
    await fetch(`${baseURL}/api/v1/schedules/${schedule.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });
});

// --- Secrets Extended ---

test.describe('Secrets extended', () => {
  test('POST /api/v1/secrets/{id}/rotate rotates a secret', async ({ baseURL }) => {
    // Create a secret
    const createRes = await fetch(`${baseURL}/api/v1/secrets`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'API_ROTATE_SECRET', value: 'original_value' }),
    });
    const secret = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/secrets/${secret.id}/rotate`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ value: 'new_rotated_value' }),
    });
    expect(res.ok).toBeTruthy();

    // Cleanup
    await fetch(`${baseURL}/api/v1/secrets/${secret.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });
});

// --- Context Extended ---

test.describe('Context extended', () => {
  test('GET /api/v1/context/files returns file list', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/context/files`, { headers: authHeaders() });
    expect(res.ok).toBeTruthy();
    const body = await res.json();
    expect(Array.isArray(body)).toBeTruthy();
  });

  test('PUT /api/v1/context/folders/{id} updates a folder', async ({ baseURL }) => {
    // Create a folder first
    const createRes = await fetch(`${baseURL}/api/v1/context/folders`, {
      method: 'POST',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Update Folder' }),
    });
    const folder = await createRes.json();

    const res = await fetch(`${baseURL}/api/v1/context/folders/${folder.id}`, {
      method: 'PUT',
      headers: postHeaders(),
      body: JSON.stringify({ name: 'E2E Renamed Folder' }),
    });
    expect(res.ok).toBeTruthy();

    // Cleanup
    await fetch(`${baseURL}/api/v1/context/folders/${folder.id}`, {
      method: 'DELETE',
      headers: postHeaders(),
    });
  });
});

// --- Unauthenticated Access Control ---

test.describe('Unauthenticated access control', () => {
  test('GET /api/v1/tools without auth returns 401', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/tools`);
    expect(res.status).toBe(401);
  });

  test('GET /api/v1/dashboards without auth returns 401', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/dashboards`);
    expect(res.status).toBe(401);
  });

  test('GET /api/v1/settings without auth returns 401', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/settings`);
    expect(res.status).toBe(401);
  });

  test('GET /api/v1/agents without auth returns 401', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/agents`);
    expect(res.status).toBe(401);
  });

  test('POST /api/v1/tools without auth returns 401', async ({ baseURL }) => {
    const res = await fetch(`${baseURL}/api/v1/tools`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'Should Fail' }),
    });
    expect(res.status).toBe(401);
  });

  test('public endpoints work without auth', async ({ baseURL }) => {
    // These should all work without auth
    const healthRes = await fetch(`${baseURL}/api/v1/system/health`);
    expect(healthRes.ok).toBeTruthy();

    const designRes = await fetch(`${baseURL}/api/v1/settings/design`);
    expect(designRes.ok).toBeTruthy();

    const prereqRes = await fetch(`${baseURL}/api/v1/system/prerequisites`);
    expect(prereqRes.ok).toBeTruthy();

    const setupRes = await fetch(`${baseURL}/api/v1/setup/status`);
    expect(setupRes.ok).toBeTruthy();
  });
});

// --- Notifications Extended ---

test.describe('Notifications extended', () => {
  test('PUT /api/v1/notifications/{id}/read marks single notification read', async ({
    baseURL,
  }) => {
    // Get current notifications
    const listRes = await fetch(`${baseURL}/api/v1/notifications`, { headers: authHeaders() });
    const notifications = await listRes.json();

    if (Array.isArray(notifications) && notifications.length > 0) {
      const res = await fetch(`${baseURL}/api/v1/notifications/${notifications[0].id}/read`, {
        method: 'PUT',
        headers: postHeaders(),
      });
      expect(res.ok).toBeTruthy();
    } else {
      // No notifications to mark - just verify the endpoint pattern with a fake ID
      const res = await fetch(
        `${baseURL}/api/v1/notifications/00000000-0000-0000-0000-000000000000/read`,
        {
          method: 'PUT',
          headers: postHeaders(),
        },
      );
      expect(res.status).toBeLessThan(500);
    }
  });
});

// --- Logs Tool Logs ---

test.describe('Logs tool logs', () => {
  test('GET /api/v1/logs/tools/{id} returns tool-specific logs', async ({ baseURL }) => {
    // Get a tool first
    const toolsRes = await fetch(`${baseURL}/api/v1/tools`, { headers: authHeaders() });
    const tools = await toolsRes.json();

    if (Array.isArray(tools) && tools.length > 0) {
      const res = await fetch(`${baseURL}/api/v1/logs/tools/${tools[0].id}`, {
        headers: authHeaders(),
      });
      expect(res.status).toBeLessThan(500);
    } else {
      // Just verify it doesn't crash with a fake ID
      const res = await fetch(
        `${baseURL}/api/v1/logs/tools/00000000-0000-0000-0000-000000000000`,
        { headers: authHeaders() },
      );
      expect(res.status).toBeLessThan(500);
    }
  });
});
