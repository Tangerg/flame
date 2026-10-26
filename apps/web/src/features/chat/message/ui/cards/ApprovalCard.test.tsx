import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "@/lib/i18n/locales/en";
import { TOOL_FAMILIES, toolFamilyId } from "@/lib/toolFamilies";
import { ApprovalCard } from "@/features/chat/message/ui/cards/ApprovalCard";
const submission = vi.hoisted(() => ({
  submit: vi.fn(),
  pending: null as "approve" | "deny" | null,
  submitted: false,
}));

const service = vi.hoisted(() => ({ available: true }));

vi.mock("@/features/agent/public/hitl", () => ({
  useApprovalSubmit: () => ({
    pending: submission.pending,
    submitted: submission.submitted,
    submit: submission.submit,
    registerActions: () => () => undefined,
  }),
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
  useRuntimeCommandsAvailable: () => service.available,
}));

const VERIFICATION_ARGS = { path: "docs/verification.md", mode: "review" };
const ACCEPTED_ARGS_TEXT = JSON.stringify({ path: "docs/accepted.md", mode: "review" }, null, 2);

function EditableApproval() {
  return (
    <ApprovalCard
      sessionId="ses-1"
      resumeRunId="run-1"
      itemId="approval-1"
      toolName="unknown_tool"
      category="unknown"
      cmd=""
      reason="Review the document."
      args={VERIFICATION_ARGS}
      rememberable
    />
  );
}

function button(name: string) {
  return screen.getByRole<HTMLButtonElement>("button", { name });
}

function argumentsField() {
  return screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Arguments" });
}

function expectFocusableBlocked(element: HTMLButtonElement | HTMLTextAreaElement) {
  expect(element.getAttribute("aria-disabled")).toBe("true");
  expect(element.disabled).toBe(false);
  element.focus();
  expect(document.activeElement).toBe(element);
}

function openEditedArguments() {
  fireEvent.click(button("Edit"));
  fireEvent.change(argumentsField(), { target: { value: ACCEPTED_ARGS_TEXT } });
}

