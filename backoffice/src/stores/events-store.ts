import { create } from "zustand";

export const JOB_EVENTS = [
  "job:queued",
  "job:started",
  "job:progress",
  "job:completed",
  "job:failed",
] as const;

export type JobEventType = (typeof JOB_EVENTS)[number];

export type JobEvent = {
  event: JobEventType;
  project_id: string;
  job_id: string;
  timestamp: string;
  data: {
    status: string;
    job_type?: string;
    files_processed?: number;
    files_total?: number;
    chunks_upserted?: number;
    vectors_deleted?: number;
    error_message?: string;
  };
};

type EventsState = {
  connected: boolean;
  jobEvents: JobEvent[];
  lastEventAt: string | null;
  setConnected: (c: boolean) => void;
  addJobEvent: (e: JobEvent) => void;
  clearEvents: () => void;
};

export const useEventsStore = create<EventsState>()((set) => ({
  connected: false,
  jobEvents: [],
  lastEventAt: null,
  setConnected: (connected) => set({ connected }),
  addJobEvent: (event) =>
    set((s) => ({
      jobEvents: [event, ...s.jobEvents].slice(0, 100),
      lastEventAt: event.timestamp,
    })),
  clearEvents: () => set({ jobEvents: [], lastEventAt: null }),
}));
