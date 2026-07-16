import { createBrowserRouter, Navigate } from 'react-router-dom';
import { useAuthStore } from './stores/auth';
import { AppLayout } from './components/layout/app-layout';
import LoginPage from './pages/login';
import OverviewPage from './pages/overview';
import ProxiesPage from './pages/proxies';
import ProxyGroupsPage from './pages/proxy-groups';
import RulesPage from './pages/rules';
import RuleProvidersPage from './pages/rule-providers';
import SystemConfigPage from './pages/system-config';
import PublishPage from './pages/publish';
import RuntimePage from './pages/runtime';
import SettingsPage from './pages/settings';

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useAuthStore((s) => !!s.accessToken);
  if (!isAuthenticated) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  {
    path: '/',
    element: (
      <ProtectedRoute>
        <AppLayout />
      </ProtectedRoute>
    ),
    children: [
      { index: true, element: <OverviewPage /> },
      { path: 'proxies', element: <ProxiesPage /> },
      { path: 'proxy-groups', element: <ProxyGroupsPage /> },
      { path: 'rules', element: <RulesPage /> },
      { path: 'rule-providers', element: <RuleProvidersPage /> },
      { path: 'system-config', element: <SystemConfigPage /> },
      { path: 'publish', element: <PublishPage /> },
      { path: 'runtime', element: <RuntimePage /> },
      { path: 'settings', element: <SettingsPage /> },
    ],
  },
]);
