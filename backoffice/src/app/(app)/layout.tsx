import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { Toaster } from "sonner";

import {
  SidebarProvider,
  SidebarInset,
  SidebarTrigger,
} from "~/components/ui/sidebar";
import { AppSidebar } from "~/components/app-sidebar";
import { AppShellClient } from "~/components/app-shell-client";
import { api } from "~/trpc/server";

export default async function AppLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const cookieStore = await cookies();
  const session = cookieStore.get("session");

  if (!session?.value) {
    redirect("/login");
  }

  let user: Awaited<ReturnType<typeof api.auth.me>>["user"];
  try {
    ({ user } = await api.auth.me());
  } catch {
    redirect("/login?expired=1");
  }

  if (!user) {
    redirect("/login?expired=1");
  }

  const sidebarOpen = cookieStore.get("sidebar_state")?.value !== "false";

  return (
    <SidebarProvider defaultOpen={sidebarOpen}>
      <AppSidebar user={user} />
      <SidebarInset>
        <AppShellClient />
        <header className="flex h-12 items-center px-4 md:hidden">
          <SidebarTrigger />
        </header>
        <div className="flex flex-1 flex-col overflow-auto p-4 md:p-6 lg:p-8">
          <div className="mx-auto flex w-full max-w-6xl flex-1 flex-col">{children}</div>
        </div>
      </SidebarInset>
      <Toaster theme="system" />
    </SidebarProvider>
  );
}
