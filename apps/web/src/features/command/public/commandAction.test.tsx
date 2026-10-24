import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RpcError } from "@flame/runtime-client";
import { useCommandAction } from "@/features/command/public/commandAction";
import { useNotificationStore } from "@/features/notifications/public/notifications";
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn(), message: vi.fn() } }));

class Retired extends Error {}

const wasRetired = (error: unknown) => error instanceof Retired;

function Harness({
  command,
  onError,
}: {
  command: () => Promise<unknown>;
  onError?: (message: string) => void;
}) {
  const { busy, run } = useCommandAction({ wasRetired, fallback: "could not do it", onError });
  return (
    <button aria-busy={busy} onClick={() => run(command)}>
      go
    </button>
  );
}

describe("useCommandAction", () => {
  beforeEach(() => {
    useNotificationStore.getState().clearAll();
  });

  it("runs one command at a time even when the caller renders no disabled state", () => {
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
  });

  it("says what the Runtime said, and takes the next command", async () => {
    const command = vi.fn(() =>
      Promise.reject(
        new RpcError({ message: "runtime said no", data: { type: "invalid_request" } }),
      ),
    );
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

  it("falls back rather than showing an internal error's own words", async () => {
    render(<Harness command={() => Promise.reject(new Error("undefined is not a function"))} />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(useNotificationStore.getState().log.at(-1)?.message).toBe("could not do it");
  });

  it("stays quiet when the owner retired the command", async () => {
    render(<Harness command={() => Promise.reject(new Retired("gone"))} />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(useNotificationStore.getState().log).toHaveLength(0);
  });

  it("hands the failure to the task that renders it instead of notifying", async () => {
    const onError = vi.fn();
    const command = vi.fn(() =>
      Promise.reject(
        new RpcError({ message: "runtime said no", data: { type: "invalid_request" } }),
      ),
    );
    render(<Harness command={command} onError={onError} />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(onError).toHaveBeenCalledOnce();
    expect(onError).toHaveBeenCalledWith("runtime said no");
    expect(useNotificationStore.getState().log).toHaveLength(0);
    expect(screen.getByRole("button").getAttribute("aria-busy")).toBe("false");

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(command).toHaveBeenCalledTimes(2);
  });

  it("keeps a retired failure away from the task's own error sink", async () => {
    const onError = vi.fn();
    render(<Harness command={() => Promise.reject(new Retired("gone"))} onError={onError} />);

    await act(async () => {
      fireEvent.click(screen.getByRole("button"));
    });
    expect(onError).not.toHaveBeenCalled();
    expect(useNotificationStore.getState().log).toHaveLength(0);
  });
});
