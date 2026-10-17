import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MCPServerMutationOwner } from "../application/mcpServerMutationOwner";
import type { MCPServerGateway, MCPServerTestOutcome } from "../application/ports/mcpServerGateway";
import { ServerForm } from "./ServerForm";

let owner: MCPServerMutationOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
});

function Harness() {
  const [opener, setOpener] = useState<HTMLButtonElement | null>(null);
  return (
    <>
      <button disabled={opener !== null} onClick={(event) => setOpener(event.currentTarget)}>
        Configure MCP server
      </button>
      {opener && (
        <ServerForm
          opener={opener}
          onDone={() => setOpener(null)}
          onCancel={() => setOpener(null)}
        />
      )}
    </>
  );
}

function openDraft(test: MCPServerGateway["test"]) {
  owner = MCPServerMutationOwner.install({ test } as unknown as MCPServerGateway);
  render(<Harness />);
  const opener = screen.getByRole("button", { name: "Configure MCP server" });
  fireEvent.click(opener);
  const status = screen.getByRole("status");
  fireEvent.change(screen.getByRole("textbox", { name: "Server name" }), {
    target: { value: "workspace-tools" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "Command" }), {
    target: { value: "npx" },
  });
  const trigger = screen.getByRole("button", { name: "Test" });
  trigger.focus();
  return { opener, status, trigger };
}

describe("MCP server test feedback", () => {
  it("updates its existing polite region for repeated successful tests without moving focus", async () => {
    const first = Promise.withResolvers<MCPServerTestOutcome>();
    const repeated = Promise.withResolvers<MCPServerTestOutcome>();
    const test = vi
      .fn<MCPServerGateway["test"]>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(repeated.promise);
    const { opener, status, trigger } = openDraft(test);
    expect(status.textContent).toBe("");
    fireEvent.click(trigger);
    await waitFor(() => expect(test).toHaveBeenCalledOnce());
    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    await act(async () => first.resolve({ ok: true }));
    await waitFor(() => expect(status.textContent).toContain("Connection OK"));
    expect(screen.getByRole("status")).toBe(status);
    expect(document.activeElement).toBe(trigger);

    fireEvent.click(trigger);
    await waitFor(() => expect(test).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    await act(async () => repeated.resolve({ ok: true }));
    await waitFor(() => expect(status.textContent).toContain("Connection OK"));
    expect(screen.getByRole("status")).toBe(status);
    expect(document.activeElement).toBe(trigger);
    const cancel = screen.getByRole("button", { name: "Cancel" });
    cancel.focus();
    fireEvent.click(cancel);
    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it.each([
    {
      name: "unsuccessful connection",
      result: { ok: false, error: "The server rejected the connection." },
      text: "The server rejected the connection.",
    },
    {
      name: "failed request",
      error: new Error("The Runtime connection was interrupted."),
      text: "The Runtime connection was interrupted.",
    },
  ])("announces a $name without moving focus", async ({ result, error, text }) => {
    const pending = Promise.withResolvers<MCPServerTestOutcome>();
    const test = vi.fn(() => pending.promise);
    const { opener, status, trigger } = openDraft(test);
    expect(status.textContent).toBe("");
    fireEvent.click(trigger);
    await waitFor(() => expect(test).toHaveBeenCalledOnce());
    expect(test).toHaveBeenCalledWith(
      expect.objectContaining({ name: "workspace-tools", transport: "stdio", command: "npx" }),
    );
    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    expect(screen.queryByRole("alert")).toBeNull();

    await act(async () => {
      if (error) pending.reject(error);
      else pending.resolve(result!);
    });

    expect((await screen.findByRole("alert")).textContent).toContain(text);
    expect(document.activeElement).toBe(trigger);
    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("");
    const cancel = screen.getByRole("button", { name: "Cancel" });
    cancel.focus();
    fireEvent.click(cancel);
    await waitFor(() => expect(document.activeElement).toBe(opener));
    expect(screen.queryByRole("textbox", { name: "Server name" })).toBeNull();
  });
});
