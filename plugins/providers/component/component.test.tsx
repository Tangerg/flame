// @vitest-environment happy-dom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { RpcTransportError } from "@flame/runtime-client";
import type {
  Provider,
  ProviderTestResult,
  UtilityRole,
} from "@flame/runtime-client/wire";
import { Providers } from "./component";
import { fixture, model, mutation, provider } from "./testing";
import { zh } from "./locales/zh";

beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(100, 100, 200, 32),
  );
  vi.spyOn(document.documentElement, "clientWidth", "get").mockReturnValue(
    1024,
  );
  vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(
    768,
  );
});

const fixtures: ReturnType<typeof fixture>[] = [];
function create(providers?: Provider[]) {
  const f = fixture(providers);
  fixtures.push(f);
  const Component = Providers.create({
    runtimeIdentity: f.context.runtime,
    component: f.context.component,
    signal: f.source.signal,
  });
  return { ...f, Component };
}
afterEach(() => {
  cleanup();
  for (const f of fixtures.splice(0)) f.dispose();
  vi.restoreAllMocks();
});
const keyLabel = `${provider.id} API key`;
const endpointLabel = `${provider.id} base URL`;
const keyInput = () => screen.getByLabelText<HTMLInputElement>(keyLabel);
const endpointInput = () =>
  screen.getByLabelText<HTMLInputElement>(endpointLabel);

async function mount(f = create()) {
  const view = render(<f.Component context={f.context} />);
  await screen.findByLabelText(keyLabel);
  return { f, view };
}

it("retains an unsaved credential draft when a view is closed and reopened", async () => {
  const { f, view } = await mount();
  fireEvent.change(keyInput(), {
    target: { value: "synthetic-private-draft" },
  });
  fireEvent.change(endpointInput(), {
    target: { value: "https://draft.example.test/v1" },
  });
  view.unmount();
  f.view.abort();
  const next = f.openView();
  render(<f.Component context={next.context} />);
  expect(await screen.findByLabelText(keyLabel)).toHaveProperty(
    "value",
    "synthetic-private-draft",
  );
  expect(endpointInput().value).toBe("https://draft.example.test/v1");
  expect(f.update).not.toHaveBeenCalled();
});

it("keeps a key draft while following an unedited Runtime endpoint change", async () => {
  const { f } = await mount();
  fireEvent.change(keyInput(), { target: { value: "replacement-key" } });
  const changed = {
    ...provider,
    baseUrl: "https://changed-by-runtime.example.test",
  };
  f.list.mockResolvedValue({ data: [changed] });
  f.update.mockReturnValueOnce(mutation(Promise.resolve(changed)));
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(endpointInput().value).toBe(changed.baseUrl));
  expect(keyInput().value).toBe("replacement-key");
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("Saved", { exact: true });
  expect(f.update).toHaveBeenCalledExactlyOnceWith({
    provider: provider.id,
    apiKey: { type: "set", value: "replacement-key" },
  });
});

it("uses the authoritative saved endpoint without losing later edits", async () => {
  const { f } = await mount();
  fireEvent.change(keyInput(), { target: { value: "submitted-key" } });
  fireEvent.change(endpointInput(), {
    target: { value: "https://entered.example.test" },
  });
  const response = Promise.withResolvers<Provider>();
  f.update.mockReturnValueOnce(mutation(response.promise));
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  fireEvent.change(keyInput(), { target: { value: "typed-during-save" } });
  const saved = { ...provider, baseUrl: "https://canonical.example.test/v1" };
  f.list.mockRejectedValue(new Error("refresh unavailable"));
  await act(async () => response.resolve(saved));
  await waitFor(() => expect(endpointInput().value).toBe(saved.baseUrl));
  expect(keyInput().value).toBe("typed-during-save");
  expect(screen.queryByText("Saved", {})).toBeNull();
  expect(f.update).toHaveBeenCalledExactlyOnceWith({
    provider: provider.id,
    apiKey: { type: "set", value: "submitted-key" },
    baseUrl: { type: "set", value: "https://entered.example.test" },
  });
});

