/**
 * Custom video and audio players for Studio.
 *
 * The native <video>/<audio> controls are drawn by the OS and ignore the app's
 * theme entirely — a chrome-blue scrubber in a pink-accented dark UI. These
 * replace them with controls built from the same design tokens as everything
 * else, and behave identically across platforms.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { Play, Pause, Volume2, VolumeX, Maximize2, Loader2 } from 'lucide-react';

/** mm:ss, or h:mm:ss once a clip runs past an hour. */
function formatTime(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
  const total = Math.floor(seconds);
  const s = total % 60;
  const m = Math.floor(total / 60) % 60;
  const h = Math.floor(total / 3600);
  const pad = (n: number) => String(n).padStart(2, '0');
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

/** Shared playback state for a media element, driven by its own events so it
 *  stays correct even when playback is changed elsewhere (keyboard, autoplay). */
function useMediaState(ref: React.RefObject<HTMLMediaElement | null>) {
  const [playing, setPlaying] = useState(false);
  const [time, setTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [muted, setMuted] = useState(false);
  const [waiting, setWaiting] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    const onPlay = () => setPlaying(true);
    const onPause = () => setPlaying(false);
    const onTime = () => setTime(el.currentTime);
    const onMeta = () => setDuration(el.duration);
    const onVolume = () => setMuted(el.muted);
    const onWaiting = () => setWaiting(true);
    const onPlaying = () => setWaiting(false);
    const onEnded = () => setPlaying(false);

    el.addEventListener('play', onPlay);
    el.addEventListener('pause', onPause);
    el.addEventListener('timeupdate', onTime);
    el.addEventListener('durationchange', onMeta);
    el.addEventListener('loadedmetadata', onMeta);
    el.addEventListener('volumechange', onVolume);
    el.addEventListener('waiting', onWaiting);
    el.addEventListener('playing', onPlaying);
    el.addEventListener('ended', onEnded);

    return () => {
      el.removeEventListener('play', onPlay);
      el.removeEventListener('pause', onPause);
      el.removeEventListener('timeupdate', onTime);
      el.removeEventListener('durationchange', onMeta);
      el.removeEventListener('loadedmetadata', onMeta);
      el.removeEventListener('volumechange', onVolume);
      el.removeEventListener('waiting', onWaiting);
      el.removeEventListener('playing', onPlaying);
      el.removeEventListener('ended', onEnded);
    };
  }, [ref]);

  const toggle = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    if (el.paused) {
      // Autoplay policies reject this when the gesture isn't trusted; the
      // element stays paused and the UI already reflects that.
      el.play().catch(() => {});
    } else {
      el.pause();
    }
  }, [ref]);

  const seek = useCallback(
    (fraction: number) => {
      const el = ref.current;
      if (!el || !Number.isFinite(el.duration)) return;
      el.currentTime = Math.min(el.duration, Math.max(0, fraction * el.duration));
      setTime(el.currentTime);
    },
    [ref],
  );

  const toggleMute = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    el.muted = !el.muted;
  }, [ref]);

  return { playing, time, duration, muted, waiting, toggle, seek, toggleMute };
}

/**
 * Scrubber — an accent-filled progress bar that seeks on click and on drag.
 *
 * Built from divs rather than <input type="range"> because range thumbs need
 * per-engine vendor pseudo-elements to restyle, and still differ subtly.
 */
function Scrubber({
  fraction,
  onSeek,
  ariaLabel,
}: {
  fraction: number;
  onSeek: (fraction: number) => void;
  ariaLabel: string;
}) {
  const trackRef = useRef<HTMLDivElement>(null);

  const seekTo = useCallback(
    (clientX: number) => {
      const rect = trackRef.current?.getBoundingClientRect();
      if (!rect || rect.width === 0) return;
      onSeek(Math.min(1, Math.max(0, (clientX - rect.left) / rect.width)));
    },
    [onSeek],
  );

  const onPointerDown = (e: React.PointerEvent) => {
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    seekTo(e.clientX);
  };

  const pct = `${Math.min(100, Math.max(0, fraction * 100))}%`;

  return (
    <div
      ref={trackRef}
      role="slider"
      aria-label={ariaLabel}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(fraction * 100)}
      tabIndex={0}
      onPointerDown={onPointerDown}
      onPointerMove={e => {
        // Only track the pointer while the button is held.
        if (e.buttons === 1) seekTo(e.clientX);
      }}
      onKeyDown={e => {
        if (e.key === 'ArrowRight') onSeek(Math.min(1, fraction + 0.05));
        if (e.key === 'ArrowLeft') onSeek(Math.max(0, fraction - 0.05));
      }}
      className="group/track relative flex-1 h-4 flex items-center cursor-pointer touch-none"
    >
      <div className="relative w-full h-1 rounded-full bg-white/20 overflow-hidden">
        <div
          className="absolute inset-y-0 left-0 rounded-full bg-accent-primary"
          style={{ width: pct }}
        />
      </div>
      {/* Handle stays hidden until hover so the resting bar reads as a slim
          progress indicator rather than a heavy control. */}
      <div
        className="absolute w-3 h-3 -ml-1.5 rounded-full bg-accent-primary shadow ring-2 ring-black/30 opacity-0 group-hover/track:opacity-100 transition-opacity"
        style={{ left: pct }}
      />
    </div>
  );
}

