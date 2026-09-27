import { render, screen } from "@testing-library/react";
import type { AnyPlugin, Host } from "dougong";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  startKernel: vi.fn(),
  stopKernel: vi.fn(),
}));

vi.mock("../sdk", () => ({
  startKernel: mocks.startKernel,
  stopKernel: mocks.stopKernel,
}));

import { PluginProvider } from "./PluginProvider";

function host(): Host {
  return {} as Host;
}

beforeEach(() => {
  mocks.startKernel.mockReset();
  mocks.stopKernel.mockReset().mockResolvedValue(undefined);
});

afterEach(() => vi.restoreAllMocks());

describe("PluginProvider kernel ownership", () => {
  it("stops the owned kernel when the provider unmounts", async () => {
    const owned = host();
    mocks.startKernel.mockResolvedValue(owned);
    const view = render(
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
    const view = render(
      <PluginProvider plugins={[]}>
        <div>workspace</div>
      </PluginProvider>,
    );

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
  const view = render(
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
  expect(mocks.stopKernel).toHaveBeenCalledExactlyOnceWith(first);
  expect(mocks.startKernel.mock.calls[1]?.[0]).toBe(successorPlugins);
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
  const view = render(
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
  expect(mocks.stopKernel).toHaveBeenCalledWith(successorHost);
});
