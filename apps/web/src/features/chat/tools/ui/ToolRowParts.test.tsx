import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { ToolCard } from "@/features/chat/tools/ui/ToolCard";
import { ToolGroupMember } from "@/features/chat/tools/ui/ToolGroupMember";
import {
  TOOL_ACTION,
  TOOL_VIEW_OPENER,
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
} from "@/features/chat/public/tools/types";
import { contributeForTest } from "@/test/kernel";
import { usePluginErrorStore } from "@/extensions/errors";

const point = { run: vi.fn(), open: vi.fn() };
let removeAction: () => void;
let removeOpener: () => void;
const actionSpec = () => ({
  id: "test-action",
  icon: "copy",
  title: "toolAction.copyArguments",
  run: point.run,
});
const openerSpec = () => ({
  id: "test-opener",
  predicate: () => true,
  label: () => "workspace.view.openBeside",
  open: point.open,
});
const consumers = [
  ["ToolCard", ToolCard],
  ["ToolGroupMember", ToolGroupMember],
] as const;

const controls = [
  {
    kind: "action",
    label: "Copy arguments",
    callback: point.run,
    detail: "tool action: test-action",
    owner: "action",
  },
  {
    kind: "view opener",
    label: "Open in the context dock",
    callback: point.open,
    detail: "tool view opener: test-opener",
    owner: "opener",
  },
] as const;

const failures = [
  [
    "a synchronous throw",
    (err: Error) => () => {
      throw err;
    },
  ],
  ["a rejected promise", (err: Error) => () => Promise.reject(err)],
] as const;

type Consumer = (typeof consumers)[number][1];

function call(tool: Partial<ToolCall> = {}): ToolCall {
  return {
    id: "t1",
    runId: "r1",
    name: "shell",
    fn: "make build",
    args: "",
    status: "ok",
    ...tool,
  };
}

function row(Row: Consumer, tool: ToolCall) {
  return <Row tool={tool} expanded={false} onToggleExpand={() => {}} />;
}

function deferred() {
  let reject!: (err: unknown) => void;
  const promise = new Promise<void>((_, rej) => (reject = rej));
  return { promise, reject };
}

const failureAlert = () => screen.queryByRole("alert");

