import React from 'react';
import { X, CheckCircle2, AlertCircle, Loader2, Trash2, Cpu } from 'lucide-react';
import { useJobs } from '../context/JobsContext';

export const JobsDrawer: React.FC = () => {
  const { jobs, isDrawerOpen, closeDrawer, clearCompletedJobs } = useJobs();

  if (!isDrawerOpen) return null;

  return (
    <div className="fixed inset-0 z-50 overflow-hidden">
      <div
        onClick={closeDrawer}
        className="absolute inset-0 bg-black/60 backdrop-blur-xs transition-opacity"
      />

      <div className="fixed inset-y-0 right-0 max-w-full flex pl-10">
        <div className="w-screen max-w-md bg-[#0d101a] border-l border-[#1e2538] shadow-2xl flex flex-col">
          {/* Header */}
          <div className="p-4 border-b border-[#181f30] flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Cpu className="w-5 h-5 text-blue-400" />
              <h3 className="text-sm font-bold text-white">Background Operations & Tasks</h3>
            </div>
            <div className="flex items-center gap-2">
              {jobs.some((j) => j.status === 'completed' || j.status === 'failed') && (
                <button
                  onClick={clearCompletedJobs}
                  className="p-1.5 rounded-lg bg-[#141824] hover:bg-[#1c2233] text-slate-400 hover:text-white transition text-xs flex items-center gap-1"
                  title="Clear completed tasks"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span className="text-[10px]">Clear</span>
                </button>
              )}
              <button
                onClick={closeDrawer}
                className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-[#141824] transition"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Task List */}
          <div className="flex-1 overflow-y-auto p-4 space-y-3">
            {jobs.length === 0 ? (
              <div className="py-12 text-center text-xs text-slate-500">
                No active or recent background jobs.
              </div>
            ) : (
              jobs.map((job) => (
                <div
                  key={job.id}
                  className={`p-3.5 rounded-xl border transition ${
                    job.status === 'running'
                      ? 'bg-blue-950/20 border-blue-500/30'
                      : job.status === 'completed'
                      ? 'bg-[#0f1a16] border-emerald-500/20'
                      : 'bg-[#1e0f14] border-rose-500/20'
                  }`}
                >
                  <div className="flex items-start justify-between">
                    <div className="flex items-start gap-2.5">
                      <div className="mt-0.5">
                        {job.status === 'running' && (
                          <Loader2 className="w-4 h-4 text-blue-400 animate-spin" />
                        )}
                        {job.status === 'completed' && (
                          <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                        )}
                        {job.status === 'failed' && (
                          <AlertCircle className="w-4 h-4 text-rose-400" />
                        )}
                      </div>
                      <div>
                        <div className="text-xs font-semibold text-white">{job.title}</div>
                        <div className="text-[11px] text-slate-400 font-mono mt-0.5">
                          Target: {job.targetName}
                        </div>
                      </div>
                    </div>

                    <span
                      className={`text-[10px] px-2 py-0.5 rounded font-mono uppercase font-semibold ${
                        job.status === 'running'
                          ? 'bg-blue-500/20 text-blue-300'
                          : job.status === 'completed'
                          ? 'bg-emerald-500/20 text-emerald-300'
                          : 'bg-rose-500/20 text-rose-300'
                      }`}
                    >
                      {job.status}
                    </span>
                  </div>

                  {job.errorMessage && (
                    <div className="mt-2 text-[11px] text-rose-300 bg-rose-950/30 border border-rose-800/30 p-2 rounded-lg">
                      {job.errorMessage}
                    </div>
                  )}

                  <div className="mt-2.5 flex items-center justify-between text-[10px] text-slate-500 font-mono">
                    <span>Started: {new Date(job.startedAt).toLocaleTimeString()}</span>
                    {job.finishedAt && (
                      <span>Finished: {new Date(job.finishedAt).toLocaleTimeString()}</span>
                    )}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
