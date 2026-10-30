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
  it("joins a canceled real startup and its rollback before installing the next kernel", async () => {
    const entered = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    finishTestClient(() => release.resolve());
    const stopped = vi.fn();
    const started = vi.fn();
    const first = definePlugin({
      name: "test.pending-startup",
      async setup(ctx) {
        ctx.cleanup(stopped);
        entered.resolve();
        await release.promise;
        return {};
      },
    });
    const next = definePlugin({ name: "test.successor", setup: started });
    const view = testClient.render(
      <PluginProvider plugins={[first]}>
        <div>ready</div>
      </PluginProvider>,
    );
    await entered.promise;
    view.rerender(
      <PluginProvider plugins={[next]}>
        <div>ready</div>
      </PluginProvider>,
    );
    expect(started).not.toHaveBeenCalled();
    release.resolve();

    await screen.findByText("ready");
    expect(stopped).toHaveBeenCalledOnce();
    expect(started).toHaveBeenCalledOnce();
  });

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
