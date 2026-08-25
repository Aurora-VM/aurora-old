import React, { useEffect, useState } from 'react';
import {
  HardDrive,
  PlusCircle,
  RotateCw,
  Trash2,
  Search,
} from 'lucide-react';
import { StoragePool, api } from '../../lib/api';
import { useToast } from '../../context/ToastContext';
import { ConfirmDialog } from '../../components/ConfirmDialog';

export const AdminStorage: React.FC = () => {
  const [pools, setPools] = useState<StoragePool[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [searchQuery, setSearchQuery] = useState<string>('');

  // Create Pool Modal State
  const [createModal, setCreateModal] = useState<boolean>(false);
  const [poolName, setPoolName] = useState<string>('zfs-nvme-pool01');
  const [poolDriver, setPoolDriver] = useState<string>('zfs');
  const [poolNodeId, setPoolNodeId] = useState<string>('node-alpha-01');
  const [poolSizeGb, setPoolSizeGb] = useState<number>(500);

  const [deleteTarget, setDeleteTarget] = useState<StoragePool | null>(null);

  const toast = useToast();

  const fetchPools = async () => {
    setLoading(true);
    try {
      const list = await api.listStoragePools();
      setPools(list);
    } catch (err: any) {
      toast.error('Failed to load storage pools', err.message);
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
      await api.createStoragePool({
        name: poolName,
        driver: poolDriver,
        nodeId: poolNodeId,
        totalBytes: poolSizeGb * 1073741824,
      });
      toast.success('Storage pool registered successfully');
      setCreateModal(false);
      fetchPools();
    } catch (err: any) {
      toast.error('Failed to create storage pool', err.message);
    }
  };

  const handleDeletePool = async () => {
    if (!deleteTarget) return;
    try {
      await api.deleteStoragePool(deleteTarget.id);
      toast.success('Storage pool removed', deleteTarget.name);
      setDeleteTarget(null);
      fetchPools();
    } catch (err: any) {
      toast.error('Failed to delete storage pool', err.message);
    }
  };

  const filteredPools = pools.filter(
    (p) =>
      p.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      p.driver.toLowerCase().includes(searchQuery.toLowerCase()) ||
      p.nodeId.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <HardDrive className="w-5 h-5 text-blue-400" />
            <span>Hypervisor Storage Architecture & Pools</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Cluster-wide storage volumes and pool management with driver-level ZFS, Btrfs, LVM-Thin, and Ceph capabilities.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => setCreateModal(true)}
            className="flex items-center gap-2 px-3.5 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold shadow-lg shadow-blue-600/20 transition"
          >
            <PlusCircle className="w-4 h-4" />
            <span>Create Storage Pool</span>
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

      {/* Driver Capabilities Information Bar */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div className="p-3.5 rounded-xl bg-[#0f121d] border border-[#1c2235]">
          <div className="text-xs font-bold text-white">ZFS Pools</div>
          <div className="text-[10px] text-emerald-400 font-mono mt-0.5">
            Instant Snapshots, CoW, Cloning
          </div>
        </div>
        <div className="p-3.5 rounded-xl bg-[#0f121d] border border-[#1c2235]">
          <div className="text-xs font-bold text-white">Btrfs Subvolumes</div>
          <div className="text-[10px] text-purple-400 font-mono mt-0.5">
            Subvolumes, Compression
          </div>
        </div>
        <div className="p-3.5 rounded-xl bg-[#0f121d] border border-[#1c2235]">
          <div className="text-xs font-bold text-white">LVM-Thin</div>
          <div className="text-[10px] text-blue-400 font-mono mt-0.5">
            Thin Provisioning, High IOPS
          </div>
        </div>
        <div className="p-3.5 rounded-xl bg-[#0f121d] border border-[#1c2235]">
          <div className="text-xs font-bold text-white">Directory Backend</div>
          <div className="text-[10px] text-slate-400 font-mono mt-0.5">
            Universal compatibility
          </div>
        </div>
      </div>

      {/* Search Bar */}
      <div className="relative">
        <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-3" />
        <input
          type="text"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Search storage pools by name, driver, or node ID..."
          className="w-full pl-10 pr-4 py-2.5 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:outline-none focus:border-blue-500"
        />
      </div>

      {/* Pools Table */}
      <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden shadow-xl">
        <table className="w-full text-left text-xs font-mono">
          <thead>
            <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
              <th className="py-3.5 px-4 font-semibold">Pool Name</th>
              <th className="py-3.5 px-4 font-semibold">Node Binding</th>
              <th className="py-3.5 px-4 font-semibold">Driver Backend</th>
              <th className="py-3.5 px-4 font-semibold">Total Capacity</th>
              <th className="py-3.5 px-4 font-semibold">Used / Free</th>
              <th className="py-3.5 px-4 font-semibold text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#141a29]">
            {filteredPools.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-12 text-center text-slate-500 font-sans">
                  No storage pools found.
                </td>
              </tr>
            ) : (
              filteredPools.map((pool) => {
                const totalGb = (pool.totalBytes / 1073741824).toFixed(1);
                const usedGb = (pool.usedBytes / 1073741824).toFixed(1);
                const freeGb = (pool.freeBytes / 1073741824).toFixed(1);
                const percent = Math.min(100, Math.round((pool.usedBytes / (pool.totalBytes || 1)) * 100));

                return (
                  <tr key={pool.id} className="hover:bg-[#141824]/60 transition">
                    <td className="py-3.5 px-4 font-bold text-white font-sans">{pool.name}</td>
                    <td className="py-3.5 px-4 text-slate-300">{pool.nodeId}</td>
                    <td className="py-3.5 px-4">
                      <span className="px-2 py-0.5 rounded text-[10px] uppercase font-bold bg-purple-500/20 text-purple-400">
                        {pool.driver}
                      </span>
                    </td>
                    <td className="py-3.5 px-4 text-white">{totalGb} GB</td>
                    <td className="py-3.5 px-4">
                      <div className="w-32 space-y-1">
                        <div className="flex justify-between text-[10px] text-slate-400 font-mono">
                          <span>{usedGb} GB ({percent}%)</span>
                          <span>{freeGb} GB free</span>
                        </div>
                        <div className="w-full h-1.5 rounded-full bg-[#141824] overflow-hidden">
                          <div
                            className={`h-full rounded-full ${
                              percent > 85
                                ? 'bg-rose-500'
                                : percent > 60
                                ? 'bg-amber-500'
                                : 'bg-emerald-500'
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
              <HardDrive className="w-5 h-5 text-blue-400" />
              <span>Define Storage Pool</span>
            </h3>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Pool Name</label>
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
                <label className="block text-xs font-semibold text-slate-300 mb-1">Storage Driver</label>
                <select
                  value={poolDriver}
                  onChange={(e) => setPoolDriver(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono"
                >
                  <option value="zfs">ZFS (Recommended)</option>
                  <option value="btrfs">Btrfs</option>
                  <option value="lvm">LVM-Thin</option>
                  <option value="dir">Directory</option>
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Node Target</label>
                <input
                  type="text"
                  required
                  value={poolNodeId}
                  onChange={(e) => setPoolNodeId(e.target.value)}
                  className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono"
                />
              </div>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-300 mb-1">Total Capacity (GB)</label>
              <input
                type="number"
                required
                value={poolSizeGb}
                onChange={(e) => setPoolSizeGb(parseInt(e.target.value))}
                className="w-full px-3.5 py-2 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono"
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
                Register Storage Pool
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Delete Pool Dialog */}
      <ConfirmDialog
        isOpen={!!deleteTarget}
        title={`Delete Storage Pool "${deleteTarget?.name}"?`}
        message="Deleting this storage pool will unbind it from Aurora. Persistent block volumes associated with this pool must be detached beforehand."
        confirmText="Delete Pool"
        isDestructive={true}
        onConfirm={handleDeletePool}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
};
