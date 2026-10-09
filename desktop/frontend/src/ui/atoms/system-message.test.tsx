import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SystemMessage } from "./system-message";

describe("system message semantics", () => {
  it.each([
    ["info", "info", "status"],
    ["warning", "alert", "alert"],
    ["error", "circle-x", "alert"],
    ["success", "circle-check", "status"],
  ] as const)("distinguishes a %s message", (variant, icon, role) => {
    const view = render(<SystemMessage variant={variant}>Result</SystemMessage>);
    const message = view.getByRole(role);
    expect(message.querySelector("svg")?.getAttribute("data-icon-name")).toBe(icon);
    expect(message.textContent).toBe("Result");
  });

  it("honors an explicit icon without changing the message role", () => {
    const view = render(
      <SystemMessage variant="warning" icon="shield">
        Approval required
      </SystemMessage>,
    );
    expect(view.getByRole("alert").querySelector("svg")?.getAttribute("data-icon-name")).toBe(
      "shield",
    );
  });

  it("can suppress the glyph while preserving the status", () => {
    const view = render(<SystemMessage hideIcon>Information</SystemMessage>);
    expect(view.getByRole("status").querySelector("svg")).toBeNull();
  });
});
