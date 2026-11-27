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
import { A2UI_VERSION, CATALOG_ID } from "../contracts/catalog";
import { limits, Presentation } from "../contracts/document";
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
      const discovery = await client.callTool({
        name: "ui_catalog",
        arguments: { kind: "catalog" },
      });
      expect(discovery.structuredContent).toMatchObject({
        catalog: {
          catalogId: CATALOG_ID,
          protocolVersion: A2UI_VERSION,
          components: {
            Dialog: {
              required: expect.arrayContaining(["component", "label", "child"]),
            },
          },
          functions: expect.objectContaining({
            formatString: expect.anything(),
          }),
        },
      });
      expect(discovery.structuredContent.catalog.functions).not.toHaveProperty(
        "openUrl",
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

      const initialResult = structuredClone(request);
      initialResult.messages[0].createSurface.dataModel.data.operations = {
        preview: { status: "pending" },
      };
      const withUpdate = (path, value) => ({
        ...request,
        messages: [
          ...request.messages,
          {
            version: A2UI_VERSION,
            updateDataModel: { surfaceId: "review", path, value },
          },
        ],
      });
      const reservedResultWrites = [
        initialResult,
        withUpdate("/data/operations/preview", { status: "pending" }),
        withUpdate("/data//operations/preview", { status: "pending" }),
        withUpdate("/data/", { title: "Updated", operations: {} }),
      ];
      const rejectedWrites = await Promise.all(
        reservedResultWrites.map((arguments_) =>
          client.callTool({ name: presentationTool, arguments: arguments_ }),
        ),
      );
      expect(rejectedWrites.map(({ isError }) => isError)).toEqual([
        true,
        true,
        true,
        true,
      ]);
      for (const rejected of rejectedWrites)
        expect(rejected.content[0].text).toMatch(/^invalid_presentation:/);

      const scalarData = structuredClone(request);
      scalarData.operations.push({
        id: "preview",
        installationId: "fixture-installation",
        contribution: "fixture",
        method: "inspect",
      });
      scalarData.messages[0].createSurface.dataModel.data = "A title";
      const blockedProjection = await client.callTool({
        name: presentationTool,
        arguments: scalarData,
      });
      expect(blockedProjection.isError).toBe(true);
      expect(blockedProjection.content[0].text).toContain(
        "/data requires an object for operation results",
      );

      const displayUpdate = withUpdate("/data/operationsSummary", "Updated");
      const updated = await client.callTool({
        name: presentationTool,
        arguments: displayUpdate,
      });
      expect(updated.structuredContent).toEqual({
        ...displayUpdate,
        contributions: [],
      });

      const businessRequest = interfaceRequest();
      const button = businessRequest.messages[0].createSurface.components.find(
        ({ id }) => id === "send",
      );
      const event = button.action.event;
      event.context.payload = {
        catalogId: "inventory",
        event: { name: "shipment.arrived" },
        component: "TextField",
        call: "fulfillment desk",
      };
      const businessPayload = await client.callTool({
        name: presentationTool,
        arguments: businessRequest,
      });
      expect(
        businessPayload.isError,
        businessPayload.content[0]?.text,
      ).not.toBe(true);
      expect(businessPayload.structuredContent).toEqual({
        ...businessRequest,
        contributions: [],
      });
      expect(
        Presentation.read(
          JSON.stringify(businessPayload.structuredContent),
        ).snapshot(),
      ).toEqual(businessPayload.structuredContent);
      event.name = "shipment.arrived";
      const undeclaredAction = await client.callTool({
        name: presentationTool,
        arguments: businessRequest,
      });
      expect(undeclaredAction.isError).toBe(true);
      expect(undeclaredAction.content[0].text).toContain(
        "action is not declared",
      );
      button.action = { functionCall: { call: "unregistered_function" } };
      const undeclaredFunction = await client.callTool({
        name: presentationTool,
        arguments: businessRequest,
      });
      expect(undeclaredFunction.isError).toBe(true);
      expect(undeclaredFunction.content[0].text).toContain(
        "renderer function is not admitted",
      );

      const data = request.messages[0].createSurface.dataModel.data;
      data.padding = "";
      const bytes = Buffer.byteLength(
        JSON.stringify({ ...request, contributions: [] }),
      );
      data.padding = "x".repeat(limits.bytes - bytes);
      const atCapacity = await client.callTool({
        name: presentationTool,
        arguments: request,
      });
      expect(atCapacity.isError).not.toBe(true);
      const recorded = JSON.stringify(atCapacity.structuredContent);
      expect(Buffer.byteLength(recorded)).toBe(limits.bytes);
      expect(Presentation.read(recorded).snapshot()).toEqual(
        atCapacity.structuredContent,
      );
      data.padding += "x";
      const overCapacity = await client.callTool({
        name: presentationTool,
        arguments: request,
      });
      expect(overCapacity.isError).toBe(true);
      expect(overCapacity.content[0].text).toMatch(
        /interface exceeds content capacity/,
      );
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
