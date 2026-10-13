import { describe, expect, it } from "vitest";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createFlameClient } from "@flame/runtime-contract/client";
import { createMemoryTransport } from "@flame/runtime-contract/client/transports/memory";
import {
  respondSuccess,
  waitForRequest,
} from "@flame/runtime-contract/client/transports/memory.testkit";
import type { WireMethodName } from "@flame/runtime-contract/methods";
import { contributeForTest } from "@/plugins/sdk/testKernel";
import type {
  WorkspaceFileChange as WorkspaceFileChangeSummary,
  WorkspaceProjectSummary,
} from "../application/workspaceQueries";
import { registerWorkspaceDataProviders } from "./runtimeDataProviders";

async function runProvider<T>(
  key: string,
  responses: Array<[method: WireMethodName, result: unknown]>,
  params?: unknown,
): Promise<{ value: T; requests: Array<{ method: string; params: unknown }> }> {
  const t = createMemoryTransport();
  const client = createFlameClient(t);
  try {
    await contributeForTest((ctx) => registerWorkspaceDataProviders(ctx, () => client));

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

describe("workspace Runtime data providers", () => {
  it("rejects missing parameters before a parameterized provider reaches RPC", async () => {
    const client = createFlameClient(createMemoryTransport());
    try {
      await contributeForTest((ctx) => registerWorkspaceDataProviders(ctx, () => client));

      for (const key of ["agent-memory", "list-files", "read-file"]) {
        const fetcher = lookupDataProvider(key);
        expect(fetcher).toBeDefined();
        await expect(fetcher!()).rejects.toThrow(`Data provider "${key}" requires parameters`);
      }
    } finally {
      await client.close();
    }
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
});
