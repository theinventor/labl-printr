import { useEffect, useState } from 'react';
import { api, type Job } from '../api';
import { Spinner, StateChip, useToast } from '../ui';

function timeAgo(iso: string): string {
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 60) return 'just now';
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return new Date(iso).toLocaleDateString();
}

export function JobsPage() {
  const toast = useToast();
  const [jobs, setJobs] = useState<Job[] | null>(null);

  const load = () => api.jobs(60).then(setJobs);

  useEffect(() => {
    load();
    const t = setInterval(load, 4000);
    return () => clearInterval(t);
  }, []);

  const reprint = async (id: number) => {
    try {
      const job = await api.reprint(id);
      toast('ok', `Job ${job.id} queued (reprint of ${id})`);
      load();
    } catch (e) {
      toast('err', (e as Error).message);
    }
  };

  if (!jobs) return <Spinner />;

  if (jobs.length === 0) {
    return (
      <div className="flex h-64 flex-col items-center justify-center gap-2 text-fg-dim">
        <span className="text-[15px]">No jobs yet</span>
        <span className="text-[13px]">Print something and it shows up here.</span>
      </div>
    );
  }

  return (
    <div className="space-y-2.5">
      {jobs.map((j) => (
        <div
          key={j.id}
          className="flex items-center gap-4 rounded-xl border border-edge bg-panel p-3.5 transition-colors hover:border-edge-2"
        >
          <div className="paper w-20 shrink-0 overflow-hidden">
            <img src={`/api/jobs/${j.id}/preview.png`} alt="" className="block w-full" loading="lazy" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2.5">
              <span className="font-mono text-[13px] text-fg-dim">#{j.id}</span>
              <span className="truncate text-[15px] font-medium">
                {j.templateId || 'raw ZPL'}
              </span>
              <StateChip state={j.state} />
            </div>
            <div className="mt-1 truncate font-mono text-[12px] text-fg-dim">
              {j.printerName} · {j.copies > 1 ? `${j.copies} copies · ` : ''}
              {j.source} · {timeAgo(j.createdAt)}
              {j.error && <span className="text-accent-hi"> — {j.error}</span>}
            </div>
          </div>
          <button
            onClick={() => reprint(j.id)}
            className="shrink-0 rounded-lg border border-edge px-3.5 py-2 text-[13px] font-medium text-fg-dim transition-colors hover:border-edge-2 hover:text-fg"
          >
            Reprint
          </button>
        </div>
      ))}
    </div>
  );
}
