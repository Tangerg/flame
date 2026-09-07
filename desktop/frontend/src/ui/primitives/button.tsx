import type { ComponentPropsWithoutRef, ReactNode, Ref } from "react";
import { Button as BaseButton } from "@base-ui/react/button";

export type ButtonPrimitiveProps = ComponentPropsWithoutRef<typeof BaseButton> & {
  children?: ReactNode;
  ref?: Ref<HTMLButtonElement>;
};

export function ButtonPrimitive({
  className,
  type = "button",
  children,
  ref,
  ...props
}: ButtonPrimitiveProps) {
  return (
    <BaseButton
      {...props}
      ref={ref}
      type={type}
      // The reset itself lives in `globals.css` under `@layer base`, keyed on this attribute:
      // as utility classes it sat at the same weight as its own consumers and a ring above had
      // to out-specify it to state a border or a fill. A layer is what "underneath" means.
      data-control="button"
      className={className}
    >
      {children}
    </BaseButton>
  );
}
