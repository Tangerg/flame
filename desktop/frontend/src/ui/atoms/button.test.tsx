import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Button } from "./button";

// `pending` and `disabled` say different things and the difference is only visible in the tab
// order, which is why forty-four call sites got it wrong: `disabled` means the action is
// unavailable and the platform enforces it by making the element unfocusable, so a control that
// disabled itself while its own work ran blurred whoever was standing on it.
//
// Measured before the fix, on Settings → Connection: press Enter on Refresh and focus is on
// `<body>` 120ms later, and still there a second after the work finished. That the product no
// longer does it is checked by `visual/asyncFocus.visual.spec.ts`, which drives the real
// control; this pins the mechanism the fix rests on.
describe("Button pending", () => {
  it("stays focusable while its action is in flight", () => {
    render(<Button pending>Refresh</Button>);
    const refresh = screen.getByRole("button", { name: "Refresh" });

    // The distinction itself: announced as disabled, still reachable. A button carrying no
    // `disabled` attribute and no negative tabindex is in the tab order by definition, which
    // is the property `disabled` takes away.
    expect(refresh.getAttribute("aria-disabled")).toBe("true");
    expect((refresh as HTMLButtonElement).disabled).toBe(false);
    expect(refresh.tabIndex).toBeGreaterThanOrEqual(0);

    refresh.focus();
    expect(document.activeElement).toBe(refresh);
  });

  it("refuses its own activation while pending", () => {
    const onClick = vi.fn();
    render(
      <Button pending onClick={onClick}>
        Refresh
      </Button>,
    );
    // Nothing in the platform refuses a click on an `aria-disabled` element, so the component
    // has to. Without this the prop would trade a lost focus for a double submit.
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(onClick).not.toHaveBeenCalled();
  });

  it("acts normally when it is not pending", () => {
    const onClick = vi.fn();
    render(<Button onClick={onClick}>Refresh</Button>);
    const button = screen.getByRole("button", { name: "Refresh" });
    expect(button.getAttribute("aria-disabled")).toBeNull();
    fireEvent.click(button);
    expect(onClick).toHaveBeenCalledOnce();
  });

  // The other half of the pair keeps its meaning: an action that is genuinely unavailable is
  // still `disabled`, and still leaves the tab order. A form that is invalid has not started
  // any work, and offering its submit button to the keyboard would be a lie.
  it("leaves an unavailable action disabled", () => {
    render(<Button disabled>Save</Button>);
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
