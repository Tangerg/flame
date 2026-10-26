import { type ReactNode } from "react";
import { ButtonPrimitive } from "@flame/ui";
import { type ButtonPrimitiveProps } from "@flame/ui";
export type PressableProps = Omit<ButtonPrimitiveProps, "children"> & {
  children?: ReactNode;
};

export function Pressable({ children, ...props }: PressableProps) {
  return <ButtonPrimitive {...props}>{children}</ButtonPrimitive>;
}
