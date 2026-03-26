"use client";

import { useEffect, useMemo, useState } from "react";
import { AlertCircle, Search } from "lucide-react";
import { keepPreviousData } from "@tanstack/react-query";
import { Button } from "~/components/ui/button";
import { Input } from "~/components/ui/input";
import { Skeleton } from "~/components/ui/skeleton";
import { api } from "~/trpc/react";
import { useDebounce } from "~/hooks/use-debounce";
import { cn } from "~/lib/utils";
import { UserTable } from "./user-table";

// ── Constants ───────────────────────────────────────────────────────────────

const PAGE_SIZE = 20;

type StatusFilter = "all" | "active" | "inactive";
type RoleFilter = "all" | "admin" | "regular";

// ── Pagination helper (from commits-content.tsx) ────────────────────────────

function getVisiblePages(
  current: number,
  total: number,
): (number | "ellipsis")[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i);
  const pages: (number | "ellipsis")[] = [0];
  const start = Math.max(1, current - 1);
  const end = Math.min(total - 2, current + 1);
  if (start > 1) pages.push("ellipsis");
  for (let i = start; i <= end; i++) pages.push(i);
  if (end < total - 2) pages.push("ellipsis");
  pages.push(total - 1);
  return pages;
}

// ── Filter button helper ────────────────────────────────────────────────────

function FilterButton<T extends string>({
  value,
  current,
  onChange,
  children,
}: {
  value: T;
  current: T;
  onChange: (v: T) => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={() => onChange(value)}
      className={cn(
        "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
        value === current
          ? "bg-primary text-primary-foreground"
          : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
      )}
    >
      {children}
    </button>
  );
}

// ── Main component ──────────────────────────────────────────────────────────

