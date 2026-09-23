import * as stylex from "@stylexjs/stylex";
import type { FileInputPrimitiveProps } from "@/ui/primitives";
import { FileInputPrimitive } from "@/ui/primitives";

const styles = stylex.create({ away: { display: "none" } });

export type HiddenFileInputProps = Omit<FileInputPrimitiveProps, "className">;

export function HiddenFileInput(props: HiddenFileInputProps) {
  return <FileInputPrimitive {...props} {...stylex.props(styles.away)} />;
}
