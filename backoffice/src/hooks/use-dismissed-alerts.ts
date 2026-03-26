"use client";

import { useCallback, useSyncExternalStore } from "react";

const STORAGE_KEY = "dashboard_dismissed_alerts";
const EXPIRY_MS = 24 * 60 * 60 * 1000; // 24 hours

type Entry = { id: string; dismissedAt: number };

function getEntries(): Entry[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const entries = JSON.parse(raw) as Entry[];
    const now = Date.now();
    // prune expired entries
    return entries.filter((e) => now - e.dismissedAt < EXPIRY_MS);
  } catch {
    return [];
  }
}

function saveEntries(entries: Entry[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
  // notify subscribers
  window.dispatchEvent(new Event("dismissed-alerts-change"));
}

function subscribe(callback: () => void): () => void {
  window.addEventListener("dismissed-alerts-change", callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener("dismissed-alerts-change", callback);
    window.removeEventListener("storage", callback);
  };
}

function getSnapshot(): string {
  return JSON.stringify(getEntries());
}

function getServerSnapshot(): string {
  return "[]";
}

export function useDismissedAlerts() {
  const raw = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  const entries = JSON.parse(raw) as Entry[];

  const isDismissed = useCallback(
    (alertId: string): boolean => {
      return entries.some((e) => e.id === alertId);
    },
    [entries],
  );

  const dismiss = useCallback((alertId: string) => {
    const current = getEntries();
    if (current.some((e) => e.id === alertId)) return;
    saveEntries([...current, { id: alertId, dismissedAt: Date.now() }]);
  }, []);

  return { isDismissed, dismiss };
}
