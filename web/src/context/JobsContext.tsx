import React, { createContext, useContext, useState, useCallback } from 'react';

export interface BackgroundJob {
  id: string;
  type: string;
  title: string;
  targetId: string;
  targetName: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  progressPercent?: number;
  errorMessage?: string;
  startedAt: string;
  finishedAt?: string;
}

interface JobsContextType {
  jobs: BackgroundJob[];
  isDrawerOpen: boolean;
  openDrawer: () => void;
  closeDrawer: () => void;
  toggleDrawer: () => void;
  addJob: (job: Omit<BackgroundJob, 'id' | 'startedAt' | 'status'>) => string;
  updateJob: (id: string, updates: Partial<BackgroundJob>) => void;
  clearCompletedJobs: () => void;
}

const JobsContext = createContext<JobsContextType | undefined>(undefined);

export const JobsProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [jobs, setJobs] = useState<BackgroundJob[]>([]);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  const addJob = useCallback((job: Omit<BackgroundJob, 'id' | 'startedAt' | 'status'>): string => {
    const id = 'job-' + Math.random().toString(36).substring(2, 9);
    const newJob: BackgroundJob = {
      ...job,
      id,
      status: 'running',
      startedAt: new Date().toISOString(),
    };
    setJobs((prev) => [newJob, ...prev]);
    return id;
  }, []);

  const updateJob = useCallback((id: string, updates: Partial<BackgroundJob>) => {
    setJobs((prev) =>
      prev.map((j) => {
        if (j.id === id) {
          const updated = { ...j, ...updates };
          if ((updates.status === 'completed' || updates.status === 'failed') && !updated.finishedAt) {
            updated.finishedAt = new Date().toISOString();
          }
          return updated;
        }
        return j;
      })
    );
  }, []);

  const clearCompletedJobs = useCallback(() => {
    setJobs((prev) => prev.filter((j) => j.status === 'running' || j.status === 'pending'));
  }, []);

  return (
    <JobsContext.Provider
      value={{
        jobs,
        isDrawerOpen,
        openDrawer: () => setIsDrawerOpen(true),
        closeDrawer: () => setIsDrawerOpen(false),
        toggleDrawer: () => setIsDrawerOpen((prev) => !prev),
        addJob,
        updateJob,
        clearCompletedJobs,
      }}
    >
      {children}
    </JobsContext.Provider>
  );
};

export const useJobs = () => {
  const context = useContext(JobsContext);
  if (!context) {
    throw new Error('useJobs must be used within a JobsProvider');
  }
  return context;
};
