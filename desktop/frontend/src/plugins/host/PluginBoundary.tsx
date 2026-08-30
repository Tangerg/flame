import { Trans } from "@/lib/i18n";
import type { ErrorInfo, ReactNode } from "react";
import { Component } from "react";
import { pickPluginErrorFallback, reportPluginError } from "../sdk";

interface Props {
  /** Plugin name — used for the fallback label and the console log. */
  plugin: string;
  /** Optional label shown to the user. Defaults to the plugin name. */
  label?: string;
  children: ReactNode;
}

interface State {
  error: Error | null;
}

/**
 * Wraps EVERY plugin-contributed component: a misbehaving region renders a fallback while
 * the rest of the app keeps running, and the failure reaches both the console and the
 * error store.
 */
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
      // Rendered OUTSIDE another PluginBoundary: a fallback that throws is the host's bug,
      // and should surface rather than be swallowed.
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
