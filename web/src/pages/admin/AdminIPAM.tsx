import React, { useEffect, useState } from 'react';
import {
  Network,
  PlusCircle,
  RotateCw,
  Trash2,
  Search,
} from 'lucide-react';
import { IPAMPool, api } from '../../lib/api';
import { useToast } from '../../context/ToastContext';
import { ConfirmDialog } from '../../components/ConfirmDialog';

export const AdminIPAM: React.FC = () => {
  const [pools, setPools] = useState<IPAMPool[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [searchQuery, setSearchQuery] = useState<string>('');

  // Create Pool Modal
  const [createModal, setCreateModal] = useState<boolean>(false);
  const [poolName, setPoolName] = useState<string>('public-ipv4-primary');
  const [poolCidr, setPoolCidr] = useState<string>('10.0.3.0/24');
  const [poolVersion, setPoolVersion] = useState<number>(4);
  const [poolGateway, setPoolGateway] = useState<string>('10.0.3.1');
  const [poolDns, setPoolDns] = useState<string>('1.1.1.1, 8.8.8.8');
  const [poolVlan, setPoolVlan] = useState<number>(100);

  const [deleteTarget, setDeleteTarget] = useState<IPAMPool | null>(null);

  const toast = useToast();

  const fetchPools = async () => {
    setLoading(true);
    try {
      const list = await api.listIPAMPools();
      setPools(list);
    } catch (err: any) {
      toast.error('Failed to load IPAM pools', err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchPools();
  }, []);

  const handleCreatePool = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const dns = poolDns.split(',').map((d) => d.trim()).filter(Boolean);
      await api.createIPAMPool({
        name: poolName,
        cidr: poolCidr,
        ipVersion: poolVersion,
        gateway: poolGateway,
        dnsServers: dns,
        vlanId: poolVlan || undefined,
      });
      toast.success('IPAM subnet pool registered successfully');
      setCreateModal(false);
      fetchPools();
    } catch (err: any) {
      toast.error('Failed to create IP pool', err.message);
    }
  };

  const handleDeletePool = async () => {
    if (!deleteTarget) return;
    try {
      await api.deleteIPAMPool(deleteTarget.id);
      toast.success('IP pool deleted', deleteTarget.name);
      setDeleteTarget(null);
      fetchPools();
    } catch (err: any) {
      toast.error('Failed to delete pool', err.message);
    }
  };

  const filteredPools = pools.filter(
    (p) =>
      p.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      p.cidr.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Network className="w-5 h-5 text-blue-400" />
            <span>IPAM & Subnet Management</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Global IPv4/IPv6 address pool allocation, gateway routing, VLAN segmentation, and reverse DNS.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => setCreateModal(true)}
            className="flex items-center gap-2 px-3.5 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-lg shadow-blue-600/20 transition"
          >
            <PlusCircle className="w-4 h-4" />
            <span>Create IP Pool</span>
          </button>
          <button
            onClick={fetchPools}
            disabled={loading}
            className="p-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-400 hover:text-white"
          >
            <RotateCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Search Bar */}
      <div className="relative">
        <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-3" />
        <input
          type="text"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Search IP pools by name or CIDR..."
          className="w-full pl-10 pr-4 py-2.5 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:outline-none focus:border-blue-500"
        />
      </div>

      {/* Pools Table */}
      <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden shadow-xl">
        <table className="w-full text-left text-xs font-mono">
          <thead>
            <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
              <th className="py-3.5 px-4 font-semibold">Pool Name</th>
              <th className="py-3.5 px-4 font-semibold">Subnet CIDR</th>
              <th className="py-3.5 px-4 font-semibold">Gateway</th>
              <th className="py-3.5 px-4 font-semibold">VLAN</th>
              <th className="py-3.5 px-4 font-semibold">IP Utilization</th>
              <th className="py-3.5 px-4 font-semibold text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#141a29]">
            {filteredPools.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-12 text-center text-slate-500 font-sans">
                  No IPAM pools registered yet.
                </td>
              </tr>
            ) : (
              filteredPools.map((pool) => {
                const total = pool.totalIps || 254;
                const allocated = pool.allocatedIps || 1;
                const percent = Math.min(100, Math.round((allocated / total) * 100));

                return (
                  <tr key={pool.id} className="hover:bg-[#141824]/60 transition">
                    <td className="py-3.5 px-4">
                      <div className="font-bold text-white font-sans">{pool.name}</div>
                      <div className="text-[10px] text-slate-400 font-mono mt-0.5">
                        IPv{pool.ipVersion} • DNS: {pool.dnsServers?.join(', ') || '1.1.1.1'}
                      </div>
                    </td>

                    <td className="py-3.5 px-4 text-emerald-400 font-bold">{pool.cidr}</td>
                    <td className="py-3.5 px-4 text-slate-300">{pool.gateway}</td>
                    <td className="py-3.5 px-4 text-purple-400">
                      {pool.vlanId ? `VLAN ${pool.vlanId}` : 'Untagged'}
                    </td>

                    <td className="py-3.5 px-4">
                      <div className="w-36 space-y-1">
                        <div className="flex justify-between text-[10px] text-slate-400 font-mono">
                          <span>{allocated} / {total}</span>
                          <span>{percent}%</span>
                        </div>
                        <div className="w-full h-1.5 rounded-full bg-[#141824] overflow-hidden">
                          <div
                            className={`h-full rounded-full ${
                              percent > 85
                                ? 'bg-rose-500'
                                : percent > 60
                                ? 'bg-amber-500'
                                : 'bg-blue-500'
                            }`}
                            style={{ width: `${percent}%` }}
                          />
                        </div>
                      </div>
                    </td>

                    <td className="py-3.5 px-4 text-right">
                      <button
                        onClick={() => setDeleteTarget(pool)}
                        className="p-1.5 text-slate-400 hover:text-rose-400 rounded-lg hover:bg-[#141824]"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Create Pool Modal */}
      {createModal && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
          <form
            onSubmit={handleCreatePool}
            className="w-full max-w-md bg-[#0d101a] border border-[#1e2538] rounded-3xl shadow-2xl p-6 space-y-4 animate-in zoom-in-95 duration-150"
          >
            <h3 className="text-base font-bold text-white flex items-center gap-2">
              <Network className="w-5 h-5 text-blue-400" />
              <span>Define IPAM Subnet Pool</span>
            </h3>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Pool Identifier</label>
              <input
                type="text"
                required
                value={poolName}
                onChange={(e) => setPoolName(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">IP Protocol</label>
                <select
                  value={poolVersion}
                  onChange={(e) => setPoolVersion(parseInt(e.target.value))}
                  className="w-full px-3 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono"
                >
                  <option value={4}>IPv4</option>
                  <option value={6}>IPv6</option>
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">VLAN Tag (Optional)</label>
                <input
                  type="number"
                  value={poolVlan}
                  onChange={(e) => setPoolVlan(parseInt(e.target.value))}
                  className="w-full px-3 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono"
                />
              </div>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Subnet CIDR Block</label>
              <input
                type="text"
                required
                value={poolCidr}
                onChange={(e) => setPoolCidr(e.target.value)}
                placeholder="10.0.3.0/24 or fd42:4242::/64"
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Default Gateway</label>
              <input
                type="text"
                required
                value={poolGateway}
                onChange={(e) => setPoolGateway(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">DNS Nameservers (Comma separated)</label>
              <input
                type="text"
                value={poolDns}
                onChange={(e) => setPoolDns(e.target.value)}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setCreateModal(false)}
                className="px-4 py-2 rounded-xl text-xs font-semibold bg-[#141824] text-slate-300"
              >
                Cancel
              </button>
              <button
                type="submit"
                className="px-4 py-2 rounded-xl text-xs font-bold bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-600/25"
              >
                Create Pool
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Delete Pool Dialog */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={`Delete Subnet Pool "${deleteTarget?.name}"?`}
        message="Deleting this pool will remove the IP allocation range. Ensure no active instances are assigned IPs in this subnet."
        confirmText="Delete Pool"
        isDestructive={true}
        onConfirm={handleDeletePool}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
