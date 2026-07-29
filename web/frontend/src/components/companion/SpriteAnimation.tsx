/**
 * SpriteAnimation
 *
 * Loops an array of PNG frame URLs at a given fps. Pixel-art is rendered crisp
 * (no smoothing). A single static frame renders without a timer.
 */

import { useEffect, useState } from 'react';

interface SpriteAnimationProps {
  frames: string[];
  fps?: number;
  /** Rendered size in px (square). */
  size?: number;
  className?: string;
  alt?: string;
  paused?: boolean;
}

export function SpriteAnimation({
  frames,
  fps = 6,
  size = 64,
  className,
  alt = '',
  paused = false,
}: SpriteAnimationProps) {
  const [index, setIndex] = useState(0);
  const [reduceMotion, setReduceMotion] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  );

  useEffect(() => {
    const query = window.matchMedia('(prefers-reduced-motion: reduce)');
    const update = (event: MediaQueryListEvent) => setReduceMotion(event.matches);
    query.addEventListener('change', update);
    return () => query.removeEventListener('change', update);
  }, []);

  useEffect(() => {
    if (paused || reduceMotion || frames.length <= 1 || fps <= 0) return;
    const interval = setInterval(() => setIndex((i) => (i + 1) % frames.length), 1000 / fps);
    return () => clearInterval(interval);
  }, [frames, fps, paused, reduceMotion]);

  if (frames.length === 0) return null;

  const src = frames[Math.min(index, frames.length - 1)];

  return (
    <img
      src={src}
      alt={alt}
      width={size}
      height={size}
      draggable={false}
      style={{ width: size, height: size, imageRendering: 'pixelated', objectFit: 'contain' }}
      className={className}
    />
  );
}
