import { useState, useRef, useEffect } from 'react';
import { SmilePlus } from 'lucide-react';

const EMOJI_SET = [
  '👍', '❤️', '😂', '🎉', '🔥',
  '👀', '💯', '🙌', '✅', '👏',
  '🤔', '😍', '💪', '🚀', '⭐',
  '😊', '🙏', '💡', '🤝', '👋',
];

export function EmojiPicker({ onSelect }: { onSelect: (emoji: string) => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [open]);

  return (
    <div ref={ref} className="relative inline-flex">
      <button
        onClick={() => setOpen(!open)}
        className="p-1 rounded hover:bg-surface-2 text-text-3 hover:text-text-1 transition-colors cursor-pointer"
        title="Add reaction"
      >
        <SmilePlus className="w-3.5 h-3.5" />
      </button>
      {open && (
        <div className="absolute bottom-full mb-1 left-0 z-50 bg-surface-1 border border-border-1 rounded-lg shadow-lg p-1.5 grid grid-cols-5 gap-0.5 w-[180px]">
          {EMOJI_SET.map(emoji => (
            <button
              key={emoji}
              onClick={() => { onSelect(emoji); setOpen(false); }}
              className="w-8 h-8 flex items-center justify-center rounded hover:bg-surface-2 text-base cursor-pointer transition-colors"
            >
              {emoji}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
