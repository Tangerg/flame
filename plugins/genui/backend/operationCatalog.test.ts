import { expect, it } from "vitest";
import type { PluginInstallation } from "@flame/runtime-client/wire";
import { stringifyJSON } from "@flame/runtime-client";
import { Capability, PROTOCOL } from "@flame/backend-sdk";
import { OperationCatalog } from "./operationCatalog";
import type { OperationRequest } from "../contracts/document";

const capability = new Capability({
  protocol: PROTOCOL,
  reference: "test.reports.v1",
  composition: "collection",
  scope: "installation",
  methods: [
    {
      name: "test.reports/capture.v1",
      kind: "command",
      target: "installation",
      description: "Capture a report.",
      input: {
        type: "object",
        properties: { period: { enum: ["daily", "weekly"] } },
        required: ["period"],
        additionalProperties: false,
      },
      output: { type: "object" },
    },
  ],
}).descriptor;

function installation(): PluginInstallation {
  return {
    id: "installed-usage",
    enabled: true,
    presentation: "admitted",
    realization: { type: "available" },
    disabledServers: [],
    disabledSkills: [],
    inputStates: {},
    source: "test",
    selected: {
      name: "flame.usage-reports",
      digest: "selected-release",
      version: "1.0.0",
      components: [],
      diagnostics: [],
      inputs: [],
      points: [],
      pointValues: [],
      servers: [],
      skills: [],
      themes: [],
      contributions: [
        {
          id: "reports",
          contract: stringifyJSON(capability),
          export: "Reports",
          server: "reports",
          requires: [],
        },
      ],
    },
  };
}

const operation: OperationRequest = {
  id: "capture",
  installationId: "installed-usage",
  contribution: "reports",
  method: "test.reports/capture.v1",
  input: { object: { period: { literal: "daily" } } },
};

it("captures the selected installed contract and derives its installation target", () => {
  const installed = installation();
  const catalog = new OperationCatalog([installed]);
  installed.selected.digest = "later-selection";
  const [captured] = catalog.capture([operation]);
  expect(captured).toEqual({
    id: "capture",
    binding: {
      installationId: "installed-usage",
      contribution: "reports",
      digest: "selected-release",
      contract: stringifyJSON(capability),
      method: "test.reports/capture.v1",
      target: { scope: "installation", reference: "installed-usage" },
    },
  });
  expect(catalog.describe()[0]?.methods).toContainEqual({
    name: operation.method,
    kind: "command",
    target: "installation",
    streaming: false,
  });
});

it.each([
  (value: PluginInstallation) => {
    value.enabled = false;
  },
  (value: PluginInstallation) => {
    value.presentation = "withheld";
  },
  (value: PluginInstallation) => {
    value.realization = { type: "releaseUnavailable" };
  },
  (value: PluginInstallation) => {
    value.disabledServers = ["reports"];
  },
  (value: PluginInstallation) => {
    value.realization = { type: "available", unavailableBackends: ["reports"] };
  },
])(
  "does not bind a contribution which cannot currently be invoked",
  (change) => {
    const installed = installation();
    change(installed);
    const catalog = new OperationCatalog([installed]);
    expect(catalog.describe()).toEqual([]);
    expect(() => catalog.capture([operation])).toThrow(
      "available admitted contribution",
    );
  },
);

it("does not accept authored authority, alternate targets or missing methods", () => {
  const catalog = new OperationCatalog([installation()]);
  expect(() =>
    catalog.capture([{ ...operation, method: "runs.start" }]),
  ).toThrow("admitted method");
  expect(() =>
    catalog.capture([
      {
        ...operation,
        target: { scope: "installation", reference: "someone-else" },
      },
    ]),
  ).toThrow("target differs");
  expect(() =>
    catalog.capture([{ ...operation, contribution: "missing" }]),
  ).toThrow("available admitted contribution");
  expect(catalog.describe("someone-else")).toEqual([]);
});

it("never chooses an arbitrary duplicate binding", () => {
  expect(() =>
    new OperationCatalog([installation(), installation()]).capture([operation]),
  ).toThrow("one available admitted contribution");
});
