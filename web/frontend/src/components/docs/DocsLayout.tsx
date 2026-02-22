import { useState, useCallback } from 'react';
import { Outlet } from 'react-router';
import { DocsHeader } from './DocsHeader';
import { DocsSidebar } from './DocsSidebar';
import { DocsTableOfContents, type TocItem } from './DocsTableOfContents';

const SCROLL_CONTAINER_ID = 'docs-scroll-container';

export function DocsLayout() {
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [tocItems, setTocItems] = useState<TocItem[]>([]);

  const registerToc = useCallback((items: TocItem[]) => {
    setTocItems(items);
  }, []);

  return (
    <div className="flex flex-col h-screen bg-surface-0">
      <DocsHeader
        scrollContainerId={SCROLL_CONTAINER_ID}
        onMenuClick={() => setMobileNavOpen(true)}
      />

      <div className="flex flex-1 overflow-hidden">
        <DocsSidebar
          mobileOpen={mobileNavOpen}
          onClose={() => setMobileNavOpen(false)}
        />

        <div
          id={SCROLL_CONTAINER_ID}
          className="flex-1 overflow-y-auto"
        >
          <div className="flex max-w-7xl mx-auto">
            <article className="flex-1 min-w-0 px-6 lg:px-10 xl:px-16 py-8 pb-16">
              <div className="prose-docs">
                <Outlet context={{ registerToc }} />
              </div>
            </article>

            <DocsTableOfContents
              items={tocItems}
              scrollContainerId={SCROLL_CONTAINER_ID}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
