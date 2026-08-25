import React, { useEffect, useState, useCallback } from 'react';
import {
  Server,
  Terminal as TerminalIcon,
  Folder,
  Network,
  HardDrive,
  Camera,
  Activity,
  Settings,
  Play,
  Square,
  RotateCw,
  Trash2,
  Cpu,
  Monitor,
  ShieldCheck,
  Plus,
  ArrowLeft,
  AlertTriangle,
} from 'lucide-react';
import { Instance, InstanceMetrics, FirewallRule, Backup, Snapshot, api } from '../lib/api';
import { useToast } from '../context/ToastContext';
import { useJobs } from '../context/JobsContext';
import { TerminalView } from '../components/TerminalView';
import { VNCView } from '../components/VNCView';
import { FileManager } from '../components/FileManager';
import { MetricChart } from '../components/MetricChart';
import { ConfirmDialog } from '../components/ConfirmDialog';

interface InstanceDetailProps {
  instanceId: string;
  initialTab?: string;
  navigate: (path: string) => void;
}

export const InstanceDetail: React.FC<InstanceDetailProps> = ({
  instanceId,
  initialTab = 'overview',
  navigate,
}) => {
  const [activeTab, setActiveTab] = useState<string>(initialTab);
  const [instance, setInstance] = useState<Instance | null>(null);
  const [metrics, setMetrics] = useState<InstanceMetrics | null>(null);
  const [firewallRules, setFirewallRules] = useState<FirewallRule[]>([]);
  const [backups, setBackups] = useState<Backup[]>([]);
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [loading, setLoading] = useState<boolean>(true);

  // Resize Spec State
  const [editCpu, setEditCpu] = useState<number>(2);
  const [editMemoryGb, setEditMemoryGb] = useState<number>(2);
  const [editStorageGb, setEditStorageGb] = useState<number>(20);

  // Firewall Rule Modal
  const [ruleModal, setRuleModal] = useState<boolean>(false);
  const [newProto, setNewProto] = useState<'tcp' | 'udp' | 'icmp' | 'all'>('tcp');
  const [newPort, setNewPort] = useState<string>('80,443');
  const [newCidr, setNewCidr] = useState<string>('0.0.0.0/0');
  const [newAction, setNewAction] = useState<'allow' | 'deny'>('allow');

  // Confirmation Modals
  const [confirmDelete, setConfirmDelete] = useState<boolean>(false);
  const [confirmRestoreBackup, setConfirmRestoreBackup] = useState<string | null>(null);
  const [confirmRestoreSnapshot, setConfirmRestoreSnapshot] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<boolean>(false);

  const toast = useToast();
  const { addJob, updateJob } = useJobs();

  const fetchInstance = useCallback(async () => {
    try {
      const inst = await api.getInstance(instanceId);
      setInstance(inst);
      setEditCpu(inst.cpuCores);
      setEditMemoryGb(Math.round(inst.memoryBytes / 1073741824));
      setEditStorageGb(Math.round(inst.storageBytes / 1073741824));
    } catch (err: any) {
      toast.error('Failed to load instance', err.message);
    } finally {
      setLoading(false);
    }
  }, [instanceId, toast]);

  const fetchSubResources = useCallback(async () => {
    if (activeTab === 'monitoring' || activeTab === 'overview') {
      try {
        const m = await api.getInstanceMetrics(instanceId);
        setMetrics(m);
      } catch {}
    }
    if (activeTab === 'networking') {
      try {
        const fw = await api.listFirewallRules(instanceId);
        setFirewallRules(fw);
      } catch {}
    }
    if (activeTab === 'backups') {
      try {
        const b = await api.listInstanceBackups(instanceId);
        setBackups(b);
      } catch {}
    }
    if (activeTab === 'snapshots') {
      try {
        const s = await api.listSnapshots(instanceId);
        setSnapshots(s);
      } catch {}
    }
  }, [instanceId, activeTab]);

  useEffect(() => {
    fetchInstance();
  }, [fetchInstance]);

  useEffect(() => {
    fetchSubResources();
  }, [fetchSubResources]);

  const handlePower = async (action: 'start' | 'stop' | 'restart' | 'force_stop') => {
    if (!instance) return;
    const jobId = addJob({
      type: `instance_power_${action}`,
      title: `${action.toUpperCase()} ${instance.name}`,
      targetId: instance.id,
      targetName: instance.name,
    });
    try {
      await api.powerAction(instance.id, action);
      updateJob(jobId, { status: 'completed' });
      toast.success(`Power command executed`, action);
      fetchInstance();
    } catch (err: any) {
      updateJob(jobId, { status: 'failed', errorMessage: err.message });
      toast.error(`Power action failed`, err.message);
    }
  };

  const handleSaveSpec = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!instance) return;
    setActionLoading(true);
    const jobId = addJob({
      type: 'instance_resize',
      title: `Resize ${instance.name}`,
      targetId: instance.id,
      targetName: instance.name,
    });
    try {
      await api.updateInstanceSpec(
        instance.id,
        editCpu,
        editMemoryGb * 1073741824,
        editStorageGb * 1073741824
      );
      updateJob(jobId, { status: 'completed' });
      toast.success('Instance specification resized', `${editCpu} vCPU, ${editMemoryGb} GB RAM`);
      fetchInstance();
    } catch (err: any) {
      updateJob(jobId, { status: 'failed', errorMessage: err.message });
      toast.error('Resize failed', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  const handleAddFirewallRule = async (e: React.FormEvent) => {
    e.preventDefault();
    const newRule: FirewallRule = {
      id: 'rule-' + Math.random().toString(36).substring(2, 7),
      protocol: newProto,
      portRange: newPort,
      sourceCidr: newCidr,
      action: newAction,
      direction: 'inbound',
    };
    const updated = [...firewallRules, newRule];
    try {
      await api.applyFirewallRules(instanceId, updated);
      setFirewallRules(updated);
      setRuleModal(false);
      toast.success('Firewall rule added');
    } catch (err: any) {
      toast.error('Failed to apply firewall rules', err.message);
    }
  };

  const handleDeleteFirewallRule = async (ruleId: string) => {
    const updated = firewallRules.filter((r) => r.id !== ruleId);
    try {
      await api.applyFirewallRules(instanceId, updated);
      setFirewallRules(updated);
      toast.success('Firewall rule removed');
    } catch (err: any) {
      toast.error('Failed to update firewall', err.message);
    }
  };

  const handleCreateBackup = async () => {
    try {
      const b = await api.createInstanceBackup(instanceId);
      setBackups([b, ...backups]);
      toast.success('Backup archive initiated', b.name);
    } catch (err: any) {
      toast.error('Backup failed', err.message);
    }
  };

  const handleCreateSnapshot = async () => {
    try {
      const s = await api.createSnapshot(instanceId, `snap-${Date.now()}`);
      setSnapshots([s, ...snapshots]);
      toast.success('Point-in-time snapshot captured', s.name);
    } catch (err: any) {
      toast.error('Snapshot failed', err.message);
    }
  };

  const handleDeleteInstance = async () => {
    setActionLoading(true);
    try {
      await api.deleteInstance(instanceId);
      toast.success('Instance deleted');
      navigate('/instances');
    } catch (err: any) {
      toast.error('Delete failed', err.message);
    } finally {
      setActionLoading(false);
    }
  };

  if (loading || !instance) {
    return (
      <div className="py-20 text-center text-xs text-slate-500 font-mono">
        Loading instance metadata...
      </div>
    );
  }

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Server },
    { id: 'console', label: 'Terminal', icon: TerminalIcon },
    { id: 'vnc', label: 'VNC Remote', icon: Monitor },
    { id: 'files', label: 'Files', icon: Folder },
    { id: 'networking', label: 'Networking & Port Forwards', icon: Network },
    { id: 'backups', label: 'Backups', icon: HardDrive },
    { id: 'snapshots', label: 'Snapshots', icon: Camera },
    { id: 'monitoring', label: 'Telemetry', icon: Activity },
    { id: 'settings', label: 'Settings & Resize', icon: Settings },
  ];

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Back Button & Top Status Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div className="flex items-center gap-3">
          <button
            onClick={() => navigate('/instances')}
            className="p-2 rounded-xl bg-[#0f121d] hover:bg-[#181f30] border border-[#1e2538] text-slate-400 hover:text-white transition"
          >
            <ArrowLeft className="w-4 h-4" />
          </button>
          <div>
            <div className="flex items-center gap-2.5">
              <h2 className="text-xl font-bold text-white">{instance.name}</h2>
              <span
                className={`text-[10px] px-2.5 py-0.5 rounded-full font-mono uppercase font-semibold border ${
                  instance.status === 'running'
                    ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30'
                    : instance.status === 'stopped'
                    ? 'bg-slate-500/10 text-slate-400 border-slate-500/30'
                    : 'bg-rose-500/10 text-rose-400 border-rose-500/30'
                }`}
              >
                {instance.status}
              </span>
            </div>
            <div className="text-xs text-slate-400 font-mono mt-0.5">
              ID: {instance.id} • Image: {instance.image}
            </div>
          </div>
        </div>

        {/* Quick Power Actions */}
        <div className="flex items-center gap-2">
          {instance.status === 'stopped' ? (
            <button
              onClick={() => handlePower('start')}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold shadow-sm transition"
            >
              <Play className="w-3.5 h-3.5" />
              <span>Start</span>
            </button>
          ) : (
            <>
              <button
                onClick={() => handlePower('restart')}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-200 text-xs font-semibold transition"
              >
                <RotateCw className="w-3.5 h-3.5" />
                <span>Restart</span>
              </button>
              <button
                onClick={() => handlePower('stop')}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-amber-600/20 hover:bg-amber-600/30 border border-amber-500/30 text-amber-300 text-xs font-semibold transition"
              >
                <Square className="w-3.5 h-3.5" />
                <span>Stop</span>
              </button>
            </>
          )}

          <button
            onClick={() => setActiveTab('console')}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold shadow-sm transition"
          >
            <TerminalIcon className="w-3.5 h-3.5" />
            <span>Console</span>
          </button>
        </div>
      </div>

      {/* Tabs Navigation Bar */}
      <div className="flex items-center gap-1.5 overflow-x-auto border-b border-[#181f30] pb-2 text-xs font-medium">
        {tabs.map((tab) => {
          const Icon = tab.icon;
          const isSelected = activeTab === tab.id;
          return (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`flex items-center gap-2 px-3.5 py-2 rounded-xl whitespace-nowrap transition ${
                isSelected
                  ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30 font-semibold'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-[#121624]'
              }`}
            >
              <Icon className="w-4 h-4" />
              <span>{tab.label}</span>
            </button>
          );
        })}
      </div>

      {/* TAB 1: OVERVIEW */}
      {activeTab === 'overview' && (
        <div className="space-y-6">
          {/* Quick Hardware Specs Grid */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <div className="p-4 rounded-2xl bg-[#0f121d] border border-[#1c2235]">
              <div className="text-[11px] text-slate-400 flex items-center gap-1.5">
                <Cpu className="w-3.5 h-3.5 text-blue-400" />
                <span>vCPU Allocation</span>
              </div>
              <div className="text-lg font-bold text-white font-mono mt-1">{instance.cpuCores} Cores</div>
            </div>

            <div className="p-4 rounded-2xl bg-[#0f121d] border border-[#1c2235]">
              <div className="text-[11px] text-slate-400 flex items-center gap-1.5">
                <HardDrive className="w-3.5 h-3.5 text-purple-400" />
                <span>System Memory</span>
              </div>
              <div className="text-lg font-bold text-white font-mono mt-1">
                {(instance.memoryBytes / 1073741824).toFixed(1)} GB
              </div>
            </div>

            <div className="p-4 rounded-2xl bg-[#0f121d] border border-[#1c2235]">
              <div className="text-[11px] text-slate-400 flex items-center gap-1.5">
                <HardDrive className="w-3.5 h-3.5 text-emerald-400" />
                <span>Root Disk</span>
              </div>
              <div className="text-lg font-bold text-white font-mono mt-1">
                {(instance.storageBytes / 1073741824).toFixed(0)} GB
              </div>
            </div>

            <div className="p-4 rounded-2xl bg-[#0f121d] border border-[#1c2235]">
              <div className="text-[11px] text-slate-400 flex items-center gap-1.5">
                <Network className="w-3.5 h-3.5 text-amber-400" />
                <span>Public IPv4</span>
              </div>
              <div className="text-xs font-bold text-white font-mono mt-1.5 truncate">
                {instance.ipv4Address || 'Allocating'}
              </div>
            </div>
          </div>

          {/* Telemetry charts summary */}
          <MetricChart metrics={metrics} instanceName={instance.name} />
        </div>
      )}

      {/* TAB 2: TERMINAL CONSOLE */}
      {activeTab === 'console' && (
        <TerminalView instanceId={instance.id} instanceName={instance.name} />
      )}

      {/* TAB 3: VNC REMOTE */}
      {activeTab === 'vnc' && (
        <VNCView instanceId={instance.id} instanceName={instance.name} />
      )}

      {/* TAB 4: GUEST FILES */}
      {activeTab === 'files' && (
        <FileManager instanceId={instance.id} />
      )}

      {/* TAB 5: NETWORKING & FIREWALL */}
      {activeTab === 'networking' && (
        <div className="space-y-6">
          {/* IP Allocations */}
          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Network className="w-4 h-4 text-blue-400" />
              <span>Assigned IP Addressing & Network Interfaces</span>
            </h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 font-mono text-xs">
              <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
                <div className="text-slate-400">IPv4 Public Address:</div>
                <div className="text-white font-bold mt-1">{instance.ipv4Address || '10.0.3.150'}</div>
              </div>
              <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
                <div className="text-slate-400">IPv6 Global Address:</div>
                <div className="text-white font-bold mt-1">{instance.ipv6Address || 'fd42:4242:4242::150'}</div>
              </div>
            </div>
          </div>

          {/* Firewall & Port Forwards */}
          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-bold text-white flex items-center gap-2">
                <ShieldCheck className="w-4 h-4 text-emerald-400" />
                <span>Firewall Rules & Port Security</span>
              </h3>
              <button
                onClick={() => setRuleModal(true)}
                className="flex items-center gap-1 px-3 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold"
              >
                <Plus className="w-3.5 h-3.5" />
                <span>Add Rule</span>
              </button>
            </div>

            <div className="rounded-xl bg-[#090b12] border border-[#181f30] overflow-hidden">
              <table className="w-full text-left text-xs font-mono">
                <thead>
                  <tr className="border-b border-[#181f30] text-slate-400">
                    <th className="py-2.5 px-4">Action</th>
                    <th className="py-2.5 px-4">Protocol</th>
                    <th className="py-2.5 px-4">Port Range</th>
                    <th className="py-2.5 px-4">Source CIDR</th>
                    <th className="py-2.5 px-4 text-right">Delete</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#141a29]">
                  {firewallRules.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="py-6 text-center text-slate-500">
                        Default allow policy applied. No restrictive rules active.
                      </td>
                    </tr>
                  ) : (
                    firewallRules.map((rule) => (
                      <tr key={rule.id}>
                        <td className="py-2.5 px-4">
                          <span
                            className={`px-2 py-0.5 rounded text-[10px] uppercase font-bold ${
                              rule.action === 'allow'
                                ? 'bg-emerald-500/20 text-emerald-400'
                                : 'bg-rose-500/20 text-rose-400'
                            }`}
                          >
                            {rule.action}
                          </span>
                        </td>
                        <td className="py-2.5 px-4 text-white uppercase">{rule.protocol}</td>
                        <td className="py-2.5 px-4 text-slate-300">{rule.portRange}</td>
                        <td className="py-2.5 px-4 text-slate-400">{rule.sourceCidr}</td>
                        <td className="py-2.5 px-4 text-right">
                          <button
                            onClick={() => handleDeleteFirewallRule(rule.id)}
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
        </div>
      )}

      {/* TAB 6: BACKUPS */}
      {activeTab === 'backups' && (
        <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <HardDrive className="w-4 h-4 text-blue-400" />
              <span>Full Workload Backups</span>
            </h3>
            <button
              onClick={handleCreateBackup}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>Create Backup Now</span>
            </button>
          </div>

          <div className="space-y-2 font-mono text-xs">
            {backups.map((b) => (
              <div
                key={b.id}
                className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30] flex items-center justify-between"
              >
                <div>
                  <div className="font-semibold text-white">{b.name}</div>
                  <div className="text-[10px] text-slate-400 mt-0.5">
                    Size: {(b.sizeBytes / 1073741824).toFixed(1)} GB • Created:{' '}
                    {new Date(b.createdAt).toLocaleDateString()}
                  </div>
                </div>
                <button
                  onClick={() => setConfirmRestoreBackup(b.id)}
                  className="px-3 py-1 rounded-lg bg-[#141824] hover:bg-amber-950/40 text-amber-400 border border-[#232a3d] text-xs font-semibold"
                >
                  Restore
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* TAB 7: SNAPSHOTS */}
      {activeTab === 'snapshots' && (
        <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Camera className="w-4 h-4 text-purple-400" />
              <span>Point-in-Time Snapshots</span>
            </h3>
            <button
              onClick={handleCreateSnapshot}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>Take Snapshot</span>
            </button>
          </div>

          <div className="space-y-2 font-mono text-xs">
            {snapshots.map((s) => (
              <div
                key={s.id}
                className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30] flex items-center justify-between"
              >
                <div>
                  <div className="font-semibold text-white">{s.name}</div>
                  <div className="text-[10px] text-slate-400 mt-0.5">
                    Created: {new Date(s.createdAt).toLocaleString()}
                  </div>
                </div>
                <button
                  onClick={() => setConfirmRestoreSnapshot(s.id)}
                  className="px-3 py-1 rounded-lg bg-[#141824] hover:bg-amber-950/40 text-amber-400 border border-[#232a3d] text-xs font-semibold"
                >
                  Rollback
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* TAB 8: MONITORING */}
      {activeTab === 'monitoring' && (
        <MetricChart metrics={metrics} instanceName={instance.name} />
      )}

      {/* TAB 9: SETTINGS & RESIZE */}
      {activeTab === 'settings' && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Resize Specs Form */}
          <form
            onSubmit={handleSaveSpec}
            className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4"
          >
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Cpu className="w-4 h-4 text-blue-400" />
              <span>Scale Compute Resources</span>
            </h3>
            <p className="text-xs text-slate-400">
              Hot-resize CPU cores, memory limits, and root storage capacity without downtime.
            </p>

            <div className="space-y-3 pt-2">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  vCPU Cores ({editCpu})
                </label>
                <input
                  type="range"
                  min={1}
                  max={16}
                  value={editCpu}
                  onChange={(e) => setEditCpu(parseInt(e.target.value))}
                  className="w-full"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  RAM in GB ({editMemoryGb} GB)
                </label>
                <input
                  type="range"
                  min={1}
                  max={64}
                  value={editMemoryGb}
                  onChange={(e) => setEditMemoryGb(parseInt(e.target.value))}
                  className="w-full"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Storage in GB ({editStorageGb} GB)
                </label>
                <input
                  type="range"
                  min={10}
                  max={500}
                  value={editStorageGb}
                  onChange={(e) => setEditStorageGb(parseInt(e.target.value))}
                  className="w-full"
                />
              </div>

              <div className="pt-2">
                <button
                  type="submit"
                  disabled={actionLoading}
                  className="px-4 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-sm"
                >
                  Apply Resource Scale
                </button>
              </div>
            </div>
          </form>

          {/* Danger Zone */}
          <div className="p-6 rounded-2xl bg-[#1a0f12] border border-rose-900/40 space-y-4">
            <h3 className="text-sm font-bold text-rose-300 flex items-center gap-2">
              <AlertTriangle className="w-4 h-4 text-rose-400" />
              <span>Danger Zone</span>
            </h3>
            <p className="text-xs text-rose-200/80 leading-relaxed">
              Permanently destroy this compute instance, releasing its assigned IP address and wiping all attached storage.
            </p>
            <button
              onClick={() => setConfirmDelete(true)}
              className="px-4 py-2 rounded-xl bg-rose-600 hover:bg-rose-500 text-white text-xs font-bold shadow-lg shadow-rose-600/20"
            >
              Destroy Instance
            </button>
          </div>
        </div>
      )}

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={confirmDelete}
        title={`Destroy Instance "${instance.name}"?`}
        message="This action is irreversible. All instance files, snapshots, and persistent root storage will be deleted immediately."
        confirmText="Destroy Workload"
        isDestructive={true}
        loading={actionLoading}
        onConfirm={handleDeleteInstance}
        onCancel={() => setConfirmDelete(false)}
      />

      {/* Restore Backup Confirmation */}
      <ConfirmDialog
        isOpen={!!confirmRestoreBackup}
        title="Restore Backup Archive?"
        message="Restoring from a backup will overwrite the current root filesystem state."
        confirmText="Restore Backup"
        isDestructive={true}
        onConfirm={() => {
          setConfirmRestoreBackup(null);
          toast.success('Backup restored');
        }}
        onCancel={() => setConfirmRestoreBackup(null)}
      />

      {/* Add Firewall Rule Modal */}
      {ruleModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <form
            onSubmit={handleAddFirewallRule}
            className="w-full max-w-md bg-[#0d101a] border border-[#1e2538] rounded-2xl shadow-2xl p-6 space-y-4"
          >
            <h3 className="text-sm font-bold text-white">Add Inbound Firewall Rule</h3>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Action</label>
              <select
                value={newAction}
                onChange={(e) => setNewAction(e.target.value as any)}
                className="w-full px-3 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-xs text-white"
              >
                <option value="allow">ALLOW</option>
                <option value="deny">DENY</option>
              </select>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Protocol</label>
              <select
                value={newProto}
                onChange={(e) => setNewProto(e.target.value as any)}
                className="w-full px-3 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-xs text-white"
              >
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
                <option value="icmp">ICMP</option>
                <option value="all">ALL</option>
              </select>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Port Range</label>
              <input
                type="text"
                required
                value={newPort}
                onChange={(e) => setNewPort(e.target.value)}
                placeholder="80,443 or 8000-8080"
                className="w-full px-3 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-xs text-white font-mono"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Source CIDR</label>
              <input
                type="text"
                required
                value={newCidr}
                onChange={(e) => setNewCidr(e.target.value)}
                placeholder="0.0.0.0/0"
                className="w-full px-3 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-xs text-white font-mono"
              />
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setRuleModal(false)}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] text-slate-300"
              >
                Cancel
              </button>
              <button
                type="submit"
                className="px-4 py-2 rounded-xl text-xs font-bold bg-blue-600 hover:bg-blue-500 text-white"
              >
                Add Rule
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Restore Snapshot Confirmation */}
      <ConfirmDialog
        isOpen={!!confirmRestoreSnapshot}
        title="Rollback to Snapshot?"
        message="Rolling back to this snapshot will revert memory and root storage to the point in time when the snapshot was captured."
        confirmText="Rollback Snapshot"
        isDestructive={true}
        onConfirm={() => {
          setConfirmRestoreSnapshot(null);
          toast.success('Snapshot rolled back');
        }}
        onCancel={() => setConfirmRestoreSnapshot(null)}
      />
    </div>
  );
};
