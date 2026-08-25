import React, { useEffect, useState, useCallback } from 'react';
import {
  Activity,
  RefreshCw,
  Radio,
  Eye,
  AlertOctagon,
  Search,
  CheckCircle2,
} from 'lucide-react';
import { api, AuroraEvent, WebhookDelivery } from '../../lib/api';
import { useToast } from '../../context/ToastContext';

export const AdminEvents: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'events' | 'dead_letters'>('events');
  const [events, setEvents] = useState<AuroraEvent[]>([]);
  const [totalEvents, setTotalEvents] = useState<number>(0);
  const [deadLetters, setDeadLetters] = useState<WebhookDelivery[]>([]);
  const [totalDeadLetters, setTotalDeadLetters] = useState<number>(0);
  const [loading, setLoading] = useState<boolean>(true);

  // Filters
  const [tenantFilter, setTenantFilter] = useState<string>('');
  const [typeFilter, setTypeFilter] = useState<string>('');
  const [selectedEvent, setSelectedEvent] = useState<AuroraEvent | null>(null);

  const toast = useToast();

  const fetchEvents = useCallback(async () => {
    setLoading(true);
    try {
      if (activeTab === 'events') {
        const res = await api.adminListEvents({
          tenantId: tenantFilter || undefined,
          type: typeFilter || undefined,
          limit: 50,
          offset: 0,
        });
        setEvents(res.events || []);
        setTotalEvents(res.total || 0);
      } else {
        const res = await api.adminListDeliveries({
          status: 'dead_letter',
          tenantId: tenantFilter || undefined,
          limit: 50,
          offset: 0,
        });
        setDeadLetters(res.deliveries || []);
        setTotalDeadLetters(res.total || 0);
      }
    } catch {
      toast.error('Failed to fetch platform events');
    } finally {
      setLoading(false);
    }
  }, [activeTab, tenantFilter, typeFilter, toast]);

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  return (
    <div className="max-w-7xl mx-auto px-4 py-8 space-y-6 animate-in fade-in duration-300">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-[#181f30] pb-6">
        <div>
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-2xl bg-cyan-600/10 border border-cyan-500/20 text-cyan-400">
              <Activity className="w-6 h-6" />
            </div>
            <div>
              <h1 className="text-2xl font-bold text-white tracking-tight">Platform Event & Delivery Hub</h1>
              <p className="text-xs text-slate-400 mt-0.5">
                Cluster-wide audit stream, webhook dispatch telemetry, and dead-letter queue inspector.
              </p>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2 px-3 py-1.5 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs font-mono">
            <Radio className="w-3.5 h-3.5 animate-pulse" />
            <span>Event Bus Active</span>
          </div>
          <button
            onClick={fetchEvents}
            className="p-2.5 rounded-xl bg-[#0f121d] hover:bg-[#181f30] border border-[#1f283d] text-slate-300 hover:text-white transition"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Tabs & Search */}
      <div className="flex flex-wrap items-center justify-between gap-4 bg-[#090b12] p-3 rounded-2xl border border-[#181f30]">
        <div className="flex items-center gap-2">
          <button
            onClick={() => setActiveTab('events')}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition ${
              activeTab === 'events' ? 'bg-cyan-600 text-white' : 'text-slate-400 hover:text-white'
            }`}
          >
            Platform Events ({totalEvents})
          </button>
          <button
            onClick={() => setActiveTab('dead_letters')}
            className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold flex items-center gap-1.5 transition ${
              activeTab === 'dead_letters' ? 'bg-rose-600 text-white' : 'text-slate-400 hover:text-white'
            }`}
          >
            <AlertOctagon className="w-3.5 h-3.5" />
            <span>Dead-Letter Queue ({totalDeadLetters})</span>
          </button>
        </div>

        <div className="flex items-center gap-3 flex-wrap">
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-slate-500 absolute left-3 top-2.5" />
            <input
              type="text"
              placeholder="Filter by Tenant ID..."
              value={tenantFilter}
              onChange={(e) => setTenantFilter(e.target.value)}
              className="bg-[#121624] border border-[#1f283d] text-slate-200 text-xs rounded-lg pl-8 pr-3 py-1.5 focus:outline-none focus:border-cyan-500 font-mono"
            />
          </div>

          <div className="relative">
            <input
              type="text"
              placeholder="Filter Event Type (e.g. instance.*)..."
              value={typeFilter}
              onChange={(e) => setTypeFilter(e.target.value)}
              className="bg-[#121624] border border-[#1f283d] text-slate-200 text-xs rounded-lg px-3 py-1.5 focus:outline-none focus:border-cyan-500 font-mono"
            />
          </div>
        </div>
      </div>

      {/* Main Content */}
      {loading ? (
        <div className="p-12 text-center text-slate-500 text-xs font-mono">Streaming platform telemetry...</div>
      ) : activeTab === 'events' ? (
        events.length === 0 ? (
          <div className="p-12 text-center rounded-3xl bg-[#090b12] border border-[#181f30] space-y-3">
            <Activity className="w-8 h-8 text-cyan-400 mx-auto" />
            <h3 className="text-sm font-bold text-white">No Events Found</h3>
            <p className="text-xs text-slate-400">No telemetry events match your active filters.</p>
          </div>
        ) : (
          <div className="overflow-x-auto rounded-2xl border border-[#181f30] bg-[#090b12]">
            <table className="w-full text-left text-xs font-mono">
              <thead className="bg-[#0e121f] text-slate-400 border-b border-[#181f30]">
                <tr>
                  <th className="p-3.5 font-semibold">Timestamp</th>
                  <th className="p-3.5 font-semibold">Event Type</th>
                  <th className="p-3.5 font-semibold">Tenant</th>
                  <th className="p-3.5 font-semibold">Resource</th>
                  <th className="p-3.5 font-semibold text-right">Payload</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#141926] text-slate-300">
                {events.map((ev) => (
                  <tr key={ev.id} className="hover:bg-[#0f1424] transition">
                    <td className="p-3.5 text-slate-500 whitespace-nowrap">
                      {new Date(ev.timestamp).toLocaleTimeString()}
                    </td>
                    <td className="p-3.5">
                      <span className="px-2 py-0.5 rounded text-[11px] bg-cyan-500/10 text-cyan-400 border border-cyan-500/20 font-bold">
                        {ev.type}
                      </span>
                    </td>
                    <td className="p-3.5 text-slate-400">{ev.tenantId}</td>
                    <td className="p-3.5">
                      <span className="text-white">{ev.resourceType}</span>
                      <span className="text-slate-500 ml-1.5">({ev.resourceId})</span>
                    </td>
                    <td className="p-3.5 text-right">
                      <button
                        onClick={() => setSelectedEvent(ev)}
                        className="p-1.5 rounded-lg bg-[#141926] hover:bg-[#1f283d] text-slate-300 hover:text-white transition"
                        title="View payload"
                      >
                        <Eye className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      ) : deadLetters.length === 0 ? (
        <div className="p-12 text-center rounded-3xl bg-[#090b12] border border-[#181f30] space-y-3">
          <CheckCircle2 className="w-8 h-8 text-emerald-400 mx-auto" />
          <h3 className="text-sm font-bold text-white">Dead-Letter Queue Clean</h3>
          <p className="text-xs text-slate-400">All webhook deliveries completed successfully or are currently scheduled for retry.</p>
        </div>
      ) : (
        <div className="space-y-3">
          {deadLetters.map((dl) => (
            <div
              key={dl.id}
              className="p-4 rounded-2xl bg-[#090b12] border border-rose-500/30 text-xs font-mono space-y-2"
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="px-2 py-0.5 rounded text-[10px] bg-rose-500/20 text-rose-400 font-bold uppercase">
                    DEAD_LETTER (6 Attempts Exceeded)
                  </span>
                  <span className="text-white font-bold">{dl.eventType}</span>
                </div>
                <span className="text-slate-400">HTTP {dl.httpStatus}</span>
              </div>

              <div className="flex items-center justify-between text-[11px] text-slate-400">
                <span>Webhook ID: {dl.webhookId}</span>
                <span>Tenant: {dl.tenantId}</span>
                <span>Created: {new Date(dl.createdAt).toLocaleString()}</span>
              </div>

              {dl.error && (
                <div className="p-2.5 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-[11px]">
                  Error: {dl.error}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Payload Modal */}
      {selectedEvent && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-[#0b0e17] border border-[#1f283d] rounded-3xl max-w-xl w-full p-6 space-y-4 shadow-2xl animate-in zoom-in-95 font-mono">
            <div className="flex items-center justify-between border-b border-[#181f30] pb-3">
              <div>
                <h3 className="text-sm font-bold text-white">{selectedEvent.type}</h3>
                <p className="text-[11px] text-slate-400">Event ID: {selectedEvent.id}</p>
              </div>
              <button
                onClick={() => setSelectedEvent(null)}
                className="text-slate-400 hover:text-white text-xs font-bold font-sans"
              >
                ✕
              </button>
            </div>

            <div className="p-4 rounded-2xl bg-[#04060a] border border-[#141926] text-xs text-cyan-300 overflow-x-auto max-h-80">
              <pre className="text-[11px] leading-relaxed">
                {JSON.stringify(selectedEvent, null, 2)}
              </pre>
            </div>

            <div className="flex justify-end">
              <button
                onClick={() => setSelectedEvent(null)}
                className="px-4 py-1.5 rounded-xl bg-[#121624] hover:bg-[#1a2034] text-slate-300 text-xs font-semibold font-sans"
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
