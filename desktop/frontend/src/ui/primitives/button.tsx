import type { ComponentPropsWithoutRef, ReactNode, Ref } from "react";
import { Button as BaseButton } from "@base-ui/react/button";

export type ButtonPrimitiveProps = ComponentPropsWithoutRef<typeof BaseButton> & {
  children?: ReactNode;
  ref?: Ref<HTMLButtonElement>;
  pending?: boolean;
};

export function ButtonPrimitive({
  className,
  type = "button",
  children,
  pending,
  onClick,
  ref,
  ...props
}: ButtonPrimitiveProps) {
  return (
    <BaseButton
      {...props}
      ref={ref}
      type={type}
      {...(pending && { "aria-disabled": true })}
      onClick={pending ? undefined : onClick}
      data-control="button"
      className={className}
    >
      {children}
    </BaseButton>
  );
}
