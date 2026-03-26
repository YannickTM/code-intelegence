"use client";

import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { useEventsStore, JOB_EVENTS, type JobEvent } from "~/stores/events-store";
import { type api } from "~/trpc/react";

type TRPCUtils = ReturnType<typeof api.useUtils>;

const MEMBER_EVENTS = [
  "member:added",
  "member:removed",
  "member:role_updated",
] as const;

type MemberEventType = (typeof MEMBER_EVENTS)[number];

function isJobEvent(data: unknown): data is JobEvent {
  if (typeof data !== "object" || data === null) return false;
  const d = data as Record<string, unknown>;
  return (
    typeof d.event === "string" &&
    (JOB_EVENTS as readonly string[]).includes(d.event) &&
    typeof d.project_id === "string" &&
    typeof d.job_id === "string" &&
    typeof d.timestamp === "string" &&
    typeof d.data === "object" &&
    d.data !== null &&
    typeof (d.data as Record<string, unknown>).status === "string"
  );
}

function isMemberEvent(
  data: unknown,
): data is { project_id: string; event: MemberEventType } {
  if (typeof data !== "object" || data === null) return false;
  const d = data as Record<string, unknown>;
  return (
    typeof d.event === "string" &&
    (MEMBER_EVENTS as readonly string[]).includes(d.event) &&
    typeof d.project_id === "string"
  );
}

function resolveProjectName(utils: TRPCUtils, projectId: string): string {
  const projects = utils.users.listMyProjects.getData();
  const project = projects?.items.find((p) => p.id === projectId);
  return project?.name ?? projectId.slice(0, 8);
}

export function useSSEConnection(utils: TRPCUtils) {
  const setConnected = useEventsStore((s) => s.setConnected);
  const addJobEvent = useEventsStore((s) => s.addJobEvent);
  const retryRef = useRef(0);
  const utilsRef = useRef(utils);
  utilsRef.current = utils;

  useEffect(() => {
    let es: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout>;
    let unmounted = false;

    function connect() {
      if (unmounted) return;

      es = new EventSource("/api/events/stream");

      es.onopen = () => {
        setConnected(true);
        retryRef.current = 0;
      };

      // ── Connected ──
      es.addEventListener("connected", () => {
        setConnected(true);
      });

      // ── Job events ──
      const handleJobEvent = (e: MessageEvent) => {
        try {
          const raw: unknown = JSON.parse(e.data as string);
          if (!isJobEvent(raw)) return;
          const data = raw;
          addJobEvent(data);

          // Toasts for terminal states
          const name = resolveProjectName(utilsRef.current, data.project_id);
          if (data.event === "job:completed") {
            toast.success(`Indexing completed for ${name}`);
            void utilsRef.current.projects.structure.invalidate({
              id: data.project_id,
            });
            void utilsRef.current.projectSearch.fileMetadata.invalidate();
          } else if (data.event === "job:failed") {
            toast.error(`Indexing failed for ${name}`);
          }

          // Invalidate queries — skip for frequent progress ticks (jobs list
          // already polls at 3 s when active jobs exist)
          if (data.event !== "job:progress") {
            void utilsRef.current.dashboard.summary.invalidate();
            void utilsRef.current.users.listMyProjects.invalidate();
            void utilsRef.current.projectIndexing.listJobs.invalidate();
          }
        } catch {
          /* ignore malformed */
        }
      };
      for (const evt of JOB_EVENTS) es.addEventListener(evt, handleJobEvent);

      // ── Snapshot events ──
      es.addEventListener("snapshot:activated", () => {
        void utilsRef.current.dashboard.summary.invalidate();
        void utilsRef.current.users.listMyProjects.invalidate();
      });

      // ── Membership events ──
      const handleMemberEvent = (e: MessageEvent) => {
        try {
          const raw: unknown = JSON.parse(e.data as string);
          if (!isMemberEvent(raw)) return;
          const data = raw;
          const name = resolveProjectName(utilsRef.current, data.project_id);

          if (data.event === "member:added") {
            toast.info(`A new member was added to ${name}`);
          } else if (data.event === "member:removed") {
            toast.info(`A member was removed from ${name}`);
          } else if (data.event === "member:role_updated") {
            toast.info(`A member's role was updated in ${name}`);
          }

          void utilsRef.current.projectMembers.list.invalidate();
        } catch {
          /* ignore malformed */
        }
      };
      for (const evt of MEMBER_EVENTS)
        es.addEventListener(evt, handleMemberEvent);

      // ── Error / reconnect ──
      es.onerror = () => {
        setConnected(false);
        es?.close();
        if (unmounted) return;
        const delay = Math.min(1000 * 2 ** retryRef.current, 30_000);
        retryRef.current++;
        timer = setTimeout(connect, delay);
      };
    }

    connect();

    return () => {
      unmounted = true;
      es?.close();
      clearTimeout(timer);
      setConnected(false);
    };
  }, [setConnected, addJobEvent]);
}
