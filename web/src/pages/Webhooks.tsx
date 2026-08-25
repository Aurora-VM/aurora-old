import React, { useEffect, useState, useCallback } from 'react';
import {
  Webhook,
  Plus,
  Key,
  Trash2,
  Copy,
  Check,
  ShieldCheck,
  Code2,
  RefreshCw,
  History,
  Send,
} from 'lucide-react';
import { api, WebhookEndpoint, WebhookDelivery } from '../lib/api';
import { useToast } from '../context/ToastContext';

const SUPPORTED_EVENT_TYPES = [
  { id: '*', label: 'All Platform Events (*)' },
  { id: 'instance.created', label: 'Instance Created (instance.created)' },
  { id: 'instance.started', label: 'Instance Started (instance.started)' },
  { id: 'instance.stopped', label: 'Instance Stopped (instance.stopped)' },
  { id: 'instance.restarted', label: 'Instance Restarted (instance.restarted)' },
  { id: 'instance.deleted', label: 'Instance Deleted (instance.deleted)' },
  { id: 'backup.created', label: 'Backup Created (backup.created)' },
  { id: 'snapshot.created', label: 'Snapshot Created (snapshot.created)' },
  { id: 'billing.invoice.created', label: 'Invoice Generated (billing.invoice.created)' },
  { id: 'billing.payment.failed', label: 'Payment Failed (billing.payment.failed)' },
  { id: 'usage.threshold.exceeded', label: 'Quota Alert (usage.threshold.exceeded)' },
  { id: 'node.unhealthy', label: 'Node Unhealthy (node.unhealthy)' },
];

