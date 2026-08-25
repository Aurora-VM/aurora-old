import React, { useEffect, useState } from 'react';
import {
  User,
  Key,
  Shield,
  Clock,
  Plus,
  Trash2,
  Copy,
  Lock,
  Smartphone,
} from 'lucide-react';
import { api, ApiKey, Session } from '../lib/api';
import { useAuth } from '../context/AuthContext';
import { useToast } from '../context/ToastContext';
import { ConfirmDialog } from '../components/ConfirmDialog';

export const AccountPage: React.FC = () => {
  const { user, refreshUser } = useAuth();
  const [activeTab, setActiveTab] = useState<'profile' | 'security' | 'apikeys' | 'sessions'>('profile');

  // Password change state
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [passLoading, setPassLoading] = useState(false);

  // 2FA state
  const [twoFactorSecret, setTwoFactorSecret] = useState<string | null>(null);
  const [totpCode, setTotpCode] = useState('');
  const [twoFactorLoading, setTwoFactorLoading] = useState(false);

  // API Keys state
  const [apiKeys, setApiKeys] = useState<ApiKey[]>([]);
  const [newKeyModal, setNewKeyModal] = useState(false);
  const [newKeyName, setNewKeyName] = useState('');
  const [selectedScopes, setSelectedScopes] = useState<string[]>([
    'instance:read',
    'instance:create',
    'instance:power',
  ]);
  const [createdSecret, setCreatedSecret] = useState<string | null>(null);
  const [deleteKeyTarget, setDeleteKeyTarget] = useState<ApiKey | null>(null);

  // Sessions state
  const [sessions, setSessions] = useState<Session[]>([]);

  const toast = useToast();

  useEffect(() => {
    if (activeTab === 'apikeys') {
      api.listApiKeys().then(setApiKeys).catch(() => {});
    } else if (activeTab === 'sessions') {
      api.listSessions().then(setSessions).catch(() => {});
    }
  }, [activeTab]);

  const handleUpdatePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentPassword || !newPassword) return;
    setPassLoading(true);
    try {
      await api.updatePassword(currentPassword, newPassword);
      toast.success('Password updated successfully');
      setCurrentPassword('');
      setNewPassword('');
    } catch (err: any) {
      toast.error('Password update failed', err.message);
    } finally {
      setPassLoading(false);
    }
  };

  const handleSetup2FA = async () => {
    setTwoFactorLoading(true);
    try {
      const res = await api.setup2FA();
      setTwoFactorSecret(res.secret);
    } catch (err: any) {
      toast.error('Failed to initiate 2FA setup', err.message);
    } finally {
      setTwoFactorLoading(false);
    }
  };

  const handleVerify2FA = async (e: React.FormEvent) => {
    e.preventDefault();
    setTwoFactorLoading(true);
    try {
      await api.verify2FA(totpCode);
      toast.success('Two-factor authentication enabled!');
      setTwoFactorSecret(null);
      setTotpCode('');
      refreshUser();
    } catch (err: any) {
      toast.error('Invalid 2FA code', err.message);
    } finally {
      setTwoFactorLoading(false);
    }
  };

  const handleDisable2FA = async () => {
    try {
      await api.disable2FA(totpCode);
      toast.success('2FA disabled');
      refreshUser();
    } catch (err: any) {
      toast.error('Failed to disable 2FA', err.message);
    }
  };

  const handleCreateApiKey = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newKeyName.trim()) return;
    try {
      const res = await api.createApiKey(newKeyName.trim(), selectedScopes, 90);
      setCreatedSecret(res.secret);
      setApiKeys([res.apiKey, ...apiKeys]);
      toast.success('API Key generated successfully');
    } catch (err: any) {
      toast.error('API key creation failed', err.message);
    }
  };

  const handleRevokeApiKey = async () => {
    if (!deleteKeyTarget) return;
    try {
      await api.revokeApiKey(deleteKeyTarget.id);
      setApiKeys(apiKeys.filter((k) => k.id !== deleteKeyTarget.id));
      toast.success('API Key revoked', deleteKeyTarget.name);
      setDeleteKeyTarget(null);
    } catch (err: any) {
      toast.error('Failed to revoke key', err.message);
    }
  };

  const handleRevokeSession = async (sessionId: string) => {
    try {
      await api.revokeSession(sessionId);
      setSessions(sessions.filter((s) => s.id !== sessionId));
      toast.success('Session terminated');
    } catch (err: any) {
      toast.error('Failed to revoke session', err.message);
    }
  };

  const availableScopes = [
    { id: 'instance:read', desc: 'Read instances & metrics' },
    { id: 'instance:create', desc: 'Deploy new instances' },
    { id: 'instance:power', desc: 'Start, stop, restart instances' },
    { id: 'instance:update', desc: 'Resize specs & update config' },
    { id: 'instance:delete', desc: 'Destroy instances' },
    { id: 'volume:read', desc: 'Inspect block volumes' },
    { id: 'volume:manage', desc: 'Attach & create snapshots' },
  ];

  return (
    <div className="max-w-4xl mx-auto space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="border-b border-[#181f30] pb-4">
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          <User className="w-5 h-5 text-blue-400" />
          <span>Account Settings & Developer API</span>
        </h2>
        <p className="text-xs text-slate-400 mt-1">
          Manage your credentials, 2FA hardware authentication, active login sessions, and scoped API tokens.
        </p>
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-2 border-b border-[#181f30] pb-2 text-xs font-semibold">
        <button
          onClick={() => setActiveTab('profile')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'profile'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <User className="w-4 h-4" />
          <span>Profile</span>
        </button>

        <button
          onClick={() => setActiveTab('security')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'security'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <Shield className="w-4 h-4" />
          <span>Security & 2FA</span>
        </button>

        <button
          onClick={() => setActiveTab('apikeys')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'apikeys'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <Key className="w-4 h-4" />
          <span>API Access Keys</span>
        </button>

        <button
          onClick={() => setActiveTab('sessions')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'sessions'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <Clock className="w-4 h-4" />
          <span>Active Sessions</span>
        </button>
      </div>

      {/* TAB 1: PROFILE */}
      {activeTab === 'profile' && user && (
        <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
          <h3 className="text-sm font-bold text-white">User Information</h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 font-mono text-xs">
            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
              <div className="text-slate-400">Username</div>
              <div className="text-white font-bold mt-1">{user.username}</div>
            </div>
            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
              <div className="text-slate-400">Email Address</div>
              <div className="text-white font-bold mt-1">{user.email}</div>
            </div>
            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
              <div className="text-slate-400">User ID</div>
              <div className="text-slate-300 mt-1">{user.id}</div>
            </div>
            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
              <div className="text-slate-400">Role & Privileges</div>
              <div className="text-emerald-400 font-bold mt-1">
                {user.roles?.join(', ') || 'Customer Tenant'}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* TAB 2: SECURITY & PASSWORD */}
      {activeTab === 'security' && (
        <div className="space-y-6">
          {/* Password Change */}
          <form
            onSubmit={handleUpdatePassword}
            className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4"
          >
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Lock className="w-4 h-4 text-blue-400" />
              <span>Change Password</span>
            </h3>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Current Password
                </label>
                <input
                  type="password"
                  required
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  New Password
                </label>
                <input
                  type="password"
                  required
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>
            </div>

            <button
              type="submit"
              disabled={passLoading}
              className="px-4 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-sm"
            >
              Update Password
            </button>
          </form>

          {/* 2FA TOTP */}
          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Smartphone className="w-4 h-4 text-purple-400" />
              <span>Two-Factor Authentication (TOTP)</span>
            </h3>

            <p className="text-xs text-slate-400">
              Protect your Aurora account using standard TOTP authenticator apps (Google Authenticator, 1Password, Bitwarden).
            </p>

            {user?.twoFactorEnabled ? (
              <div className="flex items-center justify-between p-3.5 rounded-xl bg-emerald-950/20 border border-emerald-500/30">
                <span className="text-xs font-semibold text-emerald-400">
                  Two-Factor Authentication is Active
                </span>
                <button
                  onClick={handleDisable2FA}
                  className="px-3 py-1.5 rounded-lg bg-rose-600/20 text-rose-300 hover:bg-rose-600/30 text-xs font-semibold"
                >
                  Disable 2FA
                </button>
              </div>
            ) : (
              <div>
                {!twoFactorSecret ? (
                  <button
                    onClick={handleSetup2FA}
                    disabled={twoFactorLoading}
                    className="px-4 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 text-white text-xs font-bold shadow-sm"
                  >
                    Setup Authenticator App
                  </button>
                ) : (
                  <form onSubmit={handleVerify2FA} className="space-y-4 p-4 rounded-xl bg-[#090b12] border border-[#181f30]">
                    <div className="text-xs text-slate-300 font-mono">
                      Secret Key: <span className="text-emerald-400 font-bold">{twoFactorSecret}</span>
                    </div>
                    <div>
                      <label className="block text-xs font-semibold text-slate-300 mb-1">
                        Enter 6-Digit TOTP Code
                      </label>
                      <input
                        type="text"
                        maxLength={6}
                        value={totpCode}
                        onChange={(e) => setTotpCode(e.target.value)}
                        placeholder="123456"
                        className="w-48 px-3.5 py-2 rounded-xl bg-[#07090e] border border-[#1e2538] text-white text-sm font-mono tracking-widest text-center focus:border-purple-500 focus:outline-none"
                      />
                    </div>
                    <button
                      type="submit"
                      disabled={twoFactorLoading}
                      className="px-4 py-2 rounded-xl bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold"
                    >
                      Verify & Activate 2FA
                    </button>
                  </form>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {/* TAB 3: API KEYS */}
      {activeTab === 'apikeys' && (
        <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-sm font-bold text-white flex items-center gap-2">
                <Key className="w-4 h-4 text-blue-400" />
                <span>Customer API Keys</span>
              </h3>
              <p className="text-xs text-slate-400 mt-0.5">
                Automate instance provisioning, telemetry collection, and backups via the REST API.
              </p>
            </div>

            <button
              onClick={() => {
                setCreatedSecret(null);
                setNewKeyModal(true);
              }}
              className="flex items-center gap-1.5 px-3.5 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>Create New Key</span>
            </button>
          </div>

          {/* API Keys Table */}
          <div className="rounded-xl bg-[#090b12] border border-[#181f30] overflow-hidden">
            <table className="w-full text-left text-xs font-mono">
              <thead>
                <tr className="border-b border-[#181f30] text-slate-400">
                  <th className="py-3 px-4">Name</th>
                  <th className="py-3 px-4">Prefix</th>
                  <th className="py-3 px-4">Scopes</th>
                  <th className="py-3 px-4">Created</th>
                  <th className="py-3 px-4 text-right">Revoke</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#141a29]">
                {apiKeys.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="py-8 text-center text-slate-500">
                      No API keys created yet.
                    </td>
                  </tr>
                ) : (
                  apiKeys.map((k) => (
                    <tr key={k.id}>
                      <td className="py-3 px-4 text-white font-sans font-semibold">{k.name}</td>
                      <td className="py-3 px-4 text-blue-400">{k.prefix}...</td>
                      <td className="py-3 px-4 text-slate-400">{k.scopes.join(', ')}</td>
                      <td className="py-3 px-4 text-slate-500">
                        {new Date(k.createdAt).toLocaleDateString()}
                      </td>
                      <td className="py-3 px-4 text-right">
                        <button
                          onClick={() => setDeleteKeyTarget(k)}
                          className="p-1 text-slate-400 hover:text-rose-400"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* TAB 4: SESSIONS */}
      {activeTab === 'sessions' && (
        <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
          <h3 className="text-sm font-bold text-white flex items-center gap-2">
            <Clock className="w-4 h-4 text-blue-400" />
            <span>Active Login Sessions</span>
          </h3>

          <div className="space-y-2">
            {sessions.map((s) => (
              <div
                key={s.id}
                className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30] flex items-center justify-between font-mono text-xs"
              >
                <div>
                  <div className="text-white font-semibold">{s.ipAddress}</div>
                  <div className="text-[10px] text-slate-400 mt-0.5">{s.userAgent}</div>
                </div>
                <button
                  onClick={() => handleRevokeSession(s.id)}
                  className="px-3 py-1 rounded-lg bg-[#141824] hover:bg-rose-950/40 text-rose-400 border border-[#232a3d] text-xs font-semibold"
                >
                  Revoke
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Create API Key Modal */}
      {newKeyModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-md bg-[#0d101a] border border-[#1e2538] rounded-2xl shadow-2xl p-6 space-y-4">
            <h3 className="text-sm font-bold text-white">Generate Developer API Key</h3>

            {createdSecret ? (
              <div className="space-y-3">
                <div className="p-3 rounded-xl bg-amber-500/10 border border-amber-500/30 text-amber-300 text-xs">
                  Copy your key secret now. You will not be able to see it again!
                </div>
                <div className="p-3 rounded-xl bg-[#07090e] border border-[#181f30] font-mono text-xs text-emerald-400 break-all select-all">
                  {createdSecret}
                </div>
                <button
                  onClick={() => {
                    navigator.clipboard.writeText(createdSecret);
                    toast.success('Copied secret to clipboard');
                  }}
                  className="w-full py-2 rounded-xl bg-blue-600 text-white text-xs font-bold flex items-center justify-center gap-2"
                >
                  <Copy className="w-3.5 h-3.5" />
                  <span>Copy to Clipboard</span>
                </button>
                <button
                  onClick={() => setNewKeyModal(false)}
                  className="w-full py-2 rounded-xl bg-[#141824] text-slate-300 text-xs font-semibold"
                >
                  Done
                </button>
              </div>
            ) : (
              <form onSubmit={handleCreateApiKey} className="space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Key Name</label>
                  <input
                    type="text"
                    required
                    value={newKeyName}
                    onChange={(e) => setNewKeyName(e.target.value)}
                    placeholder="CI/CD Deployment Token"
                    className="w-full px-3 py-2 rounded-xl bg-[#07090e] border border-[#1e2538] text-xs text-white focus:outline-none focus:border-blue-500"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-2">
                    Permission Scopes
                  </label>
                  <div className="space-y-1.5 max-h-40 overflow-y-auto">
                    {availableScopes.map((sc) => (
                      <label
                        key={sc.id}
                        className="flex items-center gap-2 p-2 rounded-lg bg-[#07090e] border border-[#181f30] text-xs cursor-pointer"
                      >
                        <input
                          type="checkbox"
                          checked={selectedScopes.includes(sc.id)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedScopes([...selectedScopes, sc.id]);
                            } else {
                              setSelectedScopes(selectedScopes.filter((s) => s !== sc.id));
                            }
                          }}
                          className="rounded text-blue-600"
                        />
                        <div>
                          <div className="font-mono text-white text-[11px]">{sc.id}</div>
                          <div className="text-[10px] text-slate-400">{sc.desc}</div>
                        </div>
                      </label>
                    ))}
                  </div>
                </div>

                <div className="flex justify-end gap-2 pt-2">
                  <button
                    type="button"
                    onClick={() => setNewKeyModal(false)}
                    className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] text-slate-300"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    className="px-4 py-2 rounded-xl text-xs font-bold bg-blue-600 hover:bg-blue-500 text-white"
                  >
                    Generate Key
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}

      {/* Revoke Key Confirmation */}
      <ConfirmDialog
        isOpen={!!deleteKeyTarget}
        title={`Revoke API Key "${deleteKeyTarget?.name}"?`}
        message="Any automated pipelines or integrations using this token will be immediately blocked."
        confirmText="Revoke Key"
        isDestructive={true}
        onConfirm={handleRevokeApiKey}
        onCancel={() => setDeleteKeyTarget(null)}
      />
    </div>
  );
};
