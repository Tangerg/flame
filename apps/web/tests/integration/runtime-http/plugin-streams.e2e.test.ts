import { execFile } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { promisify } from "node:util";
import { describe, expect, it } from "vitest";
import { INSTALLATION_SCOPE } from "@flame/runtime-client";
import { RPC_HTTP } from "@flame/runtime-client/wire";
import { useRuntimeFixture } from "./fixture";
import { waitUntilReady, within } from "./observation";

const exec = promisify(execFile);

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: plugin streams", () => {
  const fixture = useRuntimeFixture("plugin-streams");
  it("closes a plugin stream after its result or consumer failure", async () => {
    const packagePath = join(fixture.environmentRoot, "stream-probe-package");
    const extension = "io.github.tangerg.flame";
    await mkdir(join(packagePath, extension), { recursive: true });
    const executable = join(packagePath, "stream");
    const runtime = resolve(import.meta.dirname, "../../../../../runtime");
    await exec("go", ["build", "-o", executable, "./internal/bootstrap/testdata/stream-query"], {
      cwd: runtime,
    });
    const { stdout } = await exec(executable, ["describe"]);
    const contract = stdout.trim();
    const declaration = {
      protocol: "io.github.tangerg.flame/plugin/1",
      contributions: [
        { id: "stream", server: "stream", export: "stream", contract: extension + "/stream.json" },
      ],
    };
    const manifest = {
      $schema: "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
      name: "example.stream-probe",
      version: "1.0.0",
      extensions: { [extension]: { declaration: extension + "/declaration.json" } },
    };
    const servers = {
      $schema: "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
      mcpServers: { stream: { type: "stdio", command: "./stream" } },
    };
    await Promise.all([
      writeFile(join(packagePath, "plugin.json"), JSON.stringify(manifest)),
      writeFile(join(packagePath, "mcp.json"), JSON.stringify(servers)),
      writeFile(join(packagePath, extension, "declaration.json"), JSON.stringify(declaration)),
      writeFile(join(packagePath, extension, "stream.json"), contract),
    ]);
    await exec(
      "go",
      [
        "run",
        "./cmd/pluginpack",
        "-source",
        packagePath,
        "-os",
        process.platform,
        "-arch",
        process.arch === "x64" ? "amd64" : process.arch,
      ],
      { cwd: runtime },
    );
    const installation = await fixture.client.plugins.install({ source: packagePath });
    const reader = fixture.createRuntimeClient();
    try {
      await fixture.client.plugins.setEnablement({
        installationId: installation.installationId,
        enabled: true,
      });
      await waitUntilReady(fixture.baseUrl, () => fixture.processOutput);
      const request = {
        route: {
          ...installation,
          contribution: "stream",
          contract,
          method: "example.records/read.v1",
          target: { scope: INSTALLATION_SCOPE, reference: installation.installationId },
        },
        input: "{}",
      };
      const accept = { Accept: `${RPC_HTTP.documentMediaType}, ${RPC_HTTP.streamMediaType}` };
      const flat = await fixture.postRPC(
        "plugins.stream",
        { ...request.route, input: request.input },
        accept,
      );
      await expect(flat.json()).resolves.toMatchObject({
        error: { data: { problem: { type: "invalid_params" } } },
      });
      const invalidRoute = await fixture.postRPC(
        "plugins.stream",
        {
          ...request,
          route: { ...request.route, target: { ...request.route.target, scope: "" } },
        },
        accept,
      );
      await expect(invalidRoute.json()).resolves.toMatchObject({
        error: {
          data: {
            problem: {
              type: "invalid_params",
              errors: expect.arrayContaining([
                expect.objectContaining({ field: "route.target.scope" }),
              ]),
            },
          },
        },
      });
      const order: unknown[] = [];
      const release = Promise.withResolvers<void>();
      const firstArrived = Promise.withResolvers<void>();
      const outcome = reader.plugins
        .queryStream(
          request,
          async (chunk) => {
            order.push(chunk);
            if (chunk === "first") {
              firstArrived.resolve();
              await release.promise;
            }
          },
          (value) => value,
        )
        .then(
          (result) => ({ result }),
          (error: unknown) => {
            firstArrived.reject(error);
            return { error };
          },
        );
      try {
        await within(firstArrived.promise, "the installed plugin's first chunk");
        expect(order).toEqual(["first"]);
      } finally {
        release.resolve();
      }
      await expect(outcome).resolves.toEqual({ result: true });
      expect(order).toEqual(["first", "second"]);
      const refusal = new Error("consumer refused a benign stream chunk");
      await expect(
        reader.plugins.queryStream(
          request,
          () => {
            throw refusal;
          },
          (value) => value,
        ),
      ).rejects.toBe(refusal);
    } finally {
      try {
        await reader.close();
      } finally {
        await fixture.client.plugins.uninstall(installation.installationId);
      }
    }
  }, 30_000);
});
