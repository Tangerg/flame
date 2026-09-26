import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeServiceSnapshot } from "@/plugins/builtin/runtime/public/serviceStatus";
import { ConnectionPane } from "./ConnectionPane";

const runtime = vi.hoisted(() => ({
  applyEndpoint: vi.fn(),
  refresh: vi.fn(),
  resetEndpoint: vi.fn(),
  hasToken: false,
  snapshot: null as RuntimeServiceSnapshot | null,
}));

vi.mock("@/plugins/builtin/runtime/public/endpoint", () => ({
  hasRuntimeAccessToken: () => runtime.hasToken,
  defaultRuntimeEndpoint: () => "http://127.0.0.1:17171",
  currentRuntimeEndpoint: () => "http://127.0.0.1:17171",
  applyRuntimeEndpoint: runtime.applyEndpoint,
  resetRuntimeEndpoint: runtime.resetEndpoint,
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeServiceStatus: () => runtime.snapshot,
  refreshRuntimeServiceStatus: () => runtime.refresh(),
}));

describe("ConnectionPane runtime status", () => {
  beforeEach(() => {
    runtime.hasToken = false;
    runtime.applyEndpoint.mockReset().mockImplementation((endpoint: string) => ({
      kind: "applied",
      endpoint,
      changed: false,
    }));
    runtime.refresh.mockReset().mockResolvedValue(undefined);
    runtime.resetEndpoint.mockReset().mockReturnValue({
      kind: "applied",
      endpoint: "http://127.0.0.1:17171",
      changed: false,
    });
  });

  it("keeps its controls mounted and disabled rather than appearing as you type", () => {
    runtime.snapshot = {
      phase: "ready",
      failure: null,
      observation: {
        server: { name: "flame-runtime", version: "1.2.3" },
        protocolVersion: "2",
        health: "ready",
        checks: {},
      },
    };
    render(<ConnectionPane />);

    const apply = screen.getByRole("button", { name: "Apply" });
    const reset = screen.getByRole("button", { name: "Reset to default" });
    expect((apply as HTMLButtonElement).disabled).toBe(true);
    expect((reset as HTMLButtonElement).disabled).toBe(true);

    fireEvent.change(screen.getByLabelText("URL"), {
      target: { value: "http://127.0.0.1:9999" },
    });
    expect(screen.getByRole("button", { name: "Apply" })).toBe(apply);
    expect((apply as HTMLButtonElement).disabled).toBe(false);
  });

  it("hands an endpoint replacement to the Runtime owner without reloading the renderer", () => {
    runtime.snapshot = {
      phase: "ready",
      failure: null,
      observation: {
        server: { name: "flame-runtime", version: "1.2.3" },
        protocolVersion: "2026-07-01",
        health: "ready",
        checks: {},
      },
    };
    runtime.applyEndpoint.mockReturnValue({
      kind: "applied",
      endpoint: "http://127.0.0.1:27171",
      changed: true,
    });
    const reload = vi.spyOn(window.location, "reload").mockImplementation(() => undefined);
    render(<ConnectionPane />);

    fireEvent.change(screen.getByRole("textbox", { name: "URL" }), {
      target: { value: "http://127.0.0.1:27171" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));

    expect(runtime.applyEndpoint).toHaveBeenCalledWith("http://127.0.0.1:27171", undefined);
    expect(reload).not.toHaveBeenCalled();
  });

  it("applies a token without persisting it in the editor and can clear it explicitly", () => {
    runtime.hasToken = true;
    runtime.snapshot = { phase: "unavailable", observation: null, failure: null };
    render(<ConnectionPane />);
    const token = screen.getByLabelText("Access token") as HTMLInputElement;
    fireEvent.change(token, { target: { value: "window-token" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(runtime.applyEndpoint).toHaveBeenLastCalledWith(
      "http://127.0.0.1:17171",
      "window-token",
    );
    expect(token.value).toBe("");
    fireEvent.click(screen.getByRole("button", { name: "Clear token" }));
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(runtime.applyEndpoint).toHaveBeenLastCalledWith("http://127.0.0.1:17171", "");
  });

  it("renders degraded identity, protocol, and failing dependency checks", () => {
    runtime.snapshot = {
      phase: "degraded",
      failure: null,
      observation: {
        server: { name: "flame-runtime", version: "1.2.3" },
        protocolVersion: "2026-07-01",
        health: "degraded",
        checks: { sqlite: "ready", git: "degraded" },
      },
    };

    render(<ConnectionPane />);

    expect(screen.getAllByText("Degraded")).toHaveLength(2);
    expect(screen.getByText(/flame-runtime 1\.2\.3/)).toBeTruthy();
    expect(screen.getByText("2026-07-01")).toBeTruthy();
    expect(screen.getByText("git")).toBeTruthy();
    expect(screen.queryByText("sqlite")).toBeNull();
  });

  it("shows an unavailable detail and delegates a retry to Runtime", () => {
    runtime.snapshot = {
      phase: "unavailable",
      observation: null,
      failure: { reason: "failed", detail: "connection refused" },
    };

    render(<ConnectionPane />);

    expect(screen.getByText("Unavailable")).toBeTruthy();
    expect(screen.getByText("connection refused")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(runtime.refresh).toHaveBeenCalledOnce();
  });

  it("leaves an IME confirmation alone and keeps focus on a rejected URL", () => {
    runtime.snapshot = { phase: "checking", failure: null, observation: null };
    runtime.applyEndpoint.mockReturnValue({ kind: "rejected", reason: "invalid_url" });
    render(<ConnectionPane />);
    const field = document.getElementById("runtime-base-url") as HTMLInputElement;
    field.focus();
    fireEvent.change(field, { target: { value: "本地" } });

    fireEvent.keyDown(field, { key: "Enter", keyCode: 229 });
    expect(runtime.applyEndpoint).not.toHaveBeenCalled();

    fireEvent.keyDown(field, { key: "Enter", keyCode: 13 });
    expect(runtime.applyEndpoint).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(field);
    expect(screen.getByRole("alert")).toBeTruthy();
  });
});
