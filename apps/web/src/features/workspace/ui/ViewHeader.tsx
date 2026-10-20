import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { vocab } from "@flame/ui";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { viewStyles as vs } from "@/features/workspace/ui/viewStyles";
export interface ViewHeaderProps {
  navigation?: ReactNode;
  identity?: ReactNode;
  sub?: ReactNode;
  controls?: ReactNode;
  actions?: ReactNode;
}

export function ViewHeader({ navigation, identity, sub, controls, actions }: ViewHeaderProps) {
  if (
    navigation === undefined &&
    identity === undefined &&
    sub === undefined &&
    controls === undefined &&
    actions === undefined
  )
    return null;
  return (
    <header data-slot="workspace-view-header" {...stylex.props(vs.contentHeader)}>
      <div {...stylex.props(vs.headerHeading)}>
        {navigation}
        {(identity !== undefined || sub !== undefined) && (
          <div {...stylex.props(vs.headerIdentity, typeStep.uiSm)}>
            {identity !== undefined && <span {...stylex.props(vs.headerName)}>{identity}</span>}
            {sub !== undefined && (
              <span {...stylex.props(vs.headerMetadata, vocab.muted, typeStep.uiXs)}>{sub}</span>
            )}
          </div>
        )}
      </div>
      {(controls !== undefined || actions !== undefined) && (
        <div data-slot="workspace-view-controls" {...stylex.props(vs.headerControls)}>
          {controls}
          {actions !== undefined && <div {...stylex.props(vs.headerActions)}>{actions}</div>}
        </div>
      )}
    </header>
  );
}
