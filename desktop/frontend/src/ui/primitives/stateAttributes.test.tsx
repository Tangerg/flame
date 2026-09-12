import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CheckboxPrimitive, PopoverPrimitive, TabsPrimitive } from "./index";

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
