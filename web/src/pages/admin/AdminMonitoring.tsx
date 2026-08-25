import React, { useState } from 'react';
import {
  Activity,
  Server,
} from 'lucide-react';
import { MetricChart } from '../../components/MetricChart';

export const AdminMonitoring: React.FC = () => {
  const [timeRange, setTimeRange] = useState<'15m' | '1h' | '6h' | '24h' | '7d'>('1h');

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Activity className="w-5 h-5 text-blue-400" />
            <span>Infrastructure Telemetry & Cluster Capacity</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Global compute utilization, memory saturation, I/O latency, and cross-node network bandwidth.
          </p>
        </div>

        <div className="flex items-center gap-1.5 p-1 rounded-xl bg-[#0f121d] border border-[#1c2235]">
          {(['15m', '1h', '6h', '24h', '7d'] as const).map((r) => (
            <button
              key={r}
              onClick={() => setTimeRange(r)}
              className={`px-2.5 py-1 rounded-lg text-xs font-mono font-semibold transition ${
                timeRange === r
                  ? 'bg-blue-600 text-white shadow-sm'
                  : 'text-slate-400 hover:text-white'
              }`}
            >
              {r}
            </button>
          ))}
        </div>
      </div>

      {/* Aggregate Cluster Telemetry Graphs */}
      <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
        <h3 className="text-sm font-bold text-white flex items-center gap-2">
          <Server className="w-4 h-4 text-blue-400" />
          <span>Aggregate Hypervisor Fleet Resource Saturation</span>
        </h3>
        <MetricChart
          metrics={{
            cpuPercent: 38.4,
            memoryPercent: 68.2,
            diskPercent: 44.5,
            netRxBytes: 840000000,
            netTxBytes: 420000000,
          }}
          instanceName="Cluster Hypervisor Fleet"
        />
      </div>
    </div>
  );
};
