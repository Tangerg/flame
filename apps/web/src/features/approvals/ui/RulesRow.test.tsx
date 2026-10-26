import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ApprovalRuleSummary } from "@/features/agent/public/approvalPolicy";
import { forgetApprovalRule } from "@/features/approvals/application/approvalConfig";
import { RulesRow } from "@/features/approvals/ui/RulesRow";
const model = vi.hoisted(() => ({ rules: [] as ApprovalRuleSummary[] }));

vi.mock("@/features/agent/public/session", () => ({
  useActiveSessionId: () => undefined,
}));

vi.mock("@/features/approvals/application/approvalConfig", () => ({
  useApprovalRuleConfigs: () => ({ data: model.rules, isLoading: false, error: null }),
  forgetApprovalRule: vi.fn(() => Promise.resolve()),
  forgetApprovalRules: vi.fn(() => Promise.resolve()),
}));

afterEach(cleanup);

const sameRule: Omit<ApprovalRuleSummary, "id" | "scope" | "decision" | "tool" | "subject"> = {
  modelName: "shell",
  stale: false,
};

const rules: ApprovalRuleSummary[] = [
  {
    ...sameRule,
    id: "rule/global:1",
    scope: "global",
    decision: "allow",
    tool: { type: "builtIn", name: "shell" },
    subject: { type: "all" },
  },
  {
    ...sameRule,
    id: "rule/project:2",
    scope: "project",
    decision: "deny",
    tool: { type: "a2a", endpoint: "reviewer" },
    subject: { type: "exact", value: "rm -rf build" },
    dir: "/work/flame",
  },
  {
    ...sameRule,
    id: "rule/session:3",
    scope: "session",
    decision: "allow",
    tool: { type: "builtIn", name: "shell" },
    subject: { type: "glob", value: "git *" },
    stale: true,
  },
  {
    ...sameRule,
    id: "rule/project:4",
    scope: "project",
    decision: "allow",
    tool: { type: "builtIn", name: "shell" },
    subject: { type: "glob", value: "git *" },
    dir: "/work/other",
  },
];

const descriptions = new Map([
  ["rule/global:1", /^Global\s*allow\s*shell\s*Built-in\s*All arguments$/],
  [
    "rule/project:2",
    /^Project\s*deny\s*shell\s*a2a\/reviewer\s*Exact: rm -rf build\s*\/work\/flame$/,
  ],
  ["rule/session:3", /^Session\s*allow\s*shell\s*Source changed\s*Built-in\s*Glob: git \*$/],
  ["rule/project:4", /^Project\s*allow\s*shell\s*Built-in\s*Glob: git \*\s*\/work\/other$/],
]);

function forgetButton(ruleId: string) {
  return screen.getByRole("button", {
    name: "Forget shell",
    description: descriptions.get(ruleId),
  });
}

it("describes each identically named forget action by its own visible rule", async () => {
  model.rules = rules;
  const { rerender } = testClient.render(<RulesRow />);
  for (const rule of rules) expect(forgetButton(rule.id)).toBeTruthy();

  model.rules = [...rules].reverse();
  rerender(<RulesRow />);
  for (const rule of rules) expect(forgetButton(rule.id)).toBeTruthy();

  vi.mocked(forgetApprovalRule).mockClear();
  const target = forgetButton("rule/project:2");
  await act(async () => {
    fireEvent.click(target);
  });
  expect(forgetApprovalRule).toHaveBeenCalledTimes(1);
  expect(forgetApprovalRule).toHaveBeenCalledWith(testClient.agent.access, "rule/project:2");
});

it("keeps the rule description while the forget tooltip is open", async () => {
  model.rules = rules;
  testClient.render(<RulesRow />);
  const button = forgetButton("rule/session:3");
  act(() => button.focus());
  expect(await screen.findByRole("tooltip")).toBeTruthy();
  expect(forgetButton("rule/session:3")).toBe(button);
});

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
  testClient.render(<RulesRow />);
  expect(screen.getByText("All arguments")).toBeTruthy();
  expect(screen.getByText("Exact: echo *")).toBeTruthy();
  expect(screen.getByText("Glob: echo *")).toBeTruthy();
});
