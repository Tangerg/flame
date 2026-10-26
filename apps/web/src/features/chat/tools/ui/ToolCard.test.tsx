import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, screen } from "@testing-library/react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { ToolCard } from "@/features/chat/tools/ui/ToolCard";
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

const COPY = "Copy arguments";
const OPEN = "Open in the context dock";

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

function card(tool: Partial<ToolCall>) {
  return testClient.render(
    <ToolCard tool={call(tool)} expanded={false} onToggleExpand={() => {}} />,
  );
}

function rows(tools: ToolCall[], expandedId?: string) {
  return (
    <>
      {tools.map((tool) => (
        <ToolCard
          key={tool.id}
          tool={tool}
          expanded={tool.id === expandedId}
          onToggleExpand={() => {}}
        />
      ))}
    </>
  );
}

function describedAction(name: string, summary: string) {
  return screen.getByRole("button", { name, description: (text) => text.includes(summary) });
}

beforeEach(() => {
  opened.mockClear();
  ran.mockClear();
});

describe("ToolCard", () => {
  it("keeps every invocation on the narrative line, whatever it is doing", () => {
    const cases: Array<Partial<ToolCall>> = [
      { name: "read", safetyClass: "safe", status: "ok" },
      { name: "shell", safetyClass: "exec", status: "running" },
      { name: "apply_patch", safetyClass: "write", status: "ok" },
      { name: "apply_patch", safetyClass: "write", status: "err", error: "denied" },
      { name: "apply_patch", safetyClass: "write", status: "denied" },
    ];

    const rows = cases.map((entry) => {
      const { container, unmount } = card(entry);
      const row = container.querySelector("[data-shell]");
      const attributes = {
        shell: row?.getAttribute("data-shell"),
        tone: row?.getAttribute("data-tone"),
      };
      unmount();
      return attributes;
    });

    expect(rows).toEqual(cases.map(() => ({ shell: "line", tone: "neutral" })));
  });

  it("opens the workspace view only when its own action is pressed", () => {
    card({ name: "apply_patch", safetyClass: "write", status: "ok" });

    expect(opened).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Open in the context dock" }));
    expect(opened).toHaveBeenCalledOnce();
    expect(opened).toHaveBeenCalledWith(
      expect.objectContaining({ name: "apply_patch" }),
      screen.getByRole("button", { name: "Open in the context dock" }),
    );
  });

  it("keeps a denied call without captured arguments static", () => {
    const { container } = card({
      name: "apply_patch",
      fn: "src/denied.ts",
      safetyClass: "write",
      status: "denied",
    });

    expect(container.querySelector("[aria-expanded]")).toBeNull();
    expect(container.querySelector('[data-slot="agent-activity-chevron"]')).toBeNull();
    expect(container.querySelector('[role="region"]')).toBeNull();
    expect(screen.queryByText("No changes to show")).toBeNull();
    expect(describedAction(COPY, "denied.ts")).toBeTruthy();
    expect(describedAction(OPEN, "denied.ts")).toBeTruthy();
  });

  it("describes each identically named action by its own row summary, never the result", async () => {
    const alpha = call({
      id: "call alpha\t1",
      name: "shell",
      fn: "go test ./alpha",
      result: "alpha-result-body",
    });
    const beta = call({ id: "call beta 2", name: "shell", fn: "go test ./beta" });
    const { rerender } = testClient.render(rows([alpha, beta], alpha.id));

    expect((await screen.findByRole("region", { name: "Result" })).textContent).toContain(
      "alpha-result-body",
    );
    screen.getByRole("button", {
      name: COPY,
      description: (text) =>
        text.includes("go test ./alpha") && !text.includes("alpha-result-body"),
    });

    fireEvent.click(describedAction(COPY, "go test ./beta"));
    expect(ran).toHaveBeenCalledOnce();
    expect(ran).toHaveBeenCalledWith(beta);

    rerender(rows([beta, alpha], alpha.id));
    const openAlpha = describedAction(OPEN, "go test ./alpha");
    fireEvent.click(openAlpha);
    expect(opened).toHaveBeenCalledOnce();
    expect(opened).toHaveBeenCalledWith(alpha, openAlpha);

    rerender(rows([{ ...beta, fn: "go test ./gamma" }, alpha], alpha.id));
    expect(describedAction(COPY, "go test ./gamma")).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: COPY,
        description: (text) => text.includes("go test ./beta"),
      }),
    ).toBeNull();
  });

  it("keeps the short name and row description while an action tooltip is open", async () => {
    card({ id: "t 1", name: "shell", fn: "make build" });
    const button = describedAction(COPY, "make build");

    act(() => button.focus());

    expect((await screen.findByRole("tooltip")).textContent).toBe(COPY);
    expect(describedAction(COPY, "make build")).toBe(button);
  });
});
