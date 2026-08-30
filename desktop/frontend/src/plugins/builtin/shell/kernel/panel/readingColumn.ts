// GUTTER is applied by whatever draws INSIDE the box — a message, the composer, a banner —
// never by the scroller's content wrapper. On the wrapper it is wrong twice: the composer
// wears the box without it, so read text comes out narrower than typed text, and each
// message's box then hugs its text, where paint containment slices the action bar's inset.
export const READING_COLUMN = "mx-auto w-full max-w-[var(--reading-column-max)]";

export const READING_GUTTER =
  "px-[var(--density-column-gutter)] sm:px-[var(--density-column-gutter-wide)]";

// Two halves of ONE contract kept in one file: Tailwind reads source text, so the class has
// to spell the property the constant names, and filing them apart drifts into a last
// message nobody can scroll out from under. The extra pixel absorbs integer scrollTop
// rounding when overlay and transcript paint on fractional CSS pixels.
export const COMPOSER_OVERLAY_PROPERTY = "--composer-overlay";

export const COMPOSER_CLEARANCE = "pb-[calc(var(--composer-overlay,0px)+1rem+1px)]";
