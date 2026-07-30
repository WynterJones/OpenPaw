/**
 * Companion-backed assistant avatar.
 *
 * Enabled companions replace the matching agent's normal chat avatar. The
 * current response plays mood-specific frames; completed responses render one
 * stable idle frame so a transcript never becomes a wall of looping sprites.
 */

import { useMemo } from 'react';
import type { AgentRole } from '../../lib/types';
import {
  clipForName,
  MOOD_TO_CLIP,
  useCompanionStore,
  type PixelLabCharacter,
} from '../../lib/companionStore';
import { SpriteAnimation } from './SpriteAnimation';

function preferWorkspaceSpecific(characters: PixelLabCharacter[]): PixelLabCharacter | null {
  return characters.find((character) => Boolean(character.workspace_id))
    ?? characters[0]
    ?? null;
}

function resolveCompanion(
  characters: PixelLabCharacter[],
  agentSlug: string,
): PixelLabCharacter | null {
  const enabled = characters.filter((character) => character.pinned);
  const assigned = preferWorkspaceSpecific(
    enabled.filter((character) => character.agent_slug === agentSlug && agentSlug !== ''),
  );
  if (assigned) return assigned;

  return preferWorkspaceSpecific(
    enabled.filter((character) => !character.agent_slug),
  );
}

export function CompanionAvatar({
  role,
  active = false,
}: {
  role: AgentRole | null;
  active?: boolean;
}) {
  const { characters, mood, activeAgentSlug } = useCompanionStore();
  const agentSlug = role?.slug || activeAgentSlug || '';
  const companion = useMemo(
    () => resolveCompanion(characters, agentSlug),
    [agentSlug, characters],
  );

  const clipName = active ? MOOD_TO_CLIP[mood] : 'idle';
  const clip = companion ? clipForName(companion, clipName) : null;
  const sourceFrames = clip?.frames?.length
    ? clip.frames
    : companion?.base_url
      ? [companion.base_url]
      : [];
  const frames = active ? sourceFrames : sourceFrames.slice(0, 1);

  if (companion && frames.length > 0) {
    return (
      <div
        className="relative flex h-12 w-12 flex-shrink-0 items-center justify-center overflow-hidden"
        aria-label={`${companion.name}, ${role?.name || 'AI'} companion`}
      >
        <SpriteAnimation
          key={`${companion.id}:${clip?.id || 'base'}:${active ? 'active' : 'still'}`}
          frames={frames}
          fps={clip?.fps ?? 6}
          size={48}
          paused={!active}
          autoCrop={Boolean(clip)}
          alt=""
        />
      </div>
    );
  }

  return (
    <div
      className="relative flex h-10 w-10 flex-shrink-0 items-center justify-center overflow-hidden rounded-xl bg-surface-2/90 ring-1 ring-border-1"
    >
      {role ? (
        <img
          src={role.avatar_path}
          alt={role.name}
          className="h-10 w-10 rounded-xl object-cover"
        />
      ) : !active ? (
        <img
          src="/gateway-avatar.png"
          alt="AI"
          className="h-10 w-10 rounded-xl object-cover"
        />
      ) : (
        <span className="h-4 w-4 animate-pulse rounded-full bg-surface-3" aria-hidden="true" />
      )}
    </div>
  );
}
