import React, { useEffect, useState, useCallback } from 'react';
import { AuthProvider, useAuth } from './context/AuthContext';
import { ToastProvider } from './context/ToastContext';
import { JobsProvider } from './context/JobsContext';
import { Navigation } from './components/Navigation';
import { CommandPalette } from './components/CommandPalette';
import { JobsDrawer } from './components/JobsDrawer';
import { CustomerDashboard } from './pages/CustomerDashboard';
import { InstanceList } from './pages/InstanceList';
import { InstanceDetail } from './pages/InstanceDetail';
import { InstanceWizard } from './pages/InstanceWizard';
import { TemplatesView } from './pages/Templates';
import { Billing } from './pages/Billing';
import { Notifications } from './pages/Notifications';
import { Webhooks } from './pages/Webhooks';
import { CustomerBackups } from './pages/customer/CustomerBackups';
import { AccountPage } from './pages/AccountPage';
import { AuthPage } from './pages/AuthPage';

// Admin Components
import { AdminDashboard } from './pages/admin/AdminDashboard';
import { AdminNodes } from './pages/admin/AdminNodes';
import { AdminNodeDetail } from './pages/admin/AdminNodeDetail';
import { AdminInstances } from './pages/admin/AdminInstances';
import { AdminBilling } from './pages/admin/AdminBilling';
import { AdminEvents } from './pages/admin/AdminEvents';
import { AdminIPAM } from './pages/admin/AdminIPAM';
import { AdminStorage } from './pages/admin/AdminStorage';
import { AdminTemplatesImages } from './pages/admin/AdminTemplatesImages';
import { AdminAudit } from './pages/admin/AdminAudit';
import { AdminSIEM } from './pages/admin/AdminSIEM';
import { AdminMonitoring } from './pages/admin/AdminMonitoring';
import { AdminJobs } from './pages/admin/AdminJobs';
import { AdminRecovery } from './pages/admin/AdminRecovery';
import { AdminSettings } from './pages/admin/AdminSettings';

import { Instance, OSTemplate, api } from './lib/api';

