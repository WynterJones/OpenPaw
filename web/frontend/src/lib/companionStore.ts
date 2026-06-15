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
  created_at: string;
}

interface CompanionState {
  characters: PixelLabCharacter[];
  mood: CompanionMood;
  activeAgentSlug: string | null;
}

let state: CompanionState = {
  characters: [],
  mood: 'idle',
  activeAgentSlug: null,
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
  setCharacters(characters: PixelLabCharacter[]) {
    set({ characters });
  },

  /** Load the character library from the server. */
  async load(): Promise<PixelLabCharacter[]> {
    const characters = await api.get<PixelLabCharacter[]>('/pixellab/characters');
    set({ characters });
    return characters;
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
