import type { WidgetProps } from './WidgetRegistry';

export function TextBlockWidget({ data }: WidgetProps) {
  const content = (data.content as string) || '';

  return (
    <div className="text-sm text-text-1 leading-relaxed whitespace-pre-wrap">
      {content}
    </div>
  );
}
