import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type {
  WorkspaceFileChange as WorkspaceFileChangeSummary,
  WorkspaceProjectSummary,
  WorkspaceDiff,
} from "@/plugins/builtin/workspace/public/queries";
import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createFlameClient, JSONRPC_VERSION } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess, waitForRequest } from "@/rpc/transports/memory.testkit";
import type { WireMethodName } from "@flame/runtime-contract/methods";
import { defaultDataProviders } from "./index";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { SelectableModel } from "@/plugins/builtin/settings/providers/public/queries";

afterEach(resetContainer);

async function runProvider<T>(
  key: string,
  responses: Array<[method: WireMethodName, result: unknown]>,
  params?: unknown,
): Promise<{ value: T; requests: Array<{ method: string; params: unknown }> }> {
  const t = createMemoryTransport();
  const client = createFlameClient(t);
  try {
    setContainer({ client: () => client });
    await loadPluginsForTest(defaultDataProviders);

    const fetcher = lookupDataProvider<T>(key);
    if (!fetcher) throw new Error(`no provider for "${key}"`);
    const pending = fetcher(params);
    const requests: Array<{ method: string; params: unknown }> = [];
    for (const [method, result] of responses) {
      const req = await waitForRequest(t, method);
      requests.push({ method: req.method, params: req.params });
      respondSuccess(t, req.id, result);
    }
    return { value: await pending, requests };
  } finally {
    await client.close();
  }
}

