"use client";

import { useTheme } from "next-themes";
import { cn } from "~/lib/utils";

export function HtmlPreview({
  code,
  className,
}: {
  code: string;
  className?: string;
}) {
  const { resolvedTheme } = useTheme();
  const isDark = resolvedTheme === "dark";

  const baseStyles = `<style>
  body {
    margin: 1rem;
    font-family: ui-sans-serif, system-ui, sans-serif;
    color: ${isDark ? "#fafafa" : "#171717"};
    background: ${isDark ? "#262626" : "#ffffff"};
  }
</style>`;

  const srcdoc = baseStyles + code;

  return (
    <iframe
      sandbox=""
      srcDoc={srcdoc}
      title="HTML Preview"
      className={cn(
        "min-h-0 flex-1 overflow-y-auto rounded-md border",
        className,
      )}
      style={{ width: "100%", minHeight: "400px" }}
    />
  );
}
