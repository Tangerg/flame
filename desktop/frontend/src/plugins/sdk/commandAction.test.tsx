import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RpcError } from "@/rpc/errors";
import { useCommandAction } from "./commandAction";
import { useNotificationStore } from "./notifications";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn(), message: vi.fn() } }));

class Retired extends Error {}
const wasRetired = (error: unknown) => error instanceof Retired;

function Harness({ command }: { command: () => Promise<unknown> }) {
  const { busy, run } = useCommandAction({ wasRetired, fallback: "could not do it" });
  return (
    <button disabled={busy} aria-busy={busy} onClick={() => run(command)}>
      go
    </button>
  );
}

describe("useCommandAction", () => {
  beforeEach(() => {
    useNotificationStore.getState().clearAll();
  });

  // The button is disabled from `busy`, which lands a render after the click that started
  // the command — so the guard has to be the ref, not the rendered state.
  it("refuses a second click that lands before the first has rendered", () => {
    const command = vi.fn(() => new Promise<void>(() => {}));
    render(<Harness command={command} />);

    const button = screen.getByRole("button");
    fireEvent.click(button);
    fireEvent.click(button);

    expect(command).toHaveBeenCalledOnce();
  });

  it("shows the command running on the control that started it", async () => {
    const settle = Promise.withResolvers<void>();
    render(<Harness command={() => settle.promise} />);

    const button = screen.getByRole("button");
    fireEvent.click(button);
    expect(button.getAttribute("aria-busy")).toBe("true");

    await act(async () => {
      settle.resolve();
      await settle.promise;
    });
    expect(button.getAttribute("aria-busy")).toBe("false");
    expect((button as HTMLButtonElement).disabled).toBe(false);
  });

  it("says what the Runtime said, and takes the next command", async () => {
    const command = vi.fn(() => Promise.reject(new RpcError({ message: "runtime said no" })));
    render(<Harness command={command} />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(useNotificationStore.getState().log.at(-1)?.message).toContain("runtime said no");

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(command).toHaveBeenCalledTimes(2);
  });

  // Anything that is not an RPC refusal has no message written for a reader — an internal
  // Error's text is a stack-trace artefact, not copy.
  it("falls back rather than showing an internal error's own words", async () => {
    render(<Harness command={() => Promise.reject(new Error("undefined is not a function"))} />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(useNotificationStore.getState().log.at(-1)?.message).toBe("could not do it");
  });

  // A retired command settled nowhere the user can see it, so there is nothing to report.
  it("stays quiet when the owner retired the command", async () => {
    render(<Harness command={() => Promise.reject(new Retired("gone"))} />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(useNotificationStore.getState().log).toHaveLength(0);
  });
});
