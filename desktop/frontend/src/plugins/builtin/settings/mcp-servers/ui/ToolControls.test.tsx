import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";

const model = vi.hoisted(() => ({
  tools: {
    data: [
      { name: "read", description: "", modelName: "docs_read", nameConflicts: [] as string[] },
    ],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  },
  exposure: {
    data: [] as string[],
    isLoading: false,
    error: null as Error | null,
    refetch: vi.fn(),
  },
  rules: { data: [], isLoading: false, error: null as Error | null, refetch: vi.fn() },
  setExposure: vi.fn(),
  allow: vi.fn(),
}));

vi.mock("../application/mcpServerQueries", () => ({
  useMCPTools: () => model.tools,
  useMCPToolExposure: () => model.exposure,
}));
vi.mock("../application/mcpServerConfig", () => ({ setMCPToolExposure: model.setExposure }));
vi.mock("@/plugins/builtin/agent/public/approvalPolicy", () => ({
  useApprovalRules: () => model.rules,
  allowMCPTool: model.allow,
  forgetRules: vi.fn(),
}));

import { ToolControls } from "./ToolControls";

beforeEach(() => {
  vi.clearAllMocks();
  model.exposure.error = null;
  model.rules.error = null;
  model.tools.data[0]!.nameConflicts = [];
  model.setExposure.mockResolvedValue(undefined);
  model.allow.mockResolvedValue(undefined);
});

it("keeps exposure usable when approval rules fail, even with stale rule data", async () => {
  model.rules.error = new Error("rules unavailable");
  render(<ToolControls server="docs" />);
  const [exposure, approval] = screen.getAllByRole("switch");
  expect(exposure!.getAttribute("aria-disabled")).toBeNull();
  expect(approval!.getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(exposure!);
  await waitFor(() => expect(model.setExposure).toHaveBeenCalledWith("docs", "read", true));
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  expect(model.rules.refetch).toHaveBeenCalledOnce();
});

it("keeps approval usable when exposure fails", async () => {
  model.exposure.error = new Error("exposure unavailable");
  render(<ToolControls server="docs" />);
  const [exposure, approval] = screen.getAllByRole("switch");
  expect(exposure!.getAttribute("aria-disabled")).toBe("true");
  expect(approval!.getAttribute("aria-disabled")).toBeNull();
  fireEvent.click(approval!);
  await waitFor(() => expect(model.allow).toHaveBeenCalledWith("docs", "read"));
});

it("shows why a connected tool is excluded from model manifests", () => {
  model.tools.data[0]!.nameConflicts = ["mcp/docs.read/query"];
  render(<ToolControls server="docs" />);
  expect(
    screen.getByText("Hidden because docs_read conflicts with mcp/docs.read/query."),
  ).toBeTruthy();
});
