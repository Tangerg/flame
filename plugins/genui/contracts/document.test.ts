import { expect, it } from "vitest";
import { MessageProcessor, STRICT_VALIDATION } from "@a2ui/web_core";
import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import { CATALOG_ID, catalog } from "./catalog";
import { InvalidPresentation, Presentation } from "./document";

import { interfaceRequest as sample } from "./fixture";
it("round-trips official messages and detaches every projection from the accepted result", () => {
  const request = sample();
  const accepted = Presentation.accept(request);
  request.title = "changed";
  const projected = accepted.snapshot();
  projected.title = "changed again";
  expect(accepted.snapshot().title).toBe("Release review");
  expect(Presentation.read(accepted.json()).snapshot()).toEqual(
    accepted.snapshot(),
  );
  expect(accepted.json()).toContain("9007199254740993");
  expect(parseJSON(accepted.json())).toBeTruthy();
});
it.each([
  { ...sample(), state: [], nodes: [], actions: [] },
  {
    ...sample(),
    messages: [
      {
        version: "0.9",
        createSurface: { surfaceId: "old", catalogId: CATALOG_ID },
      },
    ],
  },
  {
    ...sample(),
    messages: [
      {
        version: "1.0",
        createSurface: {
          surfaceId: "review",
          catalogId: CATALOG_ID,
          sendDataModel: true,
        },
      },
    ],
  },
  ...["/", "/draft", "/draft/note"].map((path) => ({
    ...sample(),
    messages: [
      ...sample().messages,
      {
        version: "1.0",
        updateDataModel: {
          surfaceId: "review",
          path,
          value: "remote overwrite",
        },
      },
    ],
  })),
  {
    ...sample(),
    messages: [
      {
        version: "1.0",
        callRendererFunction: {
          functionCallId: "rpc",
          callFunction: { "@call": "writeFile" },
        },
      },
    ],
  },
])(
  "refuses incompatible formats, remote draft replacement, full-model sharing and replay RPC",
  (request) => {
    expect(() => Presentation.accept(request)).toThrow();
  },
);
it("refuses undeclared functions and operation events before processing", () => {
  const request = sample();
  const create = request.messages[0]!.createSurface;
  for (const components of [
    [
      {
        id: "root",
        component: "Text",
        text: { "@call": "arbitraryTool", args: {} },
      },
    ],
    [
      {
        id: "root",
        component: "Button",
        child: "caption",
        action: { event: { name: "undeclared", context: {} } },
      },
      { id: "caption", component: "Text", text: "Run" },
    ],
    [
      {
        id: "root",
        component: "TextField",
        label: "Invalid",
        value: { "@path": "/data/title" },
      },
    ],
  ]) {
    expect(() =>
      Presentation.accept({
        ...request,
        messages: [
          { version: "1.0", createSurface: { ...create, components } },
        ],
      }),
    ).toThrow(InvalidPresentation);
  }
});
it("keeps locally edited draft data when display data changes", () => {
  const accepted = Presentation.accept(sample());
  const processor = new MessageProcessor([catalog], undefined, {
    version: "1.0",
    validationConfig: STRICT_VALIDATION,
  });
  try {
    processor.processMessages(accepted.messages());
    const surface = processor.getSurface("review")!;
    surface.dataModel.set("/draft/note", "typed locally");
    processor.processMessages({
      version: "1.0",
      updateDataModel: {
        surfaceId: "review",
        path: "/data/title",
        value: "Updated display",
      },
    });
    expect(surface.dataModel.get("/draft/note")).toBe("typed locally");
    expect(surface.dataModel.get("/data/title")).toBe("Updated display");
  } finally {
    processor.dispose();
  }
});
it("retains an old result for readable refusal without attempting conversion", () => {
  const old = stringifyJSON({
    title: "Old report",
    nodes: [],
    state: [],
    actions: [],
    operations: [],
    contributions: [],
  });
  expect(() => Presentation.read(old)).toThrow(/raw tool result/);
});