describe("ApprovalCard actions", () => {
  afterEach(() => {
    vi.clearAllMocks();
    submission.pending = null;
    submission.submitted = false;
    service.available = true;
  });

  it.each(["approve", "deny"] as const)(
    "does not turn a locally submitted %s into a settled Runtime fact",
    (decision) => {
      submission.pending = decision;
      testClient.render(
        <ApprovalCard
          sessionId="ses-1"
          resumeRunId="run-1"
          itemId="approval-1"
          toolName="shell"
          cmd="pwd"
          reason="Runs commands in the workspace."
        />,
      );
      expect(screen.queryByText("Approved", { exact: true })).toBeNull();
      expect(screen.queryByText("Declined", { exact: true })).toBeNull();
      expectFocusableBlocked(button("Allow once"));
    },
  );

  it("submits exactly the edited arguments once", () => {
    testClient.render(<EditableApproval />);
    openEditedArguments();
    fireEvent.click(button("Allow once"));

    expect(submission.submit).toHaveBeenCalledTimes(1);
    expect(submission.submit).toHaveBeenCalledWith("approve", {
      editedArgs: { path: "docs/accepted.md", mode: "review" },
    });
  });

  it("submits edited arguments with a remembered approval scope", () => {
    testClient.render(<EditableApproval />);
    openEditedArguments();
    fireEvent.click(button("Approval options"));
    fireEvent.click(screen.getByRole("menuitem", { name: "Allow for this session" }));

    expect(submission.submit).toHaveBeenCalledWith("approve", {
      editedArgs: { path: "docs/accepted.md", mode: "review" },
      rememberScope: "session",
    });
  });

  it("keeps an invalid draft editable until a valid decision is admitted", () => {
    testClient.render(<EditableApproval />);
    fireEvent.click(button("Edit"));
    fireEvent.change(argumentsField(), { target: { value: "{" } });
    fireEvent.click(button("Allow once"));

    expect(submission.submit).not.toHaveBeenCalled();
    expect(argumentsField().value).toBe("{");
    expect(argumentsField().readOnly).toBe(false);
    expect(argumentsField().getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByRole("alert")).toBeTruthy();

    fireEvent.change(argumentsField(), { target: { value: ACCEPTED_ARGS_TEXT } });
    fireEvent.click(button("Allow once"));
    expect(submission.submit).toHaveBeenCalledExactlyOnceWith("approve", {
      editedArgs: { path: "docs/accepted.md", mode: "review" },
    });
  });

  it("allows local argument drafting while Runtime commands are unavailable", () => {
    service.available = false;
    testClient.render(<EditableApproval />);
    const edit = button("Edit");

    fireEvent.click(edit);
    expect(argumentsField().readOnly).toBe(false);
    expect(document.activeElement).toBe(argumentsField());
    fireEvent.change(argumentsField(), { target: { value: ACCEPTED_ARGS_TEXT } });
    expect(argumentsField().value).toBe(ACCEPTED_ARGS_TEXT);
    fireEvent.click(button("Allow once"));
    expect(submission.submit).not.toHaveBeenCalled();
  });

  it.each([
    { pending: "approve" as const, submitted: false },
    { pending: "deny" as const, submitted: false },
    { pending: null, submitted: true },
  ])("refuses to enter editing after decision admission: %j", ({ pending, submitted }) => {
    submission.pending = pending;
    submission.submitted = submitted;
    testClient.render(<EditableApproval />);
    const edit = button("Edit");
    expectFocusableBlocked(edit);
    fireEvent.click(edit);
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(document.activeElement).toBe(edit);
  });

  it.each(["approve", "deny"] as const)(
    "keeps the open arguments read-only but focusable while %s is pending",
    (decision) => {
      const { rerender } = testClient.render(<EditableApproval />);
      openEditedArguments();

      submission.pending = decision;
      rerender(<EditableApproval />);
      const field = argumentsField();
      expect(field.readOnly).toBe(true);
      expect(field.value).toBe(ACCEPTED_ARGS_TEXT);
      expect(document.activeElement).toBe(field);
      expectFocusableBlocked(field);

      service.available = false;
      rerender(<EditableApproval />);
      expect(argumentsField()).toBe(field);
      expect(field.readOnly).toBe(true);
    },
  );

  it("keeps the arguments read-only once the submission is staged after pending clears", () => {
    const { rerender } = testClient.render(<EditableApproval />);
    openEditedArguments();

    submission.pending = "approve";
    rerender(<EditableApproval />);
    submission.pending = null;
    submission.submitted = true;
    rerender(<EditableApproval />);

    expect(argumentsField().readOnly).toBe(true);
    expectFocusableBlocked(argumentsField());
    expectFocusableBlocked(button("Allow once"));
  });

  it("preserves an open offline draft when Runtime commands return", () => {
    const { rerender } = testClient.render(<EditableApproval />);
    openEditedArguments();

    service.available = false;
    rerender(<EditableApproval />);
    expect(argumentsField().readOnly).toBe(false);
    fireEvent.change(argumentsField(), {
      target: { value: JSON.stringify({ path: "docs/offline.md", mode: "review" }) },
    });
    fireEvent.click(button("Allow once"));
    expect(submission.submit).not.toHaveBeenCalled();

    service.available = true;
    rerender(<EditableApproval />);
    const field = argumentsField();
    expect(field.readOnly).toBe(false);
    expect(field.getAttribute("aria-disabled")).toBeNull();
    expect(field.value).toBe(JSON.stringify({ path: "docs/offline.md", mode: "review" }));

    fireEvent.click(button("Allow once"));
    expect(submission.submit).toHaveBeenCalledWith("approve", {
      editedArgs: { path: "docs/offline.md", mode: "review" },
    });
  });

  it("keeps blocked decisions focusable while refusing repeated activation and the scope menu", () => {
    submission.pending = "approve";
    testClient.render(<EditableApproval />);

    for (const name of ["Deny", "Allow once", "Approval options"]) {
      const control = button(name);
      expectFocusableBlocked(control);
      fireEvent.pointerDown(control);
      fireEvent.mouseDown(control);
      fireEvent.click(control);
      fireEvent.click(control);
      fireEvent.keyDown(control, { key: "Enter" });
      fireEvent.keyDown(control, { key: "ArrowDown" });
      expect(document.activeElement).toBe(control);
    }

    expect(submission.submit).not.toHaveBeenCalled();
    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.queryByRole("menuitem")).toBeNull();
  });

  it("orders the deny action before the primary approval action", () => {
    testClient.render(
      <ApprovalCard
        sessionId="ses-1"
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
    const { container } = testClient.render(
      <ApprovalCard
        sessionId="ses-1"
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
    testClient.render(
      <ApprovalCard
        sessionId="ses-1"
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
      testClient.render(
        <ApprovalCard
          sessionId="ses-1"
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
