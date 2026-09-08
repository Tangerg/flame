import * as stylex from "@stylexjs/stylex";
import { gap, TextArea, vocab } from "@/ui";
import { type as typeStep } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

interface LinesFieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}

export function LinesField({ label, value, onChange, placeholder }: LinesFieldProps) {
  return (
    <label {...stylex.props(vocab.column, gap.s1_5)}>
      <span {...stylex.props(ss.label, typeStep.uiMd)}>{label}</span>
      <TextArea
        size="sm"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        rows={2}
        aria-label={label}
        placeholder={placeholder}
      />
    </label>
  );
}
