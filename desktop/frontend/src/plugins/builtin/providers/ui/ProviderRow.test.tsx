import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  ProviderConfiguration,
  type ProviderConfigurationSnapshot,
} from "../application/providerModels";
import { resetProviderDraftsForTest } from "../application/providerDrafts";
import { ProviderRow } from "./ProviderRow";

const hooks = vi.hoisted(() => ({
  update: vi.fn(),
  test: vi.fn(),
  generation: 1,
}));

vi.mock("../application/providerConfig", () => ({
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
    resetProviderDraftsForTest();
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
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));

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
    expect((screen.getByRole("button", { name: /^save$/i }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("retires old Runtime feedback without discarding the user's credential draft", async () => {
    hooks.test.mockResolvedValue({ ok: true });
    const configured = provider({ credential: { masked: "sk-****", source: "stored" } });
    const view = render(<ProviderRow p={configured} />);

    fireEvent.click(screen.getByRole("button", { name: /^test$/i }));
    await screen.findByText(/connection ok/i);
    fireEvent.change(screen.getByLabelText(/openai-compatible Base URL/i), {
      target: { value: "https://draft.example.test/v1" },
    });
    expect(screen.queryByText(/connection ok/i)).toBeNull();

    hooks.generation = 2;
    view.rerender(<ProviderRow p={configured} />);
    expect((screen.getByLabelText(/openai-compatible Base URL/i) as HTMLInputElement).value).toBe(
      "https://draft.example.test/v1",
    );
  });

  it("keeps what was typed while a save was in flight", async () => {
    let resolve!: (value: ProviderConfiguration) => void;
    hooks.update.mockReturnValue(new Promise<ProviderConfiguration>((r) => (resolve = r)));
    const configured = provider({ credential: { masked: "sk-****", source: "stored" } });
    render(<ProviderRow p={configured} />);
    const url = screen.getByLabelText(/openai-compatible Base URL/i) as HTMLInputElement;
    const key = screen.getByLabelText(/openai-compatible API/i) as HTMLInputElement;

    fireEvent.change(url, { target: { value: "https://a.example.test/v1" } });
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    fireEvent.change(url, { target: { value: "https://b.example.test/v1" } });
    fireEvent.change(key, { target: { value: "typed-after" } });
    resolve(provider({ baseUrl: "https://a.example.test/v1", credential: configured.credential }));

    await screen.findByText(/saved/i);
    expect(url.value).toBe("https://b.example.test/v1");
    expect(key.value).toBe("typed-after");
  });

  it("tests the draft it shows by saving it first", async () => {
    const configured = provider({ credential: { masked: "sk-****", source: "stored" } });
    hooks.update.mockResolvedValue(configured);
    hooks.test.mockResolvedValue({ ok: true });
    render(<ProviderRow p={configured} />);

    fireEvent.change(screen.getByLabelText(/openai-compatible Base URL/i), {
      target: { value: "https://draft.example.test/v1" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save & test/i }));

    await screen.findByText(/connection ok/i);
    expect(hooks.update).toHaveBeenCalledBefore(hooks.test);
  });

  it("does not test after a save that failed", async () => {
    hooks.update.mockRejectedValue(new Error("endpoint rejected"));
    render(<ProviderRow p={provider({ credential: { masked: "sk-****", source: "stored" } })} />);

    fireEvent.change(screen.getByLabelText(/openai-compatible Base URL/i), {
      target: { value: "https://bad.example.test/v1" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save & test/i }));

    expect((await screen.findByRole("alert")).textContent).toContain("endpoint rejected");
    expect(hooks.test).not.toHaveBeenCalled();
  });

  it("presents an optional-key provider as ready without fabricating a credential", () => {
    render(
      <ProviderRow
        p={provider({
          id: "test-endpoint",
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
    expect((screen.getByRole("button", { name: /^test$/i }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("keeps an unsaved draft when the pane is left and reopened", () => {
    const configured = provider({ credential: { masked: "sk-****", source: "stored" } });
    const first = render(<ProviderRow p={configured} />);
    fireEvent.change(screen.getByLabelText(/openai-compatible Base URL/i), {
      target: { value: "https://half-typed.example.test" },
    });
    first.unmount();

    render(<ProviderRow p={configured} />);
    expect((screen.getByLabelText(/openai-compatible Base URL/i) as HTMLInputElement).value).toBe(
      "https://half-typed.example.test",
    );
  });
});
