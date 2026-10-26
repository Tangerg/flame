import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type ApprovalRuleSummary } from "@/features/agent/public/approvalPolicy";
import { forgetApprovalRule } from "@/features/approvals/application/approvalConfig";
import { RulesRow } from "@/features/approvals/ui/RulesRow";
import { ContentVisibility } from "@flame/ui";
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

it("continues at the next rule when the focused record is removed", () => {
  model.rules = [...rules];
  const { rerender } = testClient.render(<RulesRow />);
  act(() => forgetButton("rule/project:2").focus());

  model.rules = rules.filter((rule) => rule.id !== "rule/project:2");
  rerender(<RulesRow />);

  expect(document.activeElement).toBe(forgetButton("rule/session:3"));
});

it("continues at the preceding rule when the focused last record is removed", () => {
  model.rules = [...rules];
  const { rerender } = testClient.render(<RulesRow />);
  act(() => forgetButton("rule/project:4").focus());

  model.rules = rules.slice(0, -1);
  rerender(<RulesRow />);

  expect(document.activeElement).toBe(forgetButton("rule/session:3"));
});

it("uses the current order without moving focus when records reorder", () => {
  model.rules = [...rules];
  const { rerender } = testClient.render(<RulesRow />);
  const held = forgetButton("rule/project:2");
  act(() => held.focus());
  model.rules = [...rules].reverse();
  rerender(<RulesRow />);
  expect(document.activeElement).toBe(held);

  model.rules = model.rules.filter((rule) => rule.id !== "rule/project:2");
  rerender(<RulesRow />);

  expect(document.activeElement).toBe(forgetButton("rule/global:1"));
});

it("keeps the empty rules context reachable after its last action disappears", () => {
  model.rules = [rules[0]!];
  const { rerender } = testClient.render(<RulesRow />);
  act(() => forgetButton("rule/global:1").focus());

  model.rules = [];
  rerender(<RulesRow />);

  expect(document.activeElement).toBe(screen.getByRole("group", { name: "Remembered rules" }));
  expect(screen.getByText("No rules yet")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Clear all" })).toBeNull();
});

it("keeps Clear all through partial removal and lands in its empty context", () => {
  model.rules = [...rules];
  const { rerender } = testClient.render(<RulesRow />);
  const clear = screen.getByRole("button", { name: "Clear all" });
  act(() => clear.focus());
  model.rules = rules.slice(2);
  rerender(<RulesRow />);
  expect(document.activeElement).toBe(clear);

  model.rules = [];
  rerender(<RulesRow />);

  expect(document.activeElement).toBe(screen.getByRole("group", { name: "Remembered rules" }));
});

it("preserves a later outside destination when a rule disappears", () => {
  model.rules = [...rules];
  const scene = () => (
    <>
      <button>Later control</button>
      <RulesRow />
    </>
  );
  const { rerender } = testClient.render(scene());
  act(() => forgetButton("rule/project:2").focus());
  const later = screen.getByRole("button", { name: "Later control" });
  act(() => later.focus());
  model.rules = rules.filter((rule) => rule.id !== "rule/project:2");
  rerender(scene());

  expect(document.activeElement).toBe(later);
});

it("does not focus a retained hidden rules list after removal", () => {
  model.rules = [...rules];
  const scene = (visible: boolean) => (
    <ContentVisibility visible={visible}>
      <RulesRow />
    </ContentVisibility>
  );
  const { rerender } = testClient.render(scene(true));
  const held = forgetButton("rule/project:2");
  act(() => held.focus());
  rerender(scene(false));
  model.rules = rules.filter((rule) => rule.id !== "rule/project:2");
  rerender(scene(false));

  expect(document.activeElement).not.toBe(forgetButton("rule/session:3"));
  expect(document.activeElement).not.toBe(
    screen.queryByRole("group", { name: "Remembered rules" }),
  );
});

it("keeps the original action focused when its command fails", async () => {
  model.rules = [...rules];
  vi.mocked(forgetApprovalRule).mockRejectedValueOnce(new Error("Rule update rejected"));
  testClient.render(<RulesRow />);
  const held = forgetButton("rule/project:2");
  act(() => held.focus());
  fireEvent.click(held);

  await waitFor(() => expect(held.getAttribute("aria-busy")).toBe("false"));
  expect(document.activeElement).toBe(held);
});
