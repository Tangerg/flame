import * as stylex from "@stylexjs/stylex";
import { Component, createRef, type ReactNode } from "react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { ShikiCodeBlock, useContentVisible } from "@/ui/index";
import { Disclosure, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import {
  toolInspectorModel,
  type ToolInspectorBody,
} from "@/features/chat/tools/application/toolInspectorModel";
import { radius, space, type as typeStep } from "@flame/ui/tokens.stylex";
const ti = stylex.create({
  body: {
    display: "flex",
    flexDirection: "column",
    gap: space.s2,
    minWidth: 0,
    paddingTop: space.s0_5,
  },
  result: { minWidth: 0, borderRadius: radius.lg },
  empty: { margin: 0 },
});

const sourceLang = (body: ToolInspectorBody) => (body.isJson ? "json" : "text");

interface ToolResultLayoutProps {
  tool: ToolCall;
  label: string;
  visible: boolean;
  children: ReactNode;
}

class ToolResultLayout extends Component<ToolResultLayoutProps, unknown, HTMLElement | null> {
  private readonly root = createRef<HTMLDivElement>();

  // Capture before a result renderer can remove its focused reader or control.
  override getSnapshotBeforeUpdate(previous: ToolResultLayoutProps): HTMLElement | null {
    const { tool, visible } = this.props;
    const root = this.root.current;
    const active = root?.ownerDocument.activeElement;
    return visible &&
      previous.visible &&
      tool.runId === previous.tool.runId &&
      tool.id === previous.tool.id &&
      active instanceof HTMLElement &&
      root?.contains(active)
      ? active
      : null;
  }

  override componentDidUpdate(
    _previous: ToolResultLayoutProps,
    _state: unknown,
    source: HTMLElement | null,
  ) {
    const root = this.root.current;
    if (
      source &&
      !source.isConnected &&
      root?.isConnected &&
      this.props.visible &&
      !root.closest('[hidden], [inert], [aria-hidden="true"]') &&
      root.ownerDocument.activeElement === root.ownerDocument.body
    ) {
      root.focus({ preventScroll: true });
    }
  }

  override render() {
    return (
      <div
        ref={this.root}
        role="group"
        aria-label={this.props.label}
        tabIndex={-1}
        data-focus-inset
        {...stylex.props(ti.result)}
      >
        {this.props.children}
      </div>
    );
  }
}

export function ToolInspector({
  tool,
  children,
}: {
  tool: ToolCall;
  children?: (fallback: ReactNode) => ReactNode;
}) {
  const t = useT();
  const visible = useContentVisible();
  const model = toolInspectorModel(tool);
  const hasResult = model.result.text !== "";

  const result = (
    <>
      {hasResult && (
        <ShikiCodeBlock
          lang={sourceLang(model.result)}
          code={model.result.text}
          scrollLabel={t("toolInspector.result")}
        />
      )}
      {model.showNoResult && (
        <p {...stylex.props(ti.empty, vocab.faint, typeStep.uiSm)}>{t("toolInspector.noResult")}</p>
      )}
    </>
  );

  return (
    <div {...stylex.props(ti.body)}>
      {(hasResult || model.showNoResult || children !== undefined) && (
        <ToolResultLayout tool={tool} label={t("toolInspector.result")} visible={visible}>
          {children ? children(result) : result}
        </ToolResultLayout>
      )}
      {model.args.text && (
        <Disclosure title={t("toolInspector.arguments")} defaultOpen={!hasResult}>
          <ShikiCodeBlock
            lang={sourceLang(model.args)}
            code={model.args.text}
            scrollLabel={t("toolInspector.arguments")}
            copyable={false}
          />
        </Disclosure>
      )}
    </div>
  );
}
