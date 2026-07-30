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
  /** Remove transparent canvas padding while keeping the source file intact. */
  autoCrop?: boolean;
}

const cropScaleCache = new Map<string, number>();
const cropScalePromises = new Map<string, Promise<number>>();
const MAX_CROP_SCALE = 2.15;
const CROP_SAFETY = 0.92;

function measureFrameScale(src: string): Promise<number> {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => {
      const canvas = document.createElement('canvas');
      canvas.width = img.naturalWidth;
      canvas.height = img.naturalHeight;
      const context = canvas.getContext('2d', { willReadFrequently: true });
      if (!context || canvas.width === 0 || canvas.height === 0) {
        resolve(1);
        return;
      }
      context.drawImage(img, 0, 0);

      try {
        const pixels = context.getImageData(0, 0, canvas.width, canvas.height).data;
        let minX = canvas.width;
        let minY = canvas.height;
        let maxX = -1;
        let maxY = -1;
        for (let y = 0; y < canvas.height; y += 1) {
          for (let x = 0; x < canvas.width; x += 1) {
            if (pixels[(y * canvas.width + x) * 4 + 3] === 0) continue;
            minX = Math.min(minX, x);
            minY = Math.min(minY, y);
            maxX = Math.max(maxX, x);
            maxY = Math.max(maxY, y);
          }
        }
        if (maxX < minX || maxY < minY) {
          resolve(1);
          return;
        }

        // CSS scaling happens around the image centre. Measure the farthest
        // visible pixel from that centre so off-centre motion is not clipped.
        const halfWidth = canvas.width / 2;
        const halfHeight = canvas.height / 2;
        const horizontalReach = Math.max(Math.abs(minX - halfWidth), Math.abs(maxX + 1 - halfWidth));
        const verticalReach = Math.max(Math.abs(minY - halfHeight), Math.abs(maxY + 1 - halfHeight));
        const safeScale = Math.min(
          halfWidth / Math.max(1, horizontalReach),
          halfHeight / Math.max(1, verticalReach),
        ) * CROP_SAFETY;
        resolve(Math.max(1, Math.min(MAX_CROP_SCALE, safeScale)));
      } catch {
        resolve(1);
      }
    };
    img.onerror = () => resolve(1);
    img.src = src;
  });
}

function getCropScale(frames: string[]): Promise<number> {
  const key = frames.join('\n');
  const cached = cropScaleCache.get(key);
  if (cached !== undefined) return Promise.resolve(cached);

  const pending = cropScalePromises.get(key);
  if (pending) return pending;

  const promise = Promise.all(frames.map(measureFrameScale))
    .then((scales) => {
      const scale = Math.min(...scales);
      cropScaleCache.set(key, scale);
      cropScalePromises.delete(key);
      return scale;
    });
  cropScalePromises.set(key, promise);
  return promise;
}

export function SpriteAnimation({
  frames,
  fps = 6,
  size = 64,
  className,
  alt = '',
  paused = false,
  autoCrop = false,
}: SpriteAnimationProps) {
  const [index, setIndex] = useState(0);
  const [cropScale, setCropScale] = useState(1);
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

  useEffect(() => {
    let cancelled = false;
    if (!autoCrop) return;
    getCropScale(frames).then((scale) => {
      if (!cancelled) setCropScale(scale);
    });
    return () => {
      cancelled = true;
    };
  }, [autoCrop, frames]);

  if (frames.length === 0) return null;

  const src = frames[Math.min(index, frames.length - 1)];

  return (
    <span
      className={`inline-flex flex-shrink-0 items-center justify-center overflow-hidden ${className || ''}`}
      style={{ width: size, height: size }}
    >
      <img
        src={src}
        alt={alt}
        width={size}
        height={size}
        draggable={false}
        style={{
          width: size,
          height: size,
          imageRendering: 'pixelated',
          objectFit: 'contain',
          transform: `scale(${autoCrop ? cropScale : 1})`,
          transformOrigin: 'center',
        }}
      />
    </span>
  );
}
