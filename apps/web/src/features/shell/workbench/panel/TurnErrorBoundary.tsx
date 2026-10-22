import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { type FallbackProps } from "react-error-boundary";
import { ErrorBoundary } from "react-error-boundary";
import { useT } from "@/lib/i18n/index";
import { Button } from "@flame/ui";
import { readingColumn as rc } from "@/features/chat/public/message/rendering";
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
  return (
    <div {...stylex.props(rc.gutter, teb.frame)}>
      <div role="alert" {...stylex.props(teb.card, typeStep.uiMd)}>
        <span {...stylex.props(teb.label)}>{t("chat.error.title")}</span>
        <Button type="button" variant="soft" size="sm" onClick={resetErrorBoundary}>
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
    <ErrorBoundary
      FallbackComponent={TurnErrorFallback}
      resetKeys={[resetKey]}
      onError={(error, info) => {
        console.error("[turn-error-boundary]:", error, info.componentStack);
      }}
    >
      {children}
    </ErrorBoundary>
  );
}
