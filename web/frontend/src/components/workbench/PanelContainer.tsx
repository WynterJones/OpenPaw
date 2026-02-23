import { useCallback } from 'react';
import { useWorkbench, type PanelNode } from './WorkbenchProvider';
import { Panel } from './Panel';
import { SplitDivider } from './SplitDivider';

interface PanelContainerProps {
  node: PanelNode;
}

export function PanelContainer({ node }: PanelContainerProps) {
  const { updatePanelSizes } = useWorkbench();

  if (node.type === 'leaf') {
    return <Panel node={node} />;
  }

  // Split panel — render children with dividers between them
  const children = node.children || [];
  const sizes = node.sizes || children.map(() => 1);
  const isHorizontal = node.direction === 'horizontal';

  return (
    <div
      className={`flex h-full w-full min-h-0 min-w-0 ${
        isHorizontal ? 'flex-row' : 'flex-col'
      }`}
    >
      {children.map((child, index) => (
        <SplitChild
          key={child.id}
          child={child}
          index={index}
          sizes={sizes}
          isLast={index === children.length - 1}
          direction={node.direction!}
          panelId={node.id}
          onDrag={updatePanelSizes}
        />
      ))}
    </div>
  );
}

// ── Split child wrapper with optional divider ──

interface SplitChildProps {
  child: PanelNode;
  index: number;
  sizes: number[];
  isLast: boolean;
  direction: 'horizontal' | 'vertical';
  panelId: string;
  onDrag: (panelId: string, sizes: number[]) => void;
}

function SplitChild({
  child,
  index,
  sizes,
  isLast,
  direction,
  panelId,
  onDrag,
}: SplitChildProps) {
  const handleDrag = useCallback(
    (delta: number) => {
      // Convert pixel delta to ratio delta based on total sizes
      const totalRatio = sizes.reduce((a, b) => a + b, 0);
      // We need the container size — approximate by using a fraction of the ratio
      // The delta is in pixels; we scale by estimating total pixels
      const containerSize =
        direction === 'horizontal'
          ? document.body.clientWidth
          : document.body.clientHeight;
      const ratioDelta = (delta / containerSize) * totalRatio;

      const newSizes = [...sizes];
      const minSize = 0.05 * totalRatio; // minimum 5% of total
      const newPrev = newSizes[index] + ratioDelta;
      const newNext = newSizes[index + 1] - ratioDelta;

      if (newPrev >= minSize && newNext >= minSize) {
        newSizes[index] = newPrev;
        newSizes[index + 1] = newNext;
        onDrag(panelId, newSizes);
      }
    },
    [sizes, index, direction, panelId, onDrag],
  );

  return (
    <>
      <div
        className="min-h-0 min-w-0 overflow-hidden"
        style={{ flex: sizes[index] ?? 1 }}
      >
        <PanelContainer node={child} />
      </div>
      {!isLast && <SplitDivider direction={direction} onDrag={handleDrag} />}
    </>
  );
}
