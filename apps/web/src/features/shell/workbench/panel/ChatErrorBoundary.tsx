import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { type FallbackProps } from "react-error-boundary";
import { LoggingErrorBoundary } from "@/features/shell/workbench/panel/LoggingErrorBoundary";
import { useT } from "@/lib/i18n/index";
import { Well } from "@/ui/index";
import { Button } from "@flame/ui";
import { color, radius, space, surface, type as typeStep, weight } from "@flame/ui/tokens.stylex";
const eb = stylex.create({
  card: {
    margin: space.s8,
    maxWidth: "720px",
    borderRadius: radius.lg,
    backgroundColor: surface.negativeWash,
    paddingInline: space.s5,
    paddingBlock: space.s4,
    color: color.fg,
  },
  title: {
    marginBottom: space.s2,
    fontWeight: weight.semibold,
    letterSpacing: "var(--tracking-display)",
    color: color.negative,
  },
  trace: { marginBottom: space.s3 },
  actions: { display: "flex", gap: space.s2 },
});

interface Props {
  resetKey?: unknown;
  label?: string;
  children: ReactNode;
}

function ChatErrorFallback({ error, resetErrorBoundary }: FallbackProps) {
  const t = useT();
  return (
    <div role="alert" {...stylex.props(eb.card)}>
      <div {...stylex.props(eb.title, typeStep.displaySm)}>{t("chat.error.title")}</div>
      <Well cap="md" className={stylex.props(eb.trace).className}>
        {error instanceof Error ? error.message : String(error)}
      </Well>
      <div {...stylex.props(eb.actions)}>
        <Button type="button" variant="soft" size="sm" onClick={resetErrorBoundary}>
          {t("chat.error.retry")}
        </Button>
      </div>
    </div>
  );
}

export function ChatErrorBoundary({ resetKey, label, children }: Props) {
  return (
    <LoggingErrorBoundary
      tag={`chat-error-boundary ${label ?? "chat"}`}
      fallback={ChatErrorFallback}
      resetKeys={resetKey === undefined ? [] : [resetKey]}
    >
      {children}
    </LoggingErrorBoundary>
  );
}
