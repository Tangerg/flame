import { useMemo, useState } from "react";
import { hasAnsi } from "@/lib/ansi";
import { cn } from "@/lib/classNames";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { useT } from "@/lib/i18n";
import { AnsiText, Icon, IconButton, TextButton, Well } from "@/ui";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewPlaceholder } from "./PreviewPlaceholder";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";

const COLLAPSED_LINES = 9;
// Expanding used to render every line there was, and the cost is superlinear: measured at
// 120ms for a thousand lines, 700ms for ten thousand and over nine seconds for fifty
// thousand, which a `shell` running a build reaches without trying. The whole output is a
// click away in the terminal view either way, so inline expansion stops where it is still
// a frame rather than a freeze.
const EXPANDED_LINES = 1_000;

// Plain lines go through `LinkedText`, which turns a path into somewhere to click. A line
// carrying escape codes does not: the link scanner would have to be taught the codes, and a
// coloured `go test` line is the one shape where the path is already the least of what is
// there.
function OutputLine({ text }: { text: string }) {
  if (!hasAnsi(text)) return <LinkedText text={text || " "} />;
  return <AnsiText text={text} />;
}

interface ToolOutputPanelProps {
  output: string | undefined;
  status: ToolCall["status"];
  idleLabel?: string;
}

export function ToolOutputPanel({
  output,
  status,
  idleLabel = "tools.preview.idle.noOutput",
}: ToolOutputPanelProps) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);

  const lines = useMemo(() => {
    const trimmed = output?.replace(/\n+$/, "") ?? "";
    return trimmed === "" ? [] : trimmed.split("\n");
  }, [output]);
  const copyMaterial = lines.join("\n");
  const { copied, copy } = useCopyFeedback(copyMaterial);

  const hidden = lines.length - COLLAPSED_LINES;
  const shown = lines.slice(0, expanded ? EXPANDED_LINES : COLLAPSED_LINES);
  const beyond = lines.length - shown.length;

  if (lines.length === 0) {
    return (
      <Well as="div">
        <PreviewPlaceholder
          status={status}
          pending="tools.preview.pending.running"
          idle={idleLabel}
        />
      </Well>
    );
  }

  return (
    <div className="overflow-hidden rounded-sm bg-sunken">
      <div className="group/output relative">
        <div className="overflow-x-auto px-3 py-2.5 font-mono text-code leading-relaxed text-fg-soft [font-variant-ligatures:none]">
          {shown.map((line, index) => (
            <div key={index} className="whitespace-pre-wrap wrap-anywhere">
              <OutputLine text={line} />
            </div>
          ))}
        </div>
        <IconButton
          data-reveal="hover"
          icon={copied ? "check" : "copy"}
          size="xs"
          title={t(copied ? "tools.output.copied" : "tools.output.copy")}
          onClick={() => void copy()}
          className={cn(
            "pointer-events-none absolute right-1 top-1 opacity-0 transition-opacity",
            "group-hover/output:pointer-events-auto group-hover/output:opacity-100 focus-visible:pointer-events-auto focus-visible:opacity-100",
          )}
        />
      </div>
      {hidden > 0 && (
        <div className="relative">
          {!expanded && (
            <div className="pointer-events-none absolute -top-6 inset-x-0 h-6 bg-[linear-gradient(to_top,var(--color-sunken),transparent)]" />
          )}
          <TextButton
            onClick={() => setExpanded((value) => !value)}
            className="w-full justify-center py-1.5 text-ui-sm hover:bg-hover"
          >
            <Icon name={expanded ? "chevron-up" : "chevron-down"} size="xs" />
            {expanded
              ? t("tools.output.collapse")
              : lines.length > EXPANDED_LINES
                ? t("tools.output.showSome", { count: EXPANDED_LINES, total: lines.length })
                : t("tools.output.showAll", { count: lines.length })}
          </TextButton>
          {expanded && beyond > 0 && (
            <div className="px-3 pb-2 text-center text-ui-sm text-fg-faint">
              {t("tools.output.beyond", { count: beyond })}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
