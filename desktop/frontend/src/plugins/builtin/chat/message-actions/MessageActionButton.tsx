import * as stylex from "@stylexjs/stylex";
import { IconButton, type IconName, type ButtonTone } from "@/ui";

// REPORTED, not decided here. `radius` names its steps by ROLE — card, field, row, button —
// and this one has no role to name: `--button-radius` is `--shape-sm`, so the class this
// replaces made the assistant's action the only control in the product at `--shape-md`. The
// value is preserved exactly rather than quietly normalised to the button's own corner, and
// left undressed rather than borrowed from `radius.card`, which would name it a card.
//
// It travels as `styles` and not as a class. Through `className` it was a SECOND
// `border-radius` on the same button — the button's own step and this one, at equal
// specificity, settled by sheet order. Composed into the button's own `stylex.props()` it
// replaces the step, which is what "this control has a different corner" has to mean.
const styles = stylex.create({
  assistantCorner: { borderRadius: "var(--shape-md)" },
});

interface MessageActionButtonProps {
  icon: IconName;
  role: string;
  title?: string;
  onClick?: () => void;
  tone?: ButtonTone;
  className?: string;
  "aria-label"?: string;
  "aria-pressed"?: boolean;
}

/** The reader's own message gets a pill; the agent's gets a corner. One decision, said once. */
export function MessageActionButton({ role, className, ...props }: MessageActionButtonProps) {
  const isUser = role === "user";
  return (
    <IconButton
      {...props}
      iconSize="sm"
      size="sm"
      quiet
      round={isUser}
      styles={[!isUser && styles.assistantCorner]}
      className={className}
    />
  );
}
