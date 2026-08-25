import React, { useState, useEffect } from 'react';
import {
  Clock,
  Search,
  RefreshCw,
  XCircle,
  RotateCcw,
  CheckCircle2,
  AlertTriangle,
} from 'lucide-react';
import { api, Job } from '../../lib/api';
import { useToast } from '../../context/ToastContext';

export const AdminJobs: React.FC = () => {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [filterStatus, setFilterStatus] = useState<string>('all');
  const [search, setSearch] = useState<string>('');
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);

  const toast = useToast();

  const fetchJobs = async () => {
    try {
      setLoading(true);
      const params: { status?: string; limit: number } = { limit: 100 };
      if (filterStatus !== 'all') {
        params.status = filterStatus;
      }
      const data = await api.adminListJobs(params);
      setJobs(data.jobs || []);
    } catch (err: any) {
      toast.error(err.message || 'Failed to load asynchronous jobs');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchJobs();
    const timer = setInterval(fetchJobs, 5000);
    return () => clearInterval(timer);
  }, [filterStatus]);

  const handleCancel = async (job: Job) => {
    if (!confirm(`Cancel job ${job.id} (${job.type})?`)) return;
    try {
      await api.cancelJob(job.id, 'Canceled by administrator');
      toast.success('Job cancellation requested');
      fetchJobs();
    } catch (err: any) {
      toast.error(err.message || 'Failed to cancel job');
    }
  };

  const handleRetry = async (job: Job) => {
    try {
      await api.retryJob(job.id);
      toast.success('Job resubmitted for execution');
      fetchJobs();
    } catch (err: any) {
      toast.error(err.message || 'Failed to retry job');
    }
  };

  const filteredJobs = jobs.filter((j) => {
    const matchesSearch =
      j.id.toLowerCase().includes(search.toLowerCase()) ||
      j.type.toLowerCase().includes(search.toLowerCase()) ||
      (j.tenantId && j.tenantId.toLowerCase().includes(search.toLowerCase())) ||
      (j.resourceId && j.resourceId.toLowerCase().includes(search.toLowerCase()));
    return matchesSearch;
  });

  const getStatusBadge = (status: Job['status']) => {
    switch (status) {
      case 'running':
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-blue-500/10 text-blue-400 border border-blue-500/20">
            <RefreshCw className="w-3 h-3 animate-spin" /> Running
          </span>
        );
      case 'succeeded':
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
            <CheckCircle2 className="w-3 h-3" /> Succeeded
          </span>
        );
      case 'retrying':
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-amber-500/10 text-amber-400 border border-amber-500/20">
            <RotateCcw className="w-3 h-3 animate-spin" /> Retrying
          </span>
        );
      case 'failed':
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-rose-500/10 text-rose-400 border border-rose-500/20">
            <AlertTriangle className="w-3 h-3" /> Failed
          </span>
        );
      case 'canceled':
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-slate-500/10 text-slate-400 border border-slate-500/20">
            <XCircle className="w-3 h-3" /> Canceled
          </span>
        );
      default:
        return (
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-slate-500/10 text-slate-400 border border-slate-500/20">
            <Clock className="w-3 h-3" /> Pending
          </span>
        );
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-[#181f30] pb-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Clock className="w-5 h-5 text-blue-400" />
            <span>Infrastructure Asynchronous Job Orchestrator</span>
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            PostgreSQL-backed durable distributed job queue with lease heartbeats, retry backoff, and failure recovery.
          </p>
        </div>

        <button
          onClick={fetchJobs}
          className="px-3.5 py-2 rounded-xl bg-[#141824] hover:bg-[#1c2233] border border-[#232a3d] text-slate-300 text-xs font-semibold flex items-center gap-2 self-start sm:self-auto"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          <span>Refresh</span>
        </button>
      </div>

      {/* Filter Bar */}
      <div className="flex flex-col sm:flex-row gap-3">
        <div className="relative flex-1">
          <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-3" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search jobs by ID, type, tenant, or resource ID..."
            className="w-full pl-10 pr-4 py-2.5 rounded-xl bg-[#090b12] border border-[#1e2538] text-white text-xs font-mono focus:outline-none focus:border-blue-500"
          />
        </div>

        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0">
          {['all', 'pending', 'running', 'retrying', 'succeeded', 'failed', 'canceled'].map((st) => (
            <button
              key={st}
              onClick={() => setFilterStatus(st)}
              className={`px-3 py-2 rounded-xl text-xs font-semibold uppercase tracking-wider transition whitespace-nowrap ${
                filterStatus === st
                  ? 'bg-blue-600/20 text-blue-400 border border-blue-500/30'
                  : 'bg-[#0f121d] text-slate-400 border border-[#1c2235] hover:text-white'
              }`}
            >
              {st}
            </button>
          ))}
        </div>
      </div>

      {/* Jobs Table */}
      <div className="rounded-2xl bg-[#0f121d] border border-[#1c2235] overflow-hidden shadow-xl">
        <table className="w-full text-left text-xs font-mono">
          <thead>
            <tr className="border-b border-[#181f30] text-slate-400 bg-[#0a0d17]/50">
              <th className="py-3.5 px-4 font-semibold">Job ID & Type</th>
              <th className="py-3.5 px-4 font-semibold">Tenant & Target</th>
              <th className="py-3.5 px-4 font-semibold">Status & Progress</th>
              <th className="py-3.5 px-4 font-semibold">Worker / Leases</th>
              <th className="py-3.5 px-4 font-semibold">Created At</th>
              <th className="py-3.5 px-4 font-semibold text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#141a29]">
            {filteredJobs.length === 0 ? (
              <tr>
                <td colSpan={6} className="py-12 text-center text-slate-500 font-sans">
                  {loading ? 'Loading jobs from durable queue...' : 'No asynchronous jobs match your criteria.'}
                </td>
              </tr>
            ) : (
              filteredJobs.map((job) => (
                <tr
                  key={job.id}
                  onClick={() => setSelectedJob(job)}
                  className="hover:bg-[#141824]/60 transition cursor-pointer"
                >
                  <td className="py-3.5 px-4">
                    <div className="font-bold text-white font-mono">{job.id.substring(0, 8)}...</div>
                    <div className="text-[11px] text-blue-400 font-semibold uppercase">{job.type}</div>
                  </td>
                  <td className="py-3.5 px-4">
                    <div className="text-slate-300 font-medium">{job.tenantId}</div>
                    <div className="text-slate-500 text-[10px]">
                      {job.resourceType ? `${job.resourceType}:${job.resourceId || 'N/A'}` : 'System Task'}
                    </div>
                  </td>
                  <td className="py-3.5 px-4">
                    <div className="flex items-center gap-2">
                      {getStatusBadge(job.status)}
                      {job.status === 'running' && (
                        <span className="text-[11px] text-blue-400 font-bold">{job.progressPercent}%</span>
                      )}
                    </div>
                    {job.retryCount > 0 && (
                      <div className="text-[10px] text-amber-400 mt-1">
                        Attempt {job.retryCount}/{job.maxRetries}
                      </div>
                    )}
                  </td>
                  <td className="py-3.5 px-4">
                    {job.lockedByWorker ? (
                      <span className="text-slate-300 text-[11px] bg-slate-800/80 px-2 py-0.5 rounded">
                        {job.lockedByWorker}
                      </span>
                    ) : (
                      <span className="text-slate-600">—</span>
                    )}
                  </td>
                  <td className="py-3.5 px-4 text-slate-400">
                    {new Date(job.createdAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                  </td>
                  <td className="py-3.5 px-4 text-right" onClick={(e) => e.stopPropagation()}>
                    <div className="flex items-center justify-end gap-2">
                      {(job.status === 'pending' || job.status === 'running' || job.status === 'retrying') && (
                        <button
                          onClick={() => handleCancel(job)}
                          className="px-2.5 py-1 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 text-[11px] font-semibold"
                        >
                          Cancel
                        </button>
                      )}
                      {(job.status === 'failed' || job.status === 'canceled') && (
                        <button
                          onClick={() => handleRetry(job)}
                          className="px-2.5 py-1 rounded-lg bg-blue-500/10 hover:bg-blue-500/20 text-blue-400 border border-blue-500/20 text-[11px] font-semibold"
                        >
                          Retry
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

      {/* Selected Job Drawer */}
      {selectedJob && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex justify-end">
          <div className="w-full max-w-lg bg-[#0c0f18] border-l border-[#1f2639] h-full overflow-y-auto p-6 space-y-6 animate-in slide-in-from-right duration-200">
            <div className="flex items-center justify-between border-b border-[#1c2235] pb-4">
              <div>
                <h3 className="text-lg font-bold text-white font-mono">{selectedJob.id}</h3>
                <p className="text-xs text-blue-400 uppercase font-semibold">{selectedJob.type}</p>
              </div>
              <button
                onClick={() => setSelectedJob(null)}
                className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-[#141824]"
              >
                <XCircle className="w-5 h-5" />
              </button>
            </div>

            <div className="space-y-4 text-xs font-mono">
              <div className="p-4 rounded-xl bg-[#080a11] border border-[#181f30] space-y-2">
                <div className="flex justify-between">
                  <span className="text-slate-500">Status:</span>
                  <span>{getStatusBadge(selectedJob.status)}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-500">Tenant:</span>
                  <span className="text-white">{selectedJob.tenantId}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-500">Resource:</span>
                  <span className="text-white">{selectedJob.resourceType ? `${selectedJob.resourceType} (${selectedJob.resourceId})` : 'N/A'}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-500">Progress:</span>
                  <span className="text-white">{selectedJob.progressPercent}%</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-500">Retry Count:</span>
                  <span className="text-white">{selectedJob.retryCount} / {selectedJob.maxRetries}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-slate-500">Worker ID:</span>
                  <span className="text-white">{selectedJob.lockedByWorker || 'None'}</span>
                </div>
              </div>

              {selectedJob.error && (
                <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300">
                  <div className="font-bold flex items-center gap-1.5 mb-1">
                    <AlertTriangle className="w-4 h-4" /> Execution Error
                  </div>
                  <pre className="text-[11px] whitespace-pre-wrap">{selectedJob.error}</pre>
                </div>
              )}

              {selectedJob.result && (
                <div className="p-4 rounded-xl bg-[#080a11] border border-[#181f30]">
                  <div className="font-bold text-slate-400 mb-1">Execution Result Payload</div>
                  <pre className="text-[11px] text-emerald-400 overflow-x-auto">
                    {JSON.stringify(selectedJob.result, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
