import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { formatRelativeTime } from "./format";

describe("formatRelativeTime", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2025-06-15T12:00:00Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns 'just now' for less than 60 seconds ago", () => {
    expect(formatRelativeTime("2025-06-15T11:59:30Z")).toBe("just now");
  });

  it("returns minutes ago", () => {
    expect(formatRelativeTime("2025-06-15T11:55:00Z")).toBe("5m ago");
  });

  it("returns hours ago", () => {
    expect(formatRelativeTime("2025-06-15T09:00:00Z")).toBe("3h ago");
  });

  it("returns days ago", () => {
    expect(formatRelativeTime("2025-06-13T12:00:00Z")).toBe("2d ago");
  });

  it("accepts a Date object", () => {
    expect(formatRelativeTime(new Date("2025-06-15T11:55:00Z"))).toBe("5m ago");
  });

  it("accepts an ISO string", () => {
    expect(formatRelativeTime("2025-06-15T11:55:00Z")).toBe("5m ago");
  });

  it("returns empty string for invalid date", () => {
    expect(formatRelativeTime("not-a-date")).toBe("");
  });

  it("returns 'just now' for future dates (clock skew)", () => {
    expect(formatRelativeTime("2025-06-15T12:05:00Z")).toBe("just now");
  });
});
