/**
 * Shared widget auto-detection logic.
 * Mirrors the backend DetectWidgetType in internal/llm/widget.go.
 */
import { hasImageShape } from './imageUtils';

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

  const values = Object.values(data);
  const allScalar = values.length > 0 && values.every(v =>
    typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean' || v === null
  );
  if (allScalar) {
    return 'key-value';
  }

  return 'json-viewer';
}
