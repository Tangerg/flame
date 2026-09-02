import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CheckboxPrimitive, PopoverPrimitive, TabsPrimitive } from "./index";

// Every `data-[x]:` variant in the design system is a bet on which attribute Base UI writes,
// and losing that bet is silent: the rule compiles, ships, and never matches. A tab strip
// styled its active indicator with `data-[selected]` — which Base UI does not set on a Tab —
// so the accent was live and invisible until someone read the DOM.
//
// These are the three steady states that can be established by rendering. The other three —
// `data-highlighted`, `data-starting-style`, `data-ending-style` — need a real pointer or a
// running transition, so they are asserted in the visual suite, where both exist.
describe("the state attributes the design system styles against", () => {
  it("marks a checked checkbox with data-checked", () => {
    render(<CheckboxPrimitive.Root checked aria-label="ready" />);
    expect(screen.getByRole("checkbox")).toHaveProperty("dataset.checked", "");
  });

  it("marks the selected tab with data-active", () => {
    render(
      <TabsPrimitive.Root value="a">
        <TabsPrimitive.List>
          <TabsPrimitive.Tab value="a">A</TabsPrimitive.Tab>
          <TabsPrimitive.Tab value="b">B</TabsPrimitive.Tab>
        </TabsPrimitive.List>
      </TabsPrimitive.Root>,
    );
    expect(screen.getByRole("tab", { name: "A" })).toHaveProperty("dataset.active", "");
    expect(screen.getByRole("tab", { name: "B" }).dataset.active).toBeUndefined();
  });

  it("marks an open popover's trigger with data-popup-open", () => {
    render(
      <PopoverPrimitive.Root open>
        <PopoverPrimitive.Trigger>open</PopoverPrimitive.Trigger>
      </PopoverPrimitive.Root>,
    );
    expect(screen.getByRole("button", { name: "open" })).toHaveProperty("dataset.popupOpen", "");
  });
});
