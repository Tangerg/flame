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
const reconcileName = "Reconcile original command";
const confirmedName = "Original outcome confirmed";
function attempt(button: HTMLElement) {
  fireEvent.pointerDown(button, { pointerType: "mouse" });
  fireEvent.click(button);
  fireEvent.keyDown(button, { key: "Enter" });
  fireEvent.keyDown(button, { key: " " });
  fireEvent.keyUp(button, { key: " " });
}
// Accepts through the fixture's original update so later provider reads agree.
function loseSave(f: ReturnType<typeof create>) {
  const original = f.update.getMockImplementation()!;
  const retry = vi.fn();
  let accept!: () => Promise<Provider>;
  f.update.mockImplementationOnce((input) => {
    accept = () => original(input);
    return mutation(
      Promise.reject(new RpcTransportError("response lost")),
      retry,
    );
  });
  return { retry, accept: () => accept() };
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

it("keeps a completed Save focused and blocked until the next edit", async () => {
  const { f } = await mount();
  const original = f.update.getMockImplementation()!;
  const save = screen.getByRole("button", { name: "Save" });
  expect(save.hasAttribute("disabled")).toBe(true);
  attempt(save);
  expect(f.update).not.toHaveBeenCalled();

  const edited = "https://edited.example.test/v1";
  fireEvent.change(endpointInput(), { target: { value: edited } });
  const first = Promise.withResolvers<void>();
  f.update.mockImplementationOnce((input) =>
    mutation(first.promise.then(() => original(input))),
  );
  act(() => save.focus());
  fireEvent.click(save);
  expect(save.textContent).toBe("Saving…");
  expect(document.activeElement).toBe(save);
  attempt(save);
  expect(f.update).toHaveBeenCalledOnce();

  await act(async () => first.resolve());
  await screen.findByText("Saved", { exact: true });
  expect(save.textContent).toBe("Save");
  expect(screen.getByRole("button", { name: "Save" })).toBe(save);
  expect(document.activeElement).toBe(save);
  expect(save.getAttribute("aria-disabled")).toBe("true");
  expect(save.hasAttribute("disabled")).toBe(false);
  expect(save.tabIndex).toBe(0);
  expect(endpointInput().value).toBe(edited);
  attempt(save);
  expect(f.update).toHaveBeenCalledOnce();

  fireEvent.change(keyInput(), { target: { value: "second-key" } });
  expect(screen.queryByText("Saved", { exact: true })).toBeNull();
  expect(save.hasAttribute("disabled")).toBe(false);
  expect(save.getAttribute("aria-disabled")).not.toBe("true");
  expect(f.update).toHaveBeenCalledOnce();

  const second = Promise.withResolvers<void>();
  f.update.mockImplementationOnce((input) =>
    mutation(second.promise.then(() => original(input))),
  );
  fireEvent.click(save);
  act(() => keyInput().focus());
  await act(async () => second.resolve());
  await screen.findByText("Saved", { exact: true });
  expect(document.activeElement).toBe(keyInput());
  expect(keyInput().value).toBe("");
  expect(endpointInput().value).toBe(edited);
  expect(f.update.mock.calls).toEqual([
    [{ provider: provider.id, baseUrl: { type: "set", value: edited } }],
    [{ provider: provider.id, apiKey: { type: "set", value: "second-key" } }],
  ]);
});

it("keeps an unknown Save focused and blocked until reconciliation", async () => {
  const { f } = await mount();
  const save = screen.getByRole("button", { name: "Save" });
  expect(save.hasAttribute("disabled")).toBe(true);
  loseSave(f);
  fireEvent.change(keyInput(), { target: { value: "original-secret" } });
  act(() => save.focus());
  fireEvent.click(save);
  await screen.findByRole("button", { name: reconcileName });
  expect(screen.getByRole("button", { name: "Save" })).toBe(save);
  expect(document.activeElement).toBe(save);
  expect(save.getAttribute("aria-disabled")).toBe("true");
  expect(save.hasAttribute("disabled")).toBe(false);
  attempt(save);
  expect(f.update).toHaveBeenCalledOnce();
  expect(keyInput().value).toBe("original-secret");
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

async function focusReconcile() {
  fireEvent.change(keyInput(), { target: { value: "original-secret" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  const reconcile = await screen.findByRole("button", { name: reconcileName });
  act(() => reconcile.focus());
  fireEvent.click(reconcile);
  return reconcile;
}

it.each([
  { outcome: "accepted", message: "Saved", key: "" },
  { outcome: "refused", message: "save refused", key: "original-secret" },
])(
  "keeps a focused reconciliation through its $outcome outcome",
  async ({ outcome, message, key }) => {
    const { f } = await mount();
    const lost = loseSave(f);
    const settled = Promise.withResolvers<void>();
    lost.retry.mockImplementationOnce(() =>
      mutation(
        settled.promise.then(() =>
          outcome === "accepted"
            ? lost.accept()
            : Promise.reject(new Error("save refused")),
        ),
      ),
    );
    const reconcile = await focusReconcile();
    expect(screen.getByRole("button", { name: reconcileName })).toBe(reconcile);
    expect(document.activeElement).toBe(reconcile);
    expect(reconcile.getAttribute("aria-disabled")).toBe("true");
    attempt(reconcile);
    expect(lost.retry).toHaveBeenCalledOnce();

    await act(async () => settled.resolve());
    await screen.findByText(message, { exact: true });
    expect(screen.getByRole("button", { name: confirmedName })).toBe(reconcile);
    expect(document.activeElement).toBe(reconcile);
    expect(reconcile.getAttribute("aria-disabled")).toBe("true");
    expect(reconcile.hasAttribute("disabled")).toBe(false);
    attempt(reconcile);
    expect(lost.retry).toHaveBeenCalledOnce();
    expect(f.update).toHaveBeenCalledOnce();
    expect(keyInput().value).toBe(key);

    act(() => keyInput().focus());
    expect(screen.queryByRole("button", { name: confirmedName })).toBeNull();
    expect(screen.getByText(message, { exact: true })).toBeTruthy();
  },
);

it("keeps a repeated unknown reconciliation actionable for the original command", async () => {
  const { f } = await mount();
  const lost = loseSave(f);
  const first = Promise.withResolvers<void>();
  const second = Promise.withResolvers<void>();
  lost.retry
    .mockImplementationOnce(() =>
      mutation(
        first.promise.then(() =>
          Promise.reject(new RpcTransportError("still lost")),
        ),
      ),
    )
    .mockImplementationOnce(() => mutation(second.promise.then(lost.accept)));
  const reconcile = await focusReconcile();
  await act(async () => first.resolve());
  await waitFor(() =>
    expect(reconcile.getAttribute("aria-disabled")).not.toBe("true"),
  );
  expect(screen.getByRole("button", { name: reconcileName })).toBe(reconcile);
  expect(document.activeElement).toBe(reconcile);
  expect(screen.getByRole("alert").textContent).toContain(
    "The original command outcome is not confirmed.",
  );

  fireEvent.click(reconcile);
  await act(async () => second.resolve());
  await screen.findByText("Saved", { exact: true });
  expect(screen.getByRole("button", { name: confirmedName })).toBe(reconcile);
  expect(document.activeElement).toBe(reconcile);
  expect(lost.retry).toHaveBeenCalledTimes(2);
  expect(f.update).toHaveBeenCalledOnce();
});

it.each([
  { field: keyInput, value: "later-secret" },
  { field: endpointInput, value: "https://later.example.test/v1" },
])(
  "keeps a later field focused and edited when reconciliation settles",
  async ({ field, value }) => {
    const { f } = await mount();
    const lost = loseSave(f);
    const settled = Promise.withResolvers<void>();
    lost.retry.mockImplementationOnce(() =>
      mutation(settled.promise.then(lost.accept)),
    );
    await focusReconcile();
    act(() => field().focus());
    fireEvent.change(field(), { target: { value } });
    await act(async () => settled.resolve());
    await screen.findByRole("button", { name: "Save" });
    expect(document.activeElement).toBe(field());
    expect(field().value).toBe(value);
    expect(screen.queryByText("Saved", { exact: true })).toBeNull();
    expect(screen.queryByRole("button", { name: confirmedName })).toBeNull();
    expect(screen.queryByRole("button", { name: reconcileName })).toBeNull();
    expect(lost.retry).toHaveBeenCalledOnce();
    expect(f.update).toHaveBeenCalledOnce();
  },
);

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

it.each([
  {
    kind: "utility" as const,
    title: "Utility model",
    option: "Use Runtime default",
    value: {},
  },
  {
    kind: "embedding" as const,
    title: "Embedding model",
    option: "embed-default",
    value: { provider: provider.id, model: "embed-default" },
  },
])(
  "keeps the focused $kind recovery and its independent credential draft",
  async ({ kind, title, option, value }) => {
    const { f } = await mount();
    const setter = kind === "utility" ? f.setUtilityRole : f.setEmbeddingRole;
    const original = setter.getMockImplementation()!;
    const settled = Promise.withResolvers<void>();
    const retry = vi.fn();
    setter.mockImplementationOnce((input) => {
      retry.mockImplementationOnce(() =>
        mutation(settled.promise.then(() => original(input))),
      );
      return mutation(
        Promise.reject(new RpcTransportError("role response lost")),
        retry,
      );
    });
    fireEvent.change(keyInput(), { target: { value: "independent-draft" } });
    const trigger = await screen.findByRole("combobox", { name: title });
    fireEvent.click(trigger);
    const picked = await screen.findByRole("option", {
      name: new RegExp(option),
    });
    fireEvent.pointerDown(picked, { pointerType: "mouse" });
    fireEvent.click(picked);
    const reconcile = await screen.findByRole("button", {
      name: reconcileName,
    });
    act(() => reconcile.focus());
    fireEvent.click(reconcile);
    expect(document.activeElement).toBe(reconcile);
    expect(reconcile.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByRole("button", { name: reconcileName })).toBe(reconcile);
    attempt(reconcile);
    expect(retry).toHaveBeenCalledOnce();

    await act(async () => settled.resolve());
    await waitFor(() => expect(trigger.textContent).toContain(option));
    expect(screen.getByRole("button", { name: confirmedName })).toBe(reconcile);
    expect(document.activeElement).toBe(reconcile);
    expect(reconcile.getAttribute("aria-disabled")).toBe("true");
    expect(trigger.hasAttribute("disabled")).toBe(false);
    attempt(reconcile);
    expect(retry).toHaveBeenCalledOnce();
    expect(setter).toHaveBeenCalledExactlyOnceWith(value);
    expect(f.update).not.toHaveBeenCalled();
    expect(keyInput().value).toBe("independent-draft");
    act(() => keyInput().focus());
    expect(screen.queryByRole("button", { name: confirmedName })).toBeNull();
    expect(document.activeElement).toBe(keyInput());
  },
);

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

it("keeps the provider's route visible when the endpoint is populated and while it changes", async () => {
  const f = create([
    {
      ...provider,
      baseUrl: "https://gateway.example/tenant",
      endpoint: {
        example: "https://example.invalid/mount",
        appendsPath: "/v1",
      },
    },
  ]);
  render(<f.Component context={f.context} />);
  const input = await screen.findByLabelText(endpointLabel);
  expect(
    await screen.findByText(/resulting API path is \/tenant\/v1/),
  ).toBeTruthy();
  fireEvent.change(input, {
    target: { value: "https://gateway.example/tenant/v1" },
  });
  expect(
    await screen.findByText(/resulting API path is \/tenant\/v1\/v1/),
  ).toBeTruthy();
});
