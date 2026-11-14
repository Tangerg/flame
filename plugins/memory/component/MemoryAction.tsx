import { type RefObject, useRef } from "react";
import { Button, useRemovalFocus, type ButtonProps } from "@flame/ui";

export function MemoryAction({
  result,
  initiator,
  canRestoreFocus,
  ...props
}: Omit<ButtonProps, "ref"> & {
  result: RefObject<HTMLElement | null>;
  canRestoreFocus(): boolean;
  initiator?: RefObject<HTMLElement | undefined>;
}) {
  const action = useRef<HTMLButtonElement>(null);
  useRemovalFocus(
    action,
    () => result.current,
    (node) => canRestoreFocus() && (!initiator || initiator.current === node),
  );
  return <Button ref={action} {...props} />;
}
