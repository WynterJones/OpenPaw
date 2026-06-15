/**
 * pixellabClient
 *
 * Client for the PixelLab AI v2 API (https://api.pixellab.ai/v2/docs), routed
 * through OpenPaw's server-side proxy (`POST /pixellab/proxy`) so the API key
 * stays encrypted on the server and never reaches the browser.
 *
 * Powers the companion creator: generate pixel-art options, turn a chosen sprite
 * into a reusable character, and animate it into emote clips. Image payloads are
 * base64 PNG data URIs; character creation and animation are async background
 * jobs that we poll until completion.
 */

import { api } from './api';

/** The proxy wraps PixelLab's response so non-2xx upstream codes aren't treated
 *  as transport errors. */
interface ProxyEnvelope {
  status: number;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data: any;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type ApiResponse = Record<string, any>;

export interface ImageSize {
  width: number;
  height: number;
}

/** Normalised, typed error surfaced to the UI. */
export class PixelLabError extends Error {
  status: number;
  code: 'auth' | 'credits' | 'validation' | 'rate_limit' | 'server' | 'unknown';

  constructor(status: number, message: string) {
    super(message);
    this.name = 'PixelLabError';
    this.status = status;
    this.code =
      status === 401
        ? 'auth'
        : status === 402
          ? 'credits'
          : status === 422
            ? 'validation'
            : status === 429 || status === 529
              ? 'rate_limit'
              : status >= 500
                ? 'server'
                : 'unknown';
  }
}

function extractDetail(data: unknown, fallback: string): string {
  if (typeof data === 'string') return data || fallback;
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>;
    if (typeof obj.detail === 'string') return obj.detail;
    if (obj.detail) return JSON.stringify(obj.detail);
    if (typeof obj.message === 'string') return obj.message;
  }
  return fallback;
}

async function request<T>(
  path: string,
  method: 'GET' | 'POST',
  body?: unknown
): Promise<T> {
  let env: ProxyEnvelope;
  try {
    env = await api.post<ProxyEnvelope>('/pixellab/proxy', { path, method, body });
  } catch (e) {
    throw new PixelLabError(0, e instanceof Error ? e.message : String(e));
  }
  if (env.status >= 400) {
    throw new PixelLabError(env.status, extractDetail(env.data, `PixelLab error ${env.status}`));
  }
  return env.data as T;
}

/** Pull a base64 image string out of the various shapes PixelLab returns. */
function extractBase64(image: unknown): string | null {
  if (!image) return null;
  if (typeof image === 'string') return image;
  if (typeof image === 'object') {
    const obj = image as Record<string, unknown>;
    if (typeof obj.base64 === 'string') return obj.base64;
    if (typeof obj.image === 'object') return extractBase64(obj.image);
  }
  return null;
}

/** Ensure a base64 payload is a usable data URI for an <img> src. */
export function toDataUri(base64: string): string {
  return base64.startsWith('data:') ? base64 : `data:image/png;base64,${base64}`;
}

// ---------------------------------------------------------------------------
// Balance
// ---------------------------------------------------------------------------

export interface BalanceInfo {
  usd?: number;
  generations?: number;
  plan?: string;
}

export async function getBalance(): Promise<BalanceInfo> {
  const data = await request<ApiResponse>('/balance', 'GET');
  return {
    usd: data?.credits?.usd ?? data?.usd,
    generations: data?.subscription?.generations,
    plan: data?.subscription?.plan,
  };
}

// ---------------------------------------------------------------------------
// Step 1 — generate pixel-art image options from text
// ---------------------------------------------------------------------------

export interface CreateImageOptions {
  description: string;
  size?: ImageSize;
  noBackground?: boolean;
}

/** One synchronous pixflux image. */
export async function createPixelImage(
  { description, size = { width: 64, height: 64 }, noBackground = true }: CreateImageOptions
): Promise<string> {
  const data = await request<ApiResponse>('/create-image-pixflux', 'POST', {
    description,
    image_size: size,
    no_background: noBackground,
  });
  const b64 = extractBase64(data?.image);
  if (!b64) throw new PixelLabError(0, 'PixelLab returned no image data');
  return toDataUri(b64);
}

/** Generate `count` options in parallel, tolerating partial failures. */
export async function createPixelImageOptions(
  opts: CreateImageOptions,
  count = 3
): Promise<string[]> {
  const results = await Promise.allSettled(
    Array.from({ length: count }, () => createPixelImage(opts))
  );
  const images = results
    .filter((r): r is PromiseFulfilledResult<string> => r.status === 'fulfilled')
    .map((r) => r.value);
  if (images.length === 0) {
    const firstReject = results.find((r) => r.status === 'rejected') as
      | PromiseRejectedResult
      | undefined;
    throw firstReject?.reason ?? new PixelLabError(0, 'All image generations failed');
  }
  return images;
}

// ---------------------------------------------------------------------------
// Step 2 — create a reusable character from the chosen sprite
// ---------------------------------------------------------------------------

