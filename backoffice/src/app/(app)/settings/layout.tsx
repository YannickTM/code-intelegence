"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "~/lib/utils";
import { User, KeyRound, Key, Settings } from "lucide-react";
import { PageHeader } from "~/components/page-header";

const settingsNav = [
  { label: "Profile", href: "/settings/profile", icon: User },
  { label: "SSH Keys", href: "/settings/ssh-keys", icon: KeyRound },
  { label: "API Keys", href: "/settings/api-keys", icon: Key },
  { label: "System", href: "/settings/system", icon: Settings },
] as const;

export default function SettingsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Settings"
        description="Manage your account and platform settings."
      />
      <div className="flex flex-col gap-6 md:flex-row">
        <nav
          aria-label="Settings"
          className="flex gap-1 md:w-48 md:shrink-0 md:flex-col"
        >
          {settingsNav.map((item) => (
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