const AppContent: React.FC = () => {
  const { user, loading: authLoading, isAuthenticated } = useAuth();
  const [currentPath, setCurrentPath] = useState<string>(() => window.location.pathname || '/');
  const [instances, setInstances] = useState<Instance[]>([]);
  const [templates, setTemplates] = useState<OSTemplate[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [commandPaletteOpen, setCommandPaletteOpen] = useState<boolean>(false);

  // Sync browser navigation / popstate
  const navigate = useCallback((path: string) => {
    window.history.pushState({}, '', path);
    setCurrentPath(path);
  }, []);

  useEffect(() => {
    const handlePopState = () => {
      setCurrentPath(window.location.pathname || '/');
    };
    window.addEventListener('popstate', handlePopState);
    return () => window.removeEventListener('popstate', handlePopState);
  }, []);

  // Global Ctrl+K / Cmd+K Command Palette shortcut
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault();
        setCommandPaletteOpen((prev) => !prev);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [instList, tmplList] = await Promise.all([
        api.listInstances().catch(() => []),
        api.listTemplates().catch(() => []),
      ]);
      setInstances(instList);
      setTemplates(tmplList);
    } catch {
      // Offline fallback
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (isAuthenticated) {
      fetchData();
    }
  }, [isAuthenticated, fetchData]);

  if (authLoading) {
    return (
      <div className="min-h-screen bg-[#07090e] flex items-center justify-center text-slate-500 font-mono text-xs">
        Initializing Project Aurora Portal...
      </div>
    );
  }

  // If unauthenticated, show Auth view
  if (!isAuthenticated && !user) {
    return <AuthPage />;
  }

  const isAdmin =
    user?.roles?.some((r) => r === 'superadmin' || r === 'admin') ||
    user?.permissions?.includes('*') ||
    user?.permissions?.includes('node:read');

  // Route Dispatcher
  const renderCurrentView = () => {
    // Check Admin Routes Guard
    if (currentPath.startsWith('/admin')) {
      if (!isAdmin) {
        return (
          <div className="max-w-md mx-auto my-16 p-8 rounded-3xl bg-[#0f121d] border border-rose-500/30 text-center space-y-4 shadow-2xl animate-in zoom-in-95">
            <div className="w-12 h-12 rounded-2xl bg-rose-600/20 text-rose-400 flex items-center justify-center font-bold text-xl mx-auto">
              !
            </div>
            <h2 className="text-lg font-bold text-white">403 — Unauthorized Access</h2>
            <p className="text-xs text-slate-400">
              Your account does not possess operator or hypervisor administration privileges.
            </p>
            <button
              onClick={() => navigate('/')}
              className="px-4 py-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] text-white text-xs font-semibold"
            >
              Return to Customer Dashboard
            </button>
          </div>
        );
      }

      if (currentPath === '/admin' || currentPath === '/admin/') {
        return <AdminDashboard navigate={navigate} />;
      }
      if (currentPath === '/admin/nodes') {
        return <AdminNodes navigate={navigate} />;
      }
      if (currentPath.startsWith('/admin/nodes/')) {
        const parts = currentPath.split('/');
        const nodeId = parts[3];
        return <AdminNodeDetail nodeId={nodeId} navigate={navigate} />;
      }
      if (currentPath === '/admin/instances') {
        return <AdminInstances navigate={navigate} />;
      }
      if (currentPath.startsWith('/admin/billing')) {
        return <AdminBilling />;
      }
      if (currentPath.startsWith('/admin/events')) {
        return <AdminEvents />;
      }
      if (currentPath.startsWith('/admin/ipam')) {
        return <AdminIPAM />;
      }
      if (currentPath.startsWith('/admin/storage')) {
        return <AdminStorage />;
      }
      if (currentPath.startsWith('/admin/templates') || currentPath.startsWith('/admin/images')) {
        return <AdminTemplatesImages />;
      }
      if (currentPath.startsWith('/admin/audit')) {
        return <AdminAudit />;
      }
      if (currentPath.startsWith('/admin/siem')) {
        return <AdminSIEM />;
      }
      if (currentPath.startsWith('/admin/monitoring')) {
        return <AdminMonitoring />;
      }
      if (currentPath.startsWith('/admin/jobs')) {
        return <AdminJobs />;
      }
      if (currentPath.startsWith('/admin/recovery')) {
        return <AdminRecovery />;
      }
      if (currentPath.startsWith('/admin/settings')) {
        return <AdminSettings />;
      }
      return <AdminDashboard navigate={navigate} />;
    }

    // Customer Routes
    if (currentPath === '/' || currentPath === '') {
      return (
        <CustomerDashboard
          instances={instances}
          templates={templates}
          navigate={navigate}
        />
      );
    }

    if (currentPath === '/instances') {
      return (
        <InstanceList
          instances={instances}
          loading={loading}
          onRefresh={fetchData}
          navigate={navigate}
        />
      );
    }

    if (currentPath.startsWith('/instances/new')) {
      const urlParams = new URLSearchParams(window.location.search);
      const tmplParam = urlParams.get('template') || undefined;
      return (
        <InstanceWizard
          templates={templates}
          initialTemplateSlug={tmplParam}
          navigate={navigate}
        />
      );
    }

    if (currentPath.startsWith('/instances/')) {
      const parts = currentPath.split('/');
      const instId = parts[2];
      const urlParams = new URLSearchParams(window.location.search);
      const tabParam = urlParams.get('tab') || undefined;
      return (
        <InstanceDetail
          instanceId={instId}
          initialTab={tabParam}
          navigate={navigate}
        />
      );
    }

    if (currentPath.startsWith('/templates')) {
      return <TemplatesView />;
    }

    if (currentPath.startsWith('/billing')) {
      return <Billing />;
    }

    if (currentPath.startsWith('/backups')) {
      return <CustomerBackups />;
    }

    if (currentPath.startsWith('/notifications')) {
      return <Notifications />;
    }

    if (currentPath.startsWith('/webhooks')) {
      return <Webhooks />;
    }

    if (currentPath.startsWith('/account')) {
      return <AccountPage />;
    }

    return (
      <CustomerDashboard
        instances={instances}
        templates={templates}
        navigate={navigate}
      />
    );
  };

  return (
    <div className="min-h-screen bg-[#07090e] text-slate-100 flex flex-col selection:bg-blue-600 selection:text-white">
      {/* Navigation Bar */}
      <Navigation
        currentPath={currentPath}
        navigate={navigate}
        onOpenCommandPalette={() => setCommandPaletteOpen(true)}
      />

      {/* Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 py-6 sm:py-8">
        {renderCurrentView()}
      </main>

      {/* Global Modals & Drawers */}
      <CommandPalette
        isOpen={commandPaletteOpen}
        onClose={() => setCommandPaletteOpen(false)}
        navigate={navigate}
        instances={instances}
        templates={templates}
      />

      <JobsDrawer />

      {/* Footer */}
      <footer className="border-t border-[#181f30] py-4 px-6 text-center text-[11px] text-slate-500 font-mono flex flex-col sm:flex-row justify-between items-center gap-2">
        <span>Project Aurora Cloud Platform © 2026. Production Ready.</span>
        <span>Secure Incus Virtualization Engine</span>
      </footer>
    </div>
  );
};

export const App: React.FC = () => {
  return (
    <AuthProvider>
      <ToastProvider>
        <JobsProvider>
          <AppContent />
        </JobsProvider>
      </ToastProvider>
    </AuthProvider>
  );
};

export default App;
