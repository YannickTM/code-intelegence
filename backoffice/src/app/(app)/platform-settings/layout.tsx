"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { cn } from "~/lib/utils";
import { Users, Database, Bot, Activity } from "lucide-react";
import { PageHeader } from "~/components/page-header";
import { api } from "~/trpc/react";

const platformSettingsNav = [
  { label: "Users", href: "/platform-settings/users", icon: Users },
  { label: "Embedding", href: "/platform-settings/embedding", icon: Database },
  { label: "LLM", href: "/platform-settings/llm", icon: Bot },
  { label: "Workers", href: "/platform-settings/workers", icon: Activity },
] as const;

export default function PlatformSettingsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const { data, isLoading } = api.auth.me.useQuery();

  const isPlatformAdmin = data?.user?.platform_roles?.includes("platform_admin");

  useEffect(() => {
    if (!isLoading && !isPlatformAdmin) {
      router.replace("/dashboard");
    }
  }, [isLoading, isPlatformAdmin, router]);

  if (isLoading) {
    return null;
  }

  if (!isPlatformAdmin) {
    return null;
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Platform Settings"
        description="Manage platform-wide users and provider configurations."
      />
      <div className="flex flex-col gap-6 md:flex-row">
        <nav
          aria-label="Platform Settings"
          className="flex gap-1 md:w-48 md:shrink-0 md:flex-col"
        >
          {platformSettingsNav.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              aria-current={pathname === item.href ? "page" : undefined}
              className={cn(
                "hover:bg-accent hover:text-accent-foreground flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                pathname === item.href
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground",
              )}
            >
              <item.icon className="size-4" />
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="flex-1">{children}</div>
      </div>
    </div>
  );
}
