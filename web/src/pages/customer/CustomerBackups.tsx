import React, { useState, useEffect } from 'react';
import {
  ShieldCheck,
  Plus,
  RefreshCw,
  Trash2,
  CheckCircle2,
  AlertTriangle,
  Lock,
  DownloadCloud,
  FileCheck,
  Server,
  Calendar,
} from 'lucide-react';
import { api, BackupRecord, Instance } from '../../lib/api';
import { useToast } from '../../context/ToastContext';

export const CustomerBackups: React.FC = () => {
  const [backups, setBackups] = useState<BackupRecord[]>([]);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [selectedInstanceId, setSelectedInstanceId] = useState('');
  const [backupType, setBackupType] = useState<'point_in_time' | 'full'>('point_in_time');
  const [retentionDays, setRetentionDays] = useState(30);
  const [creating, setCreating] = useState(false);
  const [verifyingId, setVerifyingId] = useState<string | null>(null);

  const toast = useToast();

  const fetchData = async () => {
    try {
      setLoading(true);
      const [backupsData, instancesData] = await Promise.all([
        api.listBackups({ limit: 100 }),
        api.listInstances().catch(() => []),
      ]);
      setBackups(backupsData.backups || []);
      setInstances(instancesData || []);
      if (instancesData.length > 0 && !selectedInstanceId) {
        setSelectedInstanceId(instancesData[0].id);
      }
    } catch (err: any) {
      toast.error(err.message || 'Failed to load backups');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleCreateBackup = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedInstanceId) {
      toast.error('Please select an instance to back up');
      return;
    }

    try {
      setCreating(true);
      const targetInst = instances.find((i) => i.id === selectedInstanceId);
      await api.createBackup({
        resourceType: 'instance',
        resourceId: selectedInstanceId,
        type: backupType,
        retentionDays,
        metadata: {
          instanceName: targetInst?.name || selectedInstanceId,
          triggeredBy: 'customer_portal',
        },
      });
      toast.success('Backup initiated and stored securely');
      setShowCreateModal(false);
      fetchData();
    } catch (err: any) {
      toast.error(err.message || 'Failed to create backup');
    } finally {
      setCreating(false);
    }
  };

  const handleVerify = async (backup: BackupRecord) => {
    try {
      setVerifyingId(backup.id);
      const res = await api.verifyBackup(backup.id);
      toast.success(`Integrity verified! SHA-256 Checksum: ${res.checksum.slice(0, 16)}...`);
      fetchData();
    } catch (err: any) {
      toast.error(err.message || 'Backup verification failed');
    } finally {
      setVerifyingId(null);
    }
  };

  const handleDelete = async (backup: BackupRecord) => {
    if (backup.isProtectedPoint) {
      toast.error('Cannot delete a protected recovery point');
      return;
    }
    if (!confirm(`Permanently delete backup ${backup.id}? This action cannot be undone.`)) {
      return;
    }
    try {
      await api.deleteBackup(backup.id);
      toast.success('Backup deleted');
      fetchData();
    } catch (err: any) {
      toast.error(err.message || 'Failed to delete backup');
    }
  };

  const formatBytes = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-slate-900 border border-slate-800 rounded-xl p-6 shadow-sm">
        <div>
          <h1 className="text-2xl font-bold text-white flex items-center gap-3">
            <ShieldCheck className="w-7 h-7 text-indigo-400" />
            Backups & Recovery Points
          </h1>
          <p className="text-sm text-slate-400 mt-1">
            Encrypted snapshots with SHA-256 tamper-evident integrity verification and point-in-time recovery.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={fetchData}
            className="p-2.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg transition-colors border border-slate-700/60"
            title="Refresh"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
          <button
            onClick={() => setShowCreateModal(true)}
            className="flex items-center gap-2 px-4 py-2.5 bg-indigo-600 hover:bg-indigo-500 text-white font-medium rounded-lg shadow-sm transition-all"
          >
            <Plus className="w-4 h-4" />
            Create Backup
          </button>
        </div>
      </div>

      {/* Overview Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">
          <div className="flex items-center justify-between text-slate-400 mb-2">
            <span className="text-xs font-semibold uppercase tracking-wider">Total Backups</span>
            <DownloadCloud className="w-4 h-4 text-indigo-400" />
          </div>
          <div className="text-2xl font-bold text-white">{backups.length}</div>
          <div className="text-xs text-slate-500 mt-1">Across all instances</div>
        </div>

        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">
          <div className="flex items-center justify-between text-slate-400 mb-2">
            <span className="text-xs font-semibold uppercase tracking-wider">Verified Points</span>
            <CheckCircle2 className="w-4 h-4 text-emerald-400" />
          </div>
          <div className="text-2xl font-bold text-emerald-400">
            {backups.filter((b) => b.status === 'verified').length}
          </div>
          <div className="text-xs text-slate-500 mt-1">SHA-256 checksum verified</div>
        </div>

        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5">
          <div className="flex items-center justify-between text-slate-400 mb-2">
            <span className="text-xs font-semibold uppercase tracking-wider">Protected Points</span>
            <Lock className="w-4 h-4 text-amber-400" />
          </div>
          <div className="text-2xl font-bold text-amber-400">
            {backups.filter((b) => b.isProtectedPoint).length}
          </div>
          <div className="text-xs text-slate-500 mt-1">Deletion protection enabled</div>
        </div>
      </div>

      {/* Backups Table */}
      <div className="bg-slate-900 border border-slate-800 rounded-xl overflow-hidden shadow-sm">
        <div className="p-4 border-b border-slate-800 flex items-center justify-between">
          <h2 className="text-base font-semibold text-white">Instance Recovery Points</h2>
          <span className="text-xs text-slate-400">{backups.length} records</span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-300">
            <thead className="bg-slate-950/60 text-xs font-semibold uppercase tracking-wider text-slate-400 border-b border-slate-800">
              <tr>
                <th className="px-6 py-3.5">Backup ID / Target</th>
                <th className="px-6 py-3.5">Type</th>
                <th className="px-6 py-3.5">Status</th>
                <th className="px-6 py-3.5">Size</th>
                <th className="px-6 py-3.5">SHA-256 Integrity</th>
                <th className="px-6 py-3.5">Created</th>
                <th className="px-6 py-3.5 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60">
              {loading && backups.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-6 py-12 text-center text-slate-500">
                    <RefreshCw className="w-6 h-6 animate-spin mx-auto mb-2 text-indigo-400" />
                    Loading recovery points...
                  </td>
                </tr>
              ) : backups.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-6 py-12 text-center text-slate-500">
                    <ShieldCheck className="w-8 h-8 mx-auto mb-2 text-slate-600" />
                    No backups found. Create your first point-in-time backup above.
                  </td>
                </tr>
              ) : (
                backups.map((b) => (
                  <tr key={b.id} className="hover:bg-slate-800/30 transition-colors">
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-2">
                        <Server className="w-4 h-4 text-indigo-400" />
                        <div>
                          <div className="font-medium text-white font-mono text-xs">{b.id.slice(0, 13)}...</div>
                          <div className="text-xs text-slate-400 mt-0.5">
                            {b.metadata?.instanceName || b.resourceId || 'Cluster Target'}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-slate-800 text-slate-300 border border-slate-700">
                        {b.type}
                      </span>
                    </td>
                    <td className="px-6 py-4">
                      {b.status === 'verified' && (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                          <CheckCircle2 className="w-3.5 h-3.5" />
                          Verified
                        </span>
                      )}
                      {b.status === 'pending' && (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-amber-500/10 text-amber-400 border border-amber-500/20">
                          <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                          Pending
                        </span>
                      )}
                      {b.status === 'running' && (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
                          <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                          Running
                        </span>
                      )}
                      {b.status === 'failed' && (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-rose-500/10 text-rose-400 border border-rose-500/20">
                          <AlertTriangle className="w-3.5 h-3.5" />
                          Failed
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-4 font-mono text-xs text-slate-300">
                      {formatBytes(b.sizeBytes)}
                    </td>
                    <td className="px-6 py-4 font-mono text-xs text-slate-400">
                      <div className="flex items-center gap-1.5" title={b.checksumSha256}>
                        <FileCheck className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                        <span className="truncate max-w-[120px]">{b.checksumSha256 || 'Computing...'}</span>
                      </div>
                    </td>
                    <td className="px-6 py-4 text-xs text-slate-400">
                      {new Date(b.createdAt).toLocaleString()}
                    </td>
                    <td className="px-6 py-4 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => handleVerify(b)}
                          disabled={verifyingId === b.id}
                          className="p-1.5 text-slate-400 hover:text-emerald-400 hover:bg-slate-800 rounded transition-colors"
                          title="Verify SHA-256 Checksum"
                        >
                          <RefreshCw className={`w-4 h-4 ${verifyingId === b.id ? 'animate-spin' : ''}`} />
                        </button>
                        {b.isProtectedPoint ? (
                          <span title="Protected recovery point — deletion blocked">
                            <Lock className="w-4 h-4 text-amber-400/80 mx-1.5" />
                          </span>
                        ) : (
                          <button
                            onClick={() => handleDelete(b)}
                            className="p-1.5 text-slate-400 hover:text-rose-400 hover:bg-slate-800 rounded transition-colors"
                            title="Delete Backup"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Create Backup Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 bg-slate-950/80 backdrop-blur-sm z-50 flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-5">
            <div className="flex items-center justify-between border-b border-slate-800 pb-4">
              <div className="flex items-center gap-2.5">
                <ShieldCheck className="w-6 h-6 text-indigo-400" />
                <h3 className="text-lg font-bold text-white">Create New Backup</h3>
              </div>
              <button
                onClick={() => setShowCreateModal(false)}
                className="text-slate-400 hover:text-white transition-colors"
              >
                ✕
              </button>
            </div>

            <form onSubmit={handleCreateBackup} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold uppercase tracking-wider text-slate-400 mb-1.5">
                  Select Target Instance
                </label>
                <select
                  value={selectedInstanceId}
                  onChange={(e) => setSelectedInstanceId(e.target.value)}
                  className="w-full bg-slate-950 border border-slate-800 rounded-lg px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
                  required
                >
                  {instances.map((inst) => (
                    <option key={inst.id} value={inst.id}>
                      {inst.name} ({inst.id.slice(0, 8)}...) — {inst.status}
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wider text-slate-400 mb-1.5">
                  Backup Strategy
                </label>
                <div className="grid grid-cols-2 gap-3">
                  <button
                    type="button"
                    onClick={() => setBackupType('point_in_time')}
                    className={`p-3 rounded-lg border text-left transition-all ${
                      backupType === 'point_in_time'
                        ? 'border-indigo-500 bg-indigo-500/10 text-white'
                        : 'border-slate-800 bg-slate-950/60 text-slate-400 hover:border-slate-700'
                    }`}
                  >
                    <div className="text-sm font-semibold">Point in Time</div>
                    <div className="text-xs text-slate-500 mt-0.5">Live consistent snapshot</div>
                  </button>
                  <button
                    type="button"
                    onClick={() => setBackupType('full')}
                    className={`p-3 rounded-lg border text-left transition-all ${
                      backupType === 'full'
                        ? 'border-indigo-500 bg-indigo-500/10 text-white'
                        : 'border-slate-800 bg-slate-950/60 text-slate-400 hover:border-slate-700'
                    }`}
                  >
                    <div className="text-sm font-semibold">Full Image</div>
                    <div className="text-xs text-slate-500 mt-0.5">Complete disk & state export</div>
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold uppercase tracking-wider text-slate-400 mb-1.5">
                  Retention Policy (Days)
                </label>
                <div className="flex items-center gap-3">
                  <Calendar className="w-4 h-4 text-slate-400" />
                  <input
                    type="number"
                    min={1}
                    max={365}
                    value={retentionDays}
                    onChange={(e) => setRetentionDays(parseInt(e.target.value) || 30)}
                    className="w-full bg-slate-950 border border-slate-800 rounded-lg px-3.5 py-2.5 text-sm text-white focus:outline-none focus:border-indigo-500"
                  />
                </div>
              </div>

              <div className="pt-3 border-t border-slate-800 flex justify-end gap-3">
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 font-medium rounded-lg text-sm transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creating}
                  className="flex items-center gap-2 px-5 py-2 bg-indigo-600 hover:bg-indigo-500 text-white font-medium rounded-lg text-sm shadow-sm transition-all"
                >
                  {creating && <RefreshCw className="w-4 h-4 animate-spin" />}
                  Create Backup
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
