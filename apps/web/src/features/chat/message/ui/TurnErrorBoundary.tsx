import * as stylex from "@stylexjs/stylex";
import { useRef, type ReactNode } from "react";
import { type FallbackProps } from "react-error-boundary";
import { LoggingErrorBoundary } from "@/ui/atoms/logging-error-boundary";
import { useT } from "@/lib/i18n/index";
import { Button } from "@flame/ui";
import { readingColumn as rc } from "@/features/chat/message/ui/readingColumn";
import { color, radius, space, surface, type as typeStep } from "@flame/ui/tokens.stylex";
const teb = stylex.create({
  frame: { paddingBlock: space.s2 },
  card: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    justifyContent: "space-between",
    gap: space.s3,
    borderRadius: radius.lg,
    backgroundColor: surface.negativeWash,
    paddingInline: space.s4,
    paddingBlock: space.s3,
  },
  label: { color: color.negative },
});

function TurnErrorFallback({ resetErrorBoundary }: FallbackProps) {
  const t = useT();
  const root = useRef<HTMLDivElement>(null);
  const retry = () => {
    // Retire owned focus before recovery can replace the fallback or claim focus itself.
    const retiring = root.current;
    if (retiring?.contains(retiring.ownerDocument.activeElement)) {
      retiring.parentElement?.closest<HTMLElement>("[tabindex]")?.focus({ preventScroll: true });
    }
    resetErrorBoundary();
  };
  return (
    <div ref={root} {...stylex.props(rc.gutter, teb.frame)}>
      <div role="alert" {...stylex.props(teb.card, typeStep.uiMd)}>
        <span {...stylex.props(teb.label)}>{t("chat.error.title")}</span>
        <Button type="button" variant="soft" size="sm" onClick={retry}>
          {t("chat.error.retry")}
        </Button>
      </div>
    </div>
  );
}

interface Props {
  resetKey: unknown;
  children: ReactNode;
}

export function TurnErrorBoundary({ resetKey, children }: Props) {
  return (
    <LoggingErrorBoundary
      tag="turn-error-boundary"
      fallback={TurnErrorFallback}
      resetKeys={[resetKey]}
    >
      {children}
    </LoggingErrorBoundary>
  );
}
