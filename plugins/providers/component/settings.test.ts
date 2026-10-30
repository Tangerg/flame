import { afterEach, expect, it, vi } from "vitest";
import { RpcTransportError } from "@flame/runtime-client";
import type { Provider } from "@flame/runtime-client/wire";
import { ProviderCredentialsDraft } from "./draft";
import { providerResource, ProviderSettings } from "./settings";
import { fixture, mutation, provider } from "./testing";

const fixtures: ReturnType<typeof fixture>[] = [];
function create() {
  const f = fixture();
  fixtures.push(f);
  const settings = new ProviderSettings({
    runtimeIdentity: f.context.runtime,
    component: f.context.component,
    signal: f.source.signal,
  });
  return { ...f, settings };
}
afterEach(() => {
  for (const f of fixtures.splice(0)) f.dispose();
});

it("keeps source drafts through view closure and clears them when that source retires", () => {
  const f = create();
  const settings = f.settings;
  const draft =
    ProviderCredentialsDraft.initial().withAPIKey("synthetic-original");
  settings.edit(f.context, provider, draft);
  f.view.abort();
  const reopened = f.openView();
  expect(settings.getSnapshot().drafts.get(provider.id)?.apiKey).toBe(
    "synthetic-original",
  );
  f.source.abort();
  expect(settings.getSnapshot().drafts.size).toBe(0);
  expect(() => settings.edit(reopened.context, provider, draft)).toThrow();
  expect(() => settings.save(reopened.context, provider, draft)).toThrow();
  const nextSource = new AbortController();
  const next = f.openView(nextSource.signal);
  const nextSettings = new ProviderSettings({
    runtimeIdentity: next.context.runtime,
    component: next.context.component,
    signal: nextSource.signal,
  });
  expect(nextSettings.getSnapshot().drafts.size).toBe(0);
  nextSource.abort();
  expect(f.update).not.toHaveBeenCalled();
});

it("isolates source state by Runtime and exact component identity", () => {
  const f = create();
  const other = create();
  const settings = f.settings;
  settings.edit(
    f.context,
    provider,
    ProviderCredentialsDraft.initial().withAPIKey("private-draft"),
  );
  expect(other.settings.getSnapshot().drafts.size).toBe(0);
  const component = {
    ...f.context.component,
    componentId: "another-component",
  };
  expect(() =>
    settings.edit(
      { ...f.context, component },
      provider,
      ProviderCredentialsDraft.initial(),
    ),
  ).toThrow("another installed source");
});

it("fences older reads and keeps the accepted command response when refresh fails", async () => {
  const f = create();
  const settings = f.settings;
  settings.refreshRead(f.context, "providers");
  await vi.waitFor(() =>
    expect(settings.getSnapshot().providers.data).toEqual([provider]),
  );
  const stale = Promise.withResolvers<{ data: Provider[] }>();
  f.list
    .mockReturnValueOnce(stale.promise)
    .mockRejectedValue(new Error("refresh unavailable"));
  settings.refreshRead(f.context, "providers");
  const submitted = ProviderCredentialsDraft.initial()
    .withAPIKey("submitted-secret")
    .withBaseURL("https://entered.example.test");
  settings.edit(f.context, provider, submitted);
  const saved = {
    ...provider,
    baseUrl: "https://canonical.example.test/v1",
    credential: { masked: "synthetic-mask", source: "stored" as const },
  };
  f.update.mockReturnValueOnce(mutation(Promise.resolve(saved)));
  await expect(settings.save(f.context, provider, submitted)).resolves.toBe(
    true,
  );
  await vi.waitFor(() =>
    expect(settings.getSnapshot().providers).toMatchObject({
      phase: "failed",
      error: expect.any(Error),
    }),
  );
  stale.resolve({ data: [provider] });
  await Promise.resolve();
  expect(settings.getSnapshot().providers.data).toEqual([saved]);
  expect(settings.getSnapshot().drafts.size).toBe(0);
  expect(
    settings.getSnapshot().feedback.get(providerResource(provider.id)),
  ).toEqual({ kind: "saved" });
});

it("settles submitted fields while retaining later edits and suppressing stale feedback", async () => {
  const f = create();
  const settings = f.settings;
  const submitted =
    ProviderCredentialsDraft.initial().withAPIKey("submitted-secret");
  settings.edit(f.context, provider, submitted);
  const response = Promise.withResolvers<Provider>();
  f.update.mockReturnValueOnce(mutation(response.promise));
  const saving = settings.save(f.context, provider, submitted);
  settings.edit(
    f.context,
    provider,
    submitted.withBaseURL("https://later.example.test"),
  );
  response.resolve(provider);
  await saving;
  const retained = settings.getSnapshot().drafts.get(provider.id)!;
  expect(retained.apiKey).toBe("");
  expect(retained.baseURL(provider)).toBe("https://later.example.test");
  expect(
    settings.getSnapshot().feedback.has(providerResource(provider.id)),
  ).toBe(false);
});

