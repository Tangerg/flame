// @vitest-environment happy-dom
import { createTranslator } from "@flame/component-sdk";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, onTestFinished, vi } from "vitest";
import { en } from "./locales/en";
import { RecordedOutput } from "./recorded-output";

it("pans the focused output preview with plain horizontal arrows", () => {
  const host = document.body.appendChild(document.createElement("div"));
  const root = createRoot(host);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  onTestFinished(() => {
    act(() => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  });
  act(() =>
    root.render(
      <RecordedOutput
        text={`report finished\n${"output/path/".repeat(12)}run 2.104s`}
        t={createTranslator({ en }, "en")}
      />,
    ),
  );
  const preview = host.querySelector<HTMLElement>(
    '[role="region"][aria-label="Tool output preview"][tabindex="0"]',
  )!;
  Object.defineProperties(preview, {
    clientWidth: { value: 240 },
    scrollWidth: { value: 480 },
  });
  preview.focus();

  const press = (key: string, options?: KeyboardEventInit) => {
    const event = new KeyboardEvent("keydown", {
      key,
      bubbles: true,
      cancelable: true,
      ...options,
    });
    preview.dispatchEvent(event);
    return event;
  };
  expect(press("ArrowRight").defaultPrevented).toBe(true);
  expect(preview.scrollLeft).toBe(24);
  expect(press("ArrowRight", { shiftKey: true }).defaultPrevented).toBe(false);
  expect(press("ArrowRight", { cancelable: false }).defaultPrevented).toBe(
    false,
  );
  expect(preview.scrollLeft).toBe(24);
  expect(press("ArrowLeft").defaultPrevented).toBe(true);
  expect(preview.scrollLeft).toBe(0);
  expect(document.activeElement).toBe(preview);
  expect(getComputedStyle(preview).overflowX).toBe("auto");
  expect(getComputedStyle(preview).overscrollBehaviorX).toBe("contain");
});
