import { Film } from 'lucide-react';
import type { WidgetProps } from './WidgetRegistry';

function extractFilename(path: string): string {
  return path.split('/').pop() || path;
}

export function VideoPlayerWidget({ data }: WidgetProps) {
  const videoFile = (data.video_file || data.file_path || data.path || data.url || data.filename) as string | undefined;
  const toolId = data.__resolved_tool_id as string | undefined;
  const contentType = (data.content_type || 'video/mp4') as string;

  const filename = videoFile ? extractFilename(videoFile) : null;
  const videoUrl = toolId && filename
    ? `/api/v1/tools/${toolId}/proxy/video/${filename}`
    : null;

  if (!videoUrl) {
    return (
      <div className="rounded-lg border border-border-1 bg-surface-2/30 px-4 py-3 flex items-center gap-3">
        <div className="w-10 h-10 rounded-lg bg-purple-500/10 flex items-center justify-center flex-shrink-0">
          <Film className="w-5 h-5 text-purple-400" />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-text-1 truncate">{filename || 'Video file'}</p>
          <p className="text-xs text-text-3">{contentType}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-border-1 bg-surface-2/30 overflow-hidden">
      <video
        src={videoUrl}
        controls
        preload="metadata"
        className="w-full max-h-[400px] bg-black"
      >
        <source src={videoUrl} type={contentType} />
      </video>
      <div className="flex items-center gap-2 px-3 py-2 text-[10px] text-text-3">
        {filename && <span className="font-mono truncate">{filename}</span>}
      </div>
    </div>
  );
}
