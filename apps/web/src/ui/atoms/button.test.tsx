import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Button } from "@flame/ui/button";
describe("Button pending", () => {
  it("stays focusable while its action is in flight", () => {
    testClient.render(<Button pending>Refresh</Button>);
    const refresh = screen.getByRole("button", { name: "Refresh" });

    expect(refresh.getAttribute("aria-disabled")).toBe("true");
    expect((refresh as HTMLButtonElement).disabled).toBe(false);
    expect(refresh.tabIndex).toBeGreaterThanOrEqual(0);

    refresh.focus();
    expect(document.activeElement).toBe(refresh);
  });

  it("refuses its own activation while pending", () => {
    const onClick = vi.fn();
    testClient.render(
      <Button pending onClick={onClick}>
        Refresh
      </Button>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(onClick).not.toHaveBeenCalled();
  });

  it("acts normally when it is not pending", () => {
    const onClick = vi.fn();
    testClient.render(<Button onClick={onClick}>Refresh</Button>);
    const button = screen.getByRole("button", { name: "Refresh" });
    expect(button.getAttribute("aria-disabled")).toBeNull();
    fireEvent.click(button);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("leaves an unavailable action disabled", () => {
    testClient.render(<Button disabled>Save</Button>);
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("Button focusable when disabled", () => {
  it("announces itself disabled and keeps its place in the tab order", () => {
    testClient.render(
      <Button disabled focusableWhenDisabled>
        Next
      </Button>,
    );
    const next = screen.getByRole("button", { name: "Next" });

    expect(next.getAttribute("aria-disabled")).toBe("true");
    expect((next as HTMLButtonElement).disabled).toBe(false);
    next.focus();
    expect(document.activeElement).toBe(next);
  });
});
