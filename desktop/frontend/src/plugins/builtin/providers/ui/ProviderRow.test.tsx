import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
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

type TestResult = { ok: boolean; error?: string };

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

  describe("Save & Test feedback", () => {
    const configured = () => provider({ credential: { masked: "sk-****", source: "stored" } });
    const fields = () => ({
      url: screen.getByLabelText(/openai-compatible Base URL/i) as HTMLInputElement,
      key: screen.getByLabelText(/openai-compatible API/i) as HTMLInputElement,
    });
    const saveAndTest = () => screen.getByRole("button", { name: /save & test/i });

    it.each([
      ["a successful connection", (r: PromiseWithResolvers<TestResult>) => r.resolve({ ok: true })],
      [
        "a negative connection result",
        (r: PromiseWithResolvers<TestResult>) =>
          r.resolve({ ok: false, error: "stale key rejected" }),
      ],
      [
        "a rejected connection test",
        (r: PromiseWithResolvers<TestResult>) => r.reject(new Error("stale transport failure")),
      ],
    ])("retires %s when the draft changed before the save acknowledged", async (_, settle) => {
      const saveAck = Promise.withResolvers<ProviderConfiguration>();
      const testAck = Promise.withResolvers<TestResult>();
      hooks.update.mockReturnValue(saveAck.promise);
      hooks.test.mockReturnValue(testAck.promise);
      const original = configured();
      render(<ProviderRow p={original} />);
      const { url, key } = fields();

      fireEvent.change(url, { target: { value: "https://a.example.test/v1" } });
      fireEvent.click(saveAndTest());
      fireEvent.change(url, { target: { value: "https://b.example.test/v1" } });
      fireEvent.change(key, { target: { value: "typed-after" } });

      await act(async () => {
        saveAck.resolve(
          provider({ baseUrl: "https://a.example.test/v1", credential: original.credential }),
        );
      });
      await act(async () => {
        settle(testAck);
      });

      expect(url.value).toBe("https://b.example.test/v1");
      expect(key.value).toBe("typed-after");
      expect(screen.queryByText(/connection ok/i)).toBeNull();
      expect(screen.queryByRole("alert")).toBeNull();
      expect(hooks.update).toHaveBeenCalledTimes(1);
      expect(hooks.update).toHaveBeenCalledWith(
        expect.objectContaining({ baseUrl: { type: "set", value: "https://a.example.test/v1" } }),
      );
      expect(saveAndTest().getAttribute("aria-disabled")).toBeNull();
    });

    it("retires a save failure when the draft changed before the save settled", async () => {
      const saveAck = Promise.withResolvers<ProviderConfiguration>();
      hooks.update.mockReturnValue(saveAck.promise);
      render(<ProviderRow p={configured()} />);
      const { url } = fields();

      fireEvent.change(url, { target: { value: "https://a.example.test/v1" } });
      fireEvent.click(saveAndTest());
      fireEvent.change(url, { target: { value: "https://b.example.test/v1" } });
      await act(async () => {
        saveAck.reject(new Error("stale endpoint rejected"));
      });

      expect(url.value).toBe("https://b.example.test/v1");
      expect(screen.queryByRole("alert")).toBeNull();
      expect(hooks.test).not.toHaveBeenCalled();
    });

    it("reports the test failure of the draft it saved", async () => {
      const original = configured();
      hooks.update.mockResolvedValue(original);
      hooks.test.mockResolvedValue({ ok: false, error: "key rejected" });
      render(<ProviderRow p={original} />);

      fireEvent.change(fields().url, { target: { value: "https://draft.example.test/v1" } });
      fireEvent.click(saveAndTest());

      expect((await screen.findByRole("alert")).textContent).toContain("key rejected");
      expect(screen.queryByText(/connection ok/i)).toBeNull();
    });

    it("retires a started test when the draft changes", async () => {
      const original = configured();
      const testAck = Promise.withResolvers<TestResult>();
      hooks.update.mockResolvedValue(original);
      hooks.test.mockReturnValue(testAck.promise);
      render(<ProviderRow p={original} />);
      const { url } = fields();

      fireEvent.change(url, { target: { value: "https://a.example.test/v1" } });
      fireEvent.click(saveAndTest());
      await waitFor(() => expect(hooks.test).toHaveBeenCalledTimes(1));
      fireEvent.change(url, { target: { value: "https://b.example.test/v1" } });
      await act(async () => {
        testAck.resolve({ ok: false, error: "stale key rejected" });
      });

      expect(url.value).toBe("https://b.example.test/v1");
      expect(screen.queryByRole("alert")).toBeNull();
    });

    it("returns to idle when a Runtime generation retires the save", async () => {
      hooks.update.mockRejectedValue(new GenerationRetiredError("provider_mutation"));
      render(<ProviderRow p={configured()} />);

      fireEvent.change(fields().url, { target: { value: "https://a.example.test/v1" } });
      fireEvent.click(saveAndTest());

      await waitFor(() => expect(saveAndTest().getAttribute("aria-disabled")).toBeNull());
      expect(screen.queryByRole("alert")).toBeNull();
      expect(screen.queryByText(/connection ok/i)).toBeNull();
      expect(hooks.test).not.toHaveBeenCalled();
    });

    it("refuses actions admitted in the same tick as a pending save", async () => {
      const original = configured();
      const saveAck = Promise.withResolvers<ProviderConfiguration>();
      hooks.update.mockReturnValue(saveAck.promise);
      hooks.test.mockResolvedValue({ ok: true });
      render(<ProviderRow p={original} />);

      fireEvent.change(fields().url, { target: { value: "https://a.example.test/v1" } });
      const save = screen.getByRole("button", { name: /^save$/i });
      const combined = saveAndTest();
      act(() => {
        save.click();
        combined.click();
        combined.click();
      });
      expect((combined as HTMLButtonElement).disabled).toBe(false);
      expect(combined.getAttribute("aria-disabled")).toBe("true");
      await act(async () => {
        saveAck.resolve(original);
      });

      expect(await screen.findByText(/saved/i)).toBeTruthy();
      expect(hooks.update).toHaveBeenCalledTimes(1);
      expect(hooks.test).not.toHaveBeenCalled();
    });

    it("refuses a second Save & Test while the first is pending", async () => {
      const original = configured();
      const saveAck = Promise.withResolvers<ProviderConfiguration>();
      hooks.update.mockReturnValue(saveAck.promise);
      hooks.test.mockResolvedValue({ ok: true });
      render(<ProviderRow p={original} />);

      fireEvent.change(fields().url, { target: { value: "https://a.example.test/v1" } });
      const combined = saveAndTest();
      act(() => {
        combined.click();
        combined.click();
      });
      fireEvent.click(combined);
      await act(async () => {
        saveAck.resolve(original);
      });

      await screen.findByText(/connection ok/i);
      expect(hooks.update).toHaveBeenCalledTimes(1);
      expect(hooks.test).toHaveBeenCalledTimes(1);
    });
  });
});
