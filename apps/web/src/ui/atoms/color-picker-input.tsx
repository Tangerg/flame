import { useId, useRef } from "react";
import * as stylex from "@stylexjs/stylex";
import { ButtonPrimitive } from "@flame/ui";

export interface ColorPickerInputProps {
  id?: string;
  "aria-label": string;
  "aria-describedby"?: string;
  value: string;
  disabled?: boolean;
  onValueChange: (value: string) => void;
}

const styles = stylex.create({
  overlay: {
    position: "absolute",
    inset: 0,
    height: "100%",
    width: "100%",
    cursor: "pointer",
    opacity: 0,
  },
  native: { pointerEvents: "none" },
});

export function ColorPickerInput({
  value,
  onValueChange,
  "aria-describedby": description,
  ...props
}: ColorPickerInputProps) {
  const native = useRef<HTMLInputElement>(null);
  const valueId = useId();
  return (
    <>
      <ButtonPrimitive
        {...props}
        aria-describedby={[valueId, description].filter(Boolean).join(" ")}
        onClick={() => native.current?.showPicker()}
        {...stylex.props(styles.overlay)}
      />
      <span id={valueId} className="sr-only">
        {value.toUpperCase()}
      </span>
      <input
        ref={native}
        type="color"
        value={value}
        disabled={props.disabled}
        tabIndex={-1}
        aria-hidden="true"
        onChange={(event) => onValueChange(event.currentTarget.value)}
        {...stylex.props(styles.overlay, styles.native)}
      />
    </>
  );
}
