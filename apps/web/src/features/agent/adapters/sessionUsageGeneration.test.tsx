import { createAgentGatewayFixture } from "@/test/agentOwners";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient({ runtimeClient: getRuntimeClient });
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { type FlameClient } from "@flame/runtime-client";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, waitFor } from "@testing-library/react";
import { type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createFlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { respondSuccess } from "@flame/runtime-client/testing";
import { queryClient } from "@/lib/queryClient";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

let transport: ReturnType<typeof createMemoryTransport>;

let client: ReturnType<typeof createFlameClient>;

let restoreGateway: ReturnType<typeof createAgentGatewayFixture> | undefined;

let unmountHook: (() => void) | undefined;

let restoreQueryDefaults: (() => void) | undefined;

async function waitForUsageRequest(
  index: number,
  source: ReturnType<typeof createMemoryTransport> = transport,
) {
  for (let attempt = 0; attempt < 50; attempt++) {
    const request = source.outbox().filter(({ method }) => method === "usage.session")[index];
    if (request) return request;
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  throw new Error(`timeout waiting for usage.session request ${index + 1}`);
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

function usageReport(inputTokens: number) {
  return {
    total: { inputTokens, outputTokens: 0 },
    coverage: {
      calls: 1,
      reportedCalls: 1,
      pricedCalls: 0,
      startedCalls: 0,
      completedCalls: 1,
      failedCalls: 0,
      unknownCalls: 0,
    },
    byModel: [],
  };
}

beforeEach(() => {
  const defaults = queryClient.getDefaultOptions();
  queryClient.setDefaultOptions({
    ...defaults,
    queries: {
      ...defaults.queries,
      retry: false,
    },
  });
  restoreQueryDefaults = () => queryClient.setDefaultOptions(defaults);
  queryClient.clear();
  transport = createMemoryTransport();
  client = createFlameClient(transport);
  runtimeClient = () => client;
  restoreGateway = createAgentGatewayFixture(testClient, getRuntimeClient);
});

afterEach(async () => {
  unmountHook?.();
  unmountHook = undefined;
  restoreGateway?.dispose();
  restoreGateway = undefined;
  queryClient.clear();
  restoreQueryDefaults?.();
  restoreQueryDefaults = undefined;
  await client.close();

  vi.restoreAllMocks();
});

describe("mounted Session usage generation", () => {
  it("keeps the same Session ID in two client instances on independent cache writers", async () => {
    const first = testClient.renderHook(
      () => testClient.agent.access.usage.useAgentSessionUsage("ses_shared"),
      { wrapper },
    );
    unmountHook = first.unmount;
    const firstRequest = await waitForUsageRequest(0);
    respondSuccess(transport, firstRequest.id, usageReport(13));
    await waitFor(() => expect(first.result.current.data?.inputTokens).toBe(13));

    const secondTransport = createMemoryTransport();
    const secondClient = createFlameClient(secondTransport);
    const secondOwner = createTestClient({ runtimeClient: () => secondClient });
    const second = secondOwner.renderHook(
      () => secondOwner.agent.access.usage.useAgentSessionUsage("ses_shared"),
      { wrapper },
    );
    try {
      const secondRequest = await waitForUsageRequest(0, secondTransport);
      expect(second.result.current.data).toBeUndefined();
      expect(first.result.current.data?.inputTokens).toBe(13);
      expect(transport.outbox().filter(({ method }) => method === "usage.session")).toHaveLength(1);

      respondSuccess(secondTransport, secondRequest.id, usageReport(34));
      await waitFor(() => expect(second.result.current.data?.inputTokens).toBe(34));
      expect(first.result.current.data?.inputTokens).toBe(13);

      await act(async () => {
        restoreGateway?.replaceRuntimeGeneration();
        await Promise.resolve();
      });
      const replacement = await waitForUsageRequest(1);
      expect(
        secondTransport.outbox().filter(({ method }) => method === "usage.session"),
      ).toHaveLength(1);
      expect(second.result.current.data?.inputTokens).toBe(34);
      respondSuccess(transport, replacement.id, usageReport(21));
      await waitFor(() => expect(first.result.current.data?.inputTokens).toBe(21));
      expect(second.result.current.data?.inputTokens).toBe(34);
    } finally {
      second.unmount();
      await secondOwner.dispose();
      await secondClient.close();
    }
  });

  it("does not cancel another instance's pending read when the first instance is disposed", async () => {
    const firstSend = vi.spyOn(transport, "send");
    const first = testClient.renderHook(
      () => testClient.agent.access.usage.useAgentSessionUsage("ses_shared"),
      { wrapper },
    );
    unmountHook = first.unmount;
    const firstRequest = await waitForUsageRequest(0);
    const firstSignal = firstSend.mock.calls[0]?.[1];

    const secondTransport = createMemoryTransport();
    const secondSend = vi.spyOn(secondTransport, "send");
    const secondClient = createFlameClient(secondTransport);
    const secondOwner = createTestClient({ runtimeClient: () => secondClient });
    const second = secondOwner.renderHook(
      () => secondOwner.agent.access.usage.useAgentSessionUsage("ses_shared"),
      { wrapper },
    );
    try {
      const secondRequest = await waitForUsageRequest(0, secondTransport);
      const secondSignal = secondSend.mock.calls[0]?.[1];
      expect(firstSignal?.aborted).toBe(false);
      expect(secondSignal?.aborted).toBe(false);

      act(() => {
        restoreGateway?.dispose();
        testClient.agent.dispose();
      });

      expect(firstSignal?.aborted).toBe(true);
      expect(secondSignal?.aborted).toBe(false);
      respondSuccess(secondTransport, secondRequest.id, usageReport(34));
      await waitFor(() => expect(second.result.current.data?.inputTokens).toBe(34));
      respondSuccess(transport, firstRequest.id, usageReport(1));
      await act(async () => Promise.resolve());
      expect(second.result.current.data?.inputTokens).toBe(34);
      expect(
        secondTransport.outbox().filter(({ method }) => method === "usage.session"),
      ).toHaveLength(1);
    } finally {
      second.unmount();
      await secondOwner.dispose();
      await secondClient.close();
    }
  });

  it("does not let an empty handoff settlement invalidate a later query", async () => {
    restoreGateway?.dispose();
    restoreGateway = undefined;
    await act(async () => Promise.resolve());
    queryClient.clear();

    let releaseHandoff!: () => void;
    const handoffSettlement = new Promise<void>((resolve) => {
      releaseHandoff = resolve;
    });
    const cancelQueries = vi
      .spyOn(queryClient, "cancelQueries")
      .mockReturnValueOnce(handoffSettlement);
    restoreGateway = createAgentGatewayFixture(testClient, getRuntimeClient);

    const hook = testClient.renderHook(
      () => testClient.agent.access.usage.useAgentSessionUsage("ses_future"),
      { wrapper },
    );
    unmountHook = hook.unmount;
    const request = await waitForUsageRequest(0);
    respondSuccess(transport, request.id, {
      total: { inputTokens: 13, outputTokens: 5 },
      coverage: {
        calls: 1,
        reportedCalls: 1,
        pricedCalls: 0,
        startedCalls: 0,
        completedCalls: 1,
        failedCalls: 0,
        unknownCalls: 0,
      },
      byModel: [],
    });
    await waitFor(() => expect(hook.result.current.data?.inputTokens).toBe(13));

    await act(async () => {
      releaseHandoff();
      await handoffSettlement;
      await Promise.resolve();
    });

    expect(transport.outbox().filter(({ method }) => method === "usage.session")).toHaveLength(1);
    expect(cancelQueries).not.toHaveBeenCalled();
  });

  it("aborts the pre-event transport before committing the successor read", async () => {
    const send = vi.spyOn(transport, "send");
    const hook = testClient.renderHook(
      () => testClient.agent.access.usage.useAgentSessionUsage("ses_usage"),
      { wrapper },
    );
    unmountHook = hook.unmount;
    const first = await waitForUsageRequest(0);
    const firstSignal = send.mock.calls[0]?.[1];

    act(() => {
      void queryClient.cancelQueries({
        queryKey: testClient.agent.access.usage.queryKey("ses_usage"),
        exact: true,
      });
      void queryClient.invalidateQueries({
        queryKey: testClient.agent.access.usage.queryKey("ses_usage"),
        exact: true,
      });
    });

    const second = await waitForUsageRequest(1);
    respondSuccess(transport, second.id, {
      total: { inputTokens: 21, outputTokens: 8 },
      coverage: {
        calls: 1,
        reportedCalls: 1,
        pricedCalls: 0,
        startedCalls: 0,
        completedCalls: 1,
        failedCalls: 0,
        unknownCalls: 0,
      },
      byModel: [],
    });
    await waitFor(() => expect(hook.result.current.data?.inputTokens).toBe(21));
    respondSuccess(transport, first.id, {
      total: { inputTokens: 1, outputTokens: 1 },
      coverage: {
        calls: 1,
        reportedCalls: 1,
        pricedCalls: 0,
        startedCalls: 0,
        completedCalls: 1,
        failedCalls: 0,
        unknownCalls: 0,
      },
      byModel: [],
    });
    await act(async () => Promise.resolve());

    expect(firstSignal).toBeInstanceOf(AbortSignal);
    expect(firstSignal?.aborted).toBe(true);
    expect(send.mock.calls[1]?.[1]?.aborted).toBe(false);
    expect(hook.result.current.data?.inputTokens).toBe(21);
  });

  it("hands the cache writer to the successor Runtime gateway", async () => {
    const retiredSend = vi.spyOn(transport, "send");
    const hook = testClient.renderHook(
      () => testClient.agent.access.usage.useAgentSessionUsage("ses_usage"),
      { wrapper },
    );
    unmountHook = hook.unmount;
    const retiredRequest = await waitForUsageRequest(0);
    const retiredSignal = retiredSend.mock.calls[0]?.[1];

    const successorTransport = createMemoryTransport();
    const successorClient = createFlameClient(successorTransport);
    const successorSend = vi.spyOn(successorTransport, "send");
    runtimeClient = () => successorClient;
    let disposeSuccessor!: ReturnType<typeof createAgentGatewayFixture>;
    await act(async () => {
      disposeSuccessor = createAgentGatewayFixture(testClient, getRuntimeClient);
      await Promise.resolve();
    });
    restoreGateway?.dispose();
    restoreGateway = undefined;
    try {
      const successorRequest = await waitForUsageRequest(0, successorTransport);
      respondSuccess(successorTransport, successorRequest.id, {
        total: { inputTokens: 34, outputTokens: 13 },
        coverage: {
          calls: 1,
          reportedCalls: 1,
          pricedCalls: 0,
          startedCalls: 0,
          completedCalls: 1,
          failedCalls: 0,
          unknownCalls: 0,
        },
        byModel: [],
      });
      await waitFor(() => expect(hook.result.current.data?.inputTokens).toBe(34));

      expect(retiredSignal?.aborted).toBe(true);
      expect(successorSend.mock.calls[0]?.[1]?.aborted).toBe(false);
      respondSuccess(transport, retiredRequest.id, {
        total: { inputTokens: 1, outputTokens: 1 },
        coverage: {
          calls: 1,
          reportedCalls: 1,
          pricedCalls: 0,
          startedCalls: 0,
          completedCalls: 1,
          failedCalls: 0,
          unknownCalls: 0,
        },
        byModel: [],
      });
      await act(async () => Promise.resolve());
      expect(hook.result.current.data?.inputTokens).toBe(34);
    } finally {
      disposeSuccessor.dispose();
      await successorClient.close();
    }
  });
});
