import React, { useEffect, useState, useCallback } from 'react';
import {
  Bell,
  CheckCheck,
  Filter,
  CheckCircle2,
  AlertTriangle,
  Info,
  AlertOctagon,
  RefreshCw,
  Clock,
  Sliders,
} from 'lucide-react';
import { api, NotificationItem, NotificationPreference } from '../lib/api';
import { useToast } from '../context/ToastContext';

export const Notifications: React.FC = () => {
  const [notifications, setNotifications] = useState<NotificationItem[]>([]);
  const [total, setTotal] = useState<number>(0);
  const [unreadCount, setUnreadCount] = useState<number>(0);
  const [loading, setLoading] = useState<boolean>(true);
  const [unreadOnly, setUnreadOnly] = useState<boolean>(false);
  const [severityFilter, setSeverityFilter] = useState<string>('all');
  const [showPreferences, setShowPreferences] = useState<boolean>(false);
  const [preferences, setPreferences] = useState<NotificationPreference[]>([]);

  const toast = useToast();

  const fetchNotifications = useCallback(async () => {
    setLoading(true);
    try {
      const [listRes, count] = await Promise.all([
        api.listNotifications(unreadOnly, severityFilter === 'all' ? undefined : severityFilter, 50, 0),
        api.getUnreadNotificationCount(),
      ]);
      setNotifications(listRes.notifications || []);
      setTotal(listRes.total || 0);
      setUnreadCount(count);
    } catch {
      toast.error('Failed to fetch notifications');
    } finally {
      setLoading(false);
    }
  }, [unreadOnly, severityFilter, toast]);

  useEffect(() => {
    fetchNotifications();
  }, [fetchNotifications]);

  const loadPreferences = async () => {
    try {
      const prefs = await api.getNotificationPreferences();
      setPreferences(prefs || []);
      setShowPreferences(true);
    } catch {
      toast.error('Failed to load notification preferences');
    }
  };

  const handleMarkAsRead = async (id: string) => {
    try {
      await api.markNotificationRead(id);
      setNotifications((prev) =>
        prev.map((n) => (n.id === id ? { ...n, readAt: new Date().toISOString() } : n))
      );
      setUnreadCount((prev) => Math.max(0, prev - 1));
      toast.success('Notification marked as read');
    } catch {
      toast.error('Failed to update notification');
    }
  };

  const handleMarkAllRead = async () => {
    try {
      const count = await api.markAllNotificationsRead();
      setNotifications((prev) =>
        prev.map((n) => ({ ...n, readAt: n.readAt || new Date().toISOString() }))
      );
      setUnreadCount(0);
      toast.success(`Marked ${count} notifications as read`);
    } catch {
      toast.error('Failed to mark all as read');
    }
  };

  const handleTogglePref = async (pref: NotificationPreference, field: 'inAppEnabled' | 'emailEnabled' | 'webhookEnabled') => {
    const updated = { ...pref, [field]: !pref[field] };
    try {
      await api.setNotificationPreference(updated);
      setPreferences((prev) => prev.map((p) => (p.eventType === pref.eventType ? updated : p)));
      toast.success('Preference updated');
    } catch {
      toast.error('Failed to update preference');
    }
  };

  const renderSeverityIcon = (sev: string) => {
    switch (sev) {
      case 'success':
        return <CheckCircle2 className="w-4 h-4 text-emerald-400" />;
      case 'warning':
        return <AlertTriangle className="w-4 h-4 text-amber-400" />;
      case 'error':
      case 'critical':
        return <AlertOctagon className="w-4 h-4 text-rose-400" />;
      default:
        return <Info className="w-4 h-4 text-blue-400" />;
    }
  };

  return (
    <div className="max-w-6xl mx-auto px-4 py-8 space-y-6 animate-in fade-in duration-300">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-[#181f30] pb-6">
        <div>
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-2xl bg-blue-600/10 border border-blue-500/20 text-blue-400">
              <Bell className="w-6 h-6" />
            </div>
            <div>
              <h1 className="text-2xl font-bold text-white tracking-tight">Notification Center</h1>
              <p className="text-xs text-slate-400 mt-0.5">
                Real-time activity alerts, deployment state updates, and critical security notices.
              </p>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={loadPreferences}
            className="flex items-center gap-2 px-4 py-2 rounded-xl bg-[#0f121d] hover:bg-[#181f30] border border-[#1f283d] text-slate-200 text-xs font-semibold transition"
          >
            <Sliders className="w-3.5 h-3.5 text-blue-400" />
            <span>Preferences</span>
          </button>
          <button
            onClick={handleMarkAllRead}
            disabled={unreadCount === 0}
            className="flex items-center gap-2 px-4 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-xs font-semibold shadow-lg shadow-blue-600/20 transition"
          >
            <CheckCheck className="w-3.5 h-3.5" />
            <span>Mark All Read ({unreadCount})</span>
          </button>
        </div>
      </div>

      {/* Filter Tabs */}
      <div className="flex flex-wrap items-center justify-between gap-4 bg-[#090b12] p-3 rounded-2xl border border-[#181f30]">
        <div className="flex items-center gap-2">
          <button
            onClick={() => setUnreadOnly(false)}
            className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition ${
              !unreadOnly ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-white'
            }`}
          >
            All Activity ({total})
          </button>
          <button
            onClick={() => setUnreadOnly(true)}
            className={`px-3 py-1.5 rounded-lg text-xs font-semibold flex items-center gap-1.5 transition ${
              unreadOnly ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-white'
            }`}
          >
            <span>Unread</span>
            {unreadCount > 0 && (
              <span className="px-1.5 py-0.2 rounded-full text-[10px] bg-rose-500 text-white font-mono">
                {unreadCount}
              </span>
            )}
          </button>
        </div>

        <div className="flex items-center gap-2 text-xs">
          <Filter className="w-3.5 h-3.5 text-slate-400" />
          <span className="text-slate-400">Severity:</span>
          <select
            value={severityFilter}
            onChange={(e) => setSeverityFilter(e.target.value)}
            className="bg-[#121624] border border-[#1f283d] text-slate-200 text-xs rounded-lg px-2.5 py-1 focus:outline-none focus:border-blue-500"
          >
            <option value="all">All Severities</option>
            <option value="info">Info</option>
            <option value="success">Success</option>
            <option value="warning">Warning</option>
            <option value="error">Error</option>
            <option value="critical">Critical</option>
          </select>
          <button
            onClick={fetchNotifications}
            className="p-1.5 rounded-lg bg-[#121624] hover:bg-[#1a2034] text-slate-400 hover:text-white transition"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Notifications List */}
      {loading ? (
        <div className="p-12 text-center text-slate-500 text-xs font-mono">Loading notifications...</div>
      ) : notifications.length === 0 ? (
        <div className="p-12 text-center rounded-3xl bg-[#090b12] border border-[#181f30] space-y-3">
          <div className="w-12 h-12 rounded-2xl bg-blue-600/10 text-blue-400 flex items-center justify-center mx-auto">
            <Bell className="w-6 h-6" />
          </div>
          <h3 className="text-sm font-bold text-white">No Notifications</h3>
          <p className="text-xs text-slate-400 max-w-sm mx-auto">
            You're all caught up! System events, workload state transitions, and billing notices will appear here.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {notifications.map((n) => {
            const isRead = !!n.readAt;
            return (
              <div
                key={n.id}
                className={`p-4 rounded-2xl border transition flex items-start justify-between gap-4 ${
                  isRead
                    ? 'bg-[#090b12] border-[#141926] text-slate-400'
                    : 'bg-[#0e121f] border-blue-500/30 text-slate-100 shadow-md shadow-blue-500/5'
                }`}
              >
                <div className="flex items-start gap-3.5">
                  <div className="mt-0.5 p-2 rounded-xl bg-[#141926] border border-[#1f283d] flex-shrink-0">
                    {renderSeverityIcon(n.severity)}
                  </div>
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="font-bold text-xs text-white">{n.title}</span>
                      <span className="px-2 py-0.5 rounded text-[10px] font-mono bg-[#141926] text-slate-400 border border-[#1f283d]">
                        {n.type}
                      </span>
                      {n.resourceId && (
                        <span className="px-2 py-0.5 rounded text-[10px] font-mono bg-blue-500/10 text-blue-400 border border-blue-500/20">
                          {n.resourceType}: {n.resourceId}
                        </span>
                      )}
                    </div>
                    <p className="text-xs text-slate-300 leading-relaxed">{n.body}</p>
                    <div className="flex items-center gap-2 text-[10px] font-mono text-slate-500 pt-1">
                      <Clock className="w-3 h-3" />
                      <span>{new Date(n.createdAt).toLocaleString()}</span>
                    </div>
                  </div>
                </div>

                {!isRead && (
                  <button
                    onClick={() => handleMarkAsRead(n.id)}
                    className="p-2 rounded-xl bg-[#141926] hover:bg-blue-600 hover:text-white text-slate-400 text-xs font-semibold flex items-center gap-1.5 transition flex-shrink-0"
                    title="Mark as read"
                  >
                    <CheckCheck className="w-3.5 h-3.5" />
                    <span className="text-[11px]">Mark Read</span>
                  </button>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* Notification Preferences Modal */}
      {showPreferences && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-[#0b0e17] border border-[#1f283d] rounded-3xl max-w-xl w-full p-6 space-y-6 shadow-2xl animate-in zoom-in-95">
            <div className="flex items-center justify-between border-b border-[#181f30] pb-4">
              <div className="flex items-center gap-2.5">
                <Sliders className="w-5 h-5 text-blue-400" />
                <h3 className="text-base font-bold text-white">Notification Channel Preferences</h3>
              </div>
              <button
                onClick={() => setShowPreferences(false)}
                className="text-slate-400 hover:text-white text-xs font-bold"
              >
                ✕
              </button>
            </div>

            <p className="text-xs text-slate-400">
              Customize which event categories trigger in-app feed alerts and email notifications.
            </p>

            <div className="space-y-3 max-h-80 overflow-y-auto pr-1">
              {preferences.length === 0 ? (
                <div className="text-xs text-slate-500 text-center py-6">All channels currently enabled by default.</div>
              ) : (
                preferences.map((p) => (
                  <div
                    key={p.eventType}
                    className="p-3.5 rounded-2xl bg-[#090b12] border border-[#181f30] flex items-center justify-between"
                  >
                    <div>
                      <span className="font-mono text-xs font-bold text-white">{p.eventType}</span>
                    </div>
                    <div className="flex items-center gap-4 text-xs">
                      <label className="flex items-center gap-1.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={p.inAppEnabled}
                          onChange={() => handleTogglePref(p, 'inAppEnabled')}
                          className="rounded border-[#1f283d] text-blue-600 focus:ring-0"
                        />
                        <span className="text-slate-300">In-App</span>
                      </label>
                      <label className="flex items-center gap-1.5 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={p.emailEnabled}
                          onChange={() => handleTogglePref(p, 'emailEnabled')}
                          className="rounded border-[#1f283d] text-blue-600 focus:ring-0"
                        />
                        <span className="text-slate-300">Email</span>
                      </label>
                    </div>
                  </div>
                ))
              )}
            </div>

            <div className="flex justify-end pt-2">
              <button
                onClick={() => setShowPreferences(false)}
                className="px-4 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold"
              >
                Done
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
