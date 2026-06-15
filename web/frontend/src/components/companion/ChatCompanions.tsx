/**
 * ChatCompanions
 *
 * Renders the pinned PixelLab companions as movable floating sprites overlaid on
 * the app. Each companion is independently draggable and remembers its position.
 * Mood is derived from live chat activity (see useCompanionActivity): the active
 * agent's companion reacts while the others idle; with no active agent every
 * pinned companion reacts to the global mood. While idle a companion occasionally
 * plays a random emote so it feels alive.
 *
 * Mounted once in Layout so companions persist across navigation.
 */

import { useEffect, useMemo, useRef, useState } from 'react';
import { X } from 'lucide-react';
import { api } from '../../lib/api';
import {
  companionStore,
  useCompanionStore,
  clipForName,
  MOOD_TO_CLIP,
  type CompanionMood,
  type PixelLabCharacter,
} from '../../lib/companionStore';
import { useCompanionActivity } from '../../hooks/useCompanionActivity';
import { SpriteAnimation } from './SpriteAnimation';

const COMPANION_SIZE = 96;
const IDLE_EMOTE_MIN_MS = 9000;
const IDLE_EMOTE_MAX_MS = 20000;
const IDLE_EMOTE_DURATION_MS = 2500;

interface Pos {
  x: number;
  y: number;
}

function loadPos(id: string, index: number): Pos {
  try {
    const raw = localStorage.getItem(`companion-pos-${id}`);
    if (raw) return JSON.parse(raw) as Pos;
  } catch {
    /* ignore */
  }
  // Stagger defaults up from the bottom-right corner.
  return {
    x: window.innerWidth - COMPANION_SIZE - 24 - index * (COMPANION_SIZE + 12),
    y: window.innerHeight - COMPANION_SIZE - 24,
  };
}

function savePos(id: string, pos: Pos) {
  try {
    localStorage.setItem(`companion-pos-${id}`, JSON.stringify(pos));
  } catch {
    /* ignore */
  }
}

function CompanionSprite({
  character,
  index,
  mood,
}: {
  character: PixelLabCharacter;
  index: number;
  mood: CompanionMood;
}) {
  const [pos, setPos] = useState<Pos>(() => loadPos(character.id, index));
  const [hovered, setHovered] = useState(false);
  const [randomClip, setRandomClip] = useState<string | null>(null);
  const dragRef = useRef<{ dx: number; dy: number; moved: boolean } | null>(null);
  const posRef = useRef(pos);
  const randomTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    posRef.current = pos;
  }, [pos]);

  // Keep companions on-screen if the window shrinks.
  useEffect(() => {
    const onResize = () =>
      setPos((p) => ({
        x: Math.min(p.x, window.innerWidth - COMPANION_SIZE),
        y: Math.min(p.y, window.innerHeight - COMPANION_SIZE),
      }));
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  // Occasional random emote while idle.
  useEffect(() => {
    if (mood !== 'idle') return;
    let cancelled = false;
    const schedule = () => {
      const wait = IDLE_EMOTE_MIN_MS + Math.random() * (IDLE_EMOTE_MAX_MS - IDLE_EMOTE_MIN_MS);
      randomTimer.current = setTimeout(() => {
        if (cancelled) return;
        const pool = character.animations.filter((c) => c.name !== 'idle');
        if (pool.length > 0) {
          const pick = pool[Math.floor(Math.random() * pool.length)];
          setRandomClip(pick.name);
          setTimeout(() => {
            if (!cancelled) setRandomClip(null);
          }, IDLE_EMOTE_DURATION_MS);
        }
        schedule();
      }, wait);
    };
    schedule();
    return () => {
      cancelled = true;
      if (randomTimer.current) clearTimeout(randomTimer.current);
    };
  }, [character, mood]);

  const onPointerDown = (e: React.PointerEvent) => {
    // Don't start a drag (and don't capture the pointer) when the press begins on
    // a control like the unpin button — capturing here would redirect the pointerup
    // to this div and swallow the button's click.
    if ((e.target as HTMLElement).closest('button')) return;
    e.currentTarget.setPointerCapture?.(e.pointerId);
    dragRef.current = { dx: e.clientX - pos.x, dy: e.clientY - pos.y, moved: false };
  };

  const onPointerMove = (e: React.PointerEvent) => {
    if (!dragRef.current) return;
    dragRef.current.moved = true;
    const x = Math.max(0, Math.min(window.innerWidth - COMPANION_SIZE, e.clientX - dragRef.current.dx));
    const y = Math.max(0, Math.min(window.innerHeight - COMPANION_SIZE, e.clientY - dragRef.current.dy));
    setPos({ x, y });
  };

  const onPointerUp = () => {
    if (dragRef.current?.moved) savePos(character.id, posRef.current);
    dragRef.current = null;
  };

  const unpin = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await api.put(`/pixellab/characters/${character.id}`, { pinned: false });
      await companionStore.load();
    } catch {
      /* ignore */
    }
  };

  const clip = useMemo(() => {
    // Random emotes only apply while idle; otherwise the mood clip wins.
    const wanted = (mood === 'idle' ? randomClip : null) ?? MOOD_TO_CLIP[mood] ?? 'idle';
    return clipForName(character, wanted);
  }, [character, mood, randomClip]);

  const frames = clip?.frames?.length ? clip.frames : character.base_url ? [character.base_url] : [];

  if (frames.length === 0) return null;

  return (
    <div
      role="img"
      aria-label={`${character.name} companion`}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="fixed z-40 cursor-grab active:cursor-grabbing select-none"
      style={{ left: pos.x, top: pos.y, width: COMPANION_SIZE, height: COMPANION_SIZE, pointerEvents: 'auto' }}
    >
      <SpriteAnimation frames={frames} fps={clip?.fps ?? 6} size={COMPANION_SIZE} />
      {hovered && (
        <>
          <button
            onClick={unpin}
            className="absolute top-0 right-0 w-5 h-5 rounded-full bg-black/60 hover:bg-black/80 flex items-center justify-center text-white"
            aria-label={`Unpin ${character.name}`}
          >
            <X className="w-3 h-3" />
          </button>
          <span className="absolute -bottom-4 left-1/2 -translate-x-1/2 whitespace-nowrap text-[10px] text-text-2 bg-surface-1/80 px-1.5 rounded">
            {character.name}
          </span>
        </>
      )}
    </div>
  );
}

export function ChatCompanions() {
  const { characters, mood, activeAgentSlug } = useCompanionStore();
  const pinned = useMemo(() => characters.filter((c) => c.pinned), [characters]);

  // Load the library once on mount.
  useEffect(() => {
    companionStore.load().catch(() => {});
  }, []);

  // Only run the WS-driven activity hook while at least one companion is pinned.
  useCompanionActivity(pinned.length > 0);

  if (pinned.length === 0) return null;

  // The active agent's companion (if any) reacts; others stay idle. With no
  // active agent, every companion follows the global mood.
  const activeChar = activeAgentSlug ? pinned.find((c) => c.agent_slug === activeAgentSlug) : null;

  return (
    <div className="pointer-events-none">
      {pinned.map((c, i) => {
        const playMood: CompanionMood =
          mood === 'idle' ? 'idle' : activeChar ? (c.id === activeChar.id ? mood : 'idle') : mood;
        return <CompanionSprite key={c.id} character={c} index={i} mood={playMood} />;
      })}
    </div>
  );
}
