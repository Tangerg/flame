import * as stylex from "@stylexjs/stylex";
import { memo } from "react";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { hasAnsi } from "@/lib/ansi";
import { AnsiText } from "@/ui";
import type { WorkspaceCommandActivity } from "@/plugins/builtin/workspace/application/toolActivity";
import { type as typeStep } from "@/styles/tokens.stylex";
import { codeStyles as cs, viewStyles as vs } from "./viewStyles";

export const CommandLog = memo(function CommandLog({
  commands,
  selectedCommandId,
}: {
  commands: readonly WorkspaceCommandActivity[];
  selectedCommandId: string;
}) {
  const t = useT();
  return (
    <div {...stylex.props(cs.sheet, cs.sheetInset, typeStep.code)}>
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
            <div {...stylex.props(vs.entryPlain)}>
              <span {...stylex.props(cs.prompt)}>$</span>
              <span {...stylex.props(vs.min, vs.truncate, vs.ink)} title={c.command}>
                {c.command}
              </span>
              {c.status === "running" && (
                <span {...stylex.props(cs.running)}>{t("commandLog.running")}</span>
              )}
              {c.status === "failed" && (
                <span {...stylex.props(cs.failed)}>{t("commandLog.failed")}</span>
              )}
              {c.exitCode !== undefined && c.exitCode !== 0 && (
                <span {...stylex.props(cs.failed)}>
                  {t("commandLog.exit", { code: c.exitCode })}
                </span>
              )}
            </div>
            {/* A command that thought it was on a TTY sends its colours as escape codes. Printed
                verbatim they are the loudest thing in the pane — `[32m` before every PASS —
                and the failure they were marking is the hardest to find. Read as tone, which
                the transcript's own output panel has always done. */}
            {c.output ? (
              <pre {...stylex.props(cs.output)}>
                {hasAnsi(c.output) ? <AnsiText text={c.output} /> : c.output}
              </pre>
            ) : null}
          </div>
        );
      })}
    </div>
  );
});
