import { useState, useRef, useCallback, type MouseEvent } from 'react';

interface UseDragReorderOptions<T> {
  items: T[];
  getId: (item: T) => string;
  onReorder: (items: T[]) => void;
  direction?: 'horizontal' | 'vertical';
}

interface DragState {
  dragId: string | null;
  overId: string | null;
}

export function useDragReorder<T>({ items, getId, onReorder, direction = 'horizontal' }: UseDragReorderOptions<T>) {
  const [dragState, setDragState] = useState<DragState>({ dragId: null, overId: null });
  const dragStartPos = useRef({ x: 0, y: 0 });
  const isDragging = useRef(false);
  const dragId = useRef<string | null>(null);

  const handleDragStart = useCallback((id: string, e: MouseEvent) => {
    // Only left mouse button
    if (e.button !== 0) return;
    dragStartPos.current = { x: e.clientX, y: e.clientY };
    dragId.current = id;

    const handleMouseMove = (moveEvent: globalThis.MouseEvent) => {
      const dx = moveEvent.clientX - dragStartPos.current.x;
      const dy = moveEvent.clientY - dragStartPos.current.y;
      const distance = Math.sqrt(dx * dx + dy * dy);

      // Only start drag after 5px movement to avoid interfering with clicks
      if (!isDragging.current && distance > 5) {
        isDragging.current = true;
        setDragState({ dragId: dragId.current, overId: null });
      }

      if (isDragging.current) {
        // Find the element we're hovering over
        const elements = document.querySelectorAll(`[data-drag-id]`);
        let foundId: string | null = null;
        for (const el of elements) {
          const rect = el.getBoundingClientRect();
          const isOver = direction === 'horizontal'
            ? moveEvent.clientX >= rect.left && moveEvent.clientX <= rect.right
            : moveEvent.clientY >= rect.top && moveEvent.clientY <= rect.bottom;
          if (isOver) {
            foundId = (el as HTMLElement).dataset.dragId || null;
            break;
          }
        }
        if (foundId && foundId !== dragId.current) {
          setDragState(prev => ({ ...prev, overId: foundId }));
        }
      }
    };

    const handleMouseUp = () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);

      if (isDragging.current && dragId.current) {
        setDragState(prev => {
          if (prev.dragId && prev.overId && prev.dragId !== prev.overId) {
            const fromIdx = items.findIndex(item => getId(item) === prev.dragId);
            const toIdx = items.findIndex(item => getId(item) === prev.overId);
            if (fromIdx !== -1 && toIdx !== -1) {
              const reordered = [...items];
              const [moved] = reordered.splice(fromIdx, 1);
              reordered.splice(toIdx, 0, moved);
              onReorder(reordered);
            }
          }
          return { dragId: null, overId: null };
        });
      } else {
        setDragState({ dragId: null, overId: null });
      }

      isDragging.current = false;
      dragId.current = null;
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  }, [items, getId, onReorder, direction]);

  return {
    dragId: dragState.dragId,
    overId: dragState.overId,
    handleDragStart,
    isDragging: dragState.dragId !== null,
  };
}
