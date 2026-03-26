import React, { useState, useEffect, useCallback } from "react";
import { Button, Card, Spinner } from "@acme/ui-kit";
import { formatDate } from "../../utils/format";
import type { Theme } from "../types";

/** Shape of a single user record returned by the API. */
export interface User {
  id: string;
  name: string;
  email: string;
  createdAt: string;
}

/** Props accepted by the UserList component. */
interface UserListProps {
  endpoint: string;
  pageSize?: number;
  theme?: Theme;
}

/** Union of possible loading states. */
export type LoadingState = "idle" | "loading" | "error" | "success";

/**
 * UserList displays a paginated list of users fetched from a remote API.
 * Demonstrates hooks, JSX, fetch calls, and export patterns.
 */
export default function UserList({ endpoint, pageSize = 20, theme }: UserListProps) {
  const [users, setUsers] = useState<User[]>([]);
  const [state, setState] = useState<LoadingState>("idle");
  const [error, setError] = useState<string | null>(null);

  const fetchUsers = useCallback(async () => {
    setState("loading");
    try {
      const response = await fetch(`${endpoint}?limit=${pageSize}`);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      const data: User[] = await response.json();
      setUsers(data);
      setState("success");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      setState("error");
    }
  }, [endpoint, pageSize]);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  if (state === "loading") {
    return (
      <Card className="user-list-loading">
        <Spinner size="lg" />
        <p>Loading users...</p>
      </Card>
    );
  }

  if (state === "error") {
    return (
      <Card className="user-list-error">
        <p>Error: {error}</p>
        <Button onClick={fetchUsers}>Retry</Button>
      </Card>
    );
  }

  return (
    <Card className="user-list">
      <h2>Users ({users.length})</h2>
      <ul>
        {users.map((user) => (
          <li key={user.id}>
            <strong>{user.name}</strong>
            <span>{user.email}</span>
            <time dateTime={user.createdAt}>{formatDate(user.createdAt)}</time>
          </li>
        ))}
      </ul>
      <>
        <Button onClick={fetchUsers}>Refresh</Button>
      </>
    </Card>
  );
}

export function useUserCount(users: User[]): number {
  return users.length;
}
