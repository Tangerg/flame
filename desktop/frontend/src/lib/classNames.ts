import type { ClassValue } from "clsx";
import { clsx } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";
import { UI_TYPE_STEPS } from "./typography";

// Tailwind Merge cannot infer custom `--text-*` theme variables from generated CSS, so
// without this list it reads `text-ui-md` as a colour and drops a preceding `text-fg-soft`
// as a conflict. A missing step fails SILENTLY in two directions — dropped when a colour
// utility follows, ignored when another size does — which is why the UI steps come from the
// ladder itself and `check-design-tokens` holds the editorial half.
const EDITORIAL_STEPS = ["display-sm", "display-md", "display-lg"];

// The other two ladders `@theme inline` publishes. Tailwind Merge only disambiguates values it
// knows, so an unknown step does not CONFLICT with anything: `cn("leading-body",
// "leading-prose")` kept both and left the winner to stylesheet order, and the same held for
// every corner this system named itself. `check-design-tokens` holds all three lists to the
// stylesheet, which is what keeps a newly added step from silently opting out.
const LEADING_STEPS = ["tight", "snug", "body", "relaxed", "prose"];
const RADIUS_STEPS = ["2xs", "xs", "sm", "md", "lg", "xl", "composer", "bubble", "pill"];

const mergeTailwindClasses = extendTailwindMerge({
  extend: {
    theme: {
      text: [...UI_TYPE_STEPS, ...EDITORIAL_STEPS],
      leading: LEADING_STEPS,
      radius: RADIUS_STEPS,
    },
  },
  // Tailwind Merge assumes a font-size utility also sets line height, because Tailwind's own
  // steps do. OURS DO NOT: `@theme inline` gives `--text-ui-*` a size and a tracking and no
  // leading, so the assumption made `cn("leading-tight", "text-ui-sm")` return `text-ui-sm`
  // alone — the leading was dropped and the element fell back to the body's PROSE rhythm.
  // Silent, and order-dependent: writing the same two classes the other way round worked.
  // It cost Button the `leading-tight` in its base (every button's box was then shorter than
  // its own line at the largest UI text) and the transcript the `leading-prose` it declares.
  // Display steps do carry a `--line-height`, and there an explicit `leading-*` still wins:
  // Tailwind emits leading utilities after font-size ones, so the call site's intent is last.
  override: { conflictingClassGroups: { "font-size": [] } },
});

export function cn(...inputs: ClassValue[]) {
  return mergeTailwindClasses(clsx(inputs));
}
