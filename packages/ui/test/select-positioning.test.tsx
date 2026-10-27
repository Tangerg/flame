import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Select } from "../dist/index.js";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("keeps an open selection anchored when its scroll viewport moves the trigger", async () => {
  let triggerTop = 200;
  const originalRect = HTMLElement.prototype.getBoundingClientRect;
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    function (this: HTMLElement) {
      if (this.getAttribute("role") === "combobox") {
        return new DOMRect(100, triggerTop, 160, 32);
      }
      if (this.dataset.testid === "viewport")
        return new DOMRect(0, 0, 1024, 400);
      if (this === document.documentElement)
        return new DOMRect(0, 0, 1024, 768);
      return originalRect.call(this);
    },
  );
  vi.spyOn(document.documentElement, "clientWidth", "get").mockReturnValue(
    1024,
  );
  vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(
    768,
  );
  const view = render(
    <div data-testid="viewport" style={{ height: 400, overflowY: "auto" }}>
      <Select
        label="Interface size"
        value="14"
        onValueChange={() => {}}
        options={[
          { value: "default", label: "Default" },
          { value: "14", label: "14" },
          { value: "18", label: "18" },
        ]}
      />
    </div>,
  );
  Object.defineProperties(view.getByTestId("viewport"), {
    clientHeight: { configurable: true, value: 400 },
    clientWidth: { configurable: true, value: 1024 },
  });
  const trigger = screen.getByRole("combobox");
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const popup = await screen.findByRole("listbox");
  await vi.waitFor(() =>
    expect(popup.contains(document.activeElement)).toBe(true),
  );
  const positioner = popup.parentElement!;
  const popupTop = () => {
    const translated = positioner.style.transform.match(
      /translate\([^,]+,\s*([\d.-]+)px\)/,
    );
    const top = Number.parseFloat(translated?.[1] ?? positioner.style.top);
    return Number.isFinite(top) ? top : positioner.getBoundingClientRect().top;
  };
  const before = popupTop();
  triggerTop -= 60;
  fireEvent.scroll(view.getByTestId("viewport"));
  await vi.waitFor(() => expect(popupTop()).toBe(before - 60));

  triggerTop -= 30;
  fireEvent.resize(window);
  await vi.waitFor(() => expect(popupTop()).toBe(before - 90));
});
