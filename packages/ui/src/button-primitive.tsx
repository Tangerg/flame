import {
  useState,
  type ComponentPropsWithoutRef,
  type ReactNode,
  type Ref,
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
  disabled,
  onClick,
  onKeyDown,
  focusableWhenDisabled,
  onFocus,
  onBlur,
  render,
  ref,
  ...props
}: ButtonPrimitiveProps) {
  const [focused, setFocused] = useState(false);
  // A control disabled under the user's focus (its own command running, its
  // work done) keeps that focus until the user moves on, instead of dropping it
  // to the document. `focusableWhenDisabled` keeps a control tabbable as well.
  const focusable = pending || focusableWhenDisabled || (disabled && focused);
  return (
    <BaseButton
      {...props}
      onFocus={(event) => {
        setFocused(true);
        onFocus?.(event);
      }}
      onBlur={(event) => {
        setFocused(false);
        onBlur?.(event);
      }}
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
