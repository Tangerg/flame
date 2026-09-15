import * as stylex from "@stylexjs/stylex";
import type { StyleXArray, StyleXStyles } from "@stylexjs/stylex";
import { face } from "@/styles/tokens.stylex";
import { FilePath } from "@/ui/atoms/file-path";
import { vocab } from "@/ui/atoms/vocabulary";

interface ToolTextValue {
  kind: "path" | "machine" | "prose";
  value: string;
}

export function ToolText({
  value,
  styles,
}: {
  value: ToolTextValue;
  styles?: StyleXArray<StyleXStyles | null | false>;
}) {
  const machine = value.kind !== "prose";
  if (value.kind === "path") {
    return <FilePath path={value.value} className={stylex.props(face.mono, styles).className} />;
  }
  return (
    <span {...stylex.props(vocab.truncate, machine && face.mono, styles)} title={value.value}>
      {value.value}
    </span>
  );
}
