import { Trans, useT } from "@/lib/i18n";
import type { ErrorInfo, ReactNode } from "react";
import { Component } from "react";
import { FailureDetails, TextButton } from "@/ui";
import { failureDiagnostics } from "@/lib/diagnostics";
import { reportPluginError } from "../sdk";

interface Props {
  plugin: string;
  label?: string;
  children: ReactNode;
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
      <PluginFailure
        plugin={this.props.plugin}
        label={this.props.label ?? this.props.plugin}
        error={this.state.error}
        onRetry={() => this.setState({ error: null })}
      />
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
  return (
    <div role="alert" className="plugin-boundary-error">
      <Trans
        i18nKey="plugins.renderFailed"
        values={{ plugin: label }}
        components={{ strong: <strong /> }}
      />
      <code>{error.message}</code>
      <div className="plugin-boundary-actions">
        <TextButton tone="accent" onClick={onRetry}>
          {t("common.retry")}
        </TextButton>
      </div>
      <FailureDetails diagnostics={failureDiagnostics(`plugin ${plugin}`, error)} />
    </div>
  );
}