describe("defaultDataProviders — providers over JSON-RPC", () => {
  it("rejects missing parameters before a parameterized provider reaches RPC", async () => {
    const client = createFlameClient(createMemoryTransport());
    try {
      setContainer({ client: () => client });
      await loadPluginsForTest(defaultDataProviders);

      for (const key of [
        "diff",
        "skills",
        "skill-proposals",
        "approval-rules",
        "list-files",
        "read-file",
      ]) {
        const fetcher = lookupDataProvider(key);
        expect(fetcher).toBeDefined();
        await expect(fetcher!()).rejects.toThrow(`Data provider "${key}" requires parameters`);
      }
    } finally {
      await client.close();
    }
  });

  it("workspace catalogs bind the selected project and retain proposal decision scope", async () => {
    const { value: skills, requests: skillRequests } = await runProvider<{
      skills: Array<{ name: string; scope: string }>;
      diagnostics: [];
    }>(
      "skills",
      [
        [
          "skills.discovered.list",
          { skills: [{ name: "verify", scope: "project" }], diagnostics: [] },
        ],
      ],
      {
        cwd: "/work/alpha",
      },
    );
    expect(skillRequests[0]?.params).toEqual({ workspace: { path: "/work/alpha" } });
    expect(skills).toEqual({
      skills: [{ name: "verify", description: "", scope: "project" }],
      diagnostics: [],
    });

    const { value: proposals, requests: proposalRequests } = await runProvider<
      Array<{ workspace: string; name: string }>
    >(
      "skill-proposals",
      [
        [
          "skills.proposals.list",
          {
            data: [
              {
                name: "verify",
                revision: "a12dd3a7fd3203a452eb34d91a9be20569d5e337a3384347068895c07f3e0c5a",
                scope: "project",
                description: "Verify changes",
                instructions: "Run the checks.",
              },
            ],
          },
        ],
      ],
      { cwd: "/work/beta" },
    );
    expect(proposalRequests[0]?.params).toEqual({ workspace: { path: "/work/beta" } });
    expect(proposals[0]).toMatchObject({ workspace: "/work/beta", name: "verify" });
  });

  it("sessions: maps Page<Session>.data into AgentSessionSummary rows (updatedAt → time)", async () => {
    const { value: rows } = await runProvider<AgentSessionSummary[]>("sessions", [
      [
        "sessions.list",
        {
          data: [
            {
              id: "ses_1",
              revision: 7,
              title: "Refactor auth",
              status: "running",
              provider: "anthropic",
              model: "claude",
              workspace: {
                ref: { path: "/work/auth" },
                projectRoot: "/work/auth",
                availability: "available",
              },
              createdAt: "2026-06-01T00:00:00Z",
              updatedAt: "2026-06-01T01:00:00Z",
            },
          ],
        },
      ],
    ]);
    expect(rows).toEqual([
      {
        id: "ses_1",
        revision: 7,
        title: "Refactor auth",
        status: "running",
        provider: "anthropic",
        model: "claude",
        workspace: { path: "/work/auth", availability: "available" },
        time: "2026-06-01T01:00:00Z",
      },
    ]);
  });

  it("projects: maps WorkspaceSummary identity into workspace rows", async () => {
    const { value: rows } = await runProvider<WorkspaceProjectSummary[]>("projects", [
      [
        "workspaces.list",
        {
          data: [
            {
              workspace: {
                ref: { path: "/work/fern" },
                projectRoot: "/work/fern",
                availability: "available",
              },
              name: "fern-api",
              sessionCount: 3,
            },
          ],
        },
      ],
    ]);
    expect(rows).toEqual([
      {
        id: "/work/fern",
        name: "fern-api",
        sessionCount: 3,
      },
    ]);
  });

  it("files-changed: forwards cwd, maps statuses, keeps ± counts / binary honest", async () => {
    const { value: rows, requests } = await runProvider<WorkspaceFileChangeSummary[]>(
      "files-changed",
      [
        [
          "workspace.changes.list",
          {
            data: [
              { path: "src/a.ts", status: "modified", added: 3, removed: 1 },
              { path: "logo.png", status: "untracked", binary: true },
              { path: "new.png", previousPath: "old.png", status: "renamed", binary: true },
            ],
          },
        ],
      ],
      { cwd: "/work/auth" },
    );
    expect(requests[0]?.params).toEqual({ workspace: { path: "/work/auth" } });
    expect(rows).toEqual([
      { path: "src/a.ts", change: "mod", added: 3, removed: 1, binary: undefined },
      { path: "logo.png", change: "add", added: undefined, removed: undefined, binary: true },
      {
        path: "new.png",
        previousPath: "old.png",
        change: "renamed",
        added: undefined,
        removed: undefined,
        binary: true,
      },
    ]);
  });

  it("diff: pins format=rows on the wire and defaults files to []", async () => {
    const { value, requests } = await runProvider<WorkspaceDiff>(
      "diff",
      [["workspace.diff.get", { baseline: { type: "emptyTree" }, truncated: true }]],
      { cwd: "/work/auth", path: "src/a.ts", mode: "worktree" },
    );
    expect(requests[0]?.params).toEqual({
      path: "src/a.ts",
      mode: "worktree",
      format: "rows",
      workspace: { path: "/work/auth" },
    });
    expect(value).toEqual({ baseline: { type: "emptyTree" }, files: [], truncated: true });
  });

  it("read-file: preserves the requested and served source-line window", async () => {
    const { value, requests } = await runProvider<{
      content: string;
      startLine: number;
      totalLines: number;
      truncated?: boolean;
    }>(
      "read-file",
      [
        [
          "workspace.files.read",
          {
            path: "src/a.ts",
            content: "line 400",
            encoding: "utf-8",
            startLine: 400,
            endLine: 400,
            totalLines: 900,
            truncated: true,
          },
        ],
      ],
      { cwd: "/work/auth", path: "src/a.ts", startLine: 400, endLine: 800 },
    );
    expect(requests[0]?.params).toEqual({
      path: "src/a.ts",
      startLine: 400,
      endLine: 800,
      workspace: { path: "/work/auth" },
    });
    expect(value).toEqual({
      content: "line 400",
      startLine: 400,
      totalLines: 900,
      truncated: true,
    });
  });

  it("models: queries configured providers, including optional-auth endpoints", async () => {
    const { value, requests } = await runProvider<SelectableModel[]>("models", [
      [
        "providers.list",
        {
          data: [
            {
              id: "disabled",
              configured: false,
              credentialRequirement: "apiKeyRequired",
            },
            {
              id: "test-endpoint",
              configured: true,
              credentialRequirement: "apiKeyOptional",
            },
          ],
        },
      ],
      [
        "models.list",
        {
          data: [
            {
              id: "llama-test",
              provider: "test-endpoint",
              displayName: "Llama Test",
              tokenLimits: {
                contextWindow: 258_000,
                maxInputTokens: 250_000,
                maxOutputTokens: 32_000,
              },
              capabilities: {
                reasoning: true,
                reasoningLevels: ["low", "medium", "high"],
                reasoningDefaultLevel: "medium",
                multimodal: true,
                inputModalities: ["text", "image"],
                outputModalities: ["text"],
                toolUse: true,
                structuredOutput: true,
              },
            },
          ],
        },
      ],
    ]);

    expect(requests).toEqual([
      { method: "providers.list", params: {} },
      { method: "models.list", params: { provider: "test-endpoint" } },
    ]);
    expect(value).toHaveLength(1);
    expect(value[0]).toBeInstanceOf(SelectableModel);
    expect(value[0]).toMatchObject({
      id: "llama-test",
      provider: "test-endpoint",
      label: "Llama Test",
      tokenLimits: {
        contextWindow: 258_000,
        maxInputTokens: 250_000,
        maxOutputTokens: 32_000,
      },
      reasoning: true,
      reasoningLevels: ["low", "medium", "high"],
      reasoningDefaultLevel: "medium",
      inputModalities: ["text", "image"],
      outputModalities: ["text"],
      toolUse: true,
      structuredOutput: true,
    });
    expect(value[0]?.acceptsInput("image")).toBe(true);
    expect(value[0]?.reasoningLevelOrDefault("unsupported")).toBe("medium");
  });

  it("keeps one multi-stage provider read on its admitted client generation", async () => {
    const retiredTransport = createMemoryTransport();
    const retiredClient = createFlameClient(retiredTransport);
    const successorTransport = createMemoryTransport();
    const successorClient = createFlameClient(successorTransport);
    try {
      setContainer({ client: () => retiredClient });
      await loadPluginsForTest(defaultDataProviders);
      const fetcher = lookupDataProvider("models");
      if (!fetcher) throw new Error('no provider for "models"');

      const pending = fetcher();
      const providersRequest = await waitForRequest(retiredTransport, "providers.list");
      setContainer({ client: () => successorClient });
      respondSuccess(retiredTransport, providersRequest.id, {
        data: [
          {
            id: "openai",
            configured: true,
            credentialRequirement: "apiKeyRequired",
            credential: { masked: "sk****42", source: "stored" },
          },
        ],
      });

      await vi.waitFor(() => {
        expect(
          [...retiredTransport.outbox(), ...successorTransport.outbox()].some(
            ({ method }) => method === "models.list",
          ),
        ).toBe(true);
      });
      const retiredModelsRequest = retiredTransport
        .outbox()
        .find(({ method }) => method === "models.list");
      const successorModelsRequest = successorTransport
        .outbox()
        .find(({ method }) => method === "models.list");
      const actualTransport = retiredModelsRequest ? retiredTransport : successorTransport;
      const actualRequest = retiredModelsRequest ?? successorModelsRequest;
      if (!actualRequest) throw new Error("models.list was not requested");
      respondSuccess(actualTransport, actualRequest.id, { data: [] });
      await pending;

      expect(retiredModelsRequest).toBeDefined();
      expect(successorModelsRequest).toBeUndefined();

      const successorPending = fetcher();
      const successorProvidersRequest = await waitForRequest(successorTransport, "providers.list");
      respondSuccess(successorTransport, successorProvidersRequest.id, { data: [] });
      await successorPending;
      expect(
        retiredTransport.outbox().filter(({ method }) => method === "providers.list"),
      ).toHaveLength(1);
      expect(
        successorTransport.outbox().filter(({ method }) => method === "providers.list"),
      ).toHaveLength(1);
    } finally {
      await Promise.all([retiredClient.close(), successorClient.close()]);
    }
  });

  it("models: preserves a Runtime failure instead of presenting an empty catalog", async () => {
    const transport = createMemoryTransport();
    const client = createFlameClient(transport);
    setContainer({ client: () => client });
    await loadPluginsForTest(defaultDataProviders);
    const fetcher = lookupDataProvider("models");
    if (!fetcher) throw new Error('no provider for "models"');

    const pending = fetcher();
    const providersRequest = await waitForRequest(transport, "providers.list");
    respondSuccess(transport, providersRequest.id, {
      data: [
        {
          id: "openai",
          configured: true,
          credentialRequirement: "apiKeyRequired",
          credential: { masked: "sk****42", source: "stored" },
        },
      ],
    });
    const modelsRequest = await waitForRequest(transport, "models.list");
    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: modelsRequest.id,
      error: {
        code: -32603,
        message: "Internal error",
        data: { type: "internal_error" },
      },
    });

    await expect(pending).rejects.toMatchObject({ name: "RpcError" });
    await client.close();
  });
});
