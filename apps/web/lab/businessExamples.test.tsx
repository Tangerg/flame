import { type ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@flame/ui";
import { AppearanceProvider, createAppearanceProjection } from "@/lib/appearance";
import { ActivityExample } from "./businessActivity";
import { ComposerChipExample } from "./businessComposer";
import { DataExample } from "./businessFeedback";
import { FileIdentityExample } from "./businessFiles";
import { NavigationExample } from "./businessNavigation";

afterEach(cleanup);

function renderExample(node: ReactNode) {
  const appearance = createAppearanceProjection();
  return render(
    <AppearanceProvider value={appearance}>
      <TooltipProvider>{node}</TooltipProvider>
    </AppearanceProvider>,
  );
}

describe("business component examples", () => {
  it("keeps navigation selection local and records its callback", () => {
    const observe = vi.fn();
    renderExample(<NavigationExample variant="default" locale="en" observe={observe} />);
    fireEvent.click(screen.getByRole("button", { name: "Current tasks" }));
    expect(screen.getByRole("button", { name: "Current tasks" }).getAttribute("aria-pressed")).toBe(
      "true",
    );
    expect(screen.getByRole("status", { name: "Current sample selection" }).textContent).toBe(
      "tasks",
    );
    expect(observe).toHaveBeenCalledExactlyOnceWith("navigation.select", "tasks");
  });

  it("lets the production disclosure control its content and reports each toggle", () => {
    const observe = vi.fn();
    renderExample(<ActivityExample variant="card" locale="en" observe={observe} />);
    const trigger = screen.getByRole("button", { name: /Inspect workspace/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("region").textContent).toContain("Component interfaces");
    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(observe.mock.calls).toEqual([
      ["activity.toggle", true],
      ["activity.toggle", false],
    ]);
  });

  it("chooses a local model through the actual chip popover", async () => {
    const observe = vi.fn();
    renderExample(<ComposerChipExample variant="default" locale="en" observe={observe} />);
    fireEvent.click(
      screen.getByRole("button", { name: "Local sample model, Choose a sample model" }),
    );
    fireEvent.click(await screen.findByRole("button", { name: "Reasoning sample model" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(
      screen.getByRole("button", { name: "Reasoning sample model, Choose a sample model" }),
    ).toBeTruthy();
    expect(screen.getByRole("status", { name: "Current sample selection" }).textContent).toBe(
      "reasoning-preview",
    );
    expect(observe).toHaveBeenCalledWith("composer.model.select", "reasoning-preview");
  });

  it("records Retry without fabricating a successful read", () => {
    const observe = vi.fn();
    renderExample(<DataExample variant="failure" locale="en" observe={observe} />);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(observe).toHaveBeenCalledExactlyOnceWith("data.retry");
    expect(screen.getByText("Could not read the sample files")).toBeTruthy();
    expect(screen.queryByText("App.tsx")).toBeNull();
  });

  it("retains literal local path edits and reports only their length", () => {
    const observe = vi.fn();
    renderExample(<FileIdentityExample variant="unicode" locale="en" observe={observe} />);
    const field = screen.getByRole("textbox", { name: "Sample path" });
    const path = " recorded workspace/中文 sample.tsx ";
    fireEvent.change(field, { target: { value: path } });
    expect((field as HTMLInputElement).value).toBe(path);
    expect(screen.getByTitle(path, { normalizer: (value) => value })).toBeTruthy();
    expect(observe).toHaveBeenCalledExactlyOnceWith("file.path.length", path.length);
  });
});
