import { Alert } from "@flame/icons/react";
import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { Icon } from "./font-scale";
import { RecordBody } from "./records";
import { layout } from "./tokens.stylex";

const styles = stylex.create({
  root: {
    display: "flex",
    alignItems: "flex-start",
    gap: layout.contentGap,
    overflowWrap: "anywhere",
  },
});

export function AlertMessage({ children }: { children: ReactNode }) {
  return (
    <div role="alert" {...stylex.props(styles.root)}>
      <Icon glyph={Alert} size="sm" />
      <RecordBody>{children}</RecordBody>
    </div>
  );
}
