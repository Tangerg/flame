import * as stylex from "@stylexjs/stylex";
import type { FileInputPrimitiveProps } from "@/ui/primitives";
import { FileInputPrimitive } from "@/ui/primitives";

// The control exists for its behaviour, never for its box: a button beside it opens the picker.
const styles = stylex.create({ away: { display: "none" } });

export type HiddenFileInputProps = Omit<FileInputPrimitiveProps, "className">;

export function HiddenFileInput(props: HiddenFileInputProps) {
  return <FileInputPrimitive {...props} {...stylex.props(styles.away)} />;
}
