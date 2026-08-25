import React, { useEffect, useState } from 'react';
import {
  Server,
  Search,
  Play,
  Square,
  RotateCw,
  Trash2,
  User,
} from 'lucide-react';
import { Instance, api } from '../../lib/api';
import { useToast } from '../../context/ToastContext';
import { ConfirmDialog } from '../../components/ConfirmDialog';

interface AdminInstancesProps {
  navigate: (path: string) => void;
}

export const AdminInstances: React.FC<AdminInstancesProps> = ({ navigate }) => {
  const [instances, setInstances] = useState<Instance[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [deleteTarget, setDeleteTarget] = useState<Instance | null>(null);
  const [actionLoading, setActionLoading] = useState<boolean>(false);

  const toast = useToast();

  const fetchInstances = async () => {
    setLoading(true);
    try {
      const list = await api.listInstances();
      setInstances(list);
    } catch (err: any) {
      toast.error('Failed to load cross-tenant instances', err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchInstances();
  }, []);

  const handlePowerAction = async (instanceId: string, action: 'start' | 'stop' | 'restart') => {
    try {
      if (action === 'start') await api.startInstance(instanceId);
      if (action === 'stop') await api.stopInstance(instanceId);
      if (action === 'restart') await api.restartInstance(instanceId);
      toast.success(`Power command (${action}) dispatched`);
      fetchInstances();
    } catch (err: any) {
      toast.error(`Power command failed`, err.message);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setActionLoading(true);
    try {
      await api.deleteInstance(deleteTarget.id);
      toast.success('Instance destroyed by administrator', deleteTarget.name);
      setDeleteTarget(null);
      fetchInstances();
    } catch (err: any) {
      toast.error('Deletion failed', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  const filteredInstances = instances.filter((i) => {
    const matchesSearch =
      i.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      i.id.toLowerCase().includes(searchQuery.toLowerCase()) ||
      (i.userId && i.userId.toLowerCase().includes(searchQuery.toLowerCase())) ||
      (i.nodeId && i.nodeId.toLowerCase().includes(searchQuery.toLowerCase()));

    const matchesStatus = statusFilter === 'all' ? true : i.status === statusFilter;
    return matchesSearch && matchesStatus;
  });

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Server className="w-5 h-5 text-blue-400" />
            <span>Cross-Tenant Workload Administration</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Global view and administrative controls across all customer tenant virtual machines and LXC containers.
          </p>
        </div>

        <button
          onClick={fetchInstances}
          disabled={loading}
          className="p-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-400 hover:text-white self-start sm:self-auto"
        >
          <RotateCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {/* Filter Bar */}
      <div className="flex flex-col sm:flex-row gap-3">
        <div className="relative flex-1">
          <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-3" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search across tenants by instance name, ID, user ID, or node ID..."
            className="w-full pl-10 pr-4 py-2.5 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:outline-none focus:border-blue-500"
          />
        </div>

        <div className="flex items-center gap-2">
          {['all', 'running', 'stopped', 'error'].map((st) => (
            <button
              key={st}
              onClick={() => setStatusFilter(st)}
              className={`px-3 py-2 rounded-xl text-xs font-semibold uppercase tracking-wider transition ${
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

      {/* Instance Table */}
      <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden shadow-xl">
        <table className="w-full text-left text-xs font-mono">
          <thead>
            <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
              <th className="py-3.5 px-4 font-semibold">Instance Name</th>
              <th className="py-3.5 px-4 font-semibold">Tenant User ID</th>
              <th className="py-3.5 px-4 font-semibold">Node</th>
              <th className="py-3.5 px-4 font-semibold">Status</th>
              <th className="py-3.5 px-4 font-semibold">Hardware Sizing</th>
              <th className="py-3.5 px-4 font-semibold text-right">Admin Controls</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#141a29]">
            {filteredInstances.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-12 text-center text-slate-500 font-sans">
                  No workloads found matching the administrative query.
                </td>
              </tr>
            ) : (
              filteredInstances.map((inst) => (
                <tr
                  key={inst.id}
                  onClick={() => navigate(`/instances/${inst.id}`)}
                  className="hover:bg-[#141824]/60 cursor-pointer transition"
                >
                  <td className="py-3.5 px-4">
                    <div className="font-bold text-white font-sans">{inst.name}</div>
                    <div className="text-[10px] text-blue-400 font-mono mt-0.5 uppercase">
                      {inst.type} • {inst.ipv4Address || 'No IPv4'}
                    </div>
                  </td>

                  <td className="py-3.5 px-4 text-slate-300">
                    <div className="flex items-center gap-1.5">
                      <User className="w-3.5 h-3.5 text-slate-500" />
                      <span className="text-[11px] truncate max-w-[120px]">{inst.userId || 'system'}</span>
                    </div>
                  </td>

                  <td className="py-3.5 px-4 text-slate-400 text-[11px]">{inst.nodeId || 'auto'}</td>

                  <td className="py-3.5 px-4">
                    <span
                      className={`px-2 py-0.5 rounded text-[10px] uppercase font-bold ${
                        inst.status === 'running'
                          ? 'bg-emerald-500/20 text-emerald-400'
                          : 'bg-slate-700 text-slate-300'
                      }`}
                    >
                      {inst.status}
                    </span>
                  </td>

                  <td className="py-3.5 px-4 text-slate-400">
                    {inst.cpuCores} vCPU • {(inst.memoryBytes / 1073741824).toFixed(1)} GB RAM
                  </td>

                  <td className="py-3.5 px-4 text-right">
                    <div className="flex items-center justify-end gap-1.5" onClick={(e) => e.stopPropagation()}>
                      {inst.status === 'running' ? (
                        <button
                          onClick={() => handlePowerAction(inst.id, 'stop')}
                          className="p-1.5 rounded-lg bg-[#141824] text-slate-300 hover:text-amber-400"
                          title="Force Stop"
                        >
                          <Square className="w-3.5 h-3.5" />
                        </button>
                      ) : (
                        <button
                          onClick={() => handlePowerAction(inst.id, 'start')}
                          className="p-1.5 rounded-lg bg-[#141824] text-slate-300 hover:text-emerald-400"
                          title="Power On"
                        >
                          <Play className="w-3.5 h-3.5" />
                        </button>
                      )}

                      <button
                        onClick={() => handlePowerAction(inst.id, 'restart')}
                        className="p-1.5 rounded-lg bg-[#141824] text-slate-300 hover:text-blue-400"
                        title="Reboot"
                      >
                        <RotateCw className="w-3.5 h-3.5" />
                      </button>

                      <button
                        onClick={() => setDeleteTarget(inst)}
                        className="p-1.5 rounded-lg bg-[#141824] text-slate-400 hover:text-rose-400"
                        title="Destroy Workload"
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

      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={`Admin Force Destroy "${deleteTarget?.name}"?`}
        message="This action permanently deletes all workload files, root storage, and backups. This action will be recorded in the tamper-proof audit ledger."
        confirmText="Destroy Workload"
        isDestructive={true}
        loading={actionLoading}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
