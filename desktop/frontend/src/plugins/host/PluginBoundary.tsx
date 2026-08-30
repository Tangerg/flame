import { Trans } from "@/lib/i18n";
import type { ErrorInfo, ReactNode } from "react";
import { Component } from "react";
import { pickPluginErrorFallback, reportPluginError } from "../sdk";

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

    const fallback = pickPluginErrorFallback();
    if (fallback) {
      const Body = fallback.component;
      return <Body plugin={this.props.plugin} label={this.props.label} error={this.state.error} />;
    }

    return (
      <div className="plugin-boundary-error">
        <Trans
          i18nKey="plugins.renderFailed"
          values={{ plugin: this.props.label ?? this.props.plugin }}
          components={{ strong: <strong /> }}
        />
        <code>{this.state.error.message}</code>
      </div>
    );
  }
}
