import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  ProviderConfiguration,
  type ProviderConfigurationSnapshot,
} from "../application/providerModels";
import { ProviderRow } from "./ProviderRow";

const hooks = vi.hoisted(() => ({
  update: vi.fn(),
  test: vi.fn(),
  generation: 1,
}));

vi.mock("../application/providerConfig", () => ({
  providerMutationWasRetired: () => false,
  useProviderMutationMaterialGeneration: () => hooks.generation,
  useUpdateProvider: () => hooks.update,
  useTestProvider: () => hooks.test,
}));

const provider = (
  overrides: Partial<ProviderConfigurationSnapshot> = {},
): ProviderConfiguration => {
  const configured = overrides.configured ?? overrides.credential !== undefined;
  return ProviderConfiguration.restore({
    id: "openai-compatible",
    baseUrl: configured
      ? (overrides.baseUrl ?? "https://gateway.example.test/v1")
      : overrides.baseUrl,
    configured,
    credentialRequirement: "apiKeyRequired",
    requiresBaseUrl: true,
    ...overrides,
  });
};

describe("ProviderRow", () => {
  beforeEach(() => {
    hooks.update.mockReset();
    hooks.test.mockReset();
    hooks.generation = 1;
  });

  it("rebuilds its draft from the authoritative saved resource", async () => {
    const saved = provider({
      baseUrl: "http://127.0.0.1:19999/v1",
      credential: { masked: "p4****ey", source: "stored" },
    });
    hooks.update.mockResolvedValue(saved);
    const view = render(<ProviderRow p={provider()} />);

    fireEvent.change(screen.getByLabelText(/openai-compatible API/i), {
      target: { value: "p49-dummy-key" },
    });
    fireEvent.change(screen.getByLabelText(/openai-compatible Base URL/i), {
      target: { value: "  http://127.0.0.1:19999/v1  " },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect((screen.getByLabelText(/openai-compatible Base URL/i) as HTMLInputElement).value).toBe(
        "http://127.0.0.1:19999/v1",
      );
    });
    expect((screen.getByLabelText(/openai-compatible API/i) as HTMLInputElement).value).toBe("");
    expect(hooks.update).toHaveBeenCalledWith({
      provider: "openai-compatible",
      apiKey: { type: "set", value: "p49-dummy-key" },
      baseUrl: { type: "set", value: "http://127.0.0.1:19999/v1" },
    });

    view.rerender(<ProviderRow p={saved} />);
    expect((screen.getByRole("button", { name: /save/i }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("retires old Runtime feedback without discarding the user's credential draft", async () => {
    hooks.test.mockResolvedValue({ ok: true });
    const configured = provider({ credential: { masked: "sk-****", source: "stored" } });
    const view = render(<ProviderRow p={configured} />);

    fireEvent.change(screen.getByLabelText(/openai-compatible Base URL/i), {
      target: { value: "https://draft.example.test/v1" },
    });
    fireEvent.click(screen.getByRole("button", { name: /test/i }));
    await screen.findByText(/connection ok/i);

    hooks.generation = 2;
    view.rerender(<ProviderRow p={configured} />);

    expect(screen.queryByText(/connection ok/i)).toBeNull();
    expect((screen.getByLabelText(/openai-compatible Base URL/i) as HTMLInputElement).value).toBe(
      "https://draft.example.test/v1",
    );
  });

  it("presents an optional-key provider as ready without fabricating a credential", () => {
    render(
      <ProviderRow
        p={provider({
          id: "ollama",
          baseUrl: undefined,
          configured: true,
          credential: undefined,
          credentialRequirement: "apiKeyOptional",
          requiresBaseUrl: false,
        })}
      />,
    );

    expect(screen.getByText(/^ready$/i)).toBeTruthy();
    expect(screen.queryByText(/not configured/i)).toBeNull();
    expect((screen.getByRole("button", { name: /test/i }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });
});
