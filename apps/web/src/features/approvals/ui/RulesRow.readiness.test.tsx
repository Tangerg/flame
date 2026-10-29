import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { onlineManager } from "@tanstack/react-query";
import { act, cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { contributeForTest } from "@/test/kernel";
import { queryClient } from "@/lib/queryClient";
import {
  APPROVAL_RULES_KEY,
  type ApprovalRuleSummary,
} from "@/features/agent/public/approvalPolicy";
import { RulesRow } from "@/features/approvals/ui/RulesRow";

afterEach(() => {
  cleanup();
  queryClient.clear();
});

const rule: ApprovalRuleSummary = {
  id: "rule_01",
  scope: "global",
  decision: "allow",
  tool: { type: "builtIn", name: "shell" },
  subject: { type: "exact", value: "echo *" },
  modelName: "shell",
  stale: false,
};
const cases = [
  { name: "empty", rules: [] as ApprovalRuleSummary[], visibleText: "No rules yet" },
  { name: "nonempty", rules: [rule], visibleText: "Exact: echo *" },
];
function rulesQuery() {
  return [APPROVAL_RULES_KEY, { sessionId: testClient.agent.access.getActiveSessionId() }];
}

async function mount(rules: ApprovalRuleSummary[]) {
  const list = vi.fn(async () => rules);
  const forget = vi.fn();
  const forgetMany = vi.fn();
  Object.assign(testClient.agent.access, { forgetRule: forget, forgetRules: forgetMany });
  await contributeForTest((ctx) =>
    ctx.contribute(DATA_PROVIDER, { key: APPROVAL_RULES_KEY, fetcher: list }),
  );
  testClient.render(
    <>
      <RulesRow />
      <textarea aria-label="Later draft" defaultValue="Keep this rules draft." />
    </>,
  );
  return { list, forget, forgetMany };
}

function pauseQueries() {
  const wasOnline = onlineManager.isOnline();
  onlineManager.setOnline(false);
  finishTestClient(() => onlineManager.setOnline(wasOnline));
}

it.each(cases)("waits for the first paused $name rules result", async ({ rules, visibleText }) => {
  pauseQueries();
  const { list, forget, forgetMany } = await mount(rules);
  await waitFor(() =>
    expect(queryClient.getQueryState(rulesQuery())).toMatchObject({
      status: "pending",
      fetchStatus: "paused",
      data: undefined,
    }),
  );
  expect(list).not.toHaveBeenCalled();
  expect(screen.getByRole("status").textContent).toBe("Loading");
  expect(screen.queryByText("No rules yet")).toBeNull();
  expect(screen.queryByRole("button", { name: "Clear all" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Forget shell" })).toBeNull();
  const draft = screen.getByRole("textbox", { name: "Later draft" });
  act(() => draft.focus());

  await act(async () => onlineManager.setOnline(true));
  expect(await screen.findByText(visibleText)).toBeTruthy();
  expect(document.activeElement).toBe(draft);
  expect(draft).toHaveProperty("value", "Keep this rules draft.");
  expect(forget).not.toHaveBeenCalled();
  expect(forgetMany).not.toHaveBeenCalled();
});

it.each(cases)(
  "keeps accepted $name rules during a paused reread",
  async ({ rules, visibleText }) => {
    const { list } = await mount(rules);
    await screen.findByText(visibleText);
    list.mockClear();
    pauseQueries();
    act(() => void queryClient.refetchQueries({ queryKey: rulesQuery() }));
    await waitFor(() =>
      expect(queryClient.getQueryState(rulesQuery())).toMatchObject({
        status: "success",
        fetchStatus: "paused",
        data: rules,
      }),
    );
    expect(list).not.toHaveBeenCalled();
    expect(screen.getByText(visibleText)).toBeTruthy();
  },
);
