// @vitest-environment happy-dom
import { createTranslator } from "@flame/component-sdk";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, onTestFinished, vi } from "vitest";
import { en } from "./locales/en";
import { RecordedOutput } from "./recorded-output";

function renderOutput(text: string): HTMLElement {
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
      <RecordedOutput text={text} t={createTranslator({ en }, "en")} />,
    ),
  );
  return host;
}

function press(target: HTMLElement, key: string, options?: KeyboardEventInit) {
  const event = new KeyboardEvent("keydown", {
    key,
    bubbles: true,
    cancelable: true,
    ...options,
  });
  target.dispatchEvent(event);
  return event;
}

it("pans the focused output preview with plain horizontal arrows", () => {
  const host = renderOutput(
    `report finished\n${"output/path/".repeat(12)}run 2.104s`,
  );
  const preview = host.querySelector<HTMLElement>(
    '[role="region"][aria-label="Tool output preview"][tabindex="0"]',
  )!;
  Object.defineProperties(preview, {
    clientWidth: { value: 240 },
    scrollWidth: { value: 480 },
  });
  preview.focus();

  expect(press(preview, "ArrowRight").defaultPrevented).toBe(true);
  expect(preview.scrollLeft).toBe(24);
  expect(
    press(preview, "ArrowRight", { shiftKey: true }).defaultPrevented,
  ).toBe(false);
  expect(
    press(preview, "ArrowRight", { cancelable: false }).defaultPrevented,
  ).toBe(false);
  expect(preview.scrollLeft).toBe(24);
  expect(press(preview, "ArrowLeft").defaultPrevented).toBe(true);
  expect(preview.scrollLeft).toBe(0);
  expect(document.activeElement).toBe(preview);
  expect(getComputedStyle(preview).overflowX).toBe("auto");
  expect(getComputedStyle(preview).overscrollBehaviorX).toBe("contain");
});

it("pans overflowing expanded output from its focused reader", () => {
  const host = renderOutput(
    `${"suite passed\n".repeat(9)}${"output/path/".repeat(12)}Rollback 8.412s`,
  );
  const expand = host.querySelector<HTMLButtonElement>(
    'button[aria-expanded="false"]',
  )!;
  act(() => {
    expand.focus();
    expand.click();
  });
  const viewport = host.querySelector<HTMLElement>(
    '[role="region"][aria-label="Tool output"][tabindex="0"]',
  )!;
  let width = 480;
  Object.defineProperties(viewport, {
    clientWidth: { value: 240 },
    scrollWidth: { get: () => width },
  });
  expect(document.activeElement).toBe(viewport);

  const forward = press(viewport, "ArrowRight");
  expect(viewport.scrollLeft).toBe(24);
  expect(forward.defaultPrevented).toBe(true);
  expect(press(viewport, "ArrowLeft").defaultPrevented).toBe(true);
  expect(viewport.scrollLeft).toBe(0);
  expect(document.activeElement).toBe(viewport);

  width = 240;
  expect(press(viewport, "ArrowRight").defaultPrevented).toBe(false);
  expect(viewport.scrollLeft).toBe(0);
});
