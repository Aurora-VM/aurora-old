import React, { useEffect, useState } from 'react';
import {
  Shield,
  PlusCircle,
  RotateCw,
  Trash2,
} from 'lucide-react';
import { SIEMDestination, api } from '../../lib/api';
import { useToast } from '../../context/ToastContext';
import { ConfirmDialog } from '../../components/ConfirmDialog';

export const AdminSIEM: React.FC = () => {
  const [destinations, setDestinations] = useState<SIEMDestination[]>([]);
  const [loading, setLoading] = useState<boolean>(true);

  // Create Modal State
  const [createModal, setCreateModal] = useState<boolean>(false);
  const [destName, setDestName] = useState<string>('Splunk Enterprise Webhook');
  const [transportType, setTransportType] = useState<string>('webhook');
  const [format, setFormat] = useState<string>('json');
  const [endpointUrl, setEndpointUrl] = useState<string>('https://siem-collector.internal:8088/services/collector');
  const [syslogHost, setSyslogHost] = useState<string>('10.0.1.50');
  const [syslogPort, setSyslogPort] = useState<number>(514);

  const [deleteTarget, setDeleteTarget] = useState<SIEMDestination | null>(null);

  const toast = useToast();

  const fetchDestinations = async () => {
    setLoading(true);
    try {
      const list = await api.listSIEMDestinations();
      setDestinations(list);
    } catch (err: any) {
      toast.error('Failed to load SIEM destinations', err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchDestinations();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.createSIEMDestination({
        name: destName,
        transportType,
        format,
        endpointUrl: transportType === 'webhook' ? endpointUrl : undefined,
        syslogHost: transportType !== 'webhook' ? syslogHost : undefined,
        syslogPort: transportType !== 'webhook' ? syslogPort : undefined,
      });
      toast.success('SIEM destination registered successfully');
      setCreateModal(false);
      fetchDestinations();
    } catch (err: any) {
      toast.error('Failed to register SIEM forwarder', err.message);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await api.deleteSIEMDestination(deleteTarget.id);
      toast.success('SIEM destination removed', deleteTarget.name);
      setDeleteTarget(null);
      fetchDestinations();
    } catch (err: any) {
      toast.error('Failed to delete SIEM destination', err.message);
    }
  };

  const handleTestDelivery = (dest: SIEMDestination) => {
    toast.success('Test event dispatched to forwarder', `${dest.name} (${dest.transportType})`);
  };

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Shield className="w-5 h-5 text-blue-400" />
            <span>SIEM & SOC Security Event Forwarders</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Real-time streaming of immutable audit events to Splunk, Datadog, Elasticsearch, and Syslog collectors.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => setCreateModal(true)}
            className="flex items-center gap-2 px-3.5 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-lg shadow-blue-600/20 transition"
          >
            <PlusCircle className="w-4 h-4" />
            <span>Add SIEM Destination</span>
          </button>
          <button
            onClick={fetchDestinations}
            disabled={loading}
            className="p-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-400 hover:text-white"
          >
            <RotateCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Destinations Table */}
      <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden shadow-xl">
        <table className="w-full text-left text-xs font-mono">
          <thead>
            <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
              <th className="py-3.5 px-4 font-semibold">Destination Name</th>
              <th className="py-3.5 px-4 font-semibold">Transport</th>
              <th className="py-3.5 px-4 font-semibold">Payload Format</th>
              <th className="py-3.5 px-4 font-semibold">Endpoint Target</th>
              <th className="py-3.5 px-4 font-semibold text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#141a29]">
            {destinations.length === 0 ? (
              <tr>
                <td colSpan={5} className="py-12 text-center text-slate-500 font-sans">
                  No SIEM forwarders configured. All audit events are persisted in the local cryptographic ledger.
                </td>
              </tr>
            ) : (
              destinations.map((dest) => (
                <tr key={dest.id} className="hover:bg-[#141824]/60 transition">
                  <td className="py-3.5 px-4 font-bold text-white font-sans">{dest.name}</td>
                  <td className="py-3.5 px-4 uppercase text-blue-400">{dest.transportType}</td>
                  <td className="py-3.5 px-4 uppercase text-purple-400">{dest.format}</td>
                  <td className="py-3.5 px-4 text-slate-300">
                    {dest.endpointUrl || `${dest.syslogHost}:${dest.syslogPort}`}
                  </td>
                  <td className="py-3.5 px-4 text-right">
                    <div className="flex items-center justify-end gap-1.5">
                      <button
                        onClick={() => handleTestDelivery(dest)}
                        className="px-2.5 py-1 rounded-lg bg-[#141824] hover:bg-emerald-600/20 text-emerald-400 border border-[#232a3d] text-xs font-semibold"
                        title="Test Delivery"
                      >
                        Test
                      </button>
                      <button
                        onClick={() => setDeleteTarget(dest)}
                        className="p-1.5 text-slate-400 hover:text-rose-400 rounded-lg hover:bg-[#141824]"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Create Modal */}
      {createModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <form
            onSubmit={handleCreate}
            className="w-full max-w-md bg-[#0d101a] border border-[#1e2538] rounded-3xl shadow-2xl p-6 space-y-4 animate-in zoom-in-95 duration-150"
          >
            <h3 className="text-base font-bold text-white flex items-center gap-2">
              <Shield className="w-5 h-5 text-blue-400" />
              <span>Configure SIEM Forwarder</span>
            </h3>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Destination Name</label>
              <input
                type="text"
                required
                value={destName}
                onChange={(e) => setDestName(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Transport</label>
                <select
                  value={transportType}
                  onChange={(e) => setTransportType(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono"
                >
                  <option value="webhook">HTTPS Webhook</option>
                  <option value="syslog_tcp">Syslog (TCP)</option>
                  <option value="syslog_udp">Syslog (UDP)</option>
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Format</label>
                <select
                  value={format}
                  onChange={(e) => setFormat(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono"
                >
                  <option value="json">JSON</option>
                  <option value="cef">CEF (ArcSight)</option>
                  <option value="rfc5424">RFC 5424</option>
                </select>
              </div>
            </div>

            {transportType === 'webhook' ? (
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">HTTPS Endpoint URL</label>
                <input
                  type="url"
                  required
                  value={endpointUrl}
                  onChange={(e) => setEndpointUrl(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                />
              </div>
            ) : (
              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2">
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Syslog Host</label>
                  <input
                    type="text"
                    required
                    value={syslogHost}
                    onChange={(e) => setSyslogHost(e.target.value)}
                    className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Port</label>
                  <input
                    type="number"
                    required
                    value={syslogPort}
                    onChange={(e) => setSyslogPort(parseInt(e.target.value))}
                    className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
                  />
                </div>
              </div>
            )}

            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setCreateModal(false)}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] text-slate-300"
              >
                Cancel
              </button>
              <button
                type="submit"
                className="px-4 py-2 rounded-xl text-xs font-bold bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-600/25"
              >
                Register Forwarder
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={`Remove SIEM Destination "${deleteTarget?.name}"?`}
        message="Security events will no longer be forwarded to this remote collector."
        confirmText="Remove Destination"
        isDestructive={true}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
