import React, { useEffect, useState } from 'react';
import {
  Server,
  Layers,
  PlusCircle,
  CreditCard,
  User as UserIcon,
  LogOut,
  Command,
  Activity,
  Menu,
  X,
  Cpu,
  Shield,
  Bell,
  Webhook,
  ShieldCheck,
  LifeBuoy,
} from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { useJobs } from '../context/JobsContext';
import { api } from '../lib/api';

interface NavigationProps {
  currentPath: string;
  navigate: (path: string) => void;
  onOpenCommandPalette: () => void;
}

export const Navigation: React.FC<NavigationProps> = ({
  currentPath,
  navigate,
  onOpenCommandPalette,
}) => {
  const { user, logout } = useAuth();
  const { jobs, toggleDrawer } = useJobs();
  const [mobileMenuOpen, setMobileMenuOpen] = React.useState(false);
  const [unreadNotifications, setUnreadNotifications] = useState<number>(0);

  const activeJobsCount = jobs.filter((j) => j.status === 'running' || j.status === 'pending').length;

  useEffect(() => {
    if (!user) return;
    const checkUnread = async () => {
      try {
        const count = await api.getUnreadNotificationCount();
        setUnreadNotifications(count);
      } catch {
        // Ignore unread count fetch error
      }
    };
    checkUnread();
    const interval = setInterval(checkUnread, 15000);
    return () => clearInterval(interval);
  }, [user]);

  const isAdmin =
    user?.roles?.some((r) => r === 'superadmin' || r === 'admin') ||
    user?.permissions?.includes('*') ||
    user?.permissions?.includes('node:read');

  const navItems = [
    { path: '/', label: 'Overview', icon: Activity },
    { path: '/instances', label: 'Instances', icon: Server },
    { path: '/templates', label: 'OS Templates', icon: Layers },
    { path: '/backups', label: 'Backups', icon: ShieldCheck },
    { path: '/billing', label: 'Billing', icon: CreditCard },
    { path: '/webhooks', label: 'Webhooks', icon: Webhook },
    { path: '/account', label: 'Account', icon: UserIcon },
    ...(isAdmin
      ? [
          { path: '/admin', label: 'Admin Fleet', icon: Shield },
          { path: '/admin/recovery', label: 'Disaster Recovery', icon: LifeBuoy },
          { path: '/admin/events', label: 'Events Hub', icon: Activity },
        ]
      : []),
  ];

  return (
    <>
      {/* Top Header */}
      <header className="border-b border-[#181f30] bg-[#0c0e17]/90 backdrop-blur-md px-4 sm:px-6 py-3 flex items-center justify-between sticky top-0 z-40">
        <div className="flex items-center space-x-3 sm:space-x-6">
          {/* Logo */}
          <div
            onClick={() => navigate('/')}
            className="flex items-center space-x-2.5 cursor-pointer group"
          >
            <div className="w-8 h-8 rounded-lg bg-blue-600 flex items-center justify-center font-bold text-white shadow-lg shadow-blue-500/20 group-hover:bg-blue-500 transition">
              A
            </div>
            <div className="flex flex-col">
              <span className="font-bold text-base tracking-tight bg-gradient-to-r from-white via-slate-200 to-slate-400 bg-clip-text text-transparent">
                AURORA
              </span>
              <span className="text-[10px] text-slate-400 font-mono -mt-1">Cloud Portal</span>
            </div>
          </div>

          {/* Desktop Nav Links */}
          <nav className="hidden md:flex items-center space-x-1 pl-4 border-l border-[#181f30]">
            {navItems.map((item) => {
              const Icon = item.icon;
              const active = currentPath === item.path || (item.path !== '/' && currentPath.startsWith(item.path));
              return (
                <button
                  key={item.path}
                  onClick={() => navigate(item.path)}
                  className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                    active
                      ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
                      : 'text-slate-400 hover:text-white hover:bg-[#141824]'
                  }`}
                >
                  <Icon className="w-3.5 h-3.5" />
                  <span>{item.label}</span>
                </button>
              );
            })}
          </nav>
        </div>

        {/* Right Action Toolbar */}
        <div className="flex items-center space-x-2 sm:space-x-3">
          {/* Quick Create Button */}
          <button
            onClick={() => navigate('/instances/new')}
            className="hidden sm:flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-blue-600 hover:bg-blue-500 text-white shadow-sm shadow-blue-500/20 transition"
          >
            <PlusCircle className="w-3.5 h-3.5" />
            <span>New Instance</span>
          </button>

          {/* Command Palette Trigger */}
          <button
            onClick={onOpenCommandPalette}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs bg-[#121624] hover:bg-[#181f30] border border-[#1e2538] text-slate-400 hover:text-white transition"
            title="Command Palette (Ctrl+K)"
          >
            <Command className="w-3.5 h-3.5 text-slate-400" />
            <span className="hidden sm:inline font-mono text-[11px]">⌘K</span>
          </button>

          {/* Notification Center Bell */}
          <button
            onClick={() => navigate('/notifications')}
            className={`relative p-2 rounded-lg border transition ${
              unreadNotifications > 0
                ? 'bg-blue-600/15 border-blue-500/30 text-blue-400'
                : 'bg-[#121624] hover:bg-[#181f30] border-[#1e2538] text-slate-400 hover:text-white'
            }`}
            title="Notifications"
          >
            <Bell className="w-4 h-4" />
            {unreadNotifications > 0 && (
              <span className="absolute -top-1 -right-1 w-4 h-4 bg-rose-600 text-white rounded-full text-[9px] font-mono flex items-center justify-center font-bold animate-pulse">
                {unreadNotifications > 9 ? '9+' : unreadNotifications}
              </span>
            )}
          </button>

          {/* Active Jobs Button */}
          <button
            onClick={toggleDrawer}
            className={`relative p-2 rounded-lg border transition ${
              activeJobsCount > 0
                ? 'bg-blue-600/15 border-blue-500/30 text-blue-400 animate-pulse'
                : 'bg-[#121624] hover:bg-[#181f30] border-[#1e2538] text-slate-400 hover:text-white'
            }`}
            title="Active Tasks & Jobs"
          >
            <Cpu className="w-4 h-4" />
            {activeJobsCount > 0 && (
              <span className="absolute -top-1 -right-1 w-4 h-4 bg-blue-600 text-white rounded-full text-[9px] font-mono flex items-center justify-center font-bold">
                {activeJobsCount}
              </span>
            )}
          </button>

          {/* User Menu / Logout */}
          {user && (
            <div className="flex items-center space-x-2 pl-2 border-l border-[#181f30]">
              <div
                onClick={() => navigate('/account')}
                className="hidden sm:flex flex-col items-end cursor-pointer"
              >
                <span className="text-xs font-medium text-slate-200">{user.username}</span>
                <span className="text-[10px] text-slate-400 font-mono">{user.email}</span>
              </div>
              <button
                onClick={logout}
                className="p-2 rounded-lg bg-[#121624] hover:bg-rose-950/30 border border-[#1e2538] hover:border-rose-800/40 text-slate-400 hover:text-rose-400 transition"
                title="Log Out"
              >
                <LogOut className="w-4 h-4" />
              </button>
            </div>
          )}

          {/* Mobile Menu Toggle */}
          <button
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            className="md:hidden p-2 rounded-lg bg-[#121624] border border-[#1e2538] text-slate-400 hover:text-white"
          >
            {mobileMenuOpen ? <X className="w-4 h-4" /> : <Menu className="w-4 h-4" />}
          </button>
        </div>
      </header>

      {/* Mobile Drawer Menu */}
      {mobileMenuOpen && (
        <div className="md:hidden border-b border-[#181f30] bg-[#0c0e17] px-4 py-3 space-y-2">
          {navItems.map((item) => {
            const Icon = item.icon;
            const active = currentPath === item.path;
            return (
              <button
                key={item.path}
                onClick={() => {
                  navigate(item.path);
                  setMobileMenuOpen(false);
                }}
                className={`w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-xs font-medium transition ${
                  active ? 'bg-blue-600/15 text-blue-400' : 'text-slate-300 hover:bg-[#141824]'
                }`}
              >
                <Icon className="w-4 h-4" />
                <span>{item.label}</span>
              </button>
            );
          })}
          <button
            onClick={() => {
              navigate('/instances/new');
              setMobileMenuOpen(false);
            }}
            className="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-xs font-medium bg-blue-600 text-white"
          >
            <PlusCircle className="w-4 h-4" />
            <span>Create Instance</span>
          </button>
        </div>
      )}
    </>
  );
};
