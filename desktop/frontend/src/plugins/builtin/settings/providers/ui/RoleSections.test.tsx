import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EmbeddingModelSection, UtilityModelSection } from "./RoleSections";

const provider = vi.hoisted(() => ({
  generation: 1,
  roleState: { kind: "ready", stale: false } as
    { kind: "loading" } | { kind: "error"; retry: () => void } | { kind: "ready"; stale: boolean },
  setEmbeddingRole: vi.fn(),
  setUtilityRole: vi.fn(),
}));

vi.mock("../application/providerConfig", () => ({
  setEmbeddingRole: provider.setEmbeddingRole,
  setUtilityRole: provider.setUtilityRole,
  useEmbeddingModelConfig: () => ({
    role: undefined,
    capableProviders: [],
    isSet: false,
    isAvailable: true,
    state: provider.roleState,
  }),
  useProviderMutationMaterialGeneration: () => provider.generation,
  useUtilityModelConfig: () => ({
    role: undefined,
    modelOptions: [],
    selected: null,
    isSet: false,
    isAvailable: true,
    isError: false,
    state: provider.roleState,
  }),
}));

describe("Provider role mutation material", () => {
  beforeEach(() => {
    provider.generation = 1;
    provider.roleState = { kind: "ready", stale: false };
    provider.setEmbeddingRole.mockReset();
    provider.setUtilityRole.mockReset();
  });

  it("binds embedding-role feedback to the same Runtime material generation", async () => {
    provider.setEmbeddingRole.mockResolvedValue({ ok: false, error: "retired embedding failure" });
    const view = render(<EmbeddingModelSection />);

    fireEvent.click(screen.getByRole("button", { name: "Embedding model" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Off" }));
    await screen.findByText("retired embedding failure");

    provider.generation = 2;
    view.rerender(<EmbeddingModelSection />);

    expect(screen.queryByText("retired embedding failure")).toBeNull();
  });

  it("retires a failed role mutation with its Runtime generation", async () => {
    provider.setUtilityRole.mockResolvedValue({ ok: false, error: "retired role failure" });
    const view = render(<UtilityModelSection />);

    fireEvent.click(screen.getByRole("button", { name: "Utility model" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Use main model" }));
    await screen.findByText("retired role failure");

    provider.generation = 2;
    view.rerender(<UtilityModelSection />);

    expect(screen.queryByText("retired role failure")).toBeNull();
  });

  it("makes an admitted role mutation visible and prevents duplicate selection", async () => {
    let resolve!: (value: { ok: boolean }) => void;
    provider.setUtilityRole.mockReturnValue(
      new Promise<{ ok: boolean }>((settle) => {
        resolve = settle;
      }),
    );
    render(<UtilityModelSection />);

    const trigger = screen.getByRole("button", { name: "Utility model" });
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByRole("menuitem", { name: "Use main model" }));
    await waitFor(() => expect(provider.setUtilityRole).toHaveBeenCalledOnce());

    const ariaDisabledWhilePending = trigger.getAttribute("aria-disabled");
    const focusableWhilePending = !(trigger as HTMLButtonElement).disabled;
    const showedPendingFeedback = screen.queryByText("Saving…") !== null;
    fireEvent.click(trigger);

    resolve({ ok: true });
    await waitFor(() => expect(screen.queryByText("Saving…")).toBeNull());

    expect(ariaDisabledWhilePending).toBe("true");
    expect(focusableWhilePending).toBe(true);
    expect(showedPendingFeedback).toBe(true);
    expect(provider.setUtilityRole).toHaveBeenCalledOnce();
  });

  it("does not claim a role is off while its value is still unknown", () => {
    provider.roleState = { kind: "loading" };
    render(<EmbeddingModelSection />);
    expect(screen.queryByText("Off")).toBeNull();
    expect(screen.queryByText(/No provider/i)).toBeNull();
    expect(screen.getByRole("button", { name: /embedding model/i }).hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("offers a retry instead of the main-model default when loading failed", () => {
    const retry = vi.fn();
    provider.roleState = { kind: "error", retry };
    render(<UtilityModelSection />);
    expect(screen.queryByText(/use main model/i)).toBeNull();
    expect(screen.getByRole("alert").textContent).toMatch(/unknown/i);
    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(retry).toHaveBeenCalledOnce();
  });
});
