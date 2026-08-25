import React, { useEffect, useState } from 'react';
import {
  Server,
  Search,
  PlusCircle,
  Shield,
  Copy,
  Check,
  RotateCw,
} from 'lucide-react';
import { Node, api } from '../../lib/api';
import { useToast } from '../../context/ToastContext';

interface AdminNodesProps {
  navigate: (path: string) => void;
}

export const AdminNodes: React.FC<AdminNodesProps> = ({ navigate }) => {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [statusFilter, setStatusFilter] = useState<string>('all');

  // Enrollment Modal State
  const [enrollModal, setEnrollModal] = useState<boolean>(false);
  const [locationId, setLocationId] = useState<string>('loc-us-east-1');
  const [enrollmentResult, setEnrollmentResult] = useState<{
    token: string;
    expiresAt: string;
  } | null>(null);
  const [copied, setCopied] = useState<boolean>(false);

  const toast = useToast();

  const fetchNodes = async () => {
    setLoading(true);
    try {
      const list = await api.listNodes();
      setNodes(list);
    } catch (err: any) {
      toast.error('Failed to load nodes', err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchNodes();
  }, []);

  const handleGenerateToken = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const res = await api.createEnrollmentToken(locationId, 3600);
      setEnrollmentResult({
        token: res.enrollmentToken,
        expiresAt: res.expiresAt,
      });
      toast.success('One-time enrollment token created!');
    } catch (err: any) {
      toast.error('Failed to generate token', err.message);
    }
  };

  const filteredNodes = nodes.filter((n) => {
    const matchesSearch =
      n.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      n.fqdn.toLowerCase().includes(searchQuery.toLowerCase()) ||
      n.locationId.toLowerCase().includes(searchQuery.toLowerCase());

    const matchesStatus =
      statusFilter === 'all'
        ? true
        : statusFilter === 'maintenance'
        ? n.maintenanceMode || n.status === 'maintenance'
        : n.status === statusFilter;

    return matchesSearch && matchesStatus;
  });

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Server className="w-5 h-5 text-blue-400" />
            <span>Hypervisor Fleet Management</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Monitor and manage physical bare-metal and virtual hypervisor nodes connected via gRPC mTLS.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => {
              setEnrollmentResult(null);
              setEnrollModal(true);
            }}
            className="flex items-center gap-2 px-3.5 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-lg shadow-blue-600/20 transition"
          >
            <PlusCircle className="w-4 h-4" />
            <span>Enroll New Node</span>
          </button>
          <button
            onClick={fetchNodes}
            disabled={loading}
            className="p-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-400 hover:text-white"
          >
            <RotateCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Filter Bar */}
      <div className="flex flex-col sm:flex-row gap-3">
        <div className="relative flex-1">
          <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-3" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search nodes by name, FQDN, or location..."
            className="w-full pl-10 pr-4 py-2.5 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:outline-none focus:border-blue-500"
          />
        </div>

        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0">
          {['all', 'online', 'degraded', 'unhealthy', 'draining', 'maintenance', 'offline'].map((st) => (
            <button
              key={st}
              onClick={() => setStatusFilter(st)}
              className={`px-3 py-2 rounded-xl text-xs font-semibold uppercase tracking-wider transition whitespace-nowrap ${
                statusFilter === st
                  ? 'bg-blue-600/20 text-blue-400 border border-blue-500/30'
                  : 'bg-[#0f121d] text-slate-400 border border-[#1c2235] hover:text-white'
              }`}
            >
              {st}
            </button>
          ))}
        </div>
      </div>

      {/* Nodes Table */}
      <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden shadow-xl">
        <table className="w-full text-left text-xs font-mono">
          <thead>
            <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
              <th className="py-3.5 px-4 font-semibold">Node Name / Host</th>
              <th className="py-3.5 px-4 font-semibold">Location</th>
              <th className="py-3.5 px-4 font-semibold">Health Status</th>
              <th className="py-3.5 px-4 font-semibold">Drain / Maint</th>
              <th className="py-3.5 px-4 font-semibold">Heartbeat</th>
              <th className="py-3.5 px-4 font-semibold text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#141a29]">
            {filteredNodes.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-12 text-center text-slate-500 font-sans">
                  No hypervisor nodes match your criteria.
                </td>
              </tr>
            ) : (
              filteredNodes.map((n) => (
                <tr
                  key={n.id}
                  onClick={() => navigate(`/admin/nodes/${n.id}`)}
                  className="hover:bg-[#141824]/60 cursor-pointer transition"
                >
                  <td className="py-3.5 px-4">
                    <div className="font-bold text-white font-sans">{n.name}</div>
                    <div className="text-[10px] text-slate-400 font-mono mt-0.5">{n.fqdn}</div>
                  </td>

                  <td className="py-3.5 px-4 text-slate-300">{n.locationId}</td>

                  <td className="py-3.5 px-4">
                    <span
                      className={`px-2 py-0.5 rounded text-[10px] uppercase font-bold ${
                        n.status === 'online'
                          ? 'bg-emerald-500/20 text-emerald-400'
                          : n.status === 'degraded'
                          ? 'bg-amber-500/20 text-amber-400'
                          : n.status === 'unhealthy'
                          ? 'bg-rose-500/20 text-rose-400'
                          : n.status === 'draining'
                          ? 'bg-amber-500/20 text-amber-400'
                          : 'bg-slate-500/20 text-slate-400'
                      }`}
                    >
                      {n.drainMode ? 'DRAINING' : n.status}
                    </span>
                  </td>

                  <td className="py-3.5 px-4">
                    {n.drainMode ? (
                      <span className="text-[10px] px-2 py-0.5 rounded bg-amber-500/20 text-amber-400 font-semibold uppercase">
                        Draining
                      </span>
                    ) : n.maintenanceMode ? (
                      <span className="text-[10px] px-2 py-0.5 rounded bg-amber-500/20 text-amber-400 font-semibold uppercase">
                        Maint
                      </span>
                    ) : (
                      <span className="text-[10px] text-slate-500">Active</span>
                    )}
                  </td>

                  <td className="py-3.5 px-4 text-slate-400 text-[11px]">
                    {n.lastHeartbeatAt
                      ? new Date(n.lastHeartbeatAt).toLocaleTimeString()
                      : 'Recently'}
                  </td>

                  <td className="py-3.5 px-4 text-right">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate(`/admin/nodes/${n.id}`);
                      }}
                      className="px-3 py-1 rounded-lg bg-[#141824] hover:bg-blue-600/20 text-blue-400 border border-[#232a3d] text-xs font-semibold"
                    >
                      Manage
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Node Enrollment Modal */}
      {enrollModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-lg bg-[#0d101a] border border-[#1e2538] rounded-3xl shadow-2xl p-6 space-y-5 animate-in zoom-in-95 duration-150">
            <div>
              <h3 className="text-base font-bold text-white flex items-center gap-2">
                <Shield className="w-5 h-5 text-blue-400" />
                <span>Generate Hypervisor Enrollment Token</span>
              </h3>
              <p className="text-xs text-slate-400 mt-1">
                Generates a cryptographically signed one-time token allowing a new bare-metal or VM hypervisor to connect via mTLS.
              </p>
            </div>

            {enrollmentResult ? (
              <div className="space-y-4">
                <div className="p-3.5 rounded-xl bg-amber-500/10 border border-amber-500/30 text-amber-300 text-xs font-sans">
                  ⚠️ <strong>Important:</strong> This one-time token will never be shown again. Run the enrollment command on the target hypervisor host.
                </div>

                <div className="space-y-1.5">
                  <div className="text-xs font-semibold text-slate-300">Enrollment Token:</div>
                  <div className="p-3 rounded-xl bg-[#07090e] border border-[#181f30] font-mono text-xs text-emerald-400 break-all select-all">
                    {enrollmentResult.token}
                  </div>
                </div>

                <div className="space-y-1.5">
                  <div className="text-xs font-semibold text-slate-300">Target Host Shell Command:</div>
                  <div className="p-3 rounded-xl bg-[#07090e] border border-[#181f30] font-mono text-xs text-slate-300 break-all select-all flex items-center justify-between gap-2">
                    <code>
                      aurora-agent enroll --hub {window.location.origin} --token {enrollmentResult.token}
                    </code>
                  </div>
                </div>

                <button
                  onClick={() => {
                    const cmd = `aurora-agent enroll --hub ${window.location.origin} --token ${enrollmentResult.token}`;
                    navigator.clipboard.writeText(cmd);
                    setCopied(true);
                    toast.success('Enrollment command copied to clipboard!');
                    setTimeout(() => setCopied(false), 2000);
                  }}
                  className="w-full py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold flex items-center justify-center gap-2 shadow-lg shadow-blue-600/25 transition"
                >
                  {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                  <span>{copied ? 'Command Copied' : 'Copy Shell Command'}</span>
                </button>

                <button
                  onClick={() => setEnrollModal(false)}
                  className="w-full py-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] text-slate-300 text-xs font-semibold"
                >
                  Done
                </button>
              </div>
            ) : (
              <form onSubmit={handleGenerateToken} className="space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">
                    Datacenter / Location
                  </label>
                  <select
                    value={locationId}
                    onChange={(e) => setLocationId(e.target.value)}
                    className="w-full px-3.5 py-2.5 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs focus:outline-none focus:border-blue-500 font-mono"
                  >
                    <option value="loc-us-east-1">loc-us-east-1 (N. Virginia, US)</option>
                    <option value="loc-us-west-1">loc-us-west-1 (Oregon, US)</option>
                    <option value="loc-eu-central-1">loc-eu-central-1 (Frankfurt, DE)</option>
                    <option value="loc-ap-southeast-1">loc-ap-southeast-1 (Singapore, SG)</option>
                  </select>
                </div>

                <div className="flex justify-end gap-2 pt-2">
                  <button
                    type="button"
                    onClick={() => setEnrollModal(false)}
                    className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] text-slate-300"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    className="px-4 py-2.5 rounded-xl text-xs font-bold bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-600/20"
                  >
                    Generate Token
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
