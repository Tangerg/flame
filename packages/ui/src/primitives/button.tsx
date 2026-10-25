import type {
  ComponentPropsWithoutRef,
  KeyboardEvent,
  ReactNode,
  Ref,
} from "react";
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
  onClick,
  focusableWhenDisabled,
  render,
  ref,
  ...props
}: ButtonPrimitiveProps) {
  return (
    <BaseButton
      {...props}
      ref={ref}
      focusableWhenDisabled={focusableWhenDisabled}
      render={(elementProps, state) => (
        <ButtonElement
          {...elementProps}
          render={render}
          state={state}
          focusableWhenDisabled={focusableWhenDisabled}
        />
      )}
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

function ButtonElement({
  render,
  state,
  focusableWhenDisabled,
  onKeyDown,
  ...props
}: useRender.ElementProps<"button"> & {
  render?: ButtonPrimitiveProps["render"];
  state: BaseButton.State;
  focusableWhenDisabled?: boolean;
}) {
  return useRender({
    defaultTagName: "button",
    render,
    state: { disabled: state.disabled },
    props: {
      ...props,
      onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
        // Base UI also cancels unrelated native commands while focusably disabled.
        // Keep its activation guard, leaving other keys with their existing owner.
        if (
          state.disabled &&
          focusableWhenDisabled &&
          event.key !== "Enter" &&
          event.key !== " "
        ) {
          return;
        }
        onKeyDown?.(event);
      },
    },
  });
}
