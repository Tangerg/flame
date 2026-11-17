import { afterEach, expect, it } from "vitest";
import { handoffFocus } from "../dist/index.js";
afterEach(() => document.body.replaceChildren());
function controls() {
  const source = document.createElement("section");
  const button = document.createElement("button");
  const target = document.createElement("button");
  source.append(button);
  document.body.append(source, target);
  return { source, button, target };
}
it("hands focus off only while the initiating source still owns it", () => {
  const { source, button, target } = controls();
  expect(handoffFocus(source, target)).toBe(false);
  button.focus();
  expect(handoffFocus(source, target)).toBe(true);
  expect(document.activeElement).toBe(target);
});
it.each(["hidden", "inert", "disabled"])(
  "refuses an unavailable %s destination",
  (attribute) => {
    const { source, button, target } = controls();
    button.focus();
    target.setAttribute(attribute, "");
    expect(handoffFocus(source, target)).toBe(false);
    expect(document.activeElement).toBe(button);
  },
);
