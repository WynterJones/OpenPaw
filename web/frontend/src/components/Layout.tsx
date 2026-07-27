import { Outlet, useLocation } from 'react-router';
import { Sidebar } from './Sidebar';
import { BottomNav } from './BottomNav';
import { BackgroundImage } from './BackgroundImage';
import { ChatCompanions } from './companion/ChatCompanions';
import { ActivityDock } from './ActivityDock';
import { ViewTogglesProvider } from '../contexts/ViewTogglesContext';
import { useViewToggles } from '../contexts/viewToggles';
import { useAppViewport } from '../hooks/useAppViewport';

function LayoutInner() {
  const { sidebar, toggle } = useViewToggles();
  const location = useLocation();
  useAppViewport();
  // Companions live only on the chat screen so they aren't a distraction elsewhere.
  const onChat = location.pathname === '/chat' || location.pathname.startsWith('/chat/');

  return (
    <div className="flex op-app-shell bg-surface-0 overflow-hidden relative">
      <BackgroundImage />
      <a href="#main-content" className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-[100] focus:px-4 focus:py-2 focus:rounded-lg focus:bg-accent-primary focus:text-white focus:text-sm focus:font-semibold">
        Skip to content
      </a>
      <Sidebar collapsed={!sidebar} onToggle={() => toggle('sidebar')} />
      <main
        id="main-content"
        className="flex-1 flex flex-col overflow-hidden op-bottom-nav-space relative z-[1] min-w-0"
      >
        <Outlet />
      </main>
      <BottomNav />
      {onChat && <ChatCompanions />}
      <ActivityDock />
    </div>
  );
}

export function Layout() {
  return (
    <ViewTogglesProvider>
      <LayoutInner />
    </ViewTogglesProvider>
  );
}
