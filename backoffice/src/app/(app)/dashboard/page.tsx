import { api } from "~/trpc/server";
import { DashboardContent } from "~/components/dashboard/dashboard-content";

export const metadata = {
  title: "Dashboard - MYJUNGLE Backoffice",
};

export default async function DashboardPage() {
  const { user } = await api.auth.me();

  return <DashboardContent user={user} />;
}
