import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { Component, type ReactNode } from "react";
import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { definePlugin } from "@/extensions/definePlugin";
import { PluginProvider } from "@/extensions/host/PluginProvider";
class FailureBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  override state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  override render() {
    return this.state.failed ? <div role="alert">startup failed</div> : this.props.children;
  }
}

describe("plugin installation failure", () => {
  it("renders startup failures through the application error boundary", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    try {
      const plugin = definePlugin({
        name: "test.startup-failure",
        setup() {
          throw new Error("installation refused");
        },
      });
      testClient.render(
        <FailureBoundary>
          <PluginProvider plugins={[plugin]}>
            <div>ready</div>
          </PluginProvider>
        </FailureBoundary>,
      );
      expect((await screen.findByRole("alert")).textContent).toContain("startup failed");
      expect(screen.queryByText("ready")).toBeNull();
    } finally {
      consoleError.mockRestore();
    }
  });
});
