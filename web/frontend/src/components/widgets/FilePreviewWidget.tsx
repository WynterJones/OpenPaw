import { FileText, FileSpreadsheet, FileCode, FileArchive, File, ExternalLink } from 'lucide-react';
import type { WidgetProps } from './WidgetRegistry';

function extractFilename(path: string): string {
  return path.split('/').pop() || path;
}

function getExtension(path: string): string {
  const dot = path.lastIndexOf('.');
  if (dot === -1) return '';
  return path.slice(dot).toLowerCase();
}

function getFileIcon(ext: string) {
  switch (ext) {
    case '.pdf':
      return { icon: FileText, color: 'text-red-400', bg: 'bg-red-500/10' };
    case '.doc':
    case '.docx':
    case '.txt':
    case '.rtf':
      return { icon: FileText, color: 'text-blue-400', bg: 'bg-blue-500/10' };
    case '.xls':
    case '.xlsx':
    case '.csv':
      return { icon: FileSpreadsheet, color: 'text-emerald-400', bg: 'bg-emerald-500/10' };
    case '.json':
    case '.xml':
    case '.html':
    case '.js':
    case '.ts':
    case '.py':
    case '.go':
      return { icon: FileCode, color: 'text-amber-400', bg: 'bg-amber-500/10' };
    case '.zip':
    case '.tar':
    case '.gz':
    case '.rar':
    case '.7z':
      return { icon: FileArchive, color: 'text-purple-400', bg: 'bg-purple-500/10' };
    default:
      return { icon: File, color: 'text-text-2', bg: 'bg-surface-3' };
  }
}

function formatSize(bytes: number | undefined): string | null {
  if (!bytes || typeof bytes !== 'number') return null;
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function FilePreviewWidget({ data }: WidgetProps) {
  const filePath = (data.file_path || data.path || data.url || data.filename || data.file) as string | undefined;
  const toolId = data.__resolved_tool_id as string | undefined;
  const contentType = data.content_type as string | undefined;
  const fileSize = data.size as number | undefined;

  const filename = filePath ? extractFilename(filePath) : 'Unknown file';
  const ext = getExtension(filename);
  const { icon: Icon, color, bg } = getFileIcon(ext);
  const sizeStr = formatSize(fileSize);

  const isPdf = ext === '.pdf';
  const proxyUrl = toolId && filename
    ? `/api/v1/tools/${toolId}/proxy/files/${filename}`
    : null;

  return (
    <div className="rounded-lg border border-border-1 bg-surface-2/30 overflow-hidden">
      {isPdf && proxyUrl && (
        <div className="border-b border-border-1 bg-surface-1">
          <iframe
            src={proxyUrl}
            className="w-full h-[300px]"
            title={filename}
          />
        </div>
      )}

      <div className="flex items-center gap-3 px-4 py-3">
        <div className={`w-10 h-10 rounded-lg ${bg} flex items-center justify-center flex-shrink-0`}>
          <Icon className={`w-5 h-5 ${color}`} />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-text-1 truncate">{filename}</p>
          <div className="flex items-center gap-2 text-xs text-text-3">
            <span className="uppercase">{ext.replace('.', '')}</span>
            {contentType && <span>{contentType}</span>}
            {sizeStr && <span>{sizeStr}</span>}
          </div>
        </div>
        {proxyUrl && (
          <a
            href={proxyUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="p-2 rounded-lg hover:bg-surface-3 text-text-3 hover:text-text-1 transition-colors"
            title="Open file"
          >
            <ExternalLink className="w-4 h-4" />
          </a>
        )}
      </div>
    </div>
  );
}
