import { useRef, useEffect } from 'react';

interface CustomWidgetProps {
  toolId: string;
  data: Record<string, unknown>;
}

function getThemeVars(): Record<string, string> {
  const vars: Record<string, string> = {};
  const computed = getComputedStyle(document.documentElement);
  const props = Array.from(document.styleSheets)
    .flatMap(sheet => {
      try { return Array.from(sheet.cssRules); } catch (e) { console.warn('readCSSRules failed (cross-origin stylesheet):', e); return []; }
    })
    .flatMap(rule => {
      if (rule instanceof CSSStyleRule && rule.selectorText === ':root') {
        return Array.from(rule.style);
      }
      return [];
    })
    .filter(prop => prop.startsWith('--op-'));

  for (const prop of props) {
    vars[prop] = computed.getPropertyValue(prop).trim();
  }

  // Fallback: scan inline styles on :root
  const rootStyle = document.documentElement.style;
  for (let i = 0; i < rootStyle.length; i++) {
    const prop = rootStyle[i];
    if (prop.startsWith('--op-') && !vars[prop]) {
      vars[prop] = rootStyle.getPropertyValue(prop).trim();
    }
  }

  return vars;
}

export function CustomWidget({ toolId, data }: CustomWidgetProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);

  useEffect(() => {
    const iframe = iframeRef.current;
    if (!iframe) return;

    const themeVars = getThemeVars();

    const cssVarLines = Object.entries(themeVars)
      .map(([k, v]) => `      ${k}: ${v};`)
      .join('\n');

    const themeObj = Object.fromEntries(
      Object.entries(themeVars).map(([k, v]) => [
        k.replace(/^--op-/, '').replace(/-/g, '_'),
        v,
      ])
    );

    const html = `<!DOCTYPE html>
<html>
<head>
  <style>
    :root {
${cssVarLines}
    }
    body {
      margin: 0;
      padding: 8px;
      font-family: system-ui, sans-serif;
      color: var(--op-text-1, #e0e0e0);
      background: var(--op-surface-1, transparent);
    }
    * { box-sizing: border-box; }
  </style>
</head>
<body>
  <div id="widget-root"></div>
  <script>
    window.WIDGET_DATA = ${JSON.stringify(data)};
    window.WIDGET_THEME = ${JSON.stringify(themeObj)};

    // Auto-resize: post height changes to parent
    (function() {
      var lastHeight = 0;
      var ro = new ResizeObserver(function(entries) {
        var h = Math.ceil(entries[0].contentRect.height) + 16;
        if (h !== lastHeight) {
          lastHeight = h;
          parent.postMessage({ type: 'widget-resize', height: h }, '*');
        }
      });
      ro.observe(document.body);
    })();
  </script>
  <script src="/api/v1/tools/${toolId}/widget.js"></script>
</body>
</html>`;

    iframe.srcdoc = html;
  }, [toolId, data]);

  useEffect(() => {
    const handler = (e: MessageEvent) => {
      if (e.data?.type === 'widget-resize' && typeof e.data.height === 'number') {
        const iframe = iframeRef.current;
        if (iframe) {
          iframe.style.height = `${Math.min(e.data.height, 600)}px`;
        }
      }
    };
    window.addEventListener('message', handler);
    return () => window.removeEventListener('message', handler);
  }, []);

  return (
    <iframe
      ref={iframeRef}
      sandbox="allow-scripts"
      className="w-full border border-border-1 rounded-lg bg-transparent"
      style={{ minHeight: 100, maxHeight: 600 }}
      title="Custom Widget"
    />
  );
}
