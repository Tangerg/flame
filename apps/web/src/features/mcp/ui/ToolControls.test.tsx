import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { userMCPServer } from "@/features/mcp/application/mcpServerQueries";
import { ToolControls } from "@/features/mcp/ui/ToolControls";
const model = vi.hoisted(() => ({
  tools: {
    data: [
      { name: "read", description: "", modelName: "docs_read", nameConflicts: [] as string[] },
    ],
    isLoading: false,
    error: null as Error | null,
    refetch: vi.fn(),
  },
  exposure: {
    data: [] as string[],
    isLoading: false,
    error: null as Error | null,
    refetch: vi.fn(),
  },
  rules: {
    data: [] as unknown[],
    isLoading: false,
    error: null as Error | null,
    refetch: vi.fn(),
  },
  setExposure: vi.fn(),
  allow: vi.fn(),
  forget: vi.fn(),
}));

vi.mock("@/features/mcp/application/mcpServerQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/mcp/application/mcpServerQueries")>()),
  useMCPTools: () => model.tools,
  useMCPToolExposure: () => model.exposure,
}));

vi.mock("@/features/mcp/application/mcpServerConfig", () => ({
  setMCPToolExposure: model.setExposure,
}));

vi.mock("@/features/agent/public/approvalPolicy", () => ({ useApprovalRules: () => model.rules }));
prepareTestClient(() => {
  Object.assign(testClient.agent.access, { allowMCPTool: model.allow, forgetRules: model.forget });
});

const docs = userMCPServer("docs");

const retryFor = (title: string) =>
  within(screen.getByText(title).parentElement!).getByRole("button", { name: "Retry" });

const expectNoWrites = () => {
  expect(model.setExposure).not.toHaveBeenCalled();
  expect(model.allow).not.toHaveBeenCalled();
  expect(model.forget).not.toHaveBeenCalled();
};

beforeEach(() => {
  vi.clearAllMocks();
  model.tools.error = null;
  model.tools.data[0]!.description = "";
  model.exposure.error = null;
  model.exposure.data = [];
  model.rules.error = null;
  model.rules.data = [];
  model.tools.data[0]!.nameConflicts = [];
  model.setExposure.mockResolvedValue(undefined);
  model.allow.mockResolvedValue(undefined);
  model.forget.mockResolvedValue(undefined);
});

it("keeps exposure usable when approval rules fail, even with stale rule data", async () => {
  model.rules.error = new Error("rules unavailable");
  testClient.render(<ToolControls server={docs} />);
  const [exposure, approval] = screen.getAllByRole("switch");
  expect(exposure!.getAttribute("aria-disabled")).toBeNull();
  expect(approval!.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(exposure!);
  await waitFor(() => expect(model.setExposure).toHaveBeenCalledWith(docs, "read", true));
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(model.rules.refetch).toHaveBeenCalledOnce();
});

it("keeps approval usable when exposure fails", async () => {
  model.exposure.error = new Error("exposure unavailable");
  testClient.render(<ToolControls server={docs} />);
  const [exposure, approval] = screen.getAllByRole("switch");
  expect(exposure!.getAttribute("aria-disabled")).toBe("true");
  expect(approval!.getAttribute("aria-disabled")).toBeNull();
  fireEvent.click(approval!);
  await waitFor(() => expect(model.allow).toHaveBeenCalledWith(docs, "read"));
});

it("shows why a connected tool is excluded from model manifests", () => {
  model.tools.data[0]!.nameConflicts = ["mcp/docs.read/query"];
  testClient.render(<ToolControls server={docs} />);
  expect(
    screen.getByText("Hidden because docs_read conflicts with mcp/docs.read/query."),
  ).toBeTruthy();
});

it("reads standing approval only from the same server origin", () => {
  const rule = (server: typeof docs) => ({
    id: `rule-${server.origin.type}`,
    scope: "global",
    tool: { type: "mcp", server, name: "read" },
    modelName: "docs_read",
    stale: false,
    subject: { type: "all" },
    decision: "allow",
  });
  model.rules.data = [
    rule({
      origin: { type: "installation", installationId: "eeb329cd-c7ce-40c9-bd90-6821fef06d30" },
      name: "docs",
    }),
  ];
  const { unmount } = testClient.render(<ToolControls server={docs} />);
  expect(screen.getAllByRole("switch")[1]!.getAttribute("aria-checked")).toBe("false");
  unmount();
  model.rules.data = [rule(docs)];
  testClient.render(<ToolControls server={docs} />);
  expect(screen.getAllByRole("switch")[1]!.getAttribute("aria-checked")).toBe("true");
});

it("names each failed auxiliary read and retries only its own owner", () => {
  model.tools.data[0]!.description = "Reads documentation pages.";
  model.exposure.error = new Error("exposure unavailable");
  model.rules.error = new Error("rules unavailable");
  testClient.render(<ToolControls server={docs} />);
  expect(screen.getByText("Couldn't load tool visibility")).toBeTruthy();
  expect(screen.getByText("Couldn't load auto-approve rules")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "Retry" })).toHaveLength(2);
  expect(screen.getByText("read")).toBeTruthy();
  expect(screen.getByText("Reads documentation pages.")).toBeTruthy();

  fireEvent.click(retryFor("Couldn't load tool visibility"));
  expect(model.exposure.refetch).toHaveBeenCalledOnce();
  expect(model.rules.refetch).not.toHaveBeenCalled();
  expect(model.tools.refetch).not.toHaveBeenCalled();

  fireEvent.click(retryFor("Couldn't load auto-approve rules"));
  expect(model.exposure.refetch).toHaveBeenCalledOnce();
  expect(model.rules.refetch).toHaveBeenCalledOnce();
  expect(model.tools.refetch).not.toHaveBeenCalled();
  expectNoWrites();
});

it("names a failed tools read and retries only the tools list", () => {
  model.tools.error = new Error("tools unavailable");
  testClient.render(<ToolControls server={docs} />);
  expect(screen.queryByText("Couldn't load tool visibility")).toBeNull();
  expect(screen.queryByText("Couldn't load auto-approve rules")).toBeNull();
  expect(screen.queryAllByRole("switch")).toHaveLength(0);

  fireEvent.click(retryFor("Couldn't load tools"));
  expect(model.tools.refetch).toHaveBeenCalledOnce();
  expect(model.exposure.refetch).not.toHaveBeenCalled();
  expect(model.rules.refetch).not.toHaveBeenCalled();
  expectNoWrites();
});

it("retires only the visibility failure once visibility is accepted", () => {
  model.exposure.error = new Error("exposure unavailable");
  model.rules.error = new Error("rules unavailable");
  const { rerender } = testClient.render(<ToolControls server={docs} />);
  expect(screen.getByText("Couldn't load tool visibility")).toBeTruthy();

  model.exposure.error = null;
  model.exposure.data = ["read"];
  rerender(<ToolControls server={docs} />);
  expect(screen.queryByText("Couldn't load tool visibility")).toBeNull();
  expect(screen.getAllByRole("button", { name: "Retry" })).toHaveLength(1);
  expect(retryFor("Couldn't load auto-approve rules")).toBeTruthy();

  const [exposure, approval] = screen.getAllByRole("switch");
  expect(exposure!.getAttribute("aria-checked")).toBe("false");
  expect(exposure!.getAttribute("aria-disabled")).toBeNull();
  expect(approval!.getAttribute("aria-disabled")).toBe("true");
  expect(model.exposure.refetch).not.toHaveBeenCalled();
  expect(model.rules.refetch).not.toHaveBeenCalled();
  expectNoWrites();
});
