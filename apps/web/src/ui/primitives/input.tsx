import { type ComponentPropsWithRef } from "react";
export type FileInputPrimitiveProps = Omit<ComponentPropsWithRef<"input">, "type">;

export function FileInputPrimitive(props: FileInputPrimitiveProps) {
  return <input {...props} type="file" />;
}

export type ColorInputPrimitiveProps = Omit<ComponentPropsWithRef<"input">, "type">;

export function ColorInputPrimitive(props: ColorInputPrimitiveProps) {
  return <input {...props} type="color" />;
}
