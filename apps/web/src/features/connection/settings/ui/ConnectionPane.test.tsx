import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { type RuntimeServiceSnapshot } from "@/features/connection/public/serviceStatus";
import { ConnectionPane } from "@/features/connection/settings/ui/ConnectionPane";
const runtime = vi.hoisted(() => ({
  applyEndpoint: vi.fn(),
  refresh: vi.fn(),
  resetEndpoint: vi.fn(),
  hasToken: false,
  snapshot: null as RuntimeServiceSnapshot | null,
}));

function description(field: HTMLElement): string {
  return (field.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ");
}

function expectNoRuntimeCommand() {
  expect(runtime.applyEndpoint).not.toHaveBeenCalled();
  expect(runtime.resetEndpoint).not.toHaveBeenCalled();
  expect(runtime.refresh).not.toHaveBeenCalled();
}

describe("ConnectionPane runtime status", () => {
  beforeEach(() => {
    runtime.hasToken = false;
    const endpoint = testClient.runtime.endpoint;
    vi.spyOn(endpoint, "hasRuntimeAccessToken").mockImplementation(() => runtime.hasToken);
    vi.spyOn(endpoint, "defaultRuntimeEndpoint").mockReturnValue("http://127.0.0.1:17171");
    vi.spyOn(endpoint, "currentRuntimeEndpoint").mockReturnValue("http://127.0.0.1:17171");
    vi.spyOn(endpoint, "applyRuntimeEndpoint").mockImplementation((...args) =>
      runtime.applyEndpoint(...args),
    );
    vi.spyOn(endpoint, "resetRuntimeEndpoint").mockImplementation(() => runtime.resetEndpoint());
    vi.spyOn(testClient.runtime.status, "useSnapshot").mockImplementation(() => runtime.snapshot!);
    vi.spyOn(testClient.runtime.status, "refresh").mockImplementation(() => runtime.refresh());
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
    testClient.render(<ConnectionPane />);

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
    testClient.render(<ConnectionPane />);

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
    testClient.render(<ConnectionPane />);
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

    testClient.render(<ConnectionPane />);

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

    testClient.render(<ConnectionPane />);

    expect(screen.getByText("Unavailable")).toBeTruthy();
    expect(screen.getByText("connection refused")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(runtime.refresh).toHaveBeenCalledOnce();
  });

  it("leaves an IME confirmation alone and keeps focus on a rejected URL", () => {
    runtime.snapshot = { phase: "checking", failure: null, observation: null };
    runtime.applyEndpoint.mockReturnValue({ kind: "rejected", reason: "invalid_url" });
    testClient.render(<ConnectionPane />);
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

  it("describes the current token and shows Clear as a pending change", () => {
    runtime.hasToken = true;
    runtime.snapshot = { phase: "ready", observation: null, failure: null };
    testClient.render(<ConnectionPane />);
    const token = screen.getByLabelText("Access token");

    expect(description(token)).toContain("A token is set for the current Runtime address.");
    expect(description(token)).toContain("Kept only in this window");

    fireEvent.click(screen.getByRole("button", { name: "Clear token" }));

    expect(description(token)).toContain("Token change not applied.");
    expect(description(token)).not.toContain("A token is set");
    expect(description(token)).toContain("Kept only in this window");
    expectNoRuntimeCommand();
  });

  it("discards only the token edit and returns focus to the token field", () => {
    runtime.hasToken = true;
    runtime.snapshot = { phase: "ready", observation: null, failure: null };
    testClient.render(<ConnectionPane />);
    const url = screen.getByLabelText("URL") as HTMLInputElement;
    const token = screen.getByLabelText("Access token") as HTMLInputElement;
    const discard = screen.getByRole("button", { name: "Discard token change" });
    expect((discard as HTMLButtonElement).disabled).toBe(true);

    fireEvent.change(url, { target: { value: "http://127.0.0.1:9999" } });
    fireEvent.change(token, { target: { value: "draft-secret" } });
    expect(description(token)).not.toContain("draft-secret");
    discard.focus();
    fireEvent.click(discard);

    expect(url.value).toBe("http://127.0.0.1:9999");
    expect(token.value).toBe("");
    expect(description(token)).toContain("A token is set for the current Runtime address.");
    expect((discard as HTMLButtonElement).disabled).toBe(true);
    expect(document.activeElement).toBe(token);
    expectNoRuntimeCommand();

    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    expect(runtime.applyEndpoint).toHaveBeenLastCalledWith("http://127.0.0.1:9999", undefined);
  });

  it("hands an edited token to Runtime exactly as typed", () => {
    runtime.snapshot = { phase: "ready", observation: null, failure: null };
    testClient.render(<ConnectionPane />);

    fireEvent.change(screen.getByLabelText("Access token"), { target: { value: "  " } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));

    expect(runtime.applyEndpoint).toHaveBeenLastCalledWith("http://127.0.0.1:17171", "  ");
  });

  it("settles the pending state on the token presence Runtime reports after Apply", () => {
    runtime.hasToken = true;
    runtime.snapshot = { phase: "ready", observation: null, failure: null };
    runtime.applyEndpoint.mockImplementation((endpoint: string) => {
      runtime.hasToken = false;
      return { kind: "applied", endpoint, changed: false };
    });
    testClient.render(<ConnectionPane />);
    const token = screen.getByLabelText("Access token");

    fireEvent.click(screen.getByRole("button", { name: "Clear token" }));
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));

    expect(runtime.applyEndpoint).toHaveBeenLastCalledWith("http://127.0.0.1:17171", "");
    expect(description(token)).toContain("No token is set for the current Runtime address.");
    expect(description(token)).not.toContain("Token change not applied.");
    const discard = screen.getByRole("button", { name: "Discard token change" });
    expect((discard as HTMLButtonElement).disabled).toBe(true);
  });

  it("ties a rejected token to its field and drops only that rejection on discard", () => {
    runtime.snapshot = { phase: "ready", observation: null, failure: null };
    runtime.applyEndpoint.mockReturnValue({ kind: "rejected", reason: "invalid_token" });
    testClient.render(<ConnectionPane />);
    const url = screen.getByLabelText("URL");
    const token = screen.getByLabelText("Access token");
    const discard = screen.getByRole("button", { name: "Discard token change" });

    fireEvent.change(token, { target: { value: "bad token" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    const tokenError = screen.getByRole("alert").textContent ?? "";
    expect(description(token)).toContain(tokenError);
    expect(description(url)).not.toContain(tokenError);

    fireEvent.click(discard);
    expect(screen.queryByRole("alert")).toBeNull();

    runtime.applyEndpoint.mockReturnValue({ kind: "rejected", reason: "invalid_url" });
    fireEvent.change(url, { target: { value: "not a url" } });
    fireEvent.change(token, { target: { value: "draft" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    const urlError = screen.getByRole("alert").textContent ?? "";
    fireEvent.click(discard);

    expect(screen.getByRole("alert").textContent).toBe(urlError);
    expect(description(url)).toContain(urlError);
    expect(description(token)).not.toContain(urlError);
  });
});
