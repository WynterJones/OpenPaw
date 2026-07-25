import { Outlet, useLocation } from 'react-router';
import { Sidebar } from './Sidebar';
import { BottomNav } from './BottomNav';
import { BackgroundImage } from './BackgroundImage';
import { ChatCompanions } from './companion/ChatCompanions';
import { ActiveChatsIndicator } from './ActiveChatsIndicator';
import { ViewTogglesProvider, useViewToggles } from '../contexts/ViewTogglesContext';

function LayoutInner() {
  const { sidebar, toggle } = useViewToggles();
  const location = useLocation();
  // Companions live only on the chat screen so they aren't a distraction elsewhere.
  const onChat = location.pathname === '/chat' || location.pathname.startsWith('/chat/');

  return (
    <div className="flex h-screen bg-surface-0 overflow-hidden relative">
      <BackgroundImage />
      <a href="#main-content" className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-[100] focus:px-4 focus:py-2 focus:rounded-lg focus:bg-accent-primary focus:text-white focus:text-sm focus:font-semibold">
        Skip to content
      </a>
      <Sidebar collapsed={!sidebar} onToggle={() => toggle('sidebar')} />
      <main id="main-content" className="flex-1 flex flex-col overflow-hidden pb-14 md:pb-0 relative z-[1]">
        <Outlet />
      </main>
      <BottomNav />
      {onChat && <ChatCompanions />}
      <ActiveChatsIndicator />
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
