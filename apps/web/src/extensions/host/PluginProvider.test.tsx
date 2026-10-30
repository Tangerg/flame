import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { screen } from "@testing-library/react";
import { Component, type ReactNode } from "react";
import { type AnyPlugin, type Host } from "dougong";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PluginProvider } from "@/extensions/host/PluginProvider";
const mocks = vi.hoisted(() => ({
  startKernel: vi.fn(),
  stopKernel: vi.fn(),
}));

vi.mock("@/extensions/bootstrap", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/extensions/bootstrap")>();
  return { ...actual, startKernel: mocks.startKernel, stopKernel: mocks.stopKernel };
});

function host(): Host {
  return {} as Host;
}

class FailureBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  override state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  override render() {
    return this.state.failed ? <div role="alert">kernel failed</div> : this.props.children;
  }
}

beforeEach(() => {
  mocks.startKernel.mockReset();
  mocks.stopKernel.mockReset().mockResolvedValue(undefined);
});

afterEach(() => vi.restoreAllMocks());

describe("PluginProvider kernel ownership", () => {
  it("reports failed retirement and refuses to install a successor over it", async () => {
    const failure = new Error("retirement refused");
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    mocks.startKernel.mockResolvedValue(host());
    mocks.stopKernel.mockRejectedValueOnce(failure);
    const view = testClient.render(
      <FailureBoundary>
        <PluginProvider plugins={[]}>
          <div>workspace</div>
        </PluginProvider>
      </FailureBoundary>,
    );
    await screen.findByText("workspace");

    view.rerender(
      <FailureBoundary>
        <PluginProvider plugins={[]}>
          <div>workspace</div>
        </PluginProvider>
      </FailureBoundary>,
    );

    await screen.findByRole("alert");
    expect(mocks.startKernel).toHaveBeenCalledOnce();
    expect(consoleError).toHaveBeenCalledWith("[plugin] kernel retirement failed:", failure);
  });

  it("skips a superseded installation while its predecessor is still retiring", async () => {
    const retirement = Promise.withResolvers<void>();
    finishTestClient(() => retirement.resolve());
    mocks.startKernel.mockResolvedValue(host());
    mocks.stopKernel.mockReturnValueOnce(retirement.promise);
    const view = testClient.render(
      <PluginProvider plugins={[]}>
        <div>workspace</div>
      </PluginProvider>,
    );
    await screen.findByText("workspace");
    const skipped: AnyPlugin[] = [];
    const current: AnyPlugin[] = [];
    view.rerender(
      <PluginProvider plugins={skipped}>
        <div>workspace</div>
      </PluginProvider>,
    );
    await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledOnce());
    view.rerender(
      <PluginProvider plugins={current}>
        <div>workspace</div>
      </PluginProvider>,
    );
    expect(mocks.startKernel).toHaveBeenCalledOnce();
    retirement.resolve();

    await screen.findByText("workspace");
    expect(mocks.startKernel).toHaveBeenCalledTimes(2);
    expect(mocks.startKernel.mock.calls[1]?.[0]).toBe(current);
    expect(mocks.startKernel.mock.calls.some(([plugins]) => plugins === skipped)).toBe(false);
  });

  it("joins the retired kernel before a successor can bind the same feature owners", async () => {
    const first = host();
    const second = host();
    const retirement = Promise.withResolvers<void>();
    finishTestClient(() => retirement.resolve());
    mocks.startKernel.mockResolvedValueOnce(first).mockResolvedValueOnce(second);
    mocks.stopKernel.mockReturnValueOnce(retirement.promise);
    const view = testClient.render(
      <PluginProvider plugins={[]}>
        <div>workspace</div>
      </PluginProvider>,
    );
    await screen.findByText("workspace");

    view.rerender(
      <PluginProvider plugins={[]}>
        <div>workspace</div>
      </PluginProvider>,
    );

    await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledExactlyOnceWith(first));
    expect(mocks.startKernel).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("workspace")).toBeNull();
    retirement.resolve();
    await screen.findByText("workspace");
    expect(mocks.startKernel).toHaveBeenCalledTimes(2);
    view.unmount();
    await vi.waitFor(() => expect(mocks.stopKernel.mock.calls).toEqual([[first], [second]]));
  });

  it("stops the owned kernel when the provider unmounts", async () => {
    const owned = host();
    mocks.startKernel.mockResolvedValue(owned);
    const view = testClient.render(
      <PluginProvider plugins={[]}>
        <div>workspace</div>
      </PluginProvider>,
    );
    await screen.findByText("workspace");

    view.unmount();

    await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledExactlyOnceWith(owned));
  });

  it("stops a kernel whose startup settles after its provider was retired", async () => {
    const startup = Promise.withResolvers<Host>();
    const owned = host();
    mocks.startKernel.mockReturnValue(startup.promise);
    const view = testClient.render(
      <PluginProvider plugins={[]}>
        <div>workspace</div>
      </PluginProvider>,
    );

    await vi.waitFor(() => expect(mocks.startKernel).toHaveBeenCalledOnce());
    view.unmount();
    startup.resolve(owned);

    await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledExactlyOnceWith(owned));
  });
});

