import { useState, useRef, useEffect } from 'react';
import { Play, Pause, Volume2, VolumeX, RotateCcw, Music } from 'lucide-react';
import type { WidgetProps } from './WidgetRegistry';

function extractFilename(path: string): string {
  return path.split('/').pop() || path;
}

export function AudioPlayerWidget({ data }: WidgetProps) {
  const audioFile = data.audio_file as string | undefined;
  const filePath = data.file_path as string | undefined;
  const toolId = data.__resolved_tool_id as string | undefined;
  const voiceId = (data.voice_id || data.voice) as string | undefined;
  const text = data.text as string | undefined;
  const contentType = (data.content_type || 'audio/mpeg') as string;

  const audioRef = useRef<HTMLAudioElement>(null);
  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(false);
  const [progress, setProgress] = useState(0);
  const [duration, setDuration] = useState(0);
  const [error, setError] = useState<string | null>(null);

  // Resolve audio URL: prefer audio_file (new format), fall back to file_path (legacy)
  const filename = audioFile || (filePath ? extractFilename(filePath) : null);
  const audioUrl = toolId && filename
    ? `/api/v1/tools/${toolId}/proxy/audio/${filename}`
    : null;

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;

    const onTimeUpdate = () => setProgress(audio.currentTime);
    const onDuration = () => setDuration(audio.duration || 0);
    const onEnded = () => setPlaying(false);
    const onError = () => setError('Failed to load audio');

    audio.addEventListener('timeupdate', onTimeUpdate);
    audio.addEventListener('loadedmetadata', onDuration);
    audio.addEventListener('ended', onEnded);
    audio.addEventListener('error', onError);

    return () => {
      audio.removeEventListener('timeupdate', onTimeUpdate);
      audio.removeEventListener('loadedmetadata', onDuration);
      audio.removeEventListener('ended', onEnded);
      audio.removeEventListener('error', onError);
    };
  }, [audioUrl]);

  const togglePlay = () => {
    const audio = audioRef.current;
    if (!audio) return;
    if (playing) {
      audio.pause();
    } else {
      audio.play().catch(() => setError('Playback failed'));
    }
    setPlaying(!playing);
  };

  const toggleMute = () => {
    const audio = audioRef.current;
    if (!audio) return;
    audio.muted = !muted;
    setMuted(!muted);
  };

  const seek = (e: React.MouseEvent<HTMLDivElement>) => {
    const audio = audioRef.current;
    if (!audio || !duration) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const pct = (e.clientX - rect.left) / rect.width;
    audio.currentTime = pct * duration;
  };

  const restart = () => {
    const audio = audioRef.current;
    if (!audio) return;
    audio.currentTime = 0;
    audio.play().catch(() => setError('Playback failed'));
    setPlaying(true);
  };

  const formatTime = (s: number) => {
    if (!s || !isFinite(s)) return '0:00';
    const m = Math.floor(s / 60);
    const sec = Math.floor(s % 60);
    return `${m}:${sec.toString().padStart(2, '0')}`;
  };

  if (!audioUrl) {
    return (
      <div className="rounded-lg border border-border-1 bg-surface-2/30 px-4 py-3 flex items-center gap-3">
        <div className="w-10 h-10 rounded-lg bg-accent-primary/10 flex items-center justify-center flex-shrink-0">
          <Music className="w-5 h-5 text-accent-primary" />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-text-1 truncate">{filename || 'Audio file'}</p>
          <p className="text-xs text-text-3">
            {voiceId && <span className="mr-2">{voiceId}</span>}
            <span>{contentType}</span>
          </p>
          {text && <p className="text-xs text-text-2 italic mt-1 line-clamp-1">&ldquo;{text}&rdquo;</p>}
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-border-1 bg-surface-2/30 overflow-hidden">
      <audio ref={audioRef} src={audioUrl} preload="metadata">
        <source src={audioUrl} type={contentType} />
      </audio>

      {text && (
        <div className="px-4 pt-3 pb-1">
          <p className="text-xs text-text-2 italic line-clamp-2">&ldquo;{text}&rdquo;</p>
        </div>
      )}

      <div className="flex items-center gap-3 px-4 py-3">
        <button
          onClick={togglePlay}
          className="w-8 h-8 rounded-full bg-accent-primary/20 hover:bg-accent-primary/30 flex items-center justify-center transition-colors flex-shrink-0 cursor-pointer"
        >
          {playing
            ? <Pause className="w-4 h-4 text-accent-primary" />
            : <Play className="w-4 h-4 text-accent-primary ml-0.5" />
          }
        </button>

        <div className="flex-1 flex items-center gap-2">
          <span className="text-[10px] text-text-3 w-8 text-right font-mono">{formatTime(progress)}</span>
          <div
            className="flex-1 h-1.5 rounded-full bg-surface-3 cursor-pointer relative group"
            onClick={seek}
          >
            <div
              className="h-full rounded-full bg-accent-primary transition-all"
              style={{ width: duration ? `${(progress / duration) * 100}%` : '0%' }}
            />
            <div
              className="absolute top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-accent-primary opacity-0 group-hover:opacity-100 transition-opacity"
              style={{ left: duration ? `calc(${(progress / duration) * 100}% - 6px)` : '0' }}
            />
          </div>
          <span className="text-[10px] text-text-3 w-8 font-mono">{formatTime(duration)}</span>
        </div>

        <button
          onClick={restart}
          className="p-1.5 rounded hover:bg-surface-3 text-text-3 hover:text-text-1 transition-colors cursor-pointer"
          title="Restart"
        >
          <RotateCcw className="w-3.5 h-3.5" />
        </button>

        <button
          onClick={toggleMute}
          className="p-1.5 rounded hover:bg-surface-3 text-text-3 hover:text-text-1 transition-colors cursor-pointer"
          title={muted ? 'Unmute' : 'Mute'}
        >
          {muted ? <VolumeX className="w-3.5 h-3.5" /> : <Volume2 className="w-3.5 h-3.5" />}
        </button>
      </div>

      <div className="flex items-center gap-2 px-4 pb-2 text-[10px] text-text-3">
        {voiceId && <span className="px-1.5 py-0.5 rounded bg-surface-3">{voiceId}</span>}
        {filename && <span className="font-mono truncate">{filename}</span>}
        {error && <span className="text-red-400">{error}</span>}
      </div>
    </div>
  );
}
