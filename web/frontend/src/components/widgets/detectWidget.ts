/**
 * Shared widget auto-detection logic.
 * Mirrors the backend DetectWidgetType in internal/llm/widget.go.
 */
import { hasImageShape } from './imageUtils';

const AUDIO_EXTENSIONS = ['.mp3', '.wav', '.ogg', '.opus', '.aac', '.flac', '.m4a', '.wma'];
const VIDEO_EXTENSIONS = ['.mp4', '.webm', '.ogg', '.mov', '.avi', '.mkv', '.m4v'];
const FILE_EXTENSIONS = ['.pdf', '.doc', '.docx', '.xls', '.xlsx', '.csv', '.txt', '.json', '.xml', '.html', '.zip', '.tar', '.gz'];

function getExtension(path: string): string {
  const dot = path.lastIndexOf('.');
  if (dot === -1) return '';
  return path.slice(dot).toLowerCase().split(/[?#]/)[0];
}

function hasMediaContentType(data: Record<string, unknown>, prefix: string): boolean {
  const ct = data.content_type;
  return typeof ct === 'string' && ct.startsWith(prefix);
}

function findFilePath(data: Record<string, unknown>): string | null {
  for (const key of ['audio_file', 'file_path', 'path', 'url', 'file', 'filename', 'video_file', 'video_path', 'audio_path']) {
    const val = data[key];
    if (typeof val === 'string' && val.length > 0) return val;
  }
  return null;
}

export function detectBestWidget(data: Record<string, unknown>): string {
  const has = (key: string) => key in data;

  if (has('columns') && has('rows') && Array.isArray(data.columns) && Array.isArray(data.rows)) {
    return 'data-table';
  }

  if (has('label') && has('status')) {
    return 'status-card';
  }

  if (has('label') && has('value')) {
    return 'metric-card';
  }

  if (hasImageShape(data)) {
    return 'image';
  }

  // Audio detection: explicit audio_file field, or content_type audio/*, or file path with audio extension
  if (has('audio_file') && hasMediaContentType(data, 'audio/')) {
    return 'audio-player';
  }
  const filePath = findFilePath(data);
  if (filePath) {
    const ext = getExtension(filePath);
    if (AUDIO_EXTENSIONS.includes(ext) || hasMediaContentType(data, 'audio/')) {
      return 'audio-player';
    }
    if (VIDEO_EXTENSIONS.includes(ext) || hasMediaContentType(data, 'video/')) {
      return 'video-player';
    }
    if (ext === '.pdf') {
      return 'file-preview';
    }
    if (FILE_EXTENSIONS.includes(ext)) {
      return 'file-preview';
    }
  }

  const values = Object.values(data);
  const allScalar = values.length > 0 && values.every(v =>
    typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean' || v === null
  );
  if (allScalar) {
    return 'key-value';
  }

  return 'json-viewer';
}
