/**
 * ReferenceImages — attach images for a model to work from.
 *
 * Accepts paste, drop, and a file picker. Files are read to data URIs in the
 * browser rather than uploaded first: the generate request has to carry the
 * bytes to the provider anyway, so a separate upload round trip would only add
 * a file to clean up later.
 *
 * The slot count comes from the selected model, not a constant — OpenRouter's
 * chat-shaped image models take several references, while Replicate and fal
 * take exactly one.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { ImagePlus, X } from 'lucide-react';

/** Guard against a huge PNG becoming a multi-megabyte base64 request body. */
const MAX_FILE_BYTES = 8 << 20;

export interface ReferenceImage {
  id: string;
  name: string;
  /** data: URI — sent to the API as-is. */
  src: string;
}

interface Props {
  images: ReferenceImage[];
  max: number;
  onChange: (images: ReferenceImage[]) => void;
  onError: (message: string) => void;
}

export function ReferenceImages({ images, max, onChange, onError }: Props) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const zoneRef = useRef<HTMLDivElement>(null);

  const addFiles = useCallback(
    async (files: File[]) => {
      const room = max - images.length;
      if (room <= 0) {
        onError(`This model takes at most ${max} reference image${max === 1 ? '' : 's'}`);
        return;
      }

      const usable = files.filter(f => f.type.startsWith('image/')).slice(0, room);
      if (usable.length === 0) return;

      const read = await Promise.all(
        usable.map(
          file =>
            new Promise<ReferenceImage | null>(resolve => {
              if (file.size > MAX_FILE_BYTES) {
                onError(`${file.name || 'Image'} is larger than ${MAX_FILE_BYTES >> 20}MB`);
                resolve(null);
                return;
              }
              const reader = new FileReader();
              reader.onload = () =>
                resolve({
                  id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
                  name: file.name || 'pasted image',
                  src: String(reader.result),
                });
              reader.onerror = () => {
                onError(`Could not read ${file.name || 'that image'}`);
                resolve(null);
              };
              reader.readAsDataURL(file);
            }),
        ),
      );

      const added = read.filter((r): r is ReferenceImage => r !== null);
      if (added.length > 0) onChange([...images, ...added]);
    },
    [images, max, onChange, onError],
  );

  // Paste is bound to the panel rather than a single input so an image can be
  // pasted while the prompt box has focus, which is where the cursor usually is.
  useEffect(() => {
    const zone = zoneRef.current?.closest('[data-studio-editor]');
    if (!zone) return;

    const onPaste = (e: Event) => {
      const items = (e as ClipboardEvent).clipboardData?.items;
      if (!items) return;
      const files: File[] = [];
      for (const item of items) {
        if (item.kind === 'file' && item.type.startsWith('image/')) {
          const f = item.getAsFile();
          if (f) files.push(f);
        }
      }
      if (files.length > 0) {
        e.preventDefault();
        addFiles(files);
      }
    };

    zone.addEventListener('paste', onPaste);
    return () => zone.removeEventListener('paste', onPaste);
  }, [addFiles]);

  const full = images.length >= max;

  return (
    <div className="space-y-1.5" ref={zoneRef}>
      <div className="flex items-center justify-between">
        <label className="block text-sm font-medium text-text-1">Reference images</label>
        <span className="text-[11px] text-text-3">
          {images.length}/{max}
        </span>
      </div>

      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        multiple={max > 1}
        className="hidden"
        onChange={e => {
          addFiles(Array.from(e.target.files ?? []));
          e.target.value = '';
        }}
      />

      {images.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {images.map(img => (
            <div
              key={img.id}
              className="relative group w-14 h-14 rounded-lg overflow-hidden border border-border-1 bg-surface-0"
            >
              <img src={img.src} alt={img.name} className="w-full h-full object-cover" />
              <button
                onClick={() => onChange(images.filter(i => i.id !== img.id))}
                aria-label={`Remove ${img.name}`}
                className="absolute top-0.5 right-0.5 p-0.5 rounded bg-black/60 text-white opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
              >
                <X className="w-3 h-3" aria-hidden="true" />
              </button>
            </div>
          ))}
        </div>
      )}

      {!full && (
        <button
          onClick={() => fileRef.current?.click()}
          onDragOver={e => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={e => {
            e.preventDefault();
            setDragging(false);
            addFiles(Array.from(e.dataTransfer.files ?? []));
          }}
          className={`w-full flex items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-2.5 text-[11px] transition-colors cursor-pointer ${
            dragging
              ? 'border-accent-primary bg-accent-primary/10 text-accent-text'
              : 'border-border-1 text-text-3 hover:border-accent-primary/50 hover:text-text-2'
          }`}
        >
          <ImagePlus className="w-3.5 h-3.5" aria-hidden="true" />
          Paste, drop or click to add
        </button>
      )}
    </div>
  );
}
