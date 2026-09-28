import { Component, type ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { definePlugin } from "../sdk";
import { PluginProvider } from "./PluginProvider";

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
      render(
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
