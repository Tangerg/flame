import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { ApprovalRuleSummary } from "@/plugins/builtin/agent/public/approvalPolicy";

const model = vi.hoisted(() => ({ rules: [] as ApprovalRuleSummary[] }));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionId: () => undefined,
}));
vi.mock("../application/approvalConfig", () => ({
  useApprovalRuleConfigs: () => ({ data: model.rules, isLoading: false, error: null }),
  forgetApprovalRule: vi.fn(),
  forgetApprovalRules: vi.fn(),
}));

import { RulesRow } from "./RulesRow";

it("distinguishes a literal wildcard from a pattern and a whole-tool grant", () => {
  const subjects: ApprovalRuleSummary["subject"][] = [
    { type: "all" },
    { type: "exact", value: "echo *" },
    { type: "glob", value: "echo *" },
  ];
  model.rules = subjects.map((subject, index) => ({
    id: `rule_${index}`,
    scope: "global",
    tool: { type: "builtIn", name: "shell" },
    modelName: "shell",
    stale: false,
    subject,
    decision: "allow",
  }));
  render(<RulesRow />);
  expect(screen.getByText("· All arguments")).toBeTruthy();
  expect(screen.getByText("· Exact: echo *")).toBeTruthy();
  expect(screen.getByText("· Glob: echo *")).toBeTruthy();
});
