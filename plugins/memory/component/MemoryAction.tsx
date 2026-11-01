import { type RefObject, useEffect, useLayoutEffect, useRef } from "react";
import { Button, type ButtonProps } from "@flame/ui";

export function MemoryAction({
  result,
  initiator,
  ...props
}: Omit<ButtonProps, "ref"> & {
  result: RefObject<HTMLElement | null>;
  initiator?: RefObject<HTMLElement | undefined>;
}) {
  const action = useRef<HTMLButtonElement>(null);
  const ownedFocus = useRef(false);
  useLayoutEffect(() => {
    const button = action.current;
    if (!button) return;
    ownedFocus.current = false;
    // An idle review cannot hand focus to an unrelated result. Recovery owns its command.
    // Capture before removal; passive cleanup sees the committed result.
    return () => {
      ownedFocus.current =
        (!initiator || initiator.current === button) &&
        button === button.ownerDocument.activeElement;
    };
  }, [initiator]);
  useEffect(() => {
    const button = action.current;
    if (!button) return;
    return () => {
      // A row action can mount before its result block appears; resolve the block at removal.
      const block = result.current;
      if (
        block &&
        !button.isConnected &&
        ownedFocus.current &&
        block.isConnected &&
        block.ownerDocument.activeElement === block.ownerDocument.body
      )
        block.focus();
    };
  }, [result]);
  return <Button ref={action} {...props} />;
}
