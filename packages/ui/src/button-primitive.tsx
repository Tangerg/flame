import type { ComponentPropsWithoutRef, ReactNode, Ref } from "react";
import { Button as BaseButton } from "@base-ui/react/button";
import { useRender } from "@base-ui/react/use-render";

export type ButtonPrimitiveProps = ComponentPropsWithoutRef<
  typeof BaseButton
> & {
  children?: ReactNode;
  ref?: Ref<HTMLButtonElement>;
  pending?: boolean;
};

export function ButtonPrimitive({
  className,
  type = "button",
  children,
  pending,
  disabled,
  onClick,
  onKeyDown,
  focusableWhenDisabled,
  render,
  ref,
  ...props
}: ButtonPrimitiveProps) {
  const focusable = pending || focusableWhenDisabled;
  return (
    <BaseButton
      {...props}
      ref={ref}
      disabled={disabled || pending}
      focusableWhenDisabled={focusable}
      render={(elementProps, state) => (
        <ButtonElement
          {...elementProps}
          render={render}
          state={state}
          focusableWhenDisabled={focusable}
          nonActivationKeyDown={onKeyDown}
        />
      )}
      type={type}
      onClick={onClick}
      onKeyDown={onKeyDown}
      data-control="button"
      className={className}
    >
      {children}
    </BaseButton>
  );
}

function ButtonElement({
  render,
  state,
  focusableWhenDisabled,
  nonActivationKeyDown,
  onKeyDown,
  ...props
}: useRender.ElementProps<"button"> & {
  render?: ButtonPrimitiveProps["render"];
  state: BaseButton.State;
  focusableWhenDisabled?: boolean;
  nonActivationKeyDown?: ButtonPrimitiveProps["onKeyDown"];
}) {
  return useRender({
    defaultTagName: "button",
    render,
    state: { disabled: state.disabled },
    props: {
      ...props,
      onKeyDown(
        event: Parameters<NonNullable<ButtonPrimitiveProps["onKeyDown"]>>[0],
      ) {
        // Base UI also cancels unrelated native commands while focusably disabled.
        // Keep its activation guard, leaving other keys with their existing owner.
        if (
          state.disabled &&
          focusableWhenDisabled &&
          event.key !== "Enter" &&
          event.key !== " "
        ) {
          nonActivationKeyDown?.(event);
          return;
        }
        onKeyDown?.(event);
      },
    },
  });
}
