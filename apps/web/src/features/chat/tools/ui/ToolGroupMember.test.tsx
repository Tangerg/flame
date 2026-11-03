import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen } from "@testing-library/react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { ToolGroupMember } from "@/features/chat/tools/ui/ToolGroupMember";
const opened = vi.hoisted(() => vi.fn());
const ran = vi.hoisted(() => vi.fn());

vi.mock("@/extensions/selectors/extensions", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/extensions/selectors/extensions")>();
  return {
    ...actual,
    useExtensionPoint: (point: { id: string }) => {
      if (point.id === "flame.tool.action") {
        return [{ id: "test-action", icon: "copy", title: "toolAction.copyArguments", run: ran }];
      }
      if (point.id === "flame.tool.viewOpener") {
        return [
          {
            id: "test-opener",
            predicate: () => true,
            label: () => "workspace.view.openBeside",
            open: opened,
          },
        ];
      }
      return [];
    },
  };
});

function call(tool: Partial<ToolCall>): ToolCall {
  return {
    id: "t1",
    runId: "r1",
    name: "read",
    fn: "read",
    args: "",
    status: "ok",
    ...tool,
  };
}

function member(tool: Partial<ToolCall>) {
  return testClient.render(
    <ToolGroupMember tool={call(tool)} expanded={false} onToggleExpand={() => {}} />,
  );
}

function describedAction(name: string, summary: string) {
  return screen.getByRole("button", { name, description: (text) => text.includes(summary) });
}

beforeEach(() => {
  opened.mockClear();
  ran.mockClear();
});

describe("ToolGroupMember", () => {
  it("describes each grouped action by its own row summary", async () => {
    const notes = call({
      id: "call notes 1",
      name: "read",
      fn: "docs/計画 メモ.md",
      fnKind: "path",
      result: "notes-result-body",
    });
    const broken = call({
      id: "call broken 2",
      name: "read",
      fn: "src/missing file.ts",
      fnKind: "path",
      status: "err",
    });
    testClient.render(
      <>
        <ToolGroupMember tool={notes} expanded onToggleExpand={() => {}} />
        <ToolGroupMember tool={broken} expanded={false} onToggleExpand={() => {}} />
      </>,
    );

    expect((await screen.findByRole("region", { name: "Result" })).textContent).toContain(
      "notes-result-body",
    );
    screen.getByRole("button", {
      name: "Copy arguments",
      description: (text) => text.includes("計画 メモ.md") && !text.includes("notes-result-body"),
    });
    describedAction("Copy arguments", "missing file.ts");
    expect(
      screen.getByRole("button", {
        name: "Open in the context dock",
        description: (text) => text.includes("missing file.ts") && /failed/i.test(text),
      }),
    ).toBeTruthy();

    fireEvent.click(describedAction("Copy arguments", "計画 メモ.md"));
    expect(ran).toHaveBeenCalledWith(notes);
    const openBroken = describedAction("Open in the context dock", "missing file.ts");
    fireEvent.click(openBroken);
    expect(opened).toHaveBeenCalledWith(broken, openBroken);
  });

  it("says the act and the thing acted on, not one or the other", () => {
    member({ name: "read", fn: "src/App.tsx", fnKind: "path" });

    expect(screen.getByText("Read")).toBeTruthy();
    expect(screen.getByTitle("src/App.tsx")).toBeTruthy();
  });

  it("tells continued Skill pages apart by the byte each one starts from", () => {
    const snapshot = "0f".repeat(32);
    const skill = (id: string, name: string, offset?: number) => {
      const resource = name === "read_skill_resource";
      return call({
        id,
        name,
        category: resource ? "read" : "task",
        fn: resource ? "review-diff/checklist.md" : "review-diff",
        ...(resource ? { fnKind: "path" as const } : {}),
        args: JSON.stringify({
          name: "review-diff",
          ...(resource ? { path: "checklist.md" } : {}),
          ...(offset !== undefined ? { cursor: { offset, snapshot } } : {}),
        }),
      });
    };
    testClient.render(
      <>
        {[
          skill("load 1", "load_skill"),
          skill("load 2", "load_skill", 134),
          skill("load 3", "load_skill", 266),
          skill("resource 1", "read_skill_resource"),
          skill("resource 2", "read_skill_resource", 81),
        ].map((tool) => (
          <ToolGroupMember key={tool.id} tool={tool} expanded={false} onToggleExpand={() => {}} />
        ))}
      </>,
    );

    // getByRole fails unless exactly one collapsed row carries each name.
    const row = (verb: string, offset?: number) =>
      screen.getByRole("button", {
        name: (name) =>
          name.startsWith(verb) &&
          name.includes("review-diff") &&
          (offset === undefined
            ? !name.includes("from byte")
            : name.includes(`from byte ${offset}`)),
      });
    expect(screen.getAllByRole("button", { name: /^Loaded Skill/ })).toHaveLength(3);
    for (const offset of [undefined, 134, 266]) row("Loaded Skill", offset);
    expect(screen.getAllByRole("button", { name: /^Read Skill file/ })).toHaveLength(2);
    for (const offset of [undefined, 81]) row("Read Skill file", offset);
  });

  it("tells same-name calls apart by their requested snapshot offset", () => {
    const snapshotId = "snap-7f3a";
    const browse = (id: string, offset: number, limit: number) =>
      call({
        id,
        name: "web_web_browse",
        fn: "web_web_browse",
        args: JSON.stringify({ snapshotId, offset, limit }),
      });
    testClient.render(
      <>
        {[browse("browse 1", 8192, 128), browse("browse 2", 8320, 8192)].map((tool) => (
          <ToolGroupMember key={tool.id} tool={tool} expanded={false} onToggleExpand={() => {}} />
        ))}
      </>,
    );

    expect(screen.getAllByRole("button", { name: /^web_web_browse/ })).toHaveLength(2);
    for (const offset of [8192, 8320]) {
      const row = screen.getByRole("button", {
        name: (name) =>
          name.startsWith("web_web_browse") &&
          name.includes(`{"offset":${offset},"snapshotId":"${snapshotId}"}`),
      });
      expect(row.querySelector("button, a[href], input, [tabindex]")).toBeNull();
    }
  });

  it("shows a non-zero exit as a failure rather than as one more grey figure", () => {
    const { container } = member({ name: "shell", fn: "go test ./...", exitCode: 1 });

    const chip = container.querySelector('[data-tone="negative"]');
    expect(chip?.textContent).toContain("1");
  });

  it("carries the line counts a running edit already knows", () => {
    member({
      name: "apply_patch",
      fn: "src/App.tsx",
      fnKind: "path",
      status: "running",
      added: 12,
      removed: 3,
    });

    expect(screen.getByText("+12")).toBeTruthy();
    expect(screen.getByText("−3")).toBeTruthy();
  });

  it("keeps a failure and its reason when grouped, as a lone card would", () => {
    member({
      name: "read",
      fn: "missing.ts",
      fnKind: "path",
      status: "err",
      error: "no such file",
    });

    expect(screen.getByText(/failed/i)).toBeTruthy();
    expect(screen.getByText("no such file")).toBeTruthy();
  });

  it("marks a denied call instead of looking like a quiet success", () => {
    member({ name: "read", fn: "secret.env", fnKind: "path", status: "denied" });

    expect(screen.getByText(/denied/i)).toBeTruthy();
  });

  it("marks a failed call even when the runtime supplied no explanatory text", () => {
    const { container } = member({ status: "err" });

    expect(screen.getByText("failed").getAttribute("data-tone")).toBe("negative");
    expect(container.querySelector('[data-slot="tool-error"]')).toBeNull();
  });
});
