import { useState, useMemo } from 'react';
import { Download, X, ZoomIn } from 'lucide-react';
import type { WidgetProps } from './WidgetRegistry';
import { findBase64Field, guessImageMime, base64ToDataUri } from './imageUtils';

function extractFilename(path: string): string {
  return path.split('/').pop() || path;
}

export function ImageWidget({ data }: WidgetProps) {
  const [expanded, setExpanded] = useState(false);

  const filePath = (data.file_path || data.filename) as string | undefined;
  const toolId = data.__resolved_tool_id as string | undefined;
  const proxyFilename = filePath ? extractFilename(filePath) : null;
  const proxyUrl = toolId && proxyFilename
    ? `/api/v1/tools/${toolId}/proxy/files/${proxyFilename}`
    : null;

  const imageInfo = useMemo(() => {
    if (proxyUrl) return { src: proxyUrl, mime: '', key: 'file_path' };
    const field = findBase64Field(data);
    if (!field) return null;
    const mime = guessImageMime(data, field.value);
    const src = base64ToDataUri(field.value, mime);
    return { src, mime, key: field.key };
  }, [data, proxyUrl]);

  if (!imageInfo) return null;

  const format = typeof data.format === 'string' ? data.format.toUpperCase() : (imageInfo.mime ? imageInfo.mime.split('/')[1]?.toUpperCase() : 'PNG');
  const width = typeof data.width === 'number' ? data.width : null;
  const height = typeof data.height === 'number' ? data.height : null;

  function handleDownload() {
    if (!imageInfo) return;
    const a = document.createElement('a');
    a.href = imageInfo.src;
    a.download = proxyFilename || `image.${format?.toLowerCase() || 'png'}`;
    a.click();
  }

  return (
    <>
      <div className="rounded-lg border border-border-1 overflow-hidden bg-surface-1">
        <div className="relative group flex items-center justify-center p-2 bg-surface-2/30 min-h-[100px]">
          <img
            src={imageInfo.src}
            alt="Tool output"
            className="max-h-[400px] max-w-full object-contain rounded"
          />
          <button
            onClick={() => setExpanded(true)}
            className="absolute top-2 right-2 p-1.5 rounded-md bg-surface-1/80 text-text-2 opacity-0 group-hover:opacity-100 transition-opacity hover:bg-surface-1 hover:text-text-1"
          >
            <ZoomIn size={16} />
          </button>
        </div>
        <div className="flex items-center justify-between px-3 py-1.5 border-t border-border-1 bg-surface-1">
          <span className="text-xs text-text-3 font-mono">
            {format}
            {width && height ? ` \u00B7 ${width}\u00D7${height}` : ''}
          </span>
          <button
            onClick={handleDownload}
            className="flex items-center gap-1 text-xs text-text-3 hover:text-text-1 transition-colors"
          >
            <Download size={12} />
            Save
          </button>
        </div>
      </div>

      {expanded && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm"
          onClick={() => setExpanded(false)}
        >
          <button
            onClick={() => setExpanded(false)}
            className="absolute top-4 right-4 p-2 rounded-full bg-surface-1/80 text-text-1 hover:bg-surface-1"
          >
            <X size={20} />
          </button>
          <img
            src={imageInfo.src}
            alt="Tool output (expanded)"
            className="max-h-[90vh] max-w-[90vw] object-contain rounded-lg shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          />
        </div>
      )}
    </>
  );
}
