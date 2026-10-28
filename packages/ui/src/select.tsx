import * as stylex from "@stylexjs/stylex";
import {
  useLayoutEffect,
  useRef,
  type ComponentPropsWithRef,
  type RefObject,
} from "react";
import { ChevronDown, Check } from "@flame/icons/react";
import { Select as SelectPrimitive } from "@base-ui/react/select";
import { FIELD_SIZE, FIELD_SURFACE, MENU_SURFACE } from "./control-surface";
import { Button } from "./button";
import { Icon } from "./font-scale";
import { FLOATING_LAYER, FLOATING_PANEL } from "./floating-surface";
import {
  useHiddenPopupRelease,
  useHiddenRootRetirement,
} from "./popup-visibility";
import { space, type } from "./tokens.stylex";

const styles = stylex.create({
  popup: {
    overflow: "auto",
    maxHeight: "var(--available-height)",
    minWidth: "var(--anchor-width)",
    maxWidth: "min(var(--available-width), calc(100dvw - var(--spacing) * 8))",
    overscrollBehavior: "contain",
  },
  // An exit transition delays Base UI's final focus until unmount.
  closed: { transitionProperty: "none" },
  item: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
  },
  trigger: {
    justifySelf: "stretch",
    width: "auto",
    maxWidth: "100%",
    height: "auto",
    whiteSpace: "normal",
    textAlign: "start",
    overflowWrap: "anywhere",
    justifyContent: "space-between",
    gap: space.s3,
  },
});

export function Select({
  value,
  onValueChange,
  options,
  label,
  id,
  disabled,
  invalid,
  "aria-describedby": describedBy,
  size = "md",
}: {
  value: string | null;
  onValueChange: (value: string) => void;
  options: readonly { value: string; label: string }[];
  label: string;
  id?: string;
  disabled?: boolean;
  invalid?: boolean;
  "aria-describedby"?: string;
  size?: "sm" | "md" | "lg";
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const detached = useRef(false);
  const retirement = useHiddenRootRetirement<
    SelectPrimitive.Root.Actions,
    SelectPrimitive.Root.OpenChangeEventDetails
  >({});
  const { visible, portalRef } = useHiddenPopupRelease();
  return (
    <SelectPrimitive.Root
      items={options}
      value={value}
      disabled={disabled}
      onValueChange={(next) => {
        if (next !== null) onValueChange(next);
      }}
      {...retirement}
    >
      <SelectPrimitive.Trigger
        ref={trigger}
        render={
          <Button
            variant="bare"
            size={size}
            styles={[
              FIELD_SURFACE.face,
              FIELD_SURFACE.trigger,
              FIELD_SIZE[size],
              styles.trigger,
            ]}
          />
        }
        id={id}
        aria-label={label}
        aria-invalid={invalid || undefined}
        aria-describedby={describedBy}
      >
        <SelectPrimitive.Value
          placeholder={value === null ? label : undefined}
        />
        <SelectPrimitive.Icon>
          <Icon glyph={ChevronDown} size="xs" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal ref={portalRef}>
        <SelectPrimitive.Positioner
          {...stylex.props(FLOATING_LAYER)}
          sideOffset={4}
          alignItemWithTrigger={false}
          render={(props, state) => (
            <SelectAnchor
              {...props}
              open={state.open}
              anchorHidden={state.anchorHidden}
              detached={detached}
              actionsRef={retirement.actionsRef}
            />
          )}
        >
          <SelectPrimitive.Popup
            finalFocus={() => {
              if (!visible) return false;
              const doc = trigger.current?.ownerDocument;
              const active = doc?.activeElement;
              if (
                active &&
                active !== doc?.body &&
                active !== trigger.current &&
                !portalRef.current?.contains(active)
              )
                return false;
              if (!detached.current) return true;
              for (
                let owner = trigger.current?.parentElement;
                owner;
                owner = owner.parentElement
              ) {
                const overflow = getComputedStyle(owner).overflowY;
                if (
                  owner.tabIndex >= 0 &&
                  (overflow === "auto" || overflow === "scroll") &&
                  owner.scrollHeight > owner.clientHeight
                )
                  return owner;
              }
              return false;
            }}
            className={(state) =>
              stylex.props(
                FLOATING_PANEL,
                MENU_SURFACE.content,
                styles.popup,
                type.uiMd,
                !state.open && styles.closed,
              ).className
            }
          >
            {options.map((option) => (
              <SelectPrimitive.Item
                key={option.value}
                value={option.value}
                {...stylex.props(MENU_SURFACE.item, styles.item)}
              >
                <SelectPrimitive.ItemText>
                  {option.label}
                </SelectPrimitive.ItemText>
                <SelectPrimitive.ItemIndicator>
                  <Icon glyph={Check} size="xs" />
                </SelectPrimitive.ItemIndicator>
              </SelectPrimitive.Item>
            ))}
          </SelectPrimitive.Popup>
        </SelectPrimitive.Positioner>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
}

function SelectAnchor({
  open,
  anchorHidden,
  detached,
  actionsRef,
  ...props
}: ComponentPropsWithRef<"div"> & {
  open: boolean;
  anchorHidden: boolean;
  detached: RefObject<boolean>;
  actionsRef: RefObject<SelectPrimitive.Root.Actions | null>;
}) {
  useLayoutEffect(() => {
    // Base UI retains closed items and their last geometry. Consume a hidden
    // transition once so reopening can receive the next anchor measurement.
    if (!open) return;
    const wasDetached = detached.current;
    detached.current = anchorHidden;
    if (anchorHidden && !wasDetached) actionsRef.current?.close();
  }, [open, anchorHidden, detached, actionsRef]);
  return <div {...props} hidden={props.hidden || anchorHidden} />;
}