function IconButton({
  onClick,
  label,
  children,
}: {
  onClick: () => void;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      aria-label={label}
      title={label}
      className="p-1 rounded-md text-white/80 hover:text-white hover:bg-white/10 transition-colors cursor-pointer flex-shrink-0"
    >
      {children}
    </button>
  );
}

export function VideoPlayer({ src, poster }: { src: string; poster?: string }) {
  const ref = useRef<HTMLVideoElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const { playing, time, duration, muted, waiting, toggle, seek, toggleMute } = useMediaState(ref);

  const fullscreen = () => {
    wrapRef.current?.requestFullscreen?.().catch(() => {});
  };

  return (
    <div ref={wrapRef} className="group relative w-full h-full bg-black">
      <video
        ref={ref}
        src={src}
        poster={poster}
        preload="metadata"
        playsInline
        onClick={toggle}
        className="w-full h-full object-contain cursor-pointer"
      />

      {/* Centre affordance while paused, so a still frame doesn't look broken. */}
      {!playing && !waiting && (
        <button
          onClick={toggle}
          aria-label="Play"
          className="absolute inset-0 flex items-center justify-center cursor-pointer"
        >
          <span className="flex items-center justify-center w-12 h-12 rounded-full bg-accent-primary/90 text-white shadow-lg backdrop-blur-sm transition-transform hover:scale-105">
            <Play className="w-5 h-5 ml-0.5" aria-hidden="true" />
          </span>
        </button>
      )}

      {waiting && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <Loader2 className="w-6 h-6 text-white/80 animate-spin" aria-hidden="true" />
        </div>
      )}

      {/* Controls fade in on hover, and stay put while paused. */}
      <div
        className={`absolute inset-x-0 bottom-0 flex items-center gap-2 px-2.5 pb-2 pt-6 bg-gradient-to-t from-black/80 to-transparent transition-opacity ${
          playing ? 'opacity-0 group-hover:opacity-100' : 'opacity-100'
        }`}
      >
        <IconButton onClick={toggle} label={playing ? 'Pause' : 'Play'}>
          {playing ? <Pause className="w-4 h-4" /> : <Play className="w-4 h-4" />}
        </IconButton>

        <span className="text-[10px] tabular-nums text-white/80 flex-shrink-0">
          {formatTime(time)}
        </span>

        <Scrubber
          fraction={duration > 0 ? time / duration : 0}
          onSeek={seek}
          ariaLabel="Seek video"
        />

        <span className="text-[10px] tabular-nums text-white/60 flex-shrink-0">
          {formatTime(duration)}
        </span>

        <IconButton onClick={toggleMute} label={muted ? 'Unmute' : 'Mute'}>
          {muted ? <VolumeX className="w-4 h-4" /> : <Volume2 className="w-4 h-4" />}
        </IconButton>

        <IconButton onClick={fullscreen} label="Fullscreen">
          <Maximize2 className="w-4 h-4" />
        </IconButton>
      </div>
    </div>
  );
}

export function AudioPlayer({ src }: { src: string }) {
  const ref = useRef<HTMLAudioElement>(null);
  const { playing, time, duration, muted, waiting, toggle, seek, toggleMute } = useMediaState(ref);

  return (
    <div className="w-full rounded-xl border border-border-0 bg-surface-2 px-3 py-2.5">
      <audio ref={ref} src={src} preload="metadata" />

      <div className="flex items-center gap-2.5">
        <button
          onClick={toggle}
          aria-label={playing ? 'Pause' : 'Play'}
          className="flex items-center justify-center w-9 h-9 flex-shrink-0 rounded-full bg-accent-primary text-white shadow-sm transition-transform hover:scale-105 cursor-pointer"
        >
          {waiting ? (
            <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" />
          ) : playing ? (
            <Pause className="w-4 h-4" aria-hidden="true" />
          ) : (
            <Play className="w-4 h-4 ml-0.5" aria-hidden="true" />
          )}
        </button>

        <div className="flex-1 min-w-0">
          {/* Dark-on-light here, unlike the video overlay, so the track stays
              visible against the surface rather than against video frames. */}
          <div className="flex items-center gap-2">
            <Scrubber
              fraction={duration > 0 ? time / duration : 0}
              onSeek={seek}
              ariaLabel="Seek audio"
            />
            <IconButton onClick={toggleMute} label={muted ? 'Unmute' : 'Mute'}>
              {muted ? (
                <VolumeX className="w-3.5 h-3.5 text-text-3" />
              ) : (
                <Volume2 className="w-3.5 h-3.5 text-text-3" />
              )}
            </IconButton>
          </div>
          <div className="flex items-center justify-between mt-0.5">
            <span className="text-[10px] tabular-nums text-text-3">{formatTime(time)}</span>
            <span className="text-[10px] tabular-nums text-text-3">{formatTime(duration)}</span>
          </div>
        </div>
      </div>
    </div>
  );
}
