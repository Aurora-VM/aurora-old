import React, { useState } from 'react';
import {
  Server,
  Search,
  PlusCircle,
  Play,
  Square,
  RotateCw,
  Terminal,
  Trash2,
} from 'lucide-react';
import { Instance, api } from '../lib/api';
import { useToast } from '../context/ToastContext';
import { useJobs } from '../context/JobsContext';
import { ConfirmDialog } from '../components/ConfirmDialog';

interface InstanceListProps {
  instances: Instance[];
  loading: boolean;
  onRefresh: () => void;
  navigate: (path: string) => void;
}

export const InstanceList: React.FC<InstanceListProps> = ({
  instances,
  loading,
  onRefresh,
  navigate,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'running' | 'stopped' | 'error'>('all');
  const [deleteTarget, setDeleteTarget] = useState<Instance | null>(null);
  const [actionLoading, setActionLoading] = useState(false);

  const toast = useToast();
  const { addJob, updateJob } = useJobs();

  const handlePowerAction = async (inst: Instance, action: 'start' | 'stop' | 'restart' | 'force_stop') => {
    const jobId = addJob({
      type: `instance_power_${action}`,
      title: `${action.toUpperCase()} ${inst.name}`,
      targetId: inst.id,
      targetName: inst.name,
    });

    try {
      await api.powerAction(inst.id, action);
      updateJob(jobId, { status: 'completed' });
      toast.success(`Instance ${action} command sent`, inst.name);
      onRefresh();
    } catch (err: any) {
      updateJob(jobId, { status: 'failed', errorMessage: err.message });
      toast.error(`Failed to ${action} instance`, err.message);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setActionLoading(true);
    const jobId = addJob({
      type: 'instance_delete',
      title: `Delete ${deleteTarget.name}`,
      targetId: deleteTarget.id,
      targetName: deleteTarget.name,
    });

    try {
      await api.deleteInstance(deleteTarget.id);
      updateJob(jobId, { status: 'completed' });
      toast.success('Instance deleted successfully', deleteTarget.name);
      setDeleteTarget(null);
      onRefresh();
    } catch (err: any) {
      updateJob(jobId, { status: 'failed', errorMessage: err.message });
      toast.error('Failed to delete instance', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  const filteredInstances = instances.filter((inst) => {
    const matchesQuery =
      inst.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      (inst.ipv4Address && inst.ipv4Address.includes(searchQuery)) ||
      inst.image.toLowerCase().includes(searchQuery.toLowerCase());

    if (!matchesQuery) return false;
    if (statusFilter === 'all') return true;
    if (statusFilter === 'running') return inst.status === 'running';
    if (statusFilter === 'stopped') return inst.status === 'stopped';
    if (statusFilter === 'error') return inst.status === 'error' || inst.status === 'suspended';
    return true;
  });

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header & Controls Toolbar */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-[#181f30] pb-5">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Server className="w-5 h-5 text-blue-400" />
            <span>Virtual Machines & Containers</span>
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Manage compute instances, power states, networking, and direct console connections.
          </p>
        </div>

        <button
          onClick={() => navigate('/instances/new')}
          className="flex items-center gap-2 px-4 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-lg shadow-blue-600/20 transition self-start md:self-auto"
        >
          <PlusCircle className="w-4 h-4" />
          <span>Deploy New Instance</span>
        </button>
      </div>

      {/* Filter and Search Bar */}
      <div className="p-4 rounded-2xl bg-[#0f121d] border border-[#1c2235] flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3">
        {/* Search */}
        <div className="relative flex-1">
          <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-3" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search by instance name, IP address, or image..."
            className="w-full pl-10 pr-4 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-xs text-white placeholder-slate-500 focus:outline-none focus:border-blue-500"
          />
        </div>

        {/* Status Tabs */}
        <div className="flex items-center gap-1 bg-[#090b12] border border-[#1e2538] rounded-xl p-1 self-start md:self-auto">
          {(['all', 'running', 'stopped', 'error'] as const).map((s) => (
            <button
              key={s}
              onClick={() => setStatusFilter(s)}
              className={`px-3 py-1.5 rounded-lg text-xs font-medium uppercase font-mono transition ${
                statusFilter === s
                  ? 'bg-blue-600 text-white'
                  : 'text-slate-400 hover:text-white'
              }`}
            >
              {s}
            </button>
          ))}
        </div>
      </div>

      {/* Instance Table */}
      <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-[#181f30] bg-[#090b12] text-slate-400 font-mono">
                <th className="py-3.5 px-4">Instance</th>
                <th className="py-3.5 px-4">Status</th>
                <th className="py-3.5 px-4">IP Address</th>
                <th className="py-3.5 px-4">Resources</th>
                <th className="py-3.5 px-4">Created</th>
                <th className="py-3.5 px-4 text-right">Quick Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#141a29]">
              {loading ? (
                <tr>
                  <td colSpan={6} className="py-12 text-center text-slate-500">
                    Loading instance workloads...
                  </td>
                </tr>
              ) : filteredInstances.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-12 text-center text-slate-500 space-y-2">
                    <p>No instances matching your filter criteria.</p>
                  </td>
                </tr>
              ) : (
                filteredInstances.map((inst) => (
                  <tr key={inst.id} className="hover:bg-[#121624] transition">
                    {/* Name & Type */}
                    <td className="py-3.5 px-4">
                      <div
                        onClick={() => navigate(`/instances/${inst.id}`)}
                        className="cursor-pointer group flex items-center gap-2.5"
                      >
                        <div className="p-2 rounded-lg bg-[#141824] group-hover:bg-blue-600/20 text-slate-300 group-hover:text-blue-400 transition">
                          <Server className="w-4 h-4" />
                        </div>
                        <div>
                          <div className="font-bold text-white group-hover:text-blue-400 transition">
                            {inst.name}
                          </div>
                          <div className="text-[10px] text-slate-400 font-mono mt-0.5">
                            {inst.image} • {inst.type}
                          </div>
                        </div>
                      </div>
                    </td>

                    {/* Status Badge */}
                    <td className="py-3.5 px-4">
                      <span
                        className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[10px] font-mono uppercase font-semibold border ${
                          inst.status === 'running'
                            ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
                            : inst.status === 'stopped'
                            ? 'bg-slate-500/10 text-slate-400 border-slate-500/20'
                            : 'bg-rose-500/10 text-rose-400 border-rose-500/20'
                        }`}
                      >
                        <span
                          className={`w-1.5 h-1.5 rounded-full ${
                            inst.status === 'running'
                              ? 'bg-emerald-400'
                              : inst.status === 'stopped'
                              ? 'bg-slate-400'
                              : 'bg-rose-400'
                          }`}
                        />
                        <span>{inst.status}</span>
                      </span>
                    </td>

                    {/* Primary IP */}
                    <td className="py-3.5 px-4 font-mono text-slate-300">
                      {inst.ipv4Address ? (
                        <span className="px-2 py-0.5 rounded bg-[#141824] border border-[#232a3d]">
                          {inst.ipv4Address}
                        </span>
                      ) : (
                        <span className="text-slate-500">—</span>
                      )}
                    </td>

                    {/* Resources */}
                    <td className="py-3.5 px-4 font-mono text-slate-300">
                      <span>{inst.cpuCores} vCPU</span> •{' '}
                      <span>{(inst.memoryBytes / 1073741824).toFixed(1)} GB</span> •{' '}
                      <span>{(inst.storageBytes / 1073741824).toFixed(0)} GB</span>
                    </td>

                    {/* Created Time */}
                    <td className="py-3.5 px-4 font-mono text-slate-400">
                      {new Date(inst.createdAt).toLocaleDateString()}
                    </td>

                    {/* Actions */}
                    <td className="py-3.5 px-4 text-right space-x-1">
                      {inst.status === 'stopped' ? (
                        <button
                          onClick={() => handlePowerAction(inst, 'start')}
                          className="p-1.5 rounded-lg bg-[#141824] hover:bg-emerald-950/40 text-emerald-400 transition"
                          title="Start Instance"
                        >
                          <Play className="w-3.5 h-3.5" />
                        </button>
                      ) : (
                        <>
                          <button
                            onClick={() => handlePowerAction(inst, 'restart')}
                            className="p-1.5 rounded-lg bg-[#141824] hover:bg-[#1c2233] text-slate-300 hover:text-white transition"
                            title="Restart Instance"
                          >
                            <RotateCw className="w-3.5 h-3.5" />
                          </button>
                          <button
                            onClick={() => handlePowerAction(inst, 'stop')}
                            className="p-1.5 rounded-lg bg-[#141824] hover:bg-amber-950/40 text-amber-400 transition"
                            title="Stop Instance"
                          >
                            <Square className="w-3.5 h-3.5" />
                          </button>
                        </>
                      )}

                      <button
                        onClick={() => navigate(`/instances/${inst.id}?tab=console`)}
                        className="p-1.5 rounded-lg bg-[#141824] hover:bg-blue-950/40 text-blue-400 transition"
                        title="Open Interactive Console"
                      >
                        <Terminal className="w-3.5 h-3.5" />
                      </button>

                      <button
                        onClick={() => setDeleteTarget(inst)}
                        className="p-1.5 rounded-lg bg-[#141824] hover:bg-rose-950/40 text-rose-400 transition"
                        title="Delete Instance"
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

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={`Delete Instance "${deleteTarget?.name}"?`}
        message="Are you sure you want to permanently delete this compute instance and destroy its attached storage? This operation is immediate and irreversible."
        confirmText="Delete Instance"
        isDestructive={true}
        loading={actionLoading}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