it("saves the displayed draft before testing and then clears only submitted secrets", async () => {
  const { f } = await mount();
  fireEvent.change(keyInput(), { target: { value: " key-with-spaces " } });
  fireEvent.change(endpointInput(), {
    target: { value: " https://new.example.test/v1 " },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save & test" }));
  await screen.findByText("Connection OK");
  expect(f.update).toHaveBeenCalledExactlyOnceWith({
    provider: provider.id,
    apiKey: { type: "set", value: " key-with-spaces " },
    baseUrl: { type: "set", value: "https://new.example.test/v1" },
  });
  expect(f.test).toHaveBeenCalledWith(provider.id, expect.any(AbortSignal));
  expect(f.update.mock.invocationCallOrder[0]).toBeLessThan(
    f.test.mock.invocationCallOrder[0]!,
  );
  expect(keyInput().value).toBe("");
  expect(endpointInput().value).toBe("https://new.example.test/v1");
});

it.each([
  {
    name: "known refusal",
    error: () => new Error("save refused"),
    text: "save refused",
  },
  {
    name: "unknown command",
    error: () => new RpcTransportError("response lost"),
    text: "The original command outcome is not confirmed.",
  },
])("does not test after a save with $name", async ({ error, text }) => {
  const { f } = await mount();
  f.update.mockReturnValueOnce(mutation(Promise.reject(error())));
  fireEvent.change(keyInput(), { target: { value: "synthetic-key" } });
  fireEvent.click(screen.getByRole("button", { name: "Save & test" }));
  await waitFor(() =>
    expect(screen.getByRole("alert").textContent).toContain(text),
  );
  expect(f.test).not.toHaveBeenCalled();
  expect(keyInput().value).toBe("synthetic-key");
});

it("reconciles a response loss through the original command without submitting edited input", async () => {
  const { f } = await mount();
  const retry = vi.fn().mockReturnValue(mutation(Promise.resolve(provider)));
  f.update.mockReturnValueOnce(
    mutation(Promise.reject(new RpcTransportError("response lost")), retry),
  );
  fireEvent.change(keyInput(), { target: { value: "original-secret" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByRole("button", { name: "Reconcile original command" });
  fireEvent.change(keyInput(), { target: { value: "later-secret" } });
  fireEvent.click(
    screen.getByRole("button", { name: "Reconcile original command" }),
  );
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "Reconcile original command" }),
    ).toBeNull(),
  );
  expect(retry).toHaveBeenCalledOnce();
  expect(f.update).toHaveBeenCalledOnce();
  expect(keyInput().value).toBe("later-secret");
});

it("allows a new corrected save after a definite refusal", async () => {
  const { f } = await mount();
  f.update.mockReturnValueOnce(
    mutation(Promise.reject(new Error("save refused"))),
  );
  fireEvent.change(keyInput(), { target: { value: "first-key" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("save refused");
  fireEvent.change(keyInput(), { target: { value: "corrected-key" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("Saved", {});
  expect(f.update).toHaveBeenCalledTimes(2);
  expect(f.update.mock.calls[1]?.[0].apiKey).toEqual({
    type: "set",
    value: "corrected-key",
  });
});

it.each<ProviderTestResult["outcome"]>([
  "reachable",
  "notConfigured",
  "invalidCredentials",
  "timedOut",
  "failed",
])("presents the closed provider test outcome %s", async (outcome) => {
  const { f } = await mount();
  f.test.mockResolvedValue({ outcome });
  fireEvent.click(screen.getByRole("button", { name: "Test" }));
  if (outcome === "reachable") await screen.findByText("Connection OK");
  else await screen.findByRole("alert");
  expect(f.update).not.toHaveBeenCalled();
  expect(f.test).toHaveBeenCalledOnce();
});

it("keeps feedback tied to the draft that was tested", async () => {
  const { f } = await mount();
  const response = Promise.withResolvers<ProviderTestResult>();
  f.test.mockReturnValueOnce(response.promise);
  fireEvent.click(screen.getByRole("button", { name: "Test" }));
  fireEvent.change(keyInput(), { target: { value: "later-key" } });
  await act(async () => response.resolve({ outcome: "reachable" }));
  expect(screen.queryByText("Connection OK")).toBeNull();
  expect(keyInput().value).toBe("later-key");
});

it("validates a required endpoint locally and preserves an optional-key ready provider", async () => {
  const { f } = await mount();
  expect(screen.getByText("ready", {})).toBeTruthy();
  expect(screen.queryByText("not configured", {})).toBeNull();
  fireEvent.change(endpointInput(), { target: { value: "   " } });
  expect(endpointInput().getAttribute("aria-invalid")).toBe("true");
  expect(
    screen.getByRole("button", { name: "Save" }).hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "Save & test" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(f.update).not.toHaveBeenCalled();
});

it("confirms removal of only the stored key without submitting a private draft", async () => {
  const { f } = await mount(
    create([
      {
        ...provider,
        credential: { masked: "synthetic-mask", source: "stored" },
      },
    ]),
  );
  fireEvent.change(keyInput(), {
    target: { value: "unsubmitted-replacement" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Remove stored key" }));
  const dialog = await screen.findByRole("alertdialog", {
    name: "Remove the stored key?",
  });
  fireEvent.click(
    within(dialog).getByRole("button", { name: "Remove stored key" }),
  );
  await waitFor(() => expect(f.update).toHaveBeenCalledOnce());
  expect(f.update.mock.calls[0]?.[0]).toEqual({
    provider: provider.id,
    apiKey: { type: "clear" },
  });
  expect(keyInput().value).toBe("unsubmitted-replacement");
});

it.each([{ providers: [] as Provider[] }, { providers: [provider] }])(
  "waits for the first provider result and preserves later focus: $providers",
  async ({ providers }) => {
    const f = create();
    const arrival = Promise.withResolvers<{ data: Provider[] }>();
    f.list.mockReturnValue(arrival.promise);
    render(
      <>
        <f.Component context={f.context} />
        <textarea aria-label="Later draft" defaultValue="Retain me" />
      </>,
    );
    expect(screen.queryByText("No providers")).toBeNull();
    expect(screen.getAllByText("Loading").length).toBeGreaterThan(0);
    const later = screen.getByRole("textbox", { name: "Later draft" });
    act(() => later.focus());
    await act(async () => arrival.resolve({ data: providers }));
    expect(
      await screen.findByText(
        providers.length ? provider.id : "No providers",
        {},
      ),
    ).toBeTruthy();
    expect(document.activeElement).toBe(later);
    expect(f.setUtilityRole).not.toHaveBeenCalled();
    expect(f.setEmbeddingRole).not.toHaveBeenCalled();
  },
);

it("retains accepted providers and models when their refresh fails", async () => {
  const { f } = await mount();
  await screen.findByRole("combobox", { name: "Utility model" });
  f.list.mockRejectedValue(new Error("catalog unavailable"));
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findAllByText("catalog unavailable", { exact: false });
  expect(keyInput()).toBeTruthy();
  fireEvent.click(screen.getByRole("combobox", { name: "Utility model" }));
  expect(
    await screen.findByRole("option", { name: /Shared name/ }),
  ).toBeTruthy();
  expect(
    screen.getAllByText("The last accepted read is retained.").length,
  ).toBeGreaterThan(0);
  expect(f.setUtilityRole).not.toHaveBeenCalled();
});

it("keeps the known role distinct from an unread or failed choice catalog", async () => {
  const f = create();
  const arrival = Promise.withResolvers<{ data: (typeof model)[] }>();
  f.models.mockReturnValueOnce(arrival.promise);
  render(<f.Component context={f.context} />);
  const trigger = await screen.findByRole("combobox", {
    name: "Utility model",
  });
  expect(trigger.textContent).toContain(model.id);
  fireEvent.click(trigger);
  expect(
    await screen.findByRole("option", { name: /Use Runtime default/ }),
  ).toBeTruthy();
  await act(async () => arrival.reject(new Error("choices unavailable")));
  expect(trigger.textContent).toContain(model.id);
  expect(
    screen
      .getByRole("option", { name: new RegExp(model.id) })
      .getAttribute("aria-disabled"),
  ).toBe("true");
  fireEvent.keyDown(trigger, { key: "Escape" });
  f.models.mockResolvedValue({ data: [model] });
  fireEvent.click(screen.getByRole("button", { name: "Refresh models" }));
  await waitFor(() => expect(trigger.textContent).toContain(model.displayName));
  expect(f.setUtilityRole).not.toHaveBeenCalled();
});

it("never projects an unknown or failed role read as the Runtime default", async () => {
  const f = create();
  const arrival = Promise.withResolvers<UtilityRole>();
  f.getUtilityRole.mockReturnValueOnce(arrival.promise);
  render(<f.Component context={f.context} />);
  const loading = screen.getByRole("button", { name: "Utility model" });
  expect(loading.hasAttribute("disabled")).toBe(true);
  expect(screen.queryByText("Use Runtime default")).toBeNull();
  await act(async () => arrival.reject(new Error("role unread")));
  expect(screen.queryByRole("combobox", { name: "Utility model" })).toBeNull();
  fireEvent.click(
    screen.getByRole("button", { name: "Retry · Utility model" }),
  );
  expect(
    await screen.findByRole("combobox", { name: "Utility model" }),
  ).toBeTruthy();
  expect(f.setUtilityRole).not.toHaveBeenCalled();
});

it("distinguishes identical model names and waits for the accepted exact role", async () => {
  const second = { ...provider, id: "second-provider" };
  const f = create([provider, second]);
  const other = { ...model, provider: second.id };
  f.models.mockImplementation(async (id) => ({
    data: id === provider.id ? [model] : [other],
  }));
  await mount(f);
  const response = Promise.withResolvers<UtilityRole>();
  f.setUtilityRole.mockReturnValueOnce(mutation(response.promise));
  const trigger = await screen.findByRole("combobox", {
    name: "Utility model",
  });
  fireEvent.click(trigger);
  const picked = await screen.findByRole("option", { name: /second-provider/ });
  fireEvent.pointerDown(picked, { pointerType: "mouse" });
  fireEvent.click(picked);
  expect(f.setUtilityRole).toHaveBeenCalledExactlyOnceWith({
    provider: second.id,
    model: model.id,
  });
  expect(trigger.textContent).toContain(provider.id);
  expect(trigger.hasAttribute("disabled")).toBe(true);
  f.getUtilityRole.mockRejectedValue(new Error("refresh unavailable"));
  await act(async () =>
    response.resolve({ provider: second.id, model: model.id }),
  );
  await waitFor(() => expect(trigger.textContent).toContain(second.id));
  expect(f.setUtilityRole).toHaveBeenCalledOnce();
});

it("keeps a custom embedding identity after a rejected default-model choice", async () => {
  const f = create();
  f.getEmbeddingRole.mockResolvedValue({
    provider: provider.id,
    model: "custom-embedding",
  });
  f.setEmbeddingRole.mockImplementationOnce(() =>
    mutation(Promise.reject(new Error("role refused"))),
  );
  await mount(f);
  const trigger = await screen.findByRole("combobox", {
    name: "Embedding model",
  });
  expect(trigger.textContent).toContain("custom-embedding");
  fireEvent.click(trigger);
  const picked = await screen.findByRole("option", { name: /embed-default/ });
  fireEvent.pointerDown(picked, { pointerType: "mouse" });
  fireEvent.click(picked);
  await screen.findByText("role refused");
  expect(trigger.textContent).toContain("custom-embedding");
  expect(f.setEmbeddingRole).toHaveBeenCalledExactlyOnceWith({
    provider: provider.id,
    model: "embed-default",
  });
});

it("does not invent an embedding model when a capable provider has no preset", async () => {
  const f = create([{ ...provider, defaultEmbeddingModel: undefined }]);
  f.getEmbeddingRole.mockResolvedValue({
    provider: provider.id,
    model: "custom-embedding",
  });
  await mount(f);
  await screen.findByText(
    "No preset embedding model is available for the configured providers.",
  );
  const trigger = await screen.findByRole("combobox", {
    name: "Embedding model",
  });
  fireEvent.click(trigger);
  const off = await screen.findByRole("option", { name: "Off" });
  fireEvent.pointerDown(off, { pointerType: "mouse" });
  fireEvent.click(off);
  await waitFor(() =>
    expect(f.setEmbeddingRole).toHaveBeenCalledExactlyOnceWith({}),
  );
});

it("updates locale copy without restarting source reads or losing the current draft", async () => {
  const { f, view } = await mount();
  fireEvent.change(keyInput(), {
    target: { value: "language-independent-draft" },
  });
  const before = {
    lists: f.list.mock.calls.length,
    subscriptions: f.subscribe.mock.calls.length,
  };
  view.rerender(<f.Component context={{ ...f.context, locale: "zh" }} />);
  expect(
    screen.getByLabelText(
      zh["providers.apiKey.aria"]!.replace("{{provider}}", provider.id),
    ),
  ).toHaveProperty("value", "language-independent-draft");
  expect(f.list).toHaveBeenCalledTimes(before.lists);
  expect(f.subscribe).toHaveBeenCalledTimes(before.subscriptions);
});
