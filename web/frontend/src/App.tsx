import { lazy, Suspense, useEffect, type ReactNode } from 'react';
import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router';
import { AuthProvider, useAuth } from './contexts/AuthContext';
import { DesignProvider } from './contexts/DesignContext';
import { ToastProvider } from './components/Toast';
import { Layout } from './components/Layout';
import { LoadingSpinner } from './components/LoadingSpinner';

const Login = lazy(() => import('./pages/Login').then(m => ({ default: m.Login })));
const Setup = lazy(() => import('./pages/Setup').then(m => ({ default: m.Setup })));
const Chat = lazy(() => import('./pages/Chat').then(m => ({ default: m.Chat })));
const Tools = lazy(() => import('./pages/Tools').then(m => ({ default: m.Tools })));
const Secrets = lazy(() => import('./pages/Secrets').then(m => ({ default: m.Secrets })));
const Dashboards = lazy(() => import('./pages/Dashboards').then(m => ({ default: m.Dashboards })));
const Scheduler = lazy(() => import('./pages/Scheduler').then(m => ({ default: m.Scheduler })));
const Logs = lazy(() => import('./pages/Logs').then(m => ({ default: m.Logs })));
const Agents = lazy(() => import('./pages/Agents').then(m => ({ default: m.Agents })));
const AgentEdit = lazy(() => import('./pages/AgentEdit').then(m => ({ default: m.AgentEdit })));
const GatewayEdit = lazy(() => import('./pages/GatewayEdit').then(m => ({ default: m.GatewayEdit })));
const Skills = lazy(() => import('./pages/Skills').then(m => ({ default: m.Skills })));
const Settings = lazy(() => import('./pages/Settings').then(m => ({ default: m.Settings })));
const Context = lazy(() => import('./pages/Context').then(m => ({ default: m.Context })));
const Browser = lazy(() => import('./pages/Browser').then(m => ({ default: m.Browser })));
const Workbench = lazy(() => import('./pages/Workbench').then(m => ({ default: m.Workbench })));
const HeartbeatMonitor = lazy(() => import('./pages/HeartbeatMonitor').then(m => ({ default: m.HeartbeatMonitor })));
const Library = lazy(() => import('./pages/Library').then(m => ({ default: m.Library })));
const TodoLists = lazy(() => import('./pages/TodoLists').then(m => ({ default: m.TodoLists })));

const DocsLayout = lazy(() => import('./components/docs/DocsLayout').then(m => ({ default: m.DocsLayout })));
const DocsHome = lazy(() => import('./pages/docs/DocsHome').then(m => ({ default: m.DocsHome })));
const GetStarted = lazy(() => import('./pages/docs/GetStarted').then(m => ({ default: m.GetStarted })));
const HowItWorks = lazy(() => import('./pages/docs/HowItWorks').then(m => ({ default: m.HowItWorks })));
const Features = lazy(() => import('./pages/docs/Features').then(m => ({ default: m.Features })));
const DesktopApp = lazy(() => import('./pages/docs/DesktopApp').then(m => ({ default: m.DesktopApp })));
const UseCases = lazy(() => import('./pages/docs/UseCases').then(m => ({ default: m.UseCases })));
const Architecture = lazy(() => import('./pages/docs/Architecture').then(m => ({ default: m.Architecture })));

const pageTitles: Record<string, string> = {
  '/chat': 'Chat',
  '/tools': 'Tools',
  '/agents': 'Agents',
  '/agents/gateway': 'Gateway',
  '/skills': 'Skills',
  '/secrets': 'Secrets',
  '/dashboards': 'Dashboard',
  '/scheduler': 'Scheduler',
  '/logs': 'Logs',
  '/context': 'Context',
  '/browser': 'Browser',
  '/workbench': 'Workbench',
  '/heartbeat': 'Heartbeat',
  '/library': 'Library',
  '/todo-lists': 'Todo Lists',
  '/settings': 'Settings',
  '/login': 'Login',
  '/setup': 'Setup',
  '/docs': 'Docs',
  '/docs/get-started': 'Get Started',
  '/docs/how-it-works': 'How It Works',
  '/docs/features': 'Features',
  '/docs/desktop': 'Desktop App',
  '/docs/use-cases': 'Use Cases',
  '/docs/architecture': 'Architecture',
};

function PageTitle() {
  const { pathname } = useLocation();

  useEffect(() => {
    const title = pageTitles[pathname]
      ?? pageTitles[pathname.replace(/\/[^/]+$/, '')]
      ?? 'OpenPaw';
    document.title = title === 'OpenPaw' ? title : `${title} ~ OpenPaw`;
  }, [pathname]);

  return null;
}

function ProtectedRoute({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return <LoadingSpinner fullPage message="Loading..." />;
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

function AppRoutes() {
  return (
    <Suspense fallback={<LoadingSpinner fullPage message="Loading..." />}>
    <Routes>
      <Route path="/setup" element={<Setup />} />
      <Route path="/login" element={<Login />} />
      <Route path="/docs" element={<DocsLayout />}>
        <Route index element={<DocsHome />} />
        <Route path="get-started" element={<GetStarted />} />
        <Route path="how-it-works" element={<HowItWorks />} />
        <Route path="features" element={<Features />} />
        <Route path="desktop" element={<DesktopApp />} />
        <Route path="use-cases" element={<UseCases />} />
        <Route path="architecture" element={<Architecture />} />
      </Route>
      <Route
        element={
          <ProtectedRoute>
            <Layout />
          </ProtectedRoute>
        }
      >
        <Route path="/chat" element={<Chat />} />
        <Route path="/chat/:threadId" element={<Chat />} />
        <Route path="/tools" element={<Tools />} />
        <Route path="/agents" element={<Agents />} />
        <Route path="/agents/gateway" element={<GatewayEdit />} />
        <Route path="/agents/:slug" element={<AgentEdit />} />
        <Route path="/skills" element={<Skills />} />
        <Route path="/secrets" element={<Secrets />} />
        <Route path="/dashboards" element={<Dashboards />} />
        <Route path="/scheduler" element={<Scheduler />} />
        <Route path="/logs" element={<Logs />} />
        <Route path="/context" element={<Context />} />
        <Route path="/browser" element={<Browser />} />
        <Route path="/workbench" element={<Workbench />} />
        <Route path="/heartbeat" element={<HeartbeatMonitor />} />
        <Route path="/library" element={<Library />} />
        <Route path="/todo-lists" element={<TodoLists />} />
        <Route path="/settings" element={<Settings />} />
      </Route>
      <Route path="*" element={<Navigate to="/chat" replace />} />
    </Routes>
    </Suspense>
  );
}

function App() {
  return (
    <BrowserRouter>
      <PageTitle />
      <DesignProvider>
        <ToastProvider>
          <AuthProvider>
            <AppRoutes />
          </AuthProvider>
        </ToastProvider>
      </DesignProvider>
    </BrowserRouter>
  );
}

export default App;
