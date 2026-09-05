// What counts as a control the user operates. Three audits ask this — a control cut in half, a
// control too small to hit, a control drawing its own focus ring — and each had written its own
// answer, so each audited a different tree. The clipping audit was the narrowest and never saw
// `[role="menuitem"]` or `[role="button"]`, which is most of what Base UI renders.
//
// The two lists are not the same question. A control is something with an operable affordance,
// so its size and its clipping matter. Focusable is wider: a scroll region carries `tabindex` to
// be reachable by keyboard without being a target anyone points at, and it still must not draw a
// second focus ring over the global one.

export const CONTROL = [
  "button",
  "a[href]",
  'input:not([type="hidden"])',
  "textarea",
  "select",
  '[role="button"]',
  '[role="tab"]',
  '[role="menuitem"]',
  '[role="option"]',
  '[role="switch"]',
  '[role="checkbox"]',
  '[role="radio"]',
].join(", ");

export const FOCUSABLE = `${CONTROL}, [tabindex]:not([tabindex="-1"])`;
