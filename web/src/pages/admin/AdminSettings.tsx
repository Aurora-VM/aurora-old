import React from 'react';
import {
  Settings,
  Shield,
  Server,
} from 'lucide-react';

export const AdminSettings: React.FC = () => {
  return (
    <div className="max-w-4xl mx-auto space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="border-b border-[#181f30] pb-4">
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          <Settings className="w-5 h-5 text-blue-400" />
          <span>System & Hypervisor Infrastructure Settings</span>
        </h2>
        <p className="text-xs text-slate-400 mt-1">
          Cluster control plane configuration, mTLS root authority, and global scheduling policies.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-6">
        {/* Core Endpoints */}
        <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
          <h3 className="text-sm font-bold text-white flex items-center gap-2">
            <Server className="w-4 h-4 text-blue-400" />
            <span>Control Plane Endpoints & Networking</span>
          </h3>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 font-mono text-xs">
            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
              <div className="text-slate-400">REST API Public Endpoint</div>
              <div className="text-white font-bold mt-1">{window.location.origin}</div>
            </div>

            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
              <div className="text-slate-400">mTLS Node Gateway (gRPC)</div>
              <div className="text-emerald-400 font-bold mt-1">0.0.0.0:9499</div>
            </div>

            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
              <div className="text-slate-400">Database Connection Engine</div>
              <div className="text-white font-bold mt-1">PostgreSQL 16 (HA Clustered)</div>
            </div>

            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30]">
              <div className="text-slate-400">Incus Daemon Provider</div>
              <div className="text-purple-400 font-bold mt-1">Incus 6.0 LTS (Socket / Remote)</div>
            </div>
          </div>
        </div>

        {/* Security & Cryptographic Authority */}
        <div className="p-6 rounded-2xl bg-[#0f121d] border border-[#1c2235] space-y-4">
          <h3 className="text-sm font-bold text-white flex items-center gap-2">
            <Shield className="w-4 h-4 text-emerald-400" />
            <span>Cryptographic Trust & PKI Architecture</span>
          </h3>

          <div className="space-y-3 font-mono text-xs">
            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30] flex items-center justify-between">
              <div>
                <div className="text-white font-semibold">Node mTLS Root Certificate Authority</div>
                <div className="text-[10px] text-slate-400 mt-0.5">
                  Algorithm: ECDSA (P-384) • SHA-384 Signature
                </div>
              </div>
              <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-400 font-bold">
                ACTIVE
              </span>
            </div>

            <div className="p-3.5 rounded-xl bg-[#090b12] border border-[#181f30] flex items-center justify-between">
              <div>
                <div className="text-white font-semibold">Audit Ledger Cryptographic Hash Chain</div>
                <div className="text-[10px] text-slate-400 mt-0.5">
                  Algorithm: SHA-256 with Genesis Merkle Seed
                </div>
              </div>
              <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-400 font-bold">
                ENFORCED
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
