import React, { useEffect, useState } from 'react';
import {
  Server,
  Activity,
  HardDrive,
  ArrowLeft,
  AlertTriangle,
  PlayCircle,
  Power,
  Radio,
  Share2,
  RefreshCw,
  XCircle,
  Box,
} from 'lucide-react';
import { Node, Instance, StoragePool, api } from '../../lib/api';
import { useToast } from '../../context/ToastContext';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { MetricChart } from '../../components/MetricChart';

interface AdminNodeDetailProps {
  nodeId: string;
  navigate: (path: string) => void;
}

export const AdminNodeDetail: React.FC<AdminNodeDetailProps> = ({ nodeId, navigate }) => {
  const [node, setNode] = useState<Node | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [pools, setPools] = useState<StoragePool[]>([]);
  const [activeTab, setActiveTab] = useState<
    'overview' | 'instances' | 'telemetry' | 'storage' | 'maintenance'
  >('overview');
  const [loading, setLoading] = useState<boolean>(true);
  const [actionLoading, setActionLoading] = useState<boolean>(false);

  // Confirmation & Evacuation Modals
  const [confirmMaintenance, setConfirmMaintenance] = useState<boolean>(false);
  const [confirmDrain, setConfirmDrain] = useState<boolean>(false);
  const [confirmRevoke, setConfirmRevoke] = useState<boolean>(false);
  const [evacuateModal, setEvacuateModal] = useState<boolean>(false);
  const [selectedDestNode, setSelectedDestNode] = useState<string>('');
  const [evacuationProgress, setEvacuationProgress] = useState<{
    migrated: number;
    failed: number;
    total: number;
    running: boolean;
  } | null>(null);

  const toast = useToast();

  const fetchNodeDetails = async () => {
    setLoading(true);
    try {
      const [n, allNodes, instList, poolList] = await Promise.all([
        api.getNode(nodeId).catch(() => null),
        api.listNodes().catch(() => []),
        api.listInstances().catch(() => []),
        api.listStoragePools(nodeId).catch(() => []),
      ]);

      if (n) {
        setNode(n);
      }
      setNodes(allNodes.filter((other) => other.id !== nodeId && other.status === 'online'));
      setInstances(instList.filter((i) => i.nodeId === nodeId));
      setPools(poolList);
    } catch (err: any) {
      toast.error('Failed to load node details', err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchNodeDetails();
  }, [nodeId]);

  const handleToggleMaintenance = async () => {
    if (!node) return;
    setActionLoading(true);
    try {
      const nextState = !node.maintenanceMode;
      await api.toggleNodeMaintenance(node.id, nextState);
      setNode({ ...node, maintenanceMode: nextState });
      toast.success(
        nextState ? 'Node entered maintenance mode' : 'Node exited maintenance mode'
      );
      setConfirmMaintenance(false);
    } catch (err: any) {
      toast.error('Failed to toggle maintenance mode', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  const handleToggleDrain = async () => {
    if (!node) return;
    setActionLoading(true);
    try {
      const nextState = !node.drainMode;
      if (nextState) {
        await api.drainNode(node.id);
        toast.success('Node marked as draining. Scheduler will not place new workloads.');
      } else {
        await api.undrainNode(node.id);
        toast.success('Node un-drained and returned to active scheduling pool.');
      }
      setNode({ ...node, drainMode: nextState, status: nextState ? 'draining' : 'online' });
      setConfirmDrain(false);
    } catch (err: any) {
      toast.error('Failed to update node drain state', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  const handleEvacuateNode = async () => {
    if (!node) return;
    setActionLoading(true);
    setEvacuationProgress({
      migrated: 0,
      failed: 0,
      total: instances.length,
      running: true,
    });
    try {
      const res = await api.evacuateNode(node.id, selectedDestNode || undefined);
      setEvacuationProgress({
        migrated: res.migratedCount,
        failed: res.failedCount,
        total: res.totalWorkloads,
        running: false,
      });
      toast.success(`Batch evacuation complete: ${res.migratedCount} workloads migrated`);
      fetchNodeDetails();
    } catch (err: any) {
      toast.error('Evacuation failed', err.message);
      setEvacuationProgress(null);
    } finally {
      setActionLoading(false);
    }
  };

  const handlePingNode = async () => {
    if (!node) return;
    try {
      await api.pingNode(node.id);
      toast.success('Node communication ping verified via gRPC');
    } catch (err: any) {
      toast.error('Ping failed', err.message);
    }
  };

  const handleRevokeNode = async () => {
    if (!node) return;
    setActionLoading(true);
    try {
      await api.revokeNode(node.id);
      toast.success('Node revoked and disconnected from control plane');
      setConfirmRevoke(false);
      navigate('/admin/nodes');
    } catch (err: any) {
      toast.error('Failed to revoke node', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  if (loading || !node) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="flex items-center gap-3 text-slate-400 text-sm font-mono">
          <div className="w-5 h-5 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
          <span>Loading hypervisor node state...</span>
        </div>
      </div>
    );
  }

  const getStatusBadge = (status: Node['status'], drainMode?: boolean) => {
    if (drainMode || status === 'draining') {
      return (
        <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-bold bg-amber-500/10 text-amber-400 border border-amber-500/20">
          <Share2 className="w-3 h-3" /> DRAINING
        </span>
      );
    }
    switch (status) {
      case 'online':
        return (
          <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
            <Radio className="w-3 h-3 text-emerald-400" /> HEALTHY
          </span>
        );
      case 'degraded':
        return (
          <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-bold bg-amber-500/10 text-amber-400 border border-amber-500/20">
            <AlertTriangle className="w-3 h-3 text-amber-400" /> DEGRADED
          </span>
        );
      case 'unhealthy':
        return (
          <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-bold bg-rose-500/10 text-rose-400 border border-rose-500/20">
            <AlertTriangle className="w-3 h-3 text-rose-400" /> UNHEALTHY
          </span>
        );
      case 'maintenance':
        return (
          <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-bold bg-amber-500/10 text-amber-400 border border-amber-500/20">
            <AlertTriangle className="w-3 h-3" /> MAINTENANCE
          </span>
        );
      case 'revoked':
        return (
          <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-bold bg-slate-500/10 text-slate-400 border border-slate-500/20">
            <Power className="w-3 h-3" /> REVOKED
          </span>
        );
      default:
        return (
          <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-bold bg-slate-500/10 text-slate-400 border border-slate-500/20">
            <Power className="w-3 h-3" /> OFFLINE
          </span>
        );
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Back Button & Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div className="flex items-center gap-4">
          <button
            onClick={() => navigate('/admin/nodes')}
            className="p-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] text-slate-300 text-xs border border-[#232a3d] transition"
          >
            <ArrowLeft className="w-4 h-4" />
          </button>
          <div>
            <div className="flex items-center gap-3">
              <h2 className="text-xl font-bold text-white">{node.name}</h2>
              {getStatusBadge(node.status, node.drainMode)}
            </div>
            <p className="text-xs text-slate-400 mt-0.5 font-mono">{node.fqdn} &bull; {node.locationId}</p>
          </div>
        </div>

        <div className="flex items-center gap-2 flex-wrap">
          <button
            onClick={handlePingNode}
            className="px-3.5 py-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-300 text-xs font-semibold flex items-center gap-1.5"
          >
            <PlayCircle className="w-3.5 h-3.5 text-emerald-400" />
            <span>gRPC Ping</span>
          </button>

          <button
            onClick={() => setConfirmDrain(true)}
            className={`px-3.5 py-2 rounded-xl border text-xs font-semibold flex items-center gap-1.5 transition ${
              node.drainMode
                ? 'bg-amber-600/20 text-amber-400 border-amber-500/30 hover:bg-amber-600/30'
                : 'bg-[#141824] hover:bg-[#1c2233] border-[#232a3d] text-slate-300'
            }`}
          >
            <Share2 className="w-3.5 h-3.5" />
            <span>{node.drainMode ? 'Un-drain Node' : 'Drain Node'}</span>
          </button>

          <button
            onClick={() => setEvacuateModal(true)}
            disabled={instances.length === 0}
            className="px-3.5 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold flex items-center gap-1.5 shadow-lg shadow-blue-600/20 disabled:opacity-50"
          >
            <Share2 className="w-3.5 h-3.5" />
            <span>Evacuate Workloads ({instances.length})</span>
          </button>
        </div>
      </div>

      {node.unhealthyReason && (
        <div className="p-4 rounded-2xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex items-center gap-3">
          <AlertTriangle className="w-5 h-5 flex-shrink-0 text-rose-400" />
          <div>
            <div className="font-bold">Health Degradation Alert:</div>
            <div>{node.unhealthyReason}</div>
          </div>
        </div>
      )}

      {/* Tabs Bar */}
      <div className="flex items-center gap-2 border-b border-[#181f30] pb-2 text-xs font-semibold overflow-x-auto">
        <button
          onClick={() => setActiveTab('overview')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'overview'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <Server className="w-4 h-4" />
          <span>Overview</span>
        </button>

        <button
          onClick={() => setActiveTab('instances')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'instances'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <Box className="w-4 h-4" />
          <span>Hosted Instances ({instances.length})</span>
        </button>

        <button
          onClick={() => setActiveTab('telemetry')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'telemetry'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <Activity className="w-4 h-4" />
          <span>Telemetry & Metrics</span>
        </button>

        <button
          onClick={() => setActiveTab('storage')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'storage'
              ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <HardDrive className="w-4 h-4" />
          <span>Storage Pools ({pools.length})</span>
        </button>

        <button
          onClick={() => setActiveTab('maintenance')}
          className={`flex items-center gap-2 px-3 py-2 rounded-xl transition ${
            activeTab === 'maintenance'
              ? 'bg-rose-600/15 text-rose-400 border border-rose-500/30'
              : 'text-slate-400 hover:text-white'
          }`}
        >
          <AlertTriangle className="w-4 h-4" />
          <span>Danger Zone</span>
        </button>
      </div>

      {/* TAB 1: OVERVIEW */}
      {activeTab === 'overview' && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
            <h3 className="text-sm font-bold text-white">System & Driver Capabilities</h3>
            <div className="space-y-2 font-mono text-xs">
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Hypervisor Engine:</span>
                <span className="text-blue-400 font-bold uppercase">
                  {node.capabilities?.hypervisor || 'Incus Driver'}
                </span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Architecture:</span>
                <span className="text-white">{node.capabilities?.arch || 'x86_64'}</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Kernel Version:</span>
                <span className="text-slate-300">{node.capabilities?.kernel || 'Linux 6.8'}</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">CPU Cores:</span>
                <span className="text-white">{node.cpuCores || node.capabilities?.cpu_cores || 16} Cores</span>
              </div>
              <div className="flex justify-between py-1.5">
                <span className="text-slate-400">KVM Virtualization:</span>
                <span className="text-purple-400 font-semibold">Enabled (/dev/kvm)</span>
              </div>
            </div>
          </div>

          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
            <h3 className="text-sm font-bold text-white">mTLS & Gateway Health</h3>
            <div className="space-y-2 font-mono text-xs">
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Transport:</span>
                <span className="text-emerald-400">gRPC v2 Bidirectional mTLS</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Last Heartbeat:</span>
                <span className="text-white">{node.lastHeartbeatAt ? new Date(node.lastHeartbeatAt).toLocaleTimeString() : 'Live'}</span>
              </div>
              <div className="flex justify-between py-1.5 border-b border-[#141a29]">
                <span className="text-slate-400">Drain Mode:</span>
                <span className={node.drainMode ? 'text-amber-400 font-bold' : 'text-emerald-400'}>
                  {node.drainMode ? 'Active (Drain)' : 'Disabled'}
                </span>
              </div>
              <div className="flex justify-between py-1.5">
                <span className="text-slate-400">Total Workloads Hosted:</span>
                <span className="text-blue-400 font-bold">{instances.length} Instances</span>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* TAB 2: INSTANCES */}
      {activeTab === 'instances' && (
        <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden">
          <table className="w-full text-left text-xs font-mono">
            <thead>
              <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
                <th className="py-3 px-4">Instance Name</th>
                <th className="py-3 px-4">Type</th>
                <th className="py-3 px-4">Status</th>
                <th className="py-3 px-4">Specs</th>
                <th className="py-3 px-4 text-right">Inspect</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#141a29]">
              {instances.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-slate-500 font-sans">
                    No active workloads currently placed on this hypervisor.
                  </td>
                </tr>
              ) : (
                instances.map((inst) => (
                  <tr key={inst.id} className="hover:bg-[#141824]/50">
                    <td className="py-3 px-4 font-bold text-white font-sans">{inst.name}</td>
                    <td className="py-3 px-4 uppercase text-blue-400">{inst.type}</td>
                    <td className="py-3 px-4">
                      <span className="px-2 py-0.5 rounded text-[10px] uppercase font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                        {inst.status}
                      </span>
                    </td>
                    <td className="py-3 px-4 text-slate-300">
                      {inst.cpuCores} vCPU / {(inst.memoryBytes / 1073741824).toFixed(1)} GB RAM
                    </td>
                    <td className="py-3 px-4 text-right">
                      <button
                        onClick={() => navigate(`/admin/instances/${inst.id}`)}
                        className="px-2.5 py-1 rounded bg-[#141824] hover:bg-blue-600/20 text-blue-400 border border-[#232a3d] text-[11px]"
                      >
                        View
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}

      {/* TAB 3: TELEMETRY */}
      {activeTab === 'telemetry' && (
        <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235]">
          <MetricChart metrics={{ cpuPercent: 32, memoryPercent: 48, diskPercent: 24, netRxBytes: 1048576, netTxBytes: 2097152 }} instanceName={node.name} />
        </div>
      )}

      {/* TAB 4: STORAGE */}
      {activeTab === 'storage' && (
        <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden">
          <table className="w-full text-left text-xs font-mono">
            <thead>
              <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
                <th className="py-3 px-4">Pool Name</th>
                <th className="py-3 px-4">Driver</th>
                <th className="py-3 px-4">Total Capacity</th>
                <th className="py-3 px-4">Used Space</th>
                <th className="py-3 px-4">Free Space</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#141a29]">
              {pools.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-slate-500 font-sans">
                    No storage pools configured on this hypervisor.
                  </td>
                </tr>
              ) : (
                pools.map((p) => (
                  <tr key={p.id} className="hover:bg-[#141824]/50">
                    <td className="py-3 px-4 font-bold text-white font-sans">{p.name}</td>
                    <td className="py-3 px-4 uppercase text-blue-400">{p.driver}</td>
                    <td className="py-3 px-4 text-slate-300">
                      {(p.totalBytes / 1073741824).toFixed(1)} GB
                    </td>
                    <td className="py-3 px-4 text-slate-300">
                      {(p.usedBytes / 1073741824).toFixed(1)} GB
                    </td>
                    <td className="py-3 px-4 text-emerald-400 font-mono">
                      {(p.freeBytes / 1073741824).toFixed(1)} GB
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}

      {/* TAB 5: MAINTENANCE & DANGER ZONE */}
      {activeTab === 'maintenance' && (
        <div className="p-6 rounded-2xl bg-[#0f121d] border border-rose-500/30 space-y-6">
          <div>
            <h3 className="text-base font-bold text-white flex items-center gap-2">
              <AlertTriangle className="w-5 h-5 text-rose-400" />
              <span>Hypervisor Node Administrative Danger Zone</span>
            </h3>
            <p className="text-xs text-slate-400 mt-1">
              Actions in this section directly impact hypervisor availability, container/VM workloads, and node authorization.
            </p>
          </div>

          <div className="p-4 rounded-xl bg-[#090b12] border border-[#181f30] flex items-center justify-between">
            <div>
              <div className="text-xs font-bold text-white">Drain Mode</div>
              <div className="text-[11px] text-slate-400 mt-0.5 font-sans">
                Marks the node as draining to prevent the scheduler from placing new workloads.
              </div>
            </div>
            <button
              onClick={() => setConfirmDrain(true)}
              className="px-4 py-2 rounded-xl bg-amber-600/20 text-amber-400 hover:bg-amber-600/30 border border-amber-500/30 text-xs font-semibold"
            >
              {node.drainMode ? 'Exit Drain Mode' : 'Enable Drain Mode'}
            </button>
          </div>

          <div className="p-4 rounded-xl bg-[#090b12] border border-[#181f30] flex items-center justify-between">
            <div>
              <div className="text-xs font-bold text-white">Maintenance Mode</div>
              <div className="text-[11px] text-slate-400 mt-0.5 font-sans">
                Temporarily suspends node scheduling for physical server maintenance.
              </div>
            </div>
            <button
              onClick={() => setConfirmMaintenance(true)}
              className="px-4 py-2 rounded-xl bg-amber-600/20 text-amber-400 hover:bg-amber-600/30 border border-amber-500/30 text-xs font-semibold"
            >
              {node.maintenanceMode ? 'Exit Maintenance' : 'Enable Maintenance'}
            </button>
          </div>

          <div className="p-4 rounded-xl bg-[#090b12] border border-rose-950/50 flex items-center justify-between">
            <div>
              <div className="text-xs font-bold text-rose-400">Revoke Node & Revoke Certificate</div>
              <div className="text-[11px] text-slate-400 mt-0.5 font-sans">
                Permanently terminates the gRPC mTLS session and marks this node as revoked.
              </div>
            </div>
            <button
              onClick={() => setConfirmRevoke(true)}
              className="px-4 py-2 rounded-xl bg-rose-600 text-white hover:bg-rose-500 text-xs font-bold shadow-lg shadow-rose-600/25"
            >
              Revoke Node
            </button>
          </div>
        </div>
      )}

      {/* Confirmation Dialogs */}
      <ConfirmDialog
        isOpen={confirmDrain}
        title={node.drainMode ? 'Exit Drain Mode?' : 'Drain Hypervisor Node?'}
        message={
          node.drainMode
            ? 'The scheduler will resume placing new workload instances on this node.'
            : 'The node will be marked as draining. No new workloads will be scheduled here.'
        }
        confirmText="Confirm"
        loading={actionLoading}
        onConfirm={handleToggleDrain}
        onCancel={() => setConfirmDrain(false)}
      />

      <ConfirmDialog
        isOpen={confirmMaintenance}
        title={node.maintenanceMode ? 'Exit Maintenance Mode?' : 'Enter Maintenance Mode?'}
        message={
          node.maintenanceMode
            ? 'The scheduler will resume placing new workload instances on this node.'
            : 'The scheduler will stop placing new workload instances on this node.'
        }
        confirmText="Confirm"
        loading={actionLoading}
        onConfirm={handleToggleMaintenance}
        onCancel={() => setConfirmMaintenance(false)}
      />

      <ConfirmDialog
        isOpen={confirmRevoke}
        title={`Permanently Revoke Node "${node.name}"?`}
        message="This action will disconnect the node, revoke its client certificate, and prevent reconnection. Make sure all instances have been evacuated before revoking."
        confirmText="Revoke Node"
        isDestructive={true}
        loading={actionLoading}
        onConfirm={handleRevokeNode}
        onCancel={() => setConfirmRevoke(false)}
      />

      {/* Evacuation Modal */}
      {evacuateModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="w-full max-w-lg bg-[#0d101a] border border-[#1e2538] rounded-3xl shadow-2xl p-6 space-y-5 animate-in zoom-in-95 duration-150">
            <div className="flex items-center justify-between border-b border-[#1c2235] pb-3">
              <h3 className="text-base font-bold text-white flex items-center gap-2">
                <Share2 className="w-5 h-5 text-blue-400" />
                <span>Evacuate All Workloads from {node.name}</span>
              </h3>
              <button onClick={() => setEvacuateModal(false)} className="p-1 text-slate-400 hover:text-white">
                <XCircle className="w-5 h-5" />
              </button>
            </div>

            <div className="space-y-4 text-xs font-sans">
              <div className="p-3.5 rounded-xl bg-blue-500/10 border border-blue-500/20 text-blue-300">
                This operation will drain the node and batch migrate <strong>{instances.length} instance(s)</strong> to healthy hypervisors without service loss.
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Destination Hypervisor (Optional)
                </label>
                <select
                  value={selectedDestNode}
                  onChange={(e) => setSelectedDestNode(e.target.value)}
                  className="w-full px-3.5 py-2.5 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:outline-none focus:border-blue-500"
                >
                  <option value="">Auto-select optimal nodes via Scheduler</option>
                  {nodes.map((n) => (
                    <option key={n.id} value={n.id}>
                      {n.name} ({n.fqdn})
                    </option>
                  ))}
                </select>
              </div>

              {evacuationProgress && (
                <div className="p-4 rounded-xl bg-[#080a11] border border-[#181f30] space-y-2">
                  <div className="flex justify-between font-mono">
                    <span className="text-slate-400">Status:</span>
                    <span className={evacuationProgress.running ? 'text-blue-400' : 'text-emerald-400 font-bold'}>
                      {evacuationProgress.running ? 'Migrating workloads...' : 'Completed'}
                    </span>
                  </div>
                  <div className="flex justify-between font-mono">
                    <span className="text-slate-400">Migrated / Total:</span>
                    <span className="text-white">
                      {evacuationProgress.migrated} / {evacuationProgress.total}
                    </span>
                  </div>
                  {evacuationProgress.failed > 0 && (
                    <div className="flex justify-between font-mono text-rose-400">
                      <span>Failed:</span>
                      <span>{evacuationProgress.failed}</span>
                    </div>
                  )}
                </div>
              )}

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setEvacuateModal(false)}
                  className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] text-slate-300"
                >
                  Close
                </button>
                <button
                  type="button"
                  disabled={actionLoading || instances.length === 0}
                  onClick={handleEvacuateNode}
                  className="px-4 py-2.5 rounded-xl text-xs font-bold bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-600/20 flex items-center gap-2 disabled:opacity-50"
                >
                  {actionLoading && <RefreshCw className="w-3.5 h-3.5 animate-spin" />}
                  <span>Start Batch Evacuation</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
