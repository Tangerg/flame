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