it("retains the original unknown command across views and only reconciles its original promise", async () => {
  const f = create();
  const settings = f.settings;
  const submitted =
    ProviderCredentialsDraft.initial().withAPIKey("original-secret");
  settings.edit(f.context, provider, submitted);
  const saved = { ...provider, configured: true };
  const retry = vi
    .fn()
    .mockImplementationOnce(() =>
      mutation(Promise.reject(new RpcTransportError("lookup unavailable"))),
    )
    .mockImplementationOnce(() => mutation(Promise.resolve(saved)));
  const original = mutation(
    Promise.reject(new RpcTransportError("response lost")),
    retry,
  );
  f.update.mockReturnValueOnce(original);
  expect(await settings.save(f.context, provider, submitted)).toBe(false);
  const resource = providerResource(provider.id);
  expect(settings.getSnapshot().commands.get(resource)?.phase).toBe("unknown");
  expect(f.tracked).toContain(original);
  await settings.save(f.context, provider, submitted);
  expect(f.update).toHaveBeenCalledOnce();
  f.view.abort();
  const next = f.openView();
  const reopened = settings;
  await reopened.recover(next.context, resource);
  expect(reopened.getSnapshot().commands.get(resource)?.phase).toBe("unknown");
  await reopened.recover(next.context, resource);
  expect(reopened.getSnapshot().commands.has(resource)).toBe(false);
  expect(reopened.getSnapshot().drafts.size).toBe(0);
  expect(f.update).toHaveBeenCalledExactlyOnceWith({
    provider: provider.id,
    apiKey: { type: "set", value: "original-secret" },
  });
  expect(retry).toHaveBeenCalledTimes(2);
});

it("keeps an interrupted receipt lookup unknown until the next view reconciles it", async () => {
  const f = create();
  const settings = f.settings;
  const submitted =
    ProviderCredentialsDraft.initial().withAPIKey("original-secret");
  settings.edit(f.context, provider, submitted);
  const lookup = Promise.withResolvers<Provider>();
  const retry = vi
    .fn()
    .mockReturnValueOnce(mutation(lookup.promise))
    .mockReturnValueOnce(mutation(Promise.resolve(provider)));
  f.update.mockReturnValueOnce(
    mutation(Promise.reject(new RpcTransportError("response lost")), retry),
  );
  await settings.save(f.context, provider, submitted);
  const resource = providerResource(provider.id);
  const recovering = settings.recover(f.context, resource);
  f.view.abort();
  lookup.reject(f.view.signal.reason);
  await recovering;
  expect(settings.getSnapshot().commands.get(resource)?.phase).toBe("unknown");
  const next = f.openView();
  await settings.recover(next.context, resource);
  expect(settings.getSnapshot().commands.has(resource)).toBe(false);
  expect(f.update).toHaveBeenCalledOnce();
});

it("does not serialize independent role and provider resources", async () => {
  const f = create();
  const settings = f.settings;
  const utility = Promise.withResolvers<{}>();
  f.setUtilityRole.mockReturnValueOnce(mutation(utility.promise));
  const pending = settings.setRole(f.context, "utility", {});
  expect(await settings.setRole(f.context, "embedding", {})).toBe(true);
  expect(f.setEmbeddingRole).toHaveBeenCalledOnce();
  await settings.setRole(f.context, "utility", {});
  expect(f.setUtilityRole).toHaveBeenCalledOnce();
  utility.resolve({});
  await pending;
});

it("discards late mutations and reads after source retirement without touching another Runtime", async () => {
  const f = create();
  const next = create();
  const settings = f.settings;
  const submitted = ProviderCredentialsDraft.initial().withAPIKey("old-secret");
  settings.edit(f.context, provider, submitted);
  const response = Promise.withResolvers<Provider>();
  f.update.mockReturnValueOnce(mutation(response.promise));
  const pending = settings.save(f.context, provider, submitted);
  f.source.abort();
  const successor = next.settings;
  response.resolve(provider);
  expect(await pending).toBe(false);
  expect(settings.getSnapshot().drafts.size).toBe(0);
  expect(successor.getSnapshot().drafts.size).toBe(0);
  expect(next.update).not.toHaveBeenCalled();
  expect(next.list).not.toHaveBeenCalled();
});

it("refuses concurrent writes while a credential probe is using that resource", async () => {
  const f = create();
  const settings = f.settings;
  const response = Promise.withResolvers<{ outcome: "reachable" }>();
  f.test.mockReturnValueOnce(response.promise);
  const probing = settings.test(
    f.context,
    provider,
    ProviderCredentialsDraft.initial(),
  );
  const draft = ProviderCredentialsDraft.initial().withAPIKey("later-key");
  expect(await settings.save(f.context, provider, draft)).toBe(false);
  expect(await settings.clearKey(f.context, provider.id)).toBe(false);
  expect(f.update).not.toHaveBeenCalled();
  response.resolve({ outcome: "reachable" });
  await probing;
});
