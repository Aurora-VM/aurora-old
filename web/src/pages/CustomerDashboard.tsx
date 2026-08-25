import React from 'react';
import {
  Server,
  PlayCircle,
  PlusCircle,
  Terminal,
  Layers,
  HardDrive,
  Activity,
  Cpu,
  ArrowRight,
} from 'lucide-react';
import { Instance, OSTemplate } from '../lib/api';
import { useJobs } from '../context/JobsContext';

interface CustomerDashboardProps {
  instances: Instance[];
  templates: OSTemplate[];
  navigate: (path: string) => void;
}

export const CustomerDashboard: React.FC<CustomerDashboardProps> = ({
  instances,
  templates,
  navigate,
}) => {
  const { jobs } = useJobs();

  const runningCount = instances.filter((i) => i.status === 'running').length;
  const stoppedCount = instances.filter((i) => i.status === 'stopped').length;
  const errorCount = instances.filter((i) => i.status === 'error' || i.status === 'suspended').length;
  const totalCores = instances.reduce((sum, i) => sum + i.cpuCores, 0);
  const totalRamGb = (instances.reduce((sum, i) => sum + i.memoryBytes, 0) / 1073741824).toFixed(1);
  const totalDiskGb = (instances.reduce((sum, i) => sum + i.storageBytes, 0) / 1073741824).toFixed(0);

  const activeJobs = jobs.filter((j) => j.status === 'running' || j.status === 'pending');

  return (
    <div className="space-y-8 animate-in fade-in duration-200">
      {/* Welcome Banner & Quick Action */}
      <div className="p-6 sm:p-8 rounded-3xl bg-gradient-to-br from-[#121627] via-[#0e1220] to-[#090b14] border border-[#1e2538] shadow-2xl relative overflow-hidden flex flex-col md:flex-row md:items-center justify-between gap-6">
        <div className="relative z-10 space-y-2">
          <div className="flex items-center gap-2 text-blue-400 text-xs font-semibold uppercase tracking-wider">
            <Activity className="w-4 h-4" />
            <span>Cloud Virtualization Platform</span>
          </div>
          <h1 className="text-2xl sm:text-3xl font-extrabold text-white tracking-tight">
            Customer Workloads & Infrastructure
          </h1>
          <p className="text-xs sm:text-sm text-slate-400 max-w-xl leading-relaxed">
            Deploy, monitor, scale, and console-connect to your high-performance Incus containers and virtual machines.
          </p>
        </div>

        <div className="relative z-10 flex flex-wrap items-center gap-3">
          <button
            onClick={() => navigate('/instances/new')}
            className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-lg shadow-blue-600/25 transition"
          >
            <PlusCircle className="w-4 h-4" />
            <span>Deploy Instance</span>
          </button>
          <button
            onClick={() => navigate('/instances')}
            className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-300 hover:text-white text-xs font-semibold transition"
          >
            <Server className="w-4 h-4 text-slate-400" />
            <span>All Instances</span>
          </button>
        </div>
      </div>

      {/* Primary Statistics Grid */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Total Instances */}
        <div
          onClick={() => navigate('/instances')}
          className="p-5 rounded-2xl bg-[#0f121d] border border-[#1c2235] hover:border-slate-700 cursor-pointer transition space-y-2"
        >
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-slate-400">Total Workloads</span>
            <Server className="w-4 h-4 text-blue-400" />
          </div>
          <div className="text-2xl font-bold text-white font-mono">{instances.length}</div>
          <div className="text-[11px] text-slate-500 flex items-center gap-2 font-mono">
            <span>Containers & VMs</span>
          </div>
        </div>

        {/* Running Instances */}
        <div
          onClick={() => navigate('/instances')}
          className="p-5 rounded-2xl bg-[#0f121d] border border-[#1c2235] hover:border-slate-700 cursor-pointer transition space-y-2"
        >
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-slate-400">Running State</span>
            <PlayCircle className="w-4 h-4 text-emerald-400" />
          </div>
          <div className="text-2xl font-bold text-emerald-400 font-mono">{runningCount}</div>
          <div className="text-[11px] text-slate-500 font-mono">
            <span>{stoppedCount} stopped • {errorCount} alerts</span>
          </div>
        </div>

        {/* Allocated Compute (vCPU & RAM) */}
        <div className="p-5 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-slate-400">Allocated Compute</span>
            <Cpu className="w-4 h-4 text-purple-400" />
          </div>
          <div className="text-2xl font-bold text-white font-mono">{totalCores} <span className="text-xs font-normal text-slate-400">vCPUs</span></div>
          <div className="text-[11px] text-slate-500 font-mono">
            <span>{totalRamGb} GB RAM provisioned</span>
          </div>
        </div>

        {/* Provisioned Storage */}
        <div className="p-5 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-slate-400">Storage Footprint</span>
            <HardDrive className="w-4 h-4 text-amber-400" />
          </div>
          <div className="text-2xl font-bold text-white font-mono">{totalDiskGb} <span className="text-xs font-normal text-slate-400">GB</span></div>
          <div className="text-[11px] text-slate-500 font-mono">
            <span>ZFS / Ceph storage</span>
          </div>
        </div>
      </div>

      {/* Active Workloads & Quick Actions Row */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Recent Workloads List */}
        <div className="lg:col-span-2 p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Server className="w-4 h-4 text-blue-400" />
              <span>Active Instances</span>
            </h3>
            <button
              onClick={() => navigate('/instances')}
              className="text-xs text-blue-400 hover:text-blue-300 flex items-center gap-1 font-semibold"
            >
              <span>View all ({instances.length})</span>
              <ArrowRight className="w-3.5 h-3.5" />
            </button>
          </div>

          {instances.length === 0 ? (
            <div className="py-12 text-center text-xs text-slate-500 space-y-3">
              <Server className="w-8 h-8 text-slate-600 mx-auto" />
              <p>No compute instances deployed yet.</p>
              <button
                onClick={() => navigate('/instances/new')}
                className="px-3 py-1.5 rounded-lg bg-blue-600 text-white text-xs font-medium"
              >
                Launch First VPS
              </button>
            </div>
          ) : (
            <div className="space-y-2">
              {instances.slice(0, 5).map((inst) => (
                <div
                  key={inst.id}
                  onClick={() => navigate(`/instances/${inst.id}`)}
                  className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30] hover:border-slate-700 cursor-pointer transition flex items-center justify-between"
                >
                  <div className="flex items-center gap-3">
                    <div
                      className={`w-2.5 h-2.5 rounded-full ${
                        inst.status === 'running'
                          ? 'bg-emerald-400'
                          : inst.status === 'stopped'
                          ? 'bg-slate-500'
                          : 'bg-rose-400'
                      }`}
                    />
                    <div>
                      <div className="text-xs font-semibold text-white">{inst.name}</div>
                      <div className="text-[10px] text-slate-400 font-mono mt-0.5">
                        {inst.ipv4Address || 'Allocating IP'} • {inst.cpuCores} vCPU • {(inst.memoryBytes / 1073741824).toFixed(0)} GB
                      </div>
                    </div>
                  </div>

                  <div className="flex items-center gap-2">
                    <span className="text-[10px] px-2 py-0.5 rounded font-mono uppercase bg-[#141824] text-slate-300 border border-[#232a3d]">
                      {inst.type}
                    </span>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate(`/instances/${inst.id}?tab=console`);
                      }}
                      className="p-1.5 rounded-lg hover:bg-[#1c2233] text-slate-400 hover:text-white"
                      title="Open Terminal"
                    >
                      <Terminal className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Quick Launch & Active Operations Sidebar */}
        <div className="space-y-6">
          {/* Quick Launch Card */}
          <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Layers className="w-4 h-4 text-purple-400" />
              <span>Available OS Templates</span>
            </h3>
            <div className="space-y-2">
              {templates.slice(0, 3).map((tmpl) => (
                <div
                  key={tmpl.id}
                  onClick={() => navigate(`/instances/new?template=${tmpl.slug}`)}
                  className="p-3 rounded-xl bg-[#090b12] border border-[#181f30] hover:border-blue-500/40 cursor-pointer transition flex items-center justify-between"
                >
                  <div className="truncate">
                    <div className="text-xs font-semibold text-white truncate">{tmpl.name}</div>
                    <div className="text-[10px] text-slate-400 font-mono">{tmpl.slug}</div>
                  </div>
                  <PlusCircle className="w-4 h-4 text-blue-400 flex-shrink-0 ml-2" />
                </div>
              ))}
            </div>
            <button
              onClick={() => navigate('/templates')}
              className="w-full py-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] text-xs font-semibold text-slate-300 hover:text-white transition text-center"
            >
              Browse Template Catalog
            </button>
          </div>

          {/* Active Jobs Card */}
          {activeJobs.length > 0 && (
            <div className="p-5 rounded-2xl bg-[#0f121d] border border-blue-500/30 space-y-3">
              <div className="flex items-center justify-between">
                <span className="text-xs font-bold text-blue-400 flex items-center gap-1.5">
                  <Activity className="w-3.5 h-3.5 animate-spin" />
                  <span>Ongoing Operations ({activeJobs.length})</span>
                </span>
              </div>
              <div className="space-y-2 text-xs">
                {activeJobs.map((j) => (
                  <div key={j.id} className="p-2 rounded-lg bg-[#090b12] text-slate-300">
                    <div className="font-semibold text-white">{j.title}</div>
                    <div className="text-[10px] text-slate-400 font-mono">Target: {j.targetName}</div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
