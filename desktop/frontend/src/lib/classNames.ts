import type { ClassValue } from "clsx";
import { clsx } from "clsx";

/**
 * Join class values. Nothing to resolve: every string reaching here is either a StyleX class
 * list, which StyleX has already merged, or one of the mechanism keys `globals.css` owns.
 */
export function cn(...inputs: ClassValue[]) {
  return clsx(inputs);
}
