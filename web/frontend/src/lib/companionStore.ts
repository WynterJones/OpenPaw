/**
 * companionStore
 *
 * Lightweight external store (no zustand in this app) for the PixelLab companion
 * feature: the character library plus live runtime state — the current mood,
 * derived from chat activity, and which agent is currently active so the
 * matching assistant avatar can react.
 */

import { useSyncExternalStore } from 'react';
import { api } from './api';

export type CompanionMood = 'idle' | 'thinking' | 'toolcall' | 'responding';

/** The four default emotes generated when a character is created. */
export const DEFAULT_EMOTES = ['idle', 'thinking', 'wave', 'cheer'] as const;

/**
 * What PixelLab is asked to animate for each clip.
 *
 * Split from the clip name because the two answer different questions: the clip
 * name says when OpenPaw plays it, while PixelLab needs a physical action it
 * can actually draw. "thinking" is a mood — asking PixelLab to animate it gets
 * you nothing useful — so the thinking clip is still drawn as a walk.
 */
export const EMOTE_ACTIONS: Record<string, string> = {
  idle: 'idle breathing',
  thinking: 'walk',
  wave: 'wave',
  cheer: 'cheer',
};

/** Map a live mood to the animation clip that should play. */
export const MOOD_TO_CLIP: Record<CompanionMood, string> = {
  idle: 'idle',
  thinking: 'thinking',
  toolcall: 'wave',
  responding: 'cheer',
};

/**
 * Clip names that used to be called something else.
 *
 * Companions created before the rename have a clip named "walk" on disk, and
 * regenerating a companion is a paid round-trip — so the lookup falls back
 * rather than silently dropping those characters to their idle animation.
 */
const CLIP_ALIASES: Record<string, string[]> = {
  thinking: ['walk'],
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
   * The whole library, unfiltered by workspace.
   *
   * Separate from `characters` because chat avatars must respect the active
   * workspace, while the management list must show a companion scoped elsewhere
   * — otherwise scoping one to another workspace would make it vanish with no
   * way to change it back.
   */
  library: PixelLabCharacter[];
}

let state: CompanionState = {
  characters: [],
  mood: 'idle',
  activeAgentSlug: null,
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
  setCharacters(characters: PixelLabCharacter[]) {
    set({ characters });
  },

  /** Companions available as chat avatars in the active workspace. */
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
  const exact = character.animations.find((c) => c.name === name);
  if (exact) return exact;

  for (const alias of CLIP_ALIASES[name] ?? []) {
    const aliased = character.animations.find((c) => c.name === alias);
    if (aliased) return aliased;
  }

  return (
    character.animations.find((c) => c.name === 'idle') ??
    character.animations[0] ??
    null
  );
}
