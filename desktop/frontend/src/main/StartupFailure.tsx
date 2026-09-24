import * as stylex from "@stylexjs/stylex";
import { Component, type ErrorInfo, type ReactNode } from "react";
import { failureDiagnostics, failureMessage } from "@/lib/diagnostics";
import { t } from "@/lib/i18n";
import { Button, FailureDetails } from "@/ui";
import { color, leading, space, surface, type, weight } from "@/styles/tokens.stylex";

const styles = stylex.create({
  page: {
    display: "grid",
    minHeight: "100vh",
    placeItems: "center",
    padding: space.s8,
    backgroundColor: surface.canvas,
    color: color.fg,
  },
  card: { display: "flex", maxWidth: "520px", flexDirection: "column", gap: space.s3 },
  title: { margin: 0, fontWeight: weight.semibold },
  body: { margin: 0, lineHeight: leading.prose, color: color.fgSoft },
  actions: { display: "flex", gap: space.s2 },
  detail: { margin: 0, overflowWrap: "anywhere", color: color.fgMuted },
});

export const FAILURE_SCOPE = { startup: "startup", rootRender: "root-render" } as const;

type FailureScope = (typeof FAILURE_SCOPE)[keyof typeof FAILURE_SCOPE];

export function StartupFailure({ scope, error }: { scope: FailureScope; error: unknown }) {
  return (
    <main role="alert" {...stylex.props(styles.page)}>
      <div {...stylex.props(styles.card)}>
        <h1 {...stylex.props(styles.title, type.displaySm)}>{t("startup.failed.title")}</h1>
        <p {...stylex.props(styles.body, type.uiMd)}>{t("startup.failed.body")}</p>
        <div {...stylex.props(styles.actions)}>
          <Button variant="primary" onClick={() => window.location.reload()}>
            {t("common.retry")}
          </Button>
        </div>
        <p {...stylex.props(styles.detail, type.uiSm)}>{failureMessage(error)}</p>
        <FailureDetails diagnostics={failureDiagnostics(scope, error)} />
      </div>
    </main>
  );
}

export class RootBoundary extends Component<{ children: ReactNode }, { error: unknown }> {
  override state: { error: unknown } = { error: null };

  static getDerivedStateFromError(error: unknown) {
    return { error };
  }

  override componentDidCatch(error: unknown, info: ErrorInfo): void {
    console.error("[desktop] root render failed:", error, info.componentStack);
  }

  override render(): ReactNode {
    if (this.state.error === null) return this.props.children;
    return <StartupFailure scope={FAILURE_SCOPE.rootRender} error={this.state.error} />;
  }
}