beforeEach(async () => {
  point.run.mockReset();
  point.open.mockReset();
  vi.spyOn(console, "error").mockImplementation(() => {});
  await contributeForTest((ctx) => {
    const action = ctx.contribute(TOOL_ACTION, actionSpec());
    const opener = ctx.contribute(TOOL_VIEW_OPENER, openerSpec());
    removeAction = () => action.dispose();
    removeOpener = () => opener.dispose();
  }, "plugin.alpha");
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe.each(consumers)("%s contributed controls", (_name, Row) => {
  describe.each(controls)("$kind", (control) => {
    it.each(failures)(
      "keeps %s local to the row and reports it to the original owner",
      async (_failure, failWith) => {
        const err = new Error("disk gone");
        control.callback.mockImplementation(failWith(err));
        const tool = call();
        render(row(Row, tool));
        const button = screen.getByRole("button", { name: control.label });

        expect(() => fireEvent.click(button)).not.toThrow();

        expect(control.callback).toHaveBeenCalledOnce();
        const expectedCalls = control.kind === "action" ? [[tool]] : [[tool, button]];
        expect(control.callback.mock.calls).toEqual(expectedCalls);

        const alert = await screen.findByRole("alert");
        expect(console.error).toHaveBeenCalledWith(expect.any(String), err);
        expect(alert.getAttribute("data-slot")).toBe("tool-action-error");
        expect(alert.textContent).toBe(`Couldn’t complete “${control.label}”. disk gone`);
        expect(alert.getAttribute("aria-describedby")).toBe(
          button.getAttribute("aria-describedby"),
        );
        expect(usePluginErrorStore.getState().log).toEqual([
          expect.objectContaining({
            plugin: "plugin.alpha",
            source: "command",
            message: err.message,
            detail: control.detail,
          }),
        ]);
      },
    );

    it("attributes a delayed failure to the owner captured at activation", async () => {
      const pending = deferred();
      control.callback.mockReturnValue(pending.promise);
      const { rerender } = render(row(Row, call()));
      fireEvent.click(screen.getByRole("button", { name: control.label }));

      await act(async () => {
        if (control.owner === "action") removeAction();
        else removeOpener();
        await contributeForTest((ctx) => {
          if (control.owner === "action") ctx.contribute(TOOL_ACTION, actionSpec());
          else ctx.contribute(TOOL_VIEW_OPENER, openerSpec());
        }, "plugin.beta");
      });
      rerender(row(Row, call()));
      const currentOwner =
        control.owner === "action"
          ? lookupToolActionOwner("test-action")
          : lookupToolViewOpenerOwner("test-opener");
      expect(currentOwner).toBe("plugin.beta");
      const err = new Error("late");
      pending.reject(err);

      await waitFor(() =>
        expect(usePluginErrorStore.getState().log).toEqual([
          expect.objectContaining({
            plugin: "plugin.alpha",
            source: "command",
            message: err.message,
            detail: control.detail,
          }),
        ]),
      );
    });

    it("clears the failure on a successful retry without disturbing the row", async () => {
      control.callback.mockImplementationOnce(() => {
        throw new Error("busy");
      });
      render(row(Row, call({ status: "err", error: "exit 1" })));
      const button = screen.getByRole("button", { name: control.label });
      button.focus();

      fireEvent.click(button);
      await screen.findByRole("alert");
      fireEvent.click(button);

      await waitFor(() => expect(failureAlert()).toBeNull());
      expect(control.callback).toHaveBeenCalledTimes(2);
      expect(screen.getByRole("button", { name: control.label })).toBe(button);
      expect(document.activeElement).toBe(button);
      expect(document.querySelector('[data-slot="tool-error"]')?.textContent).toBe("exit 1");
    });
  });

  it("keeps the outcome across a snapshot refresh and retires it for another call", async () => {
    point.run.mockImplementation(() => {
      throw new Error("nope");
    });
    const { rerender } = render(row(Row, call()));
    fireEvent.click(screen.getByRole("button", { name: "Copy arguments" }));
    await screen.findByRole("alert");

    rerender(row(Row, call({ fn: "make build --verbose", result: "ok" })));
    expect(failureAlert()).not.toBeNull();

    rerender(row(Row, call({ id: "t2" })));
    expect(failureAlert()).toBeNull();
  });

  it.each([
    ["the row now shows another call", "replace"],
    ["a newer activation succeeded", "reactivate"],
  ] as const)("drops an older delayed failure once %s, still reporting it", async (_case, how) => {
    const pending = deferred();
    point.run.mockReturnValueOnce(pending.promise);
    const { rerender } = render(row(Row, call()));
    fireEvent.click(screen.getByRole("button", { name: "Copy arguments" }));

    if (how === "replace") rerender(row(Row, call({ runId: "r2" })));
    else fireEvent.click(screen.getByRole("button", { name: "Copy arguments" }));
    const err = new Error("stale");
    pending.reject(err);

    await waitFor(() => expect(usePluginErrorStore.getState().log).toHaveLength(1));
    expect(failureAlert()).toBeNull();
  });

  it("reports failures from a denied, static row", async () => {
    point.open.mockRejectedValue(new Error("no view"));
    const { container } = render(row(Row, call({ status: "denied", fn: "src/denied.ts" })));
    expect(container.querySelector("[aria-expanded]")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Open in the context dock" }));

    expect((await screen.findByRole("alert")).textContent).toBe(
      "Couldn’t complete “Open in the context dock”. no view",
    );
  });
  it("keeps action feedback beside the summary while its captured result remains open", async () => {
    point.run.mockImplementation(() => {
      throw new Error("copy refused");
    });
    const tool = call({ result: "captured output" });
    const toggle = vi.fn();
    const { container } = render(<Row tool={tool} expanded onToggleExpand={toggle} />);
    const result = await screen.findByRole("region", { name: "Result" });
    const disclosure = container.querySelector("button[aria-expanded]");
    fireEvent.click(screen.getByRole("button", { name: "Copy arguments" }));
    const alert = await screen.findByRole("alert");
    expect(alert.compareDocumentPosition(result) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(result.textContent).toContain("captured output");
    expect(disclosure?.getAttribute("aria-expanded")).toBe("true");
    expect(toggle).not.toHaveBeenCalled();
    expect(tool.status).toBe("ok");
    expect(container.querySelector('[data-slot="tool-status"]')).toBeNull();
    expect(container.querySelector('[data-slot="tool-error"]')).toBeNull();
  });
});
