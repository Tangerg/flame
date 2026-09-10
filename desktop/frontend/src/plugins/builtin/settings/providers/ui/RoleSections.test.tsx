import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EmbeddingModelSection, UtilityModelSection } from "./RoleSections";

const provider = vi.hoisted(() => ({
  generation: 1,
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
  }),
  useProviderMutationMaterialGeneration: () => provider.generation,
  useUtilityModelConfig: () => ({
    role: undefined,
    modelOptions: [],
    selected: null,
    isSet: false,
    isAvailable: true,
    isError: false,
  }),
}));

describe("Provider role mutation material", () => {
  beforeEach(() => {
    provider.generation = 1;
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

    // In flight it is `aria-disabled`, not `disabled` — the trigger has to stay a tab stop or
    // activating it would blur whoever activated it. So the guarantee is checked as the
    // guarantee rather than as an attribute: a second activation must not reach the port.
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
});