it("replaces only its owned plugin generation and waits for the injected successor", async () => {
  const first = host();
  const second = host();
  const successor = Promise.withResolvers<Host>();
  const initialPlugins: AnyPlugin[] = [];
  const successorPlugins: AnyPlugin[] = [];
  mocks.startKernel.mockResolvedValueOnce(first).mockReturnValueOnce(successor.promise);
  const view = testClient.render(
    <PluginProvider plugins={initialPlugins}>
      <div>workspace</div>
    </PluginProvider>,
  );
  await screen.findByText("workspace");
  expect(mocks.startKernel.mock.calls[0]?.[0]).toBe(initialPlugins);
  view.rerender(
    <PluginProvider plugins={successorPlugins}>
      <div>workspace</div>
    </PluginProvider>,
  );
  expect(screen.queryByText("workspace")).toBeNull();
  await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledExactlyOnceWith(first));
  await vi.waitFor(() => expect(mocks.startKernel.mock.calls[1]?.[0]).toBe(successorPlugins));
  successor.resolve(second);
  await screen.findByText("workspace");
  view.unmount();
  await vi.waitFor(() => expect(mocks.stopKernel.mock.calls).toEqual([[first], [second]]));
});

it("does not reuse readiness when the same plugin list is installed again", async () => {
  const initialPlugins: AnyPlugin[] = [];
  const intermediatePlugins: AnyPlugin[] = [];
  const initialHost = host();
  const intermediateHost = host();
  const successorHost = host();
  const intermediate = Promise.withResolvers<Host>();
  const successor = Promise.withResolvers<Host>();
  mocks.startKernel
    .mockResolvedValueOnce(initialHost)
    .mockReturnValueOnce(intermediate.promise)
    .mockReturnValueOnce(successor.promise);
  const view = testClient.render(
    <PluginProvider plugins={initialPlugins}>
      <div>workspace</div>
    </PluginProvider>,
  );
  await screen.findByText("workspace");
  view.rerender(
    <PluginProvider plugins={intermediatePlugins}>
      <div>workspace</div>
    </PluginProvider>,
  );
  await vi.waitFor(() => expect(mocks.startKernel).toHaveBeenCalledTimes(2));
  view.rerender(
    <PluginProvider plugins={initialPlugins}>
      <div>workspace</div>
    </PluginProvider>,
  );
  expect(screen.queryByText("workspace")).toBeNull();
  intermediate.resolve(intermediateHost);
  await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledWith(intermediateHost));
  expect(screen.queryByText("workspace")).toBeNull();
  successor.resolve(successorHost);
  await screen.findByText("workspace");
  view.unmount();
  await vi.waitFor(() => expect(mocks.stopKernel).toHaveBeenCalledWith(successorHost));
});