export interface CreateCharacterOptions {
  description: string;
  referenceImage?: string;
  size?: ImageSize;
}

export interface JobHandle {
  jobId: string;
  characterId?: string;
}

export async function createCharacter(
  { description, referenceImage, size = { width: 64, height: 64 } }: CreateCharacterOptions
): Promise<JobHandle> {
  const body: Record<string, unknown> = { description, image_size: size };
  if (referenceImage) {
    body.reference_image = { base64: referenceImage };
  }
  const data = await request<ApiResponse>('/create-character-v3', 'POST', body);
  const jobId = data?.background_job_id ?? data?.job_id;
  if (!jobId) throw new PixelLabError(0, 'PixelLab returned no background job id');
  return { jobId, characterId: data?.character_id };
}

// ---------------------------------------------------------------------------
// Step 3 — animate a character into an emote/action clip
// ---------------------------------------------------------------------------

export interface AnimateOptions {
  characterId: string;
  action: string;
  frameCount?: number;
  directions?: string[];
}

export async function animateCharacter(
  { characterId, action, frameCount = 4, directions = ['south'] }: AnimateOptions
): Promise<string[]> {
  const data = await request<ApiResponse>('/animate-character', 'POST', {
    character_id: characterId,
    mode: 'v3',
    action_description: action,
    frame_count: frameCount,
    directions,
  });
  const ids: string[] =
    data?.background_job_ids ?? (data?.background_job_id ? [data.background_job_id] : []);
  if (ids.length === 0) throw new PixelLabError(0, 'PixelLab returned no animation jobs');
  return ids;
}

// ---------------------------------------------------------------------------
// Background job polling
// ---------------------------------------------------------------------------

export interface PollOptions {
  signal?: AbortSignal;
  onProgress?: (status: string) => void;
  maxAttempts?: number;
}

/** A base64-encoded PNG always begins with the bytes \x89PNG…, which encode to
 *  the prefix "iVBORw0KGgo". Anything else (raw pixel buffers, masks, metadata
 *  blobs the API may also return) is not an image we can render. */
function isPngBase64(s: string): boolean {
  const body = s.startsWith('data:') ? s.slice(s.indexOf(',') + 1) : s;
  return body.startsWith('iVBORw0KGgo');
}

/** Collect every base64 *PNG* found anywhere in a completed job response.
 *  We must recurse the whole tree and accept only PNG-signed payloads: the
 *  animate response can carry non-image base64 fields (raw buffers, masks)
 *  alongside the real frames, and grabbing those produced unrenderable
 *  "broken image" frames. */
function collectFrames(response: unknown): string[] {
  const frames: string[] = [];
  const visit = (node: unknown) => {
    if (!node || typeof node !== 'object') return;
    if (Array.isArray(node)) {
      node.forEach(visit);
      return;
    }
    for (const value of Object.values(node as Record<string, unknown>)) {
      if (typeof value === 'string') {
        if (isPngBase64(value)) frames.push(toDataUri(value));
      } else if (value && typeof value === 'object') {
        visit(value);
      }
    }
  };
  visit(response);
  return frames;
}

/** Poll a background job until completion, returning its image frames. */
export async function pollJob(
  jobId: string,
  { signal, onProgress, maxAttempts = 120 }: PollOptions = {}
): Promise<string[]> {
  let delay = 1500;
  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    if (signal?.aborted) throw new PixelLabError(0, 'Cancelled');

    let data: ApiResponse;
    try {
      data = await request<ApiResponse>(`/background-jobs/${jobId}`, 'GET');
    } catch (e) {
      if (e instanceof PixelLabError && e.code === 'rate_limit') {
        delay = Math.min(delay * 2, 10000);
        await sleep(delay, signal);
        continue;
      }
      throw e;
    }

    const status = String(data?.status ?? 'processing').toLowerCase();
    onProgress?.(status);

    if (status === 'completed' || status === 'success' || status === 'done') {
      return collectFrames(data?.last_response ?? data?.result ?? data);
    }
    if (status === 'failed' || status === 'error') {
      throw new PixelLabError(0, data?.error ?? 'PixelLab job failed');
    }

    await sleep(delay, signal);
    delay = Math.min(Math.round(delay * 1.25), 6000);
  }
  throw new PixelLabError(0, 'Timed out waiting for PixelLab job');
}

/** Run an animation request through to completed frames. */
export async function animateAndCollect(
  opts: AnimateOptions,
  poll?: PollOptions
): Promise<string[]> {
  const jobIds = await animateCharacter(opts);
  const perJob = await Promise.all(jobIds.map((id) => pollJob(id, poll)));
  return perJob.flat();
}

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const t = setTimeout(resolve, ms);
    signal?.addEventListener(
      'abort',
      () => {
        clearTimeout(t);
        reject(new PixelLabError(0, 'Cancelled'));
      },
      { once: true }
    );
  });
}
