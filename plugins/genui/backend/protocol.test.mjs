import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { buildBackend } from "@flame/backend-sdk/build";
import protocol from "@flame/backend-sdk/protocol" with { type: "json" };
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import { rolldown } from "rolldown";
import { expect, it } from "vitest";
import { interfaceRequest } from "../contracts/fixture";

const presentationTool = "ui_present";
const backendTestTimeout = 60_000;

it(
  "publishes bounded presentation routes and returns the accepted document",
  async () => {
    const directory = await mkdtemp(join(tmpdir(), "flame-genui-backend-"));
    const client = new Client({ name: "genui-contract-test", version: "1" });
    try {
      await buildBackend({
        target: protocol.constants.nodeTarget,
        maximumFrameBytes: protocol.constants.maximumFrameBytes,
        input: fileURLToPath(new URL("./start.ts", import.meta.url)),
        directory,
        compiler: rolldown,
      });
      await client.connect(
        new StdioClientTransport({
          command: process.execPath,
          args: [join(directory, "start.mjs")],
          env: { PLUGIN_DATA: directory, PLUGIN_ROOT: directory },
          stderr: "inherit",
        }),
      );
      const { tools } = await client.listTools();
      const tool = tools.find((entry) => entry.name === presentationTool);
      expect(tool?.inputSchema.properties?.operations.items).toMatchObject({
        properties: {
          id: { minLength: 1, maxLength: 1024 },
          target: {
            properties: { reference: { minLength: 1, maxLength: 1024 } },
          },
        },
      });
      expect(tool?.outputSchema?.properties?.contributions.items).toMatchObject(
        {
          required: expect.arrayContaining([
            "contract",
            "contribution",
            "digest",
            "installationId",
          ]),
          additionalProperties: false,
        },
      );
      const request = interfaceRequest();
      await expect(
        client.callTool({
          name: presentationTool,
          arguments: {
            ...request,
            operations: [
              {
                id: "",
                installationId: "fixture-installation",
                contribution: "fixture",
                method: "inspect",
              },
            ],
          },
        }),
      ).rejects.toThrow();
      const refusal = await client.callTool({
        name: presentationTool,
        arguments: {
          ...request,
          messages: [{ version: request.messages[0].version }],
        },
      });
      expect(refusal.isError).toBe(true);
      expect(refusal.content[0].text).toMatch(/^invalid_presentation:/);
      const result = await client.callTool({
        name: presentationTool,
        arguments: request,
      });
      expect(result.isError).not.toBe(true);
      expect(result.structuredContent).toEqual({
        ...request,
        contributions: [],
      });
    } finally {
      try {
        await client.close();
      } finally {
        await rm(directory, { recursive: true, force: true });
      }
    }
  },
  backendTestTimeout,
);