export const Webhooks: React.FC = () => {
  const [webhooks, setWebhooks] = useState<WebhookEndpoint[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [showCreateModal, setShowCreateModal] = useState<boolean>(false);
  const [showSecretModal, setShowSecretModal] = useState<{ secret: string; name: string } | null>(null);
  const [copiedSecret, setCopiedSecret] = useState<boolean>(false);

  // Form State
  const [formName, setFormName] = useState<string>('');
  const [formUrl, setFormUrl] = useState<string>('');
  const [formDesc, setFormDesc] = useState<string>('');
  const [formEvents, setFormEvents] = useState<string[]>(['*']);
  const [submitting, setSubmitting] = useState<boolean>(false);

  // Deliveries Modal
  const [selectedWebhook, setSelectedWebhook] = useState<WebhookEndpoint | null>(null);
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [loadingDeliveries, setLoadingDeliveries] = useState<boolean>(false);
  const [testingId, setTestingId] = useState<string | null>(null);

  const toast = useToast();

  const fetchWebhooks = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.listWebhooks();
      setWebhooks(data || []);
    } catch {
      toast.error('Failed to fetch webhooks');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    fetchWebhooks();
  }, [fetchWebhooks]);

  const handleCreateWebhook = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formName || !formUrl) {
      toast.warning('Name and Destination URL are required');
      return;
    }
    setSubmitting(true);
    try {
      const res = await api.createWebhook({
        name: formName,
        url: formUrl,
        description: formDesc,
        eventTypes: formEvents.length > 0 ? formEvents : ['*'],
        active: true,
      });
      setShowCreateModal(false);
      setFormName('');
      setFormUrl('');
      setFormDesc('');
      setFormEvents(['*']);
      setShowSecretModal({ secret: res.secret, name: res.endpoint.name });
      fetchWebhooks();
      toast.success('Webhook endpoint registered successfully');
    } catch (err: any) {
      toast.error(err.message || 'Failed to create webhook');
    } finally {
      setSubmitting(false);
    }
  };

  const handleRotateSecret = async (id: string, name: string) => {
    if (!window.confirm(`Are you sure you want to rotate the HMAC secret for "${name}"? Existing integrations will fail signature verification until updated.`)) {
      return;
    }
    try {
      const newSecret = await api.rotateWebhookSecret(id);
      setShowSecretModal({ secret: newSecret, name });
      toast.success('Secret rotated successfully');
    } catch (err: any) {
      toast.error(err.message || 'Failed to rotate secret');
    }
  };

  const handleDeleteWebhook = async (id: string, name: string) => {
    if (!window.confirm(`Are you sure you want to delete webhook "${name}"?`)) return;
    try {
      await api.deleteWebhook(id);
      setWebhooks((prev) => prev.filter((w) => w.id !== id));
      toast.success('Webhook deleted');
    } catch {
      toast.error('Failed to delete webhook');
    }
  };

  const handleTestWebhook = async (id: string) => {
    setTestingId(id);
    try {
      const delivery = await api.testWebhook(id);
      if (delivery.httpStatus >= 200 && delivery.httpStatus < 300) {
        toast.success(`Test ping sent: HTTP ${delivery.httpStatus} (${delivery.responseTimeMs}ms)`);
      } else {
        toast.warning(`Test ping completed with HTTP ${delivery.httpStatus}`);
      }
      fetchWebhooks();
    } catch (err: any) {
      toast.error(err.message || 'Test delivery failed');
    } finally {
      setTestingId(null);
    }
  };

  const handleInspectDeliveries = async (w: WebhookEndpoint) => {
    setSelectedWebhook(w);
    setLoadingDeliveries(true);
    try {
      const res = await api.listWebhookDeliveries(w.id, 20, 0);
      setDeliveries(res.deliveries || []);
    } catch {
      toast.error('Failed to fetch delivery logs');
    } finally {
      setLoadingDeliveries(false);
    }
  };

  const handleCopySecret = () => {
    if (!showSecretModal) return;
    navigator.clipboard.writeText(showSecretModal.secret);
    setCopiedSecret(true);
    setTimeout(() => setCopiedSecret(false), 2000);
    toast.info('Secret copied to clipboard');
  };

  return (
    <div className="max-w-6xl mx-auto px-4 py-8 space-y-8 animate-in fade-in duration-300">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-[#181f30] pb-6">
        <div>
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-2xl bg-indigo-600/10 border border-indigo-500/20 text-indigo-400">
              <Webhook className="w-6 h-6" />
            </div>
            <div>
              <h1 className="text-2xl font-bold text-white tracking-tight">Webhooks & Event Subscriptions</h1>
              <p className="text-xs text-slate-400 mt-0.5">
                Deliver signed JSON payloads to your servers in real-time with HMAC-SHA256 verification and automatic retry.
              </p>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={fetchWebhooks}
            className="p-2.5 rounded-xl bg-[#0f121d] hover:bg-[#181f30] border border-[#1f283d] text-slate-300 hover:text-white transition"
            title="Refresh"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
          <button
            onClick={() => setShowCreateModal(true)}
            className="flex items-center gap-2 px-4 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold shadow-lg shadow-blue-600/20 transition"
          >
            <Plus className="w-4 h-4" />
            <span>Add Webhook</span>
          </button>
        </div>
      </div>

      {/* Secret Reveal Modal */}
      {showSecretModal && (
        <div className="p-6 rounded-3xl bg-amber-500/10 border border-amber-500/30 text-amber-200 space-y-4 shadow-xl animate-in zoom-in-95">
          <div className="flex items-center gap-3">
            <ShieldCheck className="w-6 h-6 text-amber-400" />
            <div>
              <h3 className="text-sm font-bold text-white">HMAC-SHA256 Signing Secret for "{showSecretModal.name}"</h3>
              <p className="text-xs text-amber-300/80">
                Please copy and store this secret safely. For security, it will never be displayed again.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 bg-[#090b12] border border-[#1f283d] rounded-2xl p-3">
            <code className="font-mono text-xs text-amber-300 select-all flex-1 break-all">
              {showSecretModal.secret}
            </code>
            <button
              onClick={handleCopySecret}
              className="p-2 rounded-xl bg-[#141926] hover:bg-[#1c2438] text-slate-200 text-xs font-semibold flex items-center gap-1.5 transition"
            >
              {copiedSecret ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
              <span>{copiedSecret ? 'Copied' : 'Copy'}</span>
            </button>
          </div>

          <div className="flex justify-end">
            <button
              onClick={() => setShowSecretModal(null)}
              className="px-4 py-1.5 rounded-xl bg-amber-500/20 hover:bg-amber-500/30 text-amber-200 text-xs font-semibold border border-amber-500/40"
            >
              I have stored my secret securely
            </button>
          </div>
        </div>
      )}

      {/* Webhooks List */}
      {loading ? (
        <div className="p-12 text-center text-slate-500 text-xs font-mono">Loading webhook endpoints...</div>
      ) : webhooks.length === 0 ? (
        <div className="p-12 text-center rounded-3xl bg-[#090b12] border border-[#181f30] space-y-3">
          <div className="w-12 h-12 rounded-2xl bg-indigo-600/10 text-indigo-400 flex items-center justify-center mx-auto">
            <Webhook className="w-6 h-6" />
          </div>
          <h3 className="text-sm font-bold text-white">No Webhooks Configured</h3>
          <p className="text-xs text-slate-400 max-w-md mx-auto">
            Configure HTTP webhooks to receive real-time updates when instances are provisioned, invoices are created, or backups complete.
          </p>
          <button
            onClick={() => setShowCreateModal(true)}
            className="inline-flex items-center gap-2 px-4 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold mt-2"
          >
            <Plus className="w-4 h-4" />
            <span>Create First Webhook</span>
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {webhooks.map((w) => (
            <div
              key={w.id}
              className="p-5 rounded-2xl bg-[#090b12] border border-[#181f30] hover:border-[#222a3f] transition space-y-4 shadow-sm"
            >
              <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex items-center gap-3">
                    <h3 className="text-sm font-bold text-white">{w.name}</h3>
                    <span
                      className={`px-2 py-0.5 rounded text-[10px] font-mono font-semibold uppercase ${
                        w.active
                          ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                          : 'bg-slate-500/10 text-slate-400 border border-slate-500/20'
                      }`}
                    >
                      {w.active ? 'Active' : 'Disabled'}
                    </span>
                    {w.failureCount > 0 && (
                      <span className="px-2 py-0.5 rounded text-[10px] font-mono bg-rose-500/10 text-rose-400 border border-rose-500/20">
                        {w.failureCount} Failures
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-2 font-mono text-xs text-slate-300">
                    <span className="text-slate-500">POST</span>
                    <span className="text-blue-400 break-all">{w.url}</span>
                  </div>
                  {w.description && <p className="text-xs text-slate-400">{w.description}</p>}
                </div>

                <div className="flex items-center gap-2 flex-wrap">
                  <button
                    onClick={() => handleTestWebhook(w.id)}
                    disabled={testingId === w.id}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-[#121624] hover:bg-[#1a2034] border border-[#1f283d] text-slate-200 text-xs font-semibold transition"
                    title="Send test ping event"
                  >
                    <Send className={`w-3.5 h-3.5 text-blue-400 ${testingId === w.id ? 'animate-spin' : ''}`} />
                    <span>Test Ping</span>
                  </button>

                  <button
                    onClick={() => handleInspectDeliveries(w)}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-[#121624] hover:bg-[#1a2034] border border-[#1f283d] text-slate-200 text-xs font-semibold transition"
                    title="View delivery logs"
                  >
                    <History className="w-3.5 h-3.5 text-indigo-400" />
                    <span>Deliveries</span>
                  </button>

                  <button
                    onClick={() => handleRotateSecret(w.id, w.name)}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-[#121624] hover:bg-[#1a2034] border border-[#1f283d] text-amber-300 text-xs font-semibold transition"
                    title="Rotate secret"
                  >
                    <Key className="w-3.5 h-3.5" />
                    <span>Rotate</span>
                  </button>

                  <button
                    onClick={() => handleDeleteWebhook(w.id, w.name)}
                    className="p-1.5 rounded-xl bg-[#121624] hover:bg-rose-600/20 text-slate-400 hover:text-rose-400 border border-[#1f283d] transition"
                    title="Delete webhook"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>

              {/* Subscribed Events Tags */}
              <div className="pt-2 border-t border-[#141926] flex items-center justify-between flex-wrap gap-2 text-xs">
                <div className="flex items-center gap-1.5 flex-wrap">
                  <span className="text-slate-500 font-mono text-[11px]">Events:</span>
                  {w.eventTypes.map((et) => (
                    <span
                      key={et}
                      className="px-2 py-0.5 rounded text-[10px] font-mono bg-[#141926] text-slate-300 border border-[#1f283d]"
                    >
                      {et}
                    </span>
                  ))}
                </div>

                <div className="text-[10px] font-mono text-slate-500 flex items-center gap-3">
                  {w.lastDeliveryAt && (
                    <span>Last Delivery: {new Date(w.lastDeliveryAt).toLocaleTimeString()}</span>
                  )}
                  <span>Created: {new Date(w.createdAt).toLocaleDateString()}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Signature Verification Guide */}
      <div className="p-6 rounded-3xl bg-[#090b12] border border-[#181f30] space-y-4">
        <div className="flex items-center gap-3">
          <Code2 className="w-5 h-5 text-blue-400" />
          <h3 className="text-sm font-bold text-white">HMAC-SHA256 Webhook Verification</h3>
        </div>
        <p className="text-xs text-slate-400 leading-relaxed">
          Aurora signs every payload using your endpoint's secret. Compute the HMAC-SHA256 signature over{' '}
          <code className="text-blue-300 bg-[#121624] px-1.5 py-0.5 rounded">timestamp + "." + raw_body</code> and verify against the{' '}
          <code className="text-blue-300 bg-[#121624] px-1.5 py-0.5 rounded">X-Aurora-Signature</code> header.
        </p>

        <div className="p-4 rounded-2xl bg-[#04060a] border border-[#141926] font-mono text-xs text-slate-300 overflow-x-auto">
          <pre className="text-[11px] leading-relaxed">
{`# Node.js Example
const crypto = require('crypto');

function verifyAuroraWebhook(rawBody, signatureHeader, timestampHeader, secret) {
  const payload = timestampHeader + '.' + rawBody;
  const expectedSig = 'sha256=' + crypto.createHmac('sha256', secret).update(payload).digest('hex');
  return crypto.timingSafeEqual(Buffer.from(signatureHeader), Buffer.from(expectedSig));
}`}
          </pre>
        </div>
      </div>

      {/* Create Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-[#0b0e17] border border-[#1f283d] rounded-3xl max-w-lg w-full p-6 space-y-6 shadow-2xl animate-in zoom-in-95">
            <div className="flex items-center justify-between border-b border-[#181f30] pb-4">
              <div className="flex items-center gap-2.5">
                <Webhook className="w-5 h-5 text-blue-400" />
                <h3 className="text-base font-bold text-white">Create Webhook Endpoint</h3>
              </div>
              <button
                onClick={() => setShowCreateModal(false)}
                className="text-slate-400 hover:text-white text-xs font-bold"
              >
                ✕
              </button>
            </div>

            <form onSubmit={handleCreateWebhook} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1.5">Webhook Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Production Billing Hook"
                  value={formName}
                  onChange={(e) => setFormName(e.target.value)}
                  className="w-full bg-[#090b12] border border-[#1f283d] rounded-xl px-3.5 py-2 text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1.5">Destination URL (HTTPS)</label>
                <input
                  type="url"
                  required
                  placeholder="https://api.yourdomain.com/webhooks/aurora"
                  value={formUrl}
                  onChange={(e) => setFormUrl(e.target.value)}
                  className="w-full bg-[#090b12] border border-[#1f283d] rounded-xl px-3.5 py-2 text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500 font-mono"
                />
                <p className="text-[10px] text-slate-500 mt-1">
                  SSRF protected. Private IPs, localhost, and cloud metadata targets are prohibited.
                </p>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1.5">Description (Optional)</label>
                <input
                  type="text"
                  placeholder="e.g. Syncs compute events with internal Slackbot"
                  value={formDesc}
                  onChange={(e) => setFormDesc(e.target.value)}
                  className="w-full bg-[#090b12] border border-[#1f283d] rounded-xl px-3.5 py-2 text-xs text-white placeholder-slate-600 focus:outline-none focus:border-blue-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-2">Subscribed Event Types</label>
                <div className="grid grid-cols-1 gap-2 max-h-48 overflow-y-auto pr-1">
                  {SUPPORTED_EVENT_TYPES.map((et) => {
                    const isChecked = formEvents.includes(et.id);
                    return (
                      <label
                        key={et.id}
                        className="flex items-center gap-2 p-2 rounded-xl bg-[#090b12] border border-[#181f30] text-xs cursor-pointer hover:border-[#2a344d]"
                      >
                        <input
                          type="checkbox"
                          checked={isChecked}
                          onChange={() => {
                            if (isChecked) {
                              setFormEvents(formEvents.filter((id) => id !== et.id));
                            } else {
                              setFormEvents([...formEvents, et.id]);
                            }
                          }}
                          className="rounded border-[#1f283d] text-blue-600 focus:ring-0"
                        />
                        <span className="text-slate-300 font-mono text-[11px]">{et.label}</span>
                      </label>
                    );
                  })}
                </div>
              </div>

              <div className="flex justify-end gap-3 pt-4 border-t border-[#181f30]">
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="px-4 py-2 rounded-xl bg-[#121624] hover:bg-[#1a2034] text-slate-300 text-xs font-semibold"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting}
                  className="px-4 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white text-xs font-semibold shadow-lg shadow-blue-600/20"
                >
                  {submitting ? 'Creating...' : 'Create Endpoint'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Deliveries History Modal */}
      {selectedWebhook && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-[#0b0e17] border border-[#1f283d] rounded-3xl max-w-2xl w-full p-6 space-y-6 shadow-2xl animate-in zoom-in-95">
            <div className="flex items-center justify-between border-b border-[#181f30] pb-4">
              <div className="flex items-center gap-2.5">
                <History className="w-5 h-5 text-indigo-400" />
                <div>
                  <h3 className="text-base font-bold text-white">Delivery History: {selectedWebhook.name}</h3>
                  <p className="text-xs text-slate-400 font-mono">{selectedWebhook.url}</p>
                </div>
              </div>
              <button
                onClick={() => setSelectedWebhook(null)}
                className="text-slate-400 hover:text-white text-xs font-bold"
              >
                ✕
              </button>
            </div>

            <div className="space-y-3 max-h-96 overflow-y-auto pr-1">
              {loadingDeliveries ? (
                <div className="text-xs text-slate-500 text-center py-8 font-mono">Loading deliveries...</div>
              ) : deliveries.length === 0 ? (
                <div className="text-xs text-slate-500 text-center py-8">No deliveries recorded yet.</div>
              ) : (
                deliveries.map((d) => (
                  <div
                    key={d.id}
                    className="p-3.5 rounded-2xl bg-[#090b12] border border-[#181f30] space-y-2 text-xs font-mono"
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <span
                          className={`px-2 py-0.5 rounded text-[10px] uppercase font-bold ${
                            d.status === 'delivered'
                              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                              : d.status === 'dead_letter'
                              ? 'bg-rose-500/10 text-rose-400 border border-rose-500/20'
                              : 'bg-amber-500/10 text-amber-400 border border-amber-500/20'
                          }`}
                        >
                          {d.status}
                        </span>
                        <span className="text-white font-bold">{d.eventType}</span>
                      </div>
                      <span className="text-slate-400">HTTP {d.httpStatus} ({d.responseTimeMs}ms)</span>
                    </div>

                    <div className="flex items-center justify-between text-[10px] text-slate-500">
                      <span>Attempt #{d.attempt}</span>
                      <span>{new Date(d.createdAt).toLocaleString()}</span>
                    </div>

                    {d.error && (
                      <div className="p-2 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-[11px]">
                        {d.error}
                      </div>
                    )}
                  </div>
                ))
              )}
            </div>

            <div className="flex justify-end pt-2">
              <button
                onClick={() => setSelectedWebhook(null)}
                className="px-4 py-2 rounded-xl bg-[#121624] hover:bg-[#1a2034] text-slate-300 text-xs font-semibold"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
