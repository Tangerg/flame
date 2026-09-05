import { memo } from "react";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { hasAnsi } from "@/lib/ansi";
import { AnsiText } from "@/ui";
import type { WorkspaceCommandActivity } from "@/plugins/builtin/workspace/application/toolActivity";

export const CommandLog = memo(function CommandLog({
  commands,
  selectedCommandId,
}: {
  commands: readonly WorkspaceCommandActivity[];
  selectedCommandId: string;
}) {
  const t = useT();
  return (
    <div className="flex flex-col gap-2.5 px-3 py-3 font-mono text-code leading-relaxed">
      {commands.map((c) => {
        const selected = c.id === selectedCommandId;
        return (
          <div
            key={c.id}
            data-command-id={c.id}
            data-command-selected={selected ? "" : undefined}
            className={cn(
              "rounded-md px-3 py-2.5 transition-colors duration-[var(--dur-color)]",
              selected ? "bg-selected" : "bg-sunken",
            )}
          >
            <div className="flex items-baseline gap-2">
              <span className="shrink-0 text-fg-faint">$</span>
              <span className="min-w-0 truncate text-fg" title={c.command}>
                {c.command}
              </span>
              {c.status === "running" && (
                <span className="shrink-0 text-accent">{t("commandLog.running")}</span>
              )}
              {c.status === "failed" && (
                <span className="shrink-0 text-negative">{t("commandLog.failed")}</span>
              )}
              {c.exitCode !== undefined && c.exitCode !== 0 && (
                <span className="shrink-0 text-negative">
                  {t("commandLog.exit", { code: c.exitCode })}
                </span>
              )}
            </div>
            {/* A command that thought it was on a TTY sends its colours as escape codes. Printed
                verbatim they are the loudest thing in the pane — `[32m` before every PASS —
                and the failure they were marking is the hardest to find. Read as tone, which
                the transcript's own output panel has always done. */}
            {c.output ? (
              <pre className="mt-1.5 whitespace-pre-wrap break-words text-fg-muted">
                {hasAnsi(c.output) ? <AnsiText text={c.output} /> : c.output}
              </pre>
            ) : null}
          </div>
        );
      })}
    </div>
  );
});
