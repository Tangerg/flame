import * as stylex from "@stylexjs/stylex";
import { type StyleXArray, type StyleXStyles } from "@stylexjs/stylex";
import { face, type as typeStep } from "@flame/ui/tokens.stylex";
import { FilePath } from "@/ui/atoms/file-path";
import { vocab } from "@flame/ui/vocabulary";
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
    return (
      <FilePath
        path={value.value}
        className={stylex.props(face.mono, typeStep.code, styles).className}
      />
    );
  }
  return (
    <span
      {...stylex.props(vocab.truncate, machine && [face.mono, typeStep.code], styles)}
      title={value.value}
    >
      {value.value}
    </span>
  );
}
