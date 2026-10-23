import { Trans, useT } from "@/lib/i18n/index";
import { type ErrorInfo, type ReactNode } from "react";
import { Component, useRef } from "react";
import { FailureDetails, SystemMessage } from "@/ui/index";
import { failureDiagnostics } from "@/lib/diagnostics";
import { reportPluginError } from "@/extensions/errors";
interface Props {
  plugin: string;
  label?: string;
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  error: Error | null;
}

export class PluginBoundary extends Component<Props, State> {
  override state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error(`[plugin] ${this.props.plugin} render failed:`, error, info.componentStack);
    reportPluginError(this.props.plugin, "render", error, info.componentStack ?? undefined);
  }

  override render(): ReactNode {
    if (!this.state.error) return this.props.children;
    return (
      <>
        <PluginFailure
          plugin={this.props.plugin}
          label={this.props.label ?? this.props.plugin}
          error={this.state.error}
          onRetry={() => this.setState({ error: null })}
        />
        {this.props.fallback}
      </>
    );
  }
}

function PluginFailure({
  plugin,
  label,
  error,
  onRetry,
}: {
  plugin: string;
  label: string;
  error: Error;
  onRetry: () => void;
}) {
  const t = useT();
  const root = useRef<HTMLDivElement>(null);
  const retry = () => {
    // The error tree unmounts on retry. Hand off to its surviving host before
    // recovered children can claim their own focus.
    const retiring = root.current;
    if (retiring?.contains(retiring.ownerDocument.activeElement)) {
      retiring.parentElement?.closest<HTMLElement>("[tabindex]")?.focus({ preventScroll: true });
    }
    onRetry();
  };
  return (
    <SystemMessage
      ref={root}
      variant="error"
      shape="form"
      icon="alert"
      action={{ label: t("common.retry"), onClick: retry }}
    >
      <div className="plugin-boundary-error">
        <Trans
          i18nKey="plugins.renderFailed"
          values={{ plugin: label }}
          components={{ strong: <strong /> }}
        />
        <code>{error.message}</code>
        <FailureDetails diagnostics={failureDiagnostics(`plugin ${plugin}`, error)} />
      </div>
    </SystemMessage>
  );
}
