import React, { useEffect, useState } from 'react';
import { HealthBadge } from '../components/HealthBadge';
import { Server, HardDrive, Network, Shield, Terminal, RefreshCw, Layers, LayoutDashboard } from 'lucide-react';
import { TemplatesView } from './Templates';

interface HealthData {
  status: 'healthy' | 'degraded' | 'unhealthy';
  version: string;
  commit: string;
  uptime: string;
  components: Record<string, { status: string; message?: string }>;
}

export const Dashboard: React.FC = () => {
  const [health, setHealth] = useState<HealthData | null>(null);
  const [loading, setLoading] = useState(true);
  const [lastRefreshed, setLastRefreshed] = useState<Date>(new Date());
  const [activeNav, setActiveNav] = useState<'overview' | 'templates'>('overview');

  const fetchHealth = async () => {
    setLoading(true);
    try {
      const res = await fetch('/api/v1/health');
      if (res.ok) {
        const json = await res.json();
        setHealth(json.data);
      } else {
        setHealth(null);
      }
    } catch {
      setHealth(null);
    } finally {
      setLoading(false);
      setLastRefreshed(new Date());
    }
  };

  useEffect(() => {
    fetchHealth();
    const interval = setInterval(fetchHealth, 10000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div className="min-h-screen bg-[#090a0f] text-slate-100 flex flex-col">
      {/* Top Navigation Bar */}
      <header className="border-b border-[#1a1f2e] bg-[#0c0e17]/80 backdrop-blur-md px-6 py-4 flex items-center justify-between sticky top-0 z-50">
        <div className="flex items-center space-x-4">
          <div className="flex items-center space-x-2">
            <div className="w-8 h-8 rounded-lg bg-blue-600 flex items-center justify-center font-bold text-white shadow-lg shadow-blue-500/20">
              A
            </div>
            <span className="font-semibold text-lg tracking-tight bg-gradient-to-r from-white via-slate-200 to-slate-400 bg-clip-text text-transparent">
              AURORA
            </span>
          </div>
          <span className="text-xs px-2 py-0.5 rounded bg-blue-500/10 border border-blue-500/20 text-blue-400 font-mono">
            Phase 1 Foundation
          </span>

          <nav className="flex items-center space-x-1 pl-4 border-l border-[#1a1f2e]">
            <button
              onClick={() => setActiveNav('overview')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                activeNav === 'overview'
                  ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
                  : 'text-slate-400 hover:text-white hover:bg-[#141824]'
              }`}
            >
              <LayoutDashboard className="w-3.5 h-3.5" />
              <span>Overview</span>
            </button>
            <button
              onClick={() => setActiveNav('templates')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition ${
                activeNav === 'templates'
                  ? 'bg-blue-600/15 text-blue-400 border border-blue-500/30'
                  : 'text-slate-400 hover:text-white hover:bg-[#141824]'
              }`}
            >
              <Layers className="w-3.5 h-3.5" />
              <span>Templates & Images</span>
            </button>
          </nav>
        </div>

        <div className="flex items-center space-x-4">
          <HealthBadge
            status={loading && !health ? 'loading' : health ? health.status : 'unhealthy'}
            uptime={health?.uptime}
            version={health?.version}
          />
          <button
            onClick={fetchHealth}
            disabled={loading}
            className="p-2 rounded-lg bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-400 hover:text-white transition"
            title="Refresh Status"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </header>

      {/* Main Content Area */}
      <main className="flex-1 max-w-7xl w-full mx-auto px-6 py-8 space-y-8">
        {activeNav === 'templates' ? (
          <TemplatesView />
        ) : (
          <>
            {/* Welcome / Architecture Banner */}
        <section className="p-6 rounded-2xl bg-gradient-to-br from-[#121624] to-[#0d101a] border border-[#1e2538] shadow-2xl relative overflow-hidden">
          <div className="absolute right-0 top-0 bottom-0 w-1/3 bg-gradient-to-l from-blue-600/5 to-transparent pointer-events-none" />
          <div className="relative z-10 space-y-3">
            <div className="flex items-center space-x-2 text-blue-400 text-sm font-medium">
              <Shield className="w-4 h-4" />
              <span>Next-Generation Virtualization Platform</span>
            </div>
            <h1 className="text-2xl font-bold text-white tracking-tight">
              Control Plane & Monorepo Foundation Active
            </h1>
            <p className="text-sm text-slate-400 max-w-2xl leading-relaxed">
              Aurora is engineered as a distributed hub-and-spoke virtualization management platform. The Go Control Plane, PostgreSQL schema migrations, gRPC contracts, and React frontend skeleton are initialized and operating.
            </p>
          </div>
        </section>

        {/* Infrastructure Status Grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          <div className="p-5 rounded-xl bg-[#0f121d] border border-[#1c2235] space-y-3">
            <div className="flex items-center justify-between text-slate-400">
              <span className="text-xs font-semibold uppercase tracking-wider">Control Plane</span>
              <Server className="w-4 h-4 text-blue-400" />
            </div>
            <div className="text-lg font-bold text-white font-mono">
              {health?.version ? `v${health.version}` : 'Connecting...'}
            </div>
            <div className="text-xs text-slate-500 flex justify-between">
              <span>Commit:</span>
              <span className="font-mono text-slate-400">{health?.commit || 'dev'}</span>
            </div>
          </div>

          <div className="p-5 rounded-xl bg-[#0f121d] border border-[#1c2235] space-y-3">
            <div className="flex items-center justify-between text-slate-400">
              <span className="text-xs font-semibold uppercase tracking-wider">Node Agent Gateway</span>
              <Network className="w-4 h-4 text-emerald-400" />
            </div>
            <div className="text-lg font-bold text-white font-mono">
              mTLS gRPC 8443
            </div>
            <div className="text-xs text-slate-500 flex justify-between">
              <span>Listener:</span>
              <span className="font-mono text-emerald-400">Ready</span>
            </div>
          </div>

          <div className="p-5 rounded-xl bg-[#0f121d] border border-[#1c2235] space-y-3">
            <div className="flex items-center justify-between text-slate-400">
              <span className="text-xs font-semibold uppercase tracking-wider">Database State</span>
              <HardDrive className="w-4 h-4 text-purple-400" />
            </div>
            <div className="text-lg font-bold text-white font-mono">
              PostgreSQL 16+
            </div>
            <div className="text-xs text-slate-500 flex justify-between">
              <span>Migrations:</span>
              <span className="font-mono text-purple-400">Schema 000001</span>
            </div>
          </div>

          <div className="p-5 rounded-xl bg-[#0f121d] border border-[#1c2235] space-y-3">
            <div className="flex items-center justify-between text-slate-400">
              <span className="text-xs font-semibold uppercase tracking-wider">Protobuf Subsystem</span>
              <Terminal className="w-4 h-4 text-amber-400" />
            </div>
            <div className="text-lg font-bold text-white font-mono">
              aurora.v1
            </div>
            <div className="text-xs text-slate-500 flex justify-between">
              <span>Stubs:</span>
              <span className="font-mono text-amber-400">Generated</span>
            </div>
          </div>
        </div>

        {/* Development & Verification Guide */}
        <section className="p-6 rounded-xl bg-[#0d101a] border border-[#181f30] space-y-4">
          <h2 className="text-base font-semibold text-white">Phase 1 Verification Matrix</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs font-mono text-slate-300">
            <div className="p-4 rounded-lg bg-[#090b12] border border-[#141a29] space-y-2">
              <div className="text-emerald-400 font-semibold flex items-center space-x-1.5">
                <span className="w-2 h-2 rounded-full bg-emerald-400"></span>
                <span>Control Plane Endpoints</span>
              </div>
              <p className="text-slate-400 font-mono">GET /healthz → 200 OK</p>
              <p className="text-slate-400 font-mono">GET /api/v1/health → System Status JSON</p>
              <p className="text-slate-400 font-mono">gRPC :8443 → HealthService.Check</p>
            </div>

            <div className="p-4 rounded-lg bg-[#090b12] border border-[#141a29] space-y-2">
              <div className="text-blue-400 font-semibold flex items-center space-x-1.5">
                <span className="w-2 h-2 rounded-full bg-blue-400"></span>
                <span>Node Agent Preflight</span>
              </div>
              <p className="text-slate-400 font-mono">aurora-agent --version</p>
              <p className="text-slate-400 font-mono">aurora-agent --check</p>
              <p className="text-slate-400 font-mono">Incus Socket Inspection</p>
            </div>
          </div>
        </section>
          </>
        )}
      </main>

      {/* Footer */}
      <footer className="border-t border-[#1a1f2e] py-4 px-6 text-center text-xs text-slate-500 flex justify-between items-center">
        <span>Project Aurora © 2026. Free & Open-Source.</span>
        <span>Last Polled: {lastRefreshed.toLocaleTimeString()}</span>
      </footer>
    </div>
  );
};
