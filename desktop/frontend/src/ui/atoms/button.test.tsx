import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Button } from "./button";

describe("Button pending", () => {
  it("stays focusable while its action is in flight", () => {
    render(<Button pending>Refresh</Button>);
    const refresh = screen.getByRole("button", { name: "Refresh" });

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

  it("leaves an unavailable action disabled", () => {
    render(<Button disabled>Save</Button>);
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
