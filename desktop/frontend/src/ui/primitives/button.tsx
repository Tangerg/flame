import type { ComponentPropsWithoutRef, ReactNode, Ref } from "react";
import { Button as BaseButton } from "@base-ui/react/button";

export type ButtonPrimitiveProps = ComponentPropsWithoutRef<typeof BaseButton> & {
  children?: ReactNode;
  ref?: Ref<HTMLButtonElement>;
  /**
   * This button's own action is in flight.
   *
   * NOT `disabled`, and the difference is the whole reason it exists. `disabled` says the action
   * is unavailable, and the platform enforces that by making the element unfocusable — so a
   * control that disables itself while its work runs blurs whoever was standing on it. Measured
   * on Settings → Connection before the fix: press Enter on Refresh and focus is on `<body>`
   * 120ms later, and still there a second after the work finished. Forty-four call sites spelled
   * in-flight as `disabled` under eight flag names, so every async action in the product dropped
   * the keyboard user's place in the document.
   *
   * `aria-disabled` says the same thing to a screen reader while leaving the element in the tab
   * order, and the click is refused here instead of by the platform. It looks identical, because
   * each atom's disabled styling reads both.
   *
   * It lives on the PRIMITIVE rather than on `Button`, because `Button`, `PillButton` and
   * `TextButton` all wrap this one and a fact spelled three times is the thing being fixed.
   *
   * A condition that is genuinely unavailable — an invalid form, a row with nothing selected —
   * stays `disabled`. Several call sites had both facts in one expression (`!valid || saving`);
   * those split.
   */
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
      aria-disabled={pending ? true : props["aria-disabled"]}
      // Nothing in the platform refuses a click on an `aria-disabled` element, so the refusal
      // lives here. Without it the prop would trade a lost focus for a double submit.
      onClick={pending ? undefined : onClick}
      // The reset itself lives in `globals.css` under `@layer base`, keyed on this attribute:
      // as utility classes it sat at the same weight as its own consumers and a ring above had
      // to out-specify it to state a border or a fill. A layer is what "underneath" means.
      data-control="button"
      className={className}
    >
      {children}
    </BaseButton>
  );
}
