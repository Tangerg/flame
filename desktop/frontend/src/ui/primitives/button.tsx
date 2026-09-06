import type { ComponentPropsWithoutRef, ReactNode, Ref } from "react";
import { Button as BaseButton } from "@base-ui/react/button";
import { cn } from "@/lib/classNames";

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
      className={cn(
        // The browser's own button chrome, off in one place. `p-0` belongs with the rest of it:
        // without it every atom resets the padding again, and a StyleX atom that does cannot be
        // re-padded by its caller — the generated selector out-specifies any utility.
        "border-0 bg-transparent p-0 font-sans text-left focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-45",
        className,
      )}
    >
      {children}
    </BaseButton>
  );
}
