import { afterEach, describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { toolPreviewPlugins } from "@/plugins/builtin";
import { ToolPreview } from "./ToolPreview";

function show(tool: ToolCall) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ToolPreview tool={tool} />
    </QueryClientProvider>,
  );
}

afterEach(async () => {
  await resetKernelForTest();
});

function mcpCall(result: string): ToolCall {
  return {
    id: "t1",
    runId: "r1",
    name: "acme.search_docs",
    fn: "acme.search_docs",
    args: "{}",
    status: "ok",
    result,
  };
}

describe("tool previews for a tool nobody registered", () => {
  it("renders a search result by its shape rather than dumping JSON", async () => {
    await loadPluginsForTest(...toolPreviewPlugins);

    show(
      mcpCall(
        JSON.stringify({ hits: [{ path: "src/a.ts", lineNumber: 12, snippet: "needle here" }] }),
      ),
    );

    expect(screen.getByText("src/a.ts")).toBeTruthy();
    expect(screen.queryByText(/"hits"/)).toBeNull();
  });

  it("renders a command result's output, not the envelope around it", async () => {
    await loadPluginsForTest(...toolPreviewPlugins);

    show(mcpCall(JSON.stringify({ output: "built in 2s", exitCode: 0 })));

    expect(screen.getByText(/built in 2s/)).toBeTruthy();
    expect(screen.queryByText(/exitCode/)).toBeNull();
  });

  it("falls back to the inspector when no shape matches", async () => {
    await loadPluginsForTest(...toolPreviewPlugins);

    show(mcpCall(JSON.stringify({ nothing: "recognisable" })));

    expect(screen.getByText(/recognisable/)).toBeTruthy();
  });

  it("prefers a registered name over the shape it happens to match", async () => {
    await loadPluginsForTest(...toolPreviewPlugins);
    const named: ToolCall = {
      ...mcpCall(JSON.stringify({ hits: [{ path: "src/named.ts" }] })),
      name: "glob",
      fn: "glob",
    };

    show(named);

    expect(screen.getByText("src/named.ts")).toBeTruthy();
  });
});
