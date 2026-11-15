export const CONTROL = [
  "button",
  "a[href]",
  // The visible primitive owns interaction; its hidden form mirror is not another target.
  'input:not([type="hidden"]):not([aria-hidden="true"])',
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

// Base UI's clipped sentinels route focus; they are not visible controls.
export const FOCUSABLE = `:is(${CONTROL}, [tabindex]:not([tabindex="-1"])):not([data-base-ui-focus-guard])`;
