import React from 'react';
import { CheckCircle2, AlertTriangle, XCircle, Activity } from 'lucide-react';

interface HealthBadgeProps {
  status: 'healthy' | 'degraded' | 'unhealthy' | 'loading';
  uptime?: string;
  version?: string;
}

export const HealthBadge: React.FC<HealthBadgeProps> = ({ status, uptime, version }) => {
  if (status === 'loading') {
    return (
      <div className="flex items-center space-x-2 px-3 py-1.5 rounded-full bg-slate-800/80 border border-slate-700 text-slate-300 text-xs font-mono">
        <Activity className="w-3.5 h-3.5 animate-spin text-blue-400" />
        <span>Checking system status...</span>
      </div>
    );
  }

  if (status === 'healthy') {
    return (
      <div className="flex items-center space-x-2 px-3 py-1.5 rounded-full bg-emerald-950/60 border border-emerald-800/60 text-emerald-300 text-xs font-medium">
        <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
        <span>Control Plane Operational</span>
        {version && <span className="text-emerald-500 font-mono">v{version}</span>}
        {uptime && <span className="text-emerald-600 font-mono">({uptime})</span>}
      </div>
    );
  }

  if (status === 'degraded') {
    return (
      <div className="flex items-center space-x-2 px-3 py-1.5 rounded-full bg-amber-950/60 border border-amber-800/60 text-amber-300 text-xs font-medium">
        <AlertTriangle className="w-3.5 h-3.5 text-amber-400" />
        <span>System Degraded</span>
      </div>
    );
  }

  return (
    <div className="flex items-center space-x-2 px-3 py-1.5 rounded-full bg-rose-950/60 border border-rose-800/60 text-rose-300 text-xs font-medium">
      <XCircle className="w-3.5 h-3.5 text-rose-400" />
      <span>System Offline / Unreachable</span>
    </div>
  );
};
