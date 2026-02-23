import { useCallback, useRef } from 'react';

interface SplitDividerProps {
  direction: 'horizontal' | 'vertical';
  onDrag: (delta: number) => void;
}

export function SplitDivider({ direction, onDrag }: SplitDividerProps) {
  const startPos = useRef(0);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      startPos.current = direction === 'horizontal' ? e.clientX : e.clientY;

      // Prevent text selection during drag
      document.body.style.userSelect = 'none';
      document.body.style.cursor =
        direction === 'horizontal' ? 'col-resize' : 'row-resize';

      const handleMouseMove = (moveEvent: MouseEvent) => {
        const current =
          direction === 'horizontal' ? moveEvent.clientX : moveEvent.clientY;
        const delta = current - startPos.current;
        startPos.current = current;
        onDrag(delta);
      };

      const handleMouseUp = () => {
        document.removeEventListener('mousemove', handleMouseMove);
        document.removeEventListener('mouseup', handleMouseUp);
        document.body.style.userSelect = '';
        document.body.style.cursor = '';
      };

      document.addEventListener('mousemove', handleMouseMove);
      document.addEventListener('mouseup', handleMouseUp);
    },
    [direction, onDrag],
  );

  const isHorizontal = direction === 'horizontal';

  return (
    <div
      className={`flex-shrink-0 bg-border-0 hover:bg-accent-primary transition-colors ${
        isHorizontal ? 'cursor-col-resize' : 'cursor-row-resize'
      }`}
      style={{
        width: isHorizontal ? 4 : '100%',
        height: isHorizontal ? '100%' : 4,
      }}
      onMouseDown={handleMouseDown}
      role="separator"
      aria-orientation={isHorizontal ? 'vertical' : 'horizontal'}
    />
  );
}
