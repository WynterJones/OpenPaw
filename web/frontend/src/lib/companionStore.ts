/**
 * companionStore
 *
 * Lightweight external store (no zustand in this app) for the PixelLab companion
 * feature: the character library plus live runtime state — the current mood,
 * derived from chat activity, and which agent is currently active (so a pinned
 * companion assigned to that agent can react while others rest).
 */

import { useSyncExternalStore } from 'react';
import { api } from './api';

export type CompanionMood = 'idle' | 'thinking' | 'toolcall' | 'responding';

/** The four default emotes generated when a character is created. */
export const DEFAULT_EMOTES = ['idle', 'walk', 'wave', 'cheer'] as const;

/** Map a live mood to the animation clip that should play. */
export const MOOD_TO_CLIP: Record<CompanionMood, string> = {
  idle: 'idle',
  toolcall: 'walk',
  thinking: 'wave',
  responding: 'cheer',
};

export interface AnimationClip {
  id: string;
  name: string;
  fps: number;
  /** Frame image URLs (served from /api/v1/pixellab/sprites/...). */
  frames: string[];
}

export interface PixelLabCharacter {
  id: string;
  name: string;
  pixellab_id: string;
  base_url: string;
  animations: AnimationClip[];
  pinned: boolean;
  agent_slug: string;
  /** Absent/null = every workspace, matching agents and skills. */
  workspace_id?: string | null;
  created_at: string;
}

interface CompanionState {
  characters: PixelLabCharacter[];
  mood: CompanionMood;
  activeAgentSlug: string | null;
  /**
   * Whether a chat thread is actually open on screen.
   *
   * Companions are mounted once in Layout so they survive navigation, which
   * also meant they hovered over Settings, the Scheduler and every other page
   * with nothing to react to. Chat publishes this so they appear only where
   * their whole purpose — reacting to a conversation — applies.
   */
  chatOpen: boolean;
  /**
   * The whole library, unfiltered by workspace.
   *
   * Separate from `characters` because the two views want opposite things: the
   * floating sprites must respect the active workspace, while the management
   * list must show a companion scoped elsewhere — otherwise scoping one to
   * another workspace would make it vanish with no way to change it back.
   */
  library: PixelLabCharacter[];
}

let state: CompanionState = {
  characters: [],
  mood: 'idle',
  activeAgentSlug: null,
  chatOpen: false,
  library: [],
};

const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

function set(patch: Partial<CompanionState>) {
  state = { ...state, ...patch };
  emit();
}

export const companionStore = {
  subscribe(l: () => void) {
    listeners.add(l);
    return () => {
      listeners.delete(l);
    };
  },
  getState: () => state,

  setMood(mood: CompanionMood) {
    if (state.mood !== mood) set({ mood });
  },
  setActiveAgent(slug: string | null) {
    if (state.activeAgentSlug !== slug) set({ activeAgentSlug: slug });
  },
  setChatOpen(open: boolean) {
    if (state.chatOpen !== open) set({ chatOpen: open });
  },
  setCharacters(characters: PixelLabCharacter[]) {
    set({ characters });
  },

  /** Companions visible in the active workspace — what actually gets pinned. */
  async load(): Promise<PixelLabCharacter[]> {
    const characters = await api.get<PixelLabCharacter[]>('/pixellab/characters');
    set({ characters });
    return characters;
  },

  /** Every companion, for the management list. Also refreshes `characters`. */
  async loadAll(): Promise<PixelLabCharacter[]> {
    const [library] = await Promise.all([
      api.get<PixelLabCharacter[]>('/pixellab/characters?all=true'),
      companionStore.load().catch(() => []),
    ]);
    set({ library });
    return library;
  },
};

export function useCompanionStore(): CompanionState {
  return useSyncExternalStore(companionStore.subscribe, companionStore.getState);
}

/** Resolve a character's clip by name, falling back to idle / the first clip. */
export function clipForName(
  character: PixelLabCharacter,
  name: string
): AnimationClip | null {
  return (
    character.animations.find((c) => c.name === name) ??
    character.animations.find((c) => c.name === 'idle') ??
    character.animations[0] ??
    null
  );
}
