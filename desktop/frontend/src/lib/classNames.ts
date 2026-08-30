import type { ClassValue } from "clsx";
import { clsx } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";
import { UI_TYPE_STEPS } from "./typography";

// Tailwind Merge cannot infer custom `--text-*` theme variables from generated CSS, so
// without this list it reads `text-ui-md` as a colour and drops a preceding `text-fg-soft`
// as a conflict. A missing step fails SILENTLY in two directions — dropped when a colour
// utility follows, ignored when another size does — which is why the UI steps come from the
// ladder itself and `check-design-tokens` holds the editorial half.
const EDITORIAL_STEPS = ["display-sm", "display-md", "display-lg", "display-xl"];

const mergeTailwindClasses = extendTailwindMerge({
  extend: { theme: { text: [...UI_TYPE_STEPS, ...EDITORIAL_STEPS] } },
});

export function cn(...inputs: ClassValue[]) {
  return mergeTailwindClasses(clsx(inputs));
}
