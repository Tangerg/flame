import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "@/lib/i18n/locales/en";
import { TOOL_FAMILIES, toolFamilyId } from "@/lib/toolFamilies";
import { ApprovalCard } from "./ApprovalCard";

const submission = vi.hoisted(() => ({
  submit: vi.fn(),
  pending: null as "approve" | "deny" | null,
}));

vi.mock("../../application/approvalArgsEditor", () => ({
  useApprovalArgsEditor: () => ({
    editing: false,
    argsText: "",
    invalid: false,
    setEditing: vi.fn(),
    setArgsText: vi.fn(),
  }),
}));

vi.mock("@/plugins/builtin/agent/public/hitl", () => ({
  useApprovalSubmit: () => ({
    pending: submission.pending,
    submit: submission.submit,
    registerActions: () => () => undefined,
  }),
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => true,
}));

describe("ApprovalCard actions", () => {
  afterEach(() => {
    vi.clearAllMocks();
    submission.pending = null;
  });

  it.each(["approve", "deny"] as const)(
    "does not turn a locally submitted %s into a settled Runtime fact",
    (decision) => {
      submission.pending = decision;
      render(
        <ApprovalCard
          resumeRunId="run-1"
          itemId="approval-1"
          toolName="shell"
          cmd="pwd"
          reason="Runs commands in the workspace."
        />,
      );
      expect(screen.queryByText("Approved", { exact: true })).toBeNull();
      expect(screen.queryByText("Declined", { exact: true })).toBeNull();
      expect(screen.getByRole<HTMLButtonElement>("button", { name: "Allow once" }).disabled).toBe(
        true,
      );
    },
  );

  it("orders the deny action before the primary approval action", () => {
    render(
      <ApprovalCard
        resumeRunId="run-1"
        itemId="approval-1"
        toolName="shell"
        cmd="npm test"
        reason="Run the test suite."
      />,
    );

    expect(screen.getAllByRole("button").map((button) => button.textContent)).toEqual([
      "Deny",
      "Allow once",
    ]);
  });

  it("uses the Codex request hierarchy without local danger chrome", () => {
    const { container } = render(
      <ApprovalCard
        resumeRunId="run-1"
        itemId="approval-1"
        toolName="shell"
        cmd="rm -rf node_modules && pnpm install"
        reason="Reinstall dependencies from the lockfile."
        rememberable
      />,
    );

    expect(container.querySelector('[data-slot="approval-surface"]')).toBeTruthy();
    expect(screen.getByText("Shell", { exact: true })).toBeTruthy();
    expect(
      screen.getByText("Reinstall dependencies from the lockfile.", { exact: true }),
    ).toBeTruthy();
    expect(screen.queryByText("Approval required", { exact: true })).toBeNull();
    expect(screen.queryByText(/Potentially destructive/)).toBeNull();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("keeps remembered approval scopes behind the primary split action", () => {
    render(
      <ApprovalCard
        resumeRunId="run-1"
        itemId="approval-1"
        toolName="shell"
        cmd="npm test"
        reason="Run the test suite."
        rememberable
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Approval options" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Allow for this session" }));
    expect(submission.submit).toHaveBeenCalledWith("approve", { rememberScope: "session" });
  });

  it.each(TOOL_FAMILIES.flatMap((family) => family.tools.map((tool) => tool.name)))(
    "names a family rather than the wire name when approving %s",
    (name) => {
      render(
        <ApprovalCard
          resumeRunId="run-1"
          itemId="approval-1"
          toolName={name}
          cmd=""
          reason="Do the thing."
        />,
      );
      const family = en[`tools.family.${toolFamilyId(name)}`];
      expect(family).toBeDefined();
      expect(screen.getAllByText(family!, { exact: true }).length).toBeGreaterThan(0);
      expect(screen.queryByText(name, { exact: true })).toBeNull();
    },
  );
});
