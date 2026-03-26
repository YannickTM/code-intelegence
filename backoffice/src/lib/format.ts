/**
 * Shared formatting utilities.
 */

export function getInitials(name: string): string {
  return name
    .split(/[\s_-]+/)
    .map((w) => w[0])
    .filter(Boolean)
    .slice(0, 2)
    .join("")
    .toUpperCase();
}

export function formatRelativeTime(date: string | Date): string {
  const dateObj = new Date(date);
  if (isNaN(dateObj.getTime())) return "";

  // Clamp future timestamps (clock skew) to zero
  const ms = Math.max(0, Date.now() - dateObj.getTime());
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function formatExpiresAt(date: string | Date): string {
  const dateObj = new Date(date);
  if (isNaN(dateObj.getTime())) return "";

  const ms = dateObj.getTime() - Date.now();
  if (ms <= 0) return "Expired";

  if (ms >= 365 * 86_400_000) return `in ${Math.ceil(ms / (365 * 86_400_000))}y`;
  if (ms >= 86_400_000) return `in ${Math.ceil(ms / 86_400_000)}d`;
  if (ms >= 3_600_000) return `in ${Math.ceil(ms / 3_600_000)}h`;
  return "< 1h";
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "0 B";
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  );
  let value = bytes / Math.pow(1024, i);
  // Bump unit when rounding pushes value to 1024 (e.g. 1023.95 → "1024.0 KB")
  while (i > 0 && i < units.length - 1 && Number(value.toFixed(1)) >= 1024) {
    i++;
    value /= 1024;
  }
  return `${i === 0 ? value : value.toFixed(1)} ${units[i]}`;
}

export function formatUptime(startedAt: string): string {
  const start = new Date(startedAt).getTime();
  if (isNaN(start)) return "—";

  const ms = Math.max(0, Date.now() - start);
  const totalSeconds = Math.floor(ms / 1000);

  if (totalSeconds < 60) return `${totalSeconds}s`;
  const totalMinutes = Math.floor(totalSeconds / 60);
  if (totalMinutes < 60) {
    const secs = totalSeconds % 60;
    return secs > 0 ? `${totalMinutes}m ${secs}s` : `${totalMinutes}m`;
  }
  const hours = Math.floor(totalMinutes / 60);
  const mins = totalMinutes % 60;
  if (hours < 24) {
    return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
  }
  const days = Math.floor(hours / 24);
  const remainHours = hours % 24;
  return remainHours > 0 ? `${days}d ${remainHours}h` : `${days}d`;
}

export function formatHeartbeat(date: string | Date): string {
  const dateObj = new Date(date);
  if (isNaN(dateObj.getTime())) return "";

  const ms = Math.max(0, Date.now() - dateObj.getTime());
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function formatDuration(startedAt: string, finishedAt: string): string {
  const start = new Date(startedAt).getTime();
  const end = new Date(finishedAt).getTime();
  if (isNaN(start) || isNaN(end)) return "—";

  const ms = Math.max(0, end - start);
  const totalSeconds = Math.floor(ms / 1000);

  if (totalSeconds < 60) return `${totalSeconds}s`;
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return seconds > 0 ? `${minutes}m ${seconds}s` : `${minutes}m`;
}
