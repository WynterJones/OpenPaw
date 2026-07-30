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

export type CanonicalCompanionMood = 'idle' | 'thinking' | 'working' | 'talking';
/** Legacy mood names remain accepted so older callers do not break. */
export type CompanionMood = CanonicalCompanionMood | 'toolcall' | 'responding';

/** Agent-specific states generated for every new character. */
export const DEFAULT_EMOTES = ['idle', 'thinking', 'working', 'talking'] as const;

/**
 * What PixelLab is asked to animate for each clip.
 *
 * Split from the clip name because the two answer different questions: the clip
 * name says when OpenPaw plays it, while PixelLab needs a physical action it
 * can actually draw. Each prompt therefore describes a distinct pose and
 * movement while explicitly excluding the gestures owned by the other states.
 */
const EMOTE_ACTIONS: Record<string, string> = {
  idle: 'Seamless calm idle loop. The character stands planted in place, breathes subtly, shifts weight once, and blinks once. Keep the silhouette steady; no walking, waving, speaking, jumping, or large gestures.',
  thinking: 'Seamless thoughtful loop. The character pauses, tilts their head slightly, looks upward, and brings one hand to their chin before returning to neutral. Deliberate and contemplative; no walking, typing, waving, cheering, or speaking.',
  working: 'Seamless focused work loop. The character leans forward with concentration and performs quick purposeful hand motions as if typing on a small invisible keyboard or operating controls, then briefly checks the result. Energetic but contained; no waving, cheering, walking, or talking.',
  talking: 'Seamless conversational speaking loop. The character faces forward, moves their mouth clearly, and uses one natural open-hand explanatory gesture before settling. Friendly and expressive; no jumping, cheering, typing, walking, or waving.',
  // Old stored clip names regenerate into the new behavior rather than their
  // former generic actions.
  walk: 'Seamless thoughtful loop. The character pauses, tilts their head slightly, looks upward, and brings one hand to their chin before returning to neutral. Do not walk.',
  wave: 'Seamless focused work loop. The character leans forward and performs quick purposeful hand motions as if typing or operating controls. Do not wave.',
  cheer: 'Seamless conversational speaking loop. The character faces forward, moves their mouth clearly, and makes one natural explanatory hand gesture. Do not jump or cheer.',
};

/** Map a live mood to the animation clip that should play. */
export const MOOD_TO_CLIP: Record<CompanionMood, string> = {
  idle: 'idle',
  thinking: 'thinking',
  working: 'working',
  talking: 'talking',
  toolcall: 'working',
  responding: 'talking',
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
  working: ['wave', 'toolcall'],
  talking: ['cheer', 'responding'],
};

const ANIMATION_LABELS: Record<string, string> = {
  idle: 'Idle',
  thinking: 'Thinking',
  working: 'Working',
  talking: 'Talking',
  walk: 'Thinking',
  wave: 'Working',
  cheer: 'Talking',
  toolcall: 'Working',
  responding: 'Talking',
};

export function animationLabelFor(name: string): string {
  return ANIMATION_LABELS[name] || name.replace(/[-_]/g, ' ');
}

export function animationPromptFor(name: string): string {
  return EMOTE_ACTIONS[name] || `Create a seamless, readable pixel-art loop of this agent performing this distinct action: ${name}. Keep the character centered and return cleanly to the starting pose.`;
}

export function animationFpsFor(name: string): number {
  return name === 'idle' ? 4 : name === 'thinking' || name === 'walk' ? 5 : 7;
}

export function animationFrameCountFor(name: string): number {
  return name === 'idle' ? 4 : 6;
}

function canonicalMood(mood: CompanionMood): CanonicalCompanionMood {
  if (mood === 'toolcall') return 'working';
  if (mood === 'responding') return 'talking';
  return mood;
}

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
  mood: CanonicalCompanionMood;
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
    const next = canonicalMood(mood);
    if (state.mood !== next) set({ mood: next });
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
