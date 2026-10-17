import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { vocab } from "@flame/ui";
import { face, type as typeStep } from "@flame/ui/tokens.stylex";
import { viewStyles as vs } from "./viewStyles";

export interface ViewHeaderProps {
  identity?: ReactNode;
  sub?: ReactNode;
  actions?: ReactNode;
}

export function ViewHeader({ identity, sub, actions }: ViewHeaderProps) {
  if (identity === undefined && sub === undefined && actions === undefined) return null;
  return (
    <header data-slot="workspace-view-header" {...stylex.props(vs.contentHeader)}>
      {(identity !== undefined || sub !== undefined) && (
        <div {...stylex.props(vs.headerIdentity, vocab.muted, typeStep.uiSm, face.mono)}>
          {identity !== undefined && <span {...stylex.props(vs.headerName)}>{identity}</span>}
          {sub !== undefined && <span {...stylex.props(vs.headerMetadata)}>{sub}</span>}
        </div>
      )}
      {actions !== undefined && <div {...stylex.props(vs.headerActions)}>{actions}</div>}
    </header>
  );
}