export function UserManagementContent() {
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebounce(search, 300);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [roleFilter, setRoleFilter] = useState<RoleFilter>("all");
  const [page, setPage] = useState(0);

  const meQuery = api.auth.me.useQuery();
  const currentUserId = meQuery.data?.user?.id ?? "";

  // When a role filter is active the backend cannot filter by role, so we
  // fetch ALL users and paginate client-side. When roleFilter is "all" we
  // keep efficient server-side pagination.
  const isRoleFiltered = roleFilter !== "all";

  // Reset page when any filter changes
  useEffect(() => {
    setPage(0);
  }, [debouncedSearch, statusFilter, roleFilter]);

  const usersQuery = api.platformUsers.list.useQuery(
    {
      // Fetch all when role-filtering client-side; otherwise paginate server-side
      limit: isRoleFiltered ? 200 : PAGE_SIZE,
      offset: isRoleFiltered ? 0 : page * PAGE_SIZE,
      search: debouncedSearch || undefined,
      is_active:
        statusFilter === "all" ? undefined : statusFilter === "active",
      sort: "created_at",
    },
    { placeholderData: keepPreviousData },
  );

  // Client-side role filter (only meaningful when isRoleFiltered)
  const allFilteredItems = useMemo(() => {
    const items = usersQuery.data?.items ?? [];
    if (!isRoleFiltered) return items;
    if (roleFilter === "admin")
      return items.filter((u) => u.platform_roles.includes("platform_admin"));
    return items.filter((u) => !u.platform_roles.includes("platform_admin"));
  }, [usersQuery.data?.items, roleFilter, isRoleFiltered]);

  // Derive totals and paginate client-side when role-filtered
  const total = isRoleFiltered
    ? allFilteredItems.length
    : (usersQuery.data?.total ?? 0);
  const totalPages = Math.ceil(total / PAGE_SIZE);
  const filteredItems = isRoleFiltered
    ? allFilteredItems.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE)
    : allFilteredItems;

  // Clamp page when totalPages shrinks (e.g. after deactivation or filter change)
  useEffect(() => {
    if (totalPages > 0 && page >= totalPages) {
      setPage(totalPages - 1);
    }
  }, [totalPages, page]);

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div>
        <h2 className="text-lg font-semibold">User Management</h2>
        <p className="text-muted-foreground text-sm">
          Manage platform users, roles, and account status.
        </p>
      </div>

      {/* Toolbar */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        {/* Search */}
        <div className="relative max-w-sm flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
          <Input
            placeholder="Search users..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>

        {/* Status filter */}
        <div className="bg-muted flex items-center gap-0.5 rounded-lg p-1">
          <FilterButton
            value="all"
            current={statusFilter}
            onChange={setStatusFilter}
          >
            All
          </FilterButton>
          <FilterButton
            value="active"
            current={statusFilter}
            onChange={setStatusFilter}
          >
            Active
          </FilterButton>
          <FilterButton
            value="inactive"
            current={statusFilter}
            onChange={setStatusFilter}
          >
            Inactive
          </FilterButton>
        </div>
      </div>

      {/* Role filter */}
      <div className="flex items-center gap-2">
        <span className="text-muted-foreground text-sm">Role:</span>
        <div className="bg-muted flex items-center gap-0.5 rounded-lg p-1">
          <FilterButton
            value="all"
            current={roleFilter}
            onChange={setRoleFilter}
          >
            All
          </FilterButton>
          <FilterButton
            value="admin"
            current={roleFilter}
            onChange={setRoleFilter}
          >
            Platform Admin
          </FilterButton>
          <FilterButton
            value="regular"
            current={roleFilter}
            onChange={setRoleFilter}
          >
            Regular User
          </FilterButton>
        </div>
      </div>

      {/* Content */}
      {usersQuery.isLoading ? (
        <div className="rounded-md border">
          <div className="space-y-3 p-4">
            {[1, 2, 3, 4, 5].map((i) => (
              <div key={i} className="flex items-center gap-3">
                <Skeleton className="h-8 w-8 rounded-full" />
                <div className="flex-1 space-y-1">
                  <Skeleton className="h-4 w-32" />
                  <Skeleton className="h-3 w-24" />
                </div>
                <Skeleton className="h-5 w-20 rounded-full" />
                <Skeleton className="h-5 w-16 rounded-full" />
                <Skeleton className="h-4 w-16" />
                <Skeleton className="h-8 w-8" />
              </div>
            ))}
          </div>
        </div>
      ) : usersQuery.isError ? (
        <div className="text-destructive flex items-center gap-2 text-sm">
          <AlertCircle className="size-4" />
          <span>Failed to load users</span>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void usersQuery.refetch()}
          >
            Retry
          </Button>
        </div>
      ) : total === 0 ? (
        <div className="flex flex-1 items-center justify-center rounded-xl border border-dashed p-12">
          <div className="text-center">
            <p className="text-muted-foreground mb-2">
              No users match your filters
            </p>
            {(search || statusFilter !== "all") && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setSearch("");
                  setStatusFilter("all");
                  setRoleFilter("all");
                }}
              >
                Reset filters
              </Button>
            )}
          </div>
        </div>
      ) : (
        <>
          <UserTable items={filteredItems} currentUserId={currentUserId} />

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between">
              <p className="text-muted-foreground text-sm">
                Showing {page * PAGE_SIZE + 1}–
                {Math.min((page + 1) * PAGE_SIZE, total)} of {total} users
              </p>
              <div className="flex items-center gap-1">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page === 0}
                  onClick={() => setPage(page - 1)}
                >
                  Previous
                </Button>
                {getVisiblePages(page, totalPages).map((entry, idx) =>
                  entry === "ellipsis" ? (
                    <span
                      key={`ellipsis-${idx}`}
                      className="text-muted-foreground px-1 text-sm"
                    >
                      ...
                    </span>
                  ) : (
                    <Button
                      key={entry}
                      variant={entry === page ? "default" : "outline"}
                      size="sm"
                      onClick={() => setPage(entry)}
                    >
                      {entry + 1}
                    </Button>
                  ),
                )}
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages - 1}
                  onClick={() => setPage(page + 1)}
                >
                  Next
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
