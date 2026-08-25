import React, { useEffect, useState } from 'react';
import {
  Server,
  Activity,
  Shield,
  HardDrive,
  Network,
  Cpu,
  PlusCircle,
  PlayCircle,
  FileCheck,
  ArrowRight,
  RefreshCw,
  Layers,
} from 'lucide-react';
import { Node, Instance, StoragePool, IPAMPool, AuditLog, api } from '../../lib/api';

interface AdminDashboardProps {
  navigate: (path: string) => void;
}

export const AdminDashboard: React.FC<AdminDashboardProps> = ({ navigate }) => {
  const [nodes, setNodes] = useState<Node[]>([]);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [pools, setPools] = useState<StoragePool[]>([]);
  const [ipPools, setIpPools] = useState<IPAMPool[]>([]);
  const [recentLogs, setRecentLogs] = useState<AuditLog[]>([]);
  const [loading, setLoading] = useState<boolean>(true);

  const fetchClusterState = async () => {
    setLoading(true);
    try {
      const [nList, instList, stPools, ipList, auditRes] = await Promise.all([
        api.listNodes().catch(() => []),
        api.listInstances().catch(() => []),
        api.listStoragePools().catch(() => []),
        api.listIPAMPools().catch(() => []),
        api.listAuditLogs(5, 0).catch(() => ({ logs: [], total: 0 })),
      ]);
      setNodes(nList);
      setInstances(instList);
      setPools(stPools);
      setIpPools(ipList);
      setRecentLogs(auditRes.logs || []);
    } catch {
      // offline fallback
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchClusterState();
  }, []);

  const onlineNodes = nodes.filter((n) => n.status === 'online').length;
  const maintenanceNodes = nodes.filter((n) => n.maintenanceMode || n.status === 'maintenance').length;
  const offlineNodes = nodes.filter((n) => n.status === 'offline').length;

  const runningInstances = instances.filter((i) => i.status === 'running').length;
  const stoppedInstances = instances.filter((i) => i.status === 'stopped').length;

  const totalCoresAllocated = instances.reduce((sum, i) => sum + i.cpuCores, 0);
  const totalRamAllocatedGb = (instances.reduce((sum, i) => sum + i.memoryBytes, 0) / 1073741824).toFixed(1);
  const totalStorageAllocatedGb = (instances.reduce((sum, i) => sum + i.storageBytes, 0) / 1073741824).toFixed(0);

  return (
    <div className="space-y-8 animate-in fade-in duration-200">
      {/* Admin Infrastructure Header */}
      <div className="p-6 sm:p-8 rounded-3xl bg-gradient-to-br from-[#121627] via-[#0e1220] to-[#090b14] border border-[#1e2538] shadow-2xl relative overflow-hidden flex flex-col md:flex-row md:items-center justify-between gap-6">
        <div className="relative z-10 space-y-2">
          <div className="flex items-center gap-2 text-blue-400 text-xs font-semibold uppercase tracking-wider">
            <Shield className="w-4 h-4 text-blue-400" />
            <span>Infrastructure Operator Control Plane</span>
          </div>
          <h1 className="text-2xl sm:text-3xl font-extrabold text-white tracking-tight">
            Cluster Health & Hypervisor Fleet
          </h1>
          <p className="text-xs sm:text-sm text-slate-400 max-w-xl leading-relaxed">
            Global management of Incus virtualization nodes, storage pools, IPAM subnets, image registries, and SIEM security forwarders.
          </p>
        </div>

        <div className="relative z-10 flex flex-wrap items-center gap-3">
          <button
            onClick={() => navigate('/admin/nodes')}
            className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-lg shadow-blue-600/25 transition"
          >
            <PlusCircle className="w-4 h-4" />
            <span>Enroll Node</span>
          </button>
          <button
            onClick={fetchClusterState}
            disabled={loading}
            className="p-2.5 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-400 hover:text-white transition"
            title="Refresh Cluster Status"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Cluster Vital Stats Grid */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Hypervisor Fleet */}
        <div
          onClick={() => navigate('/admin/nodes')}
          className="p-5 rounded-2xl bg-[#0f121d] border border-[#1c2235] hover:border-slate-700 cursor-pointer transition space-y-2"
        >
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-slate-400">Hypervisor Nodes</span>
            <Server className="w-4 h-4 text-blue-400" />
          </div>
          <div className="text-2xl font-bold text-white font-mono">{nodes.length}</div>
          <div className="text-[11px] text-slate-500 font-mono">
            <span className="text-emerald-400">{onlineNodes} online</span> •{' '}
            <span className="text-amber-400">{maintenanceNodes} maint</span> •{' '}
            <span className="text-rose-400">{offlineNodes} offline</span>
          </div>
        </div>

        {/* Global Workloads */}
        <div
          onClick={() => navigate('/admin/instances')}
          className="p-5 rounded-2xl bg-[#0f121d] border border-[#1c2235] hover:border-slate-700 cursor-pointer transition space-y-2"
        >
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-slate-400">Total Workloads</span>
            <PlayCircle className="w-4 h-4 text-emerald-400" />
          </div>
          <div className="text-2xl font-bold text-emerald-400 font-mono">{instances.length}</div>
          <div className="text-[11px] text-slate-500 font-mono">
            <span>{runningInstances} running • {stoppedInstances} stopped</span>
          </div>
        </div>

        {/* Allocated Compute & RAM */}
        <div
          onClick={() => navigate('/admin/monitoring')}
          className="p-5 rounded-2xl bg-[#0f121d] border border-[#1c2235] hover:border-slate-700 cursor-pointer transition space-y-2"
        >
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-slate-400">Allocated vCPU / RAM</span>
            <Cpu className="w-4 h-4 text-purple-400" />
          </div>
          <div className="text-2xl font-bold text-white font-mono">{totalCoresAllocated} <span className="text-xs font-normal text-slate-400">Cores</span></div>
          <div className="text-[11px] text-slate-500 font-mono">
            <span>{totalRamAllocatedGb} GB RAM across cluster</span>
          </div>
        </div>

        {/* Storage Pools */}
        <div
          onClick={() => navigate('/admin/storage')}
          className="p-5 rounded-2xl bg-[#0f121d] border border-[#1c2235] hover:border-slate-700 cursor-pointer transition space-y-2"
        >
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-slate-400">Storage Pools</span>
            <HardDrive className="w-4 h-4 text-amber-400" />
          </div>
          <div className="text-2xl font-bold text-white font-mono">{pools.length || 1} <span className="text-xs font-normal text-slate-400">Pools</span></div>
          <div className="text-[11px] text-slate-500 font-mono">
            <span>{totalStorageAllocatedGb} GB provisioned volumes</span>
          </div>
        </div>
      </div>

      {/* Main Administrative Control Hub */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Nodes Fleet Status */}
        <div className="lg:col-span-2 p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Server className="w-4 h-4 text-blue-400" />
              <span>Hypervisor Nodes Status</span>
            </h3>
            <button
              onClick={() => navigate('/admin/nodes')}
              className="text-xs text-blue-400 hover:text-blue-300 flex items-center gap-1 font-semibold"
            >
              <span>Manage Nodes ({nodes.length})</span>
              <ArrowRight className="w-3.5 h-3.5" />
            </button>
          </div>

          <div className="space-y-2">
            {nodes.map((n) => (
              <div
                key={n.id}
                onClick={() => navigate(`/admin/nodes/${n.id}`)}
                className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30] hover:border-slate-700 cursor-pointer transition flex items-center justify-between font-mono text-xs"
              >
                <div className="flex items-center gap-3">
                  <span
                    className={`w-2.5 h-2.5 rounded-full ${
                      n.status === 'online'
                        ? 'bg-emerald-400'
                        : n.maintenanceMode || n.status === 'maintenance'
                        ? 'bg-amber-400'
                        : 'bg-rose-400'
                    }`}
                  />
                  <div>
                    <div className="font-bold text-white">{n.name}</div>
                    <div className="text-[10px] text-slate-400 mt-0.5">
                      FQDN: {n.fqdn} • Location: {n.locationId}
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-3">
                  <span className="text-[10px] px-2 py-0.5 rounded uppercase font-semibold bg-[#141824] text-slate-300 border border-[#232a3d]">
                    {n.maintenanceMode ? 'Maintenance' : n.status}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Quick Administrative Actions & Audit Feed */}
        <div className="space-y-6">
          {/* Quick Operators Actions */}
          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-3">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Activity className="w-4 h-4 text-purple-400" />
              <span>Operator Shortcuts</span>
            </h3>
            <div className="space-y-1.5 text-xs font-semibold">
              <button
                onClick={() => navigate('/admin/ipam')}
                className="w-full text-left p-2.5 rounded-xl bg-[#090b12] hover:bg-[#141824] text-slate-300 hover:text-white transition flex items-center justify-between"
              >
                <div className="flex items-center gap-2">
                  <Network className="w-4 h-4 text-blue-400" />
                  <span>IPAM & Subnet Pools</span>
                </div>
                <span className="text-[10px] font-mono text-slate-500">{ipPools.length} Pools</span>
              </button>

              <button
                onClick={() => navigate('/admin/templates')}
                className="w-full text-left p-2.5 rounded-xl bg-[#090b12] hover:bg-[#141824] text-slate-300 hover:text-white transition flex items-center justify-between"
              >
                <div className="flex items-center gap-2">
                  <Layers className="w-4 h-4 text-purple-400" />
                  <span>OS Templates & Images</span>
                </div>
                <span className="text-[10px] font-mono text-slate-500">Registry</span>
              </button>

              <button
                onClick={() => navigate('/admin/audit')}
                className="w-full text-left p-2.5 rounded-xl bg-[#090b12] hover:bg-[#141824] text-slate-300 hover:text-white transition flex items-center justify-between"
              >
                <div className="flex items-center gap-2">
                  <FileCheck className="w-4 h-4 text-emerald-400" />
                  <span>Audit Trail & SIEM</span>
                </div>
                <span className="text-[10px] font-mono text-slate-500">SHA-256 Ledger</span>
              </button>
            </div>
          </div>

          {/* Security & Audit Events Stream */}
          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-3">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-bold text-white flex items-center gap-2">
                <Shield className="w-4 h-4 text-emerald-400" />
                <span>Recent Audit Activity</span>
              </h3>
              <button
                onClick={() => navigate('/admin/audit')}
                className="text-[11px] text-blue-400 hover:underline"
              >
                View all
              </button>
            </div>

            <div className="space-y-2 font-mono text-xs">
              {recentLogs.slice(0, 4).map((log) => (
                <div key={log.id} className="p-2.5 rounded-xl bg-[#090b12] border border-[#181f30]">
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-white truncate">{log.action}</span>
                    <span className="text-[9px] text-slate-500">
                      {new Date(log.createdAt).toLocaleTimeString()}
                    </span>
                  </div>
                  <div className="text-[10px] text-slate-400 mt-1 truncate">
                    Hash: {log.tamperProofHash.substring(0, 16)}...
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
