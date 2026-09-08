import type { ClassValue } from "clsx";
import { clsx } from "clsx";

/**
 * Join class values. That is all it does now.
 *
 * It used to wrap `clsx` in an extended Tailwind Merge, configured with three ladders and one
 * override, all of it about resolving conflicts BETWEEN UTILITY CLASSES: which of two `text-*`
 * wins, whether a font-size utility also claims the leading. There are no utility classes left,
 * so there is nothing to resolve — every string reaching here is either a StyleX class list,
 * which StyleX has already merged, or one of the mechanism keys `globals.css` owns.
 *
 * The configuration is worth remembering rather than just deleting, because each line was a
 * silent bug once: Tailwind Merge read `text-ui-md` as a COLOUR and dropped the ink beside it;
 * it assumed a font-size utility carried a line-height, which ours did not, and dropped
 * `leading-tight` from every button; and an unregistered step did not conflict with anything,
 * so `cn("leading-body", "leading-prose")` kept both and let stylesheet order decide.
 */
export function cn(...inputs: ClassValue[]) {
  return clsx(inputs);
}
