import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});

import { describe, expect, it } from "vitest";
import { SystemMessage } from "@/ui/atoms/system-message";
describe("system message semantics", () => {
  it.each([
    ["info", "info", "status"],
    ["warning", "alert", "alert"],
    ["error", "circle-x", "alert"],
    ["success", "circle-check", "status"],
  ] as const)("distinguishes a %s message", (variant, icon, role) => {
    const view = testClient.render(<SystemMessage variant={variant}>Result</SystemMessage>);
    const message = view.getByRole(role);
    expect(message.querySelector("svg")?.getAttribute("data-icon-name")).toBe(icon);
    expect(message.textContent).toBe("Result");
  });

  it("honors an explicit icon without changing the message role", () => {
    const view = testClient.render(
      <SystemMessage variant="warning" icon="shield">
        Approval required
      </SystemMessage>,
    );
    expect(view.getByRole("alert").querySelector("svg")?.getAttribute("data-icon-name")).toBe(
      "shield",
    );
  });

  it("can suppress the glyph while preserving the status", () => {
    const view = testClient.render(<SystemMessage hideIcon>Information</SystemMessage>);
    expect(view.getByRole("status").querySelector("svg")).toBeNull();
  });
});
