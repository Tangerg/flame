import { useMemo, useState } from "react";
import {
  type AnsiSpan,
  type AnsiTone,
  hasAnsi,
  parseAnsi,
} from "@/plugins/builtin/chat/tools/domain/ansi";
import { cn } from "@/lib/classNames";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { useT } from "@/lib/i18n";
import { Badge, Icon, IconButton, TextButton, Well } from "@/ui";
import { LinkedText } from "@/plugins/builtin/chat/file-references/public/LinkedText";
import { PreviewPlaceholder } from "./PreviewPlaceholder";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";

const COLLAPSED_LINES = 9;

const TONE_CLASS: Record<AnsiTone, string> = {
  negative: "text-negative",
  success: "text-success",
  warning: "text-warning",
  info: "text-info",
  accent: "text-accent",
  muted: "text-fg-faint",
};

function spanClass(span: AnsiSpan): string | undefined {
  const parts = [
    span.tone ? TONE_CLASS[span.tone] : undefined,
    span.bold ? "font-semibold" : undefined,
    span.dim ? "opacity-70" : undefined,
    span.underline ? "underline" : undefined,
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" ") : undefined;
}

function OutputLine({ text }: { text: string }) {
  if (!hasAnsi(text)) return <LinkedText text={text || " "} />;
  return (
    <>
      {parseAnsi(text).map((span, index) => (
        <span key={index} className={spanClass(span)}>
          {span.text}
        </span>
      ))}
    </>
  );
}

interface ToolOutputPanelProps {
  output: string | undefined;
  status: ToolCall["status"];
  truncated?: boolean;
  idleLabel?: string;
}

export function ToolOutputPanel({
  output,
  status,
  truncated,
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
  const shown = expanded ? lines : lines.slice(0, COLLAPSED_LINES);

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
      {truncated && (
        <div className="px-3 pt-2.5">
          <Badge>{t("tools.overflow.truncated")}</Badge>
        </div>
      )}
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
            "absolute right-1 top-1 opacity-0 transition-opacity",
            "group-hover/output:opacity-100 focus-visible:opacity-100",
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
              : t("tools.output.showAll", { count: lines.length })}
          </TextButton>
        </div>
      )}
    </div>
  );
}
