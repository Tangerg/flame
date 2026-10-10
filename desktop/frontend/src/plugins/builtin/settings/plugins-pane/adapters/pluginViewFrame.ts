import { z } from "zod";
import { validateWire } from "@flame/runtime-contract/validate";
import type { PluginCarrier, PluginPage } from "@/foundation/pluginCarrier";
import type { PluginViewReads, PluginViewStatus } from "../application/pluginView";
import type { Scheme } from "@/lib/appearance";

const incoming = z.discriminatedUnion("type", [
  z.strictObject({ type: z.literal("ready") }),
  z.strictObject({ type: z.literal("connected") }),
  z.strictObject({ type: z.literal("failure"), reason: z.string().max(1024) }),
  z.strictObject({
    type: z.literal("request"),
    request: z.strictObject({
      type: z.literal("read"),
      cursor: z
        .string()
        .refine((cursor) => validateWire("PageQuery", { cursor }).length === 0)
        .optional(),
    }),
  }),
]);

export function mountPluginViewFrame<T>({
  container,
  reads,
  carrier,
  signal,
  status,
  scheme,
}: {
  container: HTMLElement;
  reads: PluginViewReads<T>;
  carrier: PluginCarrier;
  signal: AbortSignal;
  status: (value: PluginViewStatus) => void;
  scheme: Scheme;
}): () => Promise<void> {
  const lifetime = new AbortController();
  const owned = AbortSignal.any([signal, lifetime.signal]);
  const openingSettled = Promise.withResolvers<void>();
  let phase:
    | { type: "opening" | "qualified" | "retired" }
    | { type: "booting" | "connected"; page: PluginPage } = { type: "opening" };
  let reading: Promise<void> | undefined;
  let closing: Promise<void> | undefined;
  const close = () =>
    (closing ??= (async () => {
      phase = { type: "retired" };
      lifetime.abort();
      clearTimeout(deadline);
      signal.removeEventListener("abort", retire);
      openingSettled.resolve();
      const results = await Promise.allSettled([
        allocation.then(
          (page) => page.close(),
          () => undefined,
        ),
        startup,
        reading,
      ]);
      const errors = results.flatMap((result) =>
        result.status === "rejected" ? [result.reason] : [],
      );
      if (errors.length) throw new AggregateError(errors, "The plugin page could not be retired.");
    })());
  const reject = (reason: string) => {
    if (owned.aborted) return;
    status({ type: "failure", reason });
    void close().catch(console.error);
  };
  const retire = () => {
    void close().catch(console.error);
  };
  const deadline = setTimeout(() => reject("The plugin did not initialize."), 10000);
  const allocation = Promise.resolve().then(() => {
    owned.throwIfAborted();
    return carrier.open(container, owned, (value) => {
      if (owned.aborted) return;
      const parsed = incoming.safeParse(value);
      if (!parsed.success) {
        reject("The plugin requested an unsupported operation.");
        return;
      }
      const message = parsed.data;
      if (message.type === "failure") {
        reject(message.reason);
        return;
      }
      if (message.type === "ready") {
        if (phase.type !== "opening") {
          reject("The plugin carrier repeated initialization.");
          return;
        }
        phase = { type: "qualified" };
        openingSettled.resolve();
        return;
      }
      if (message.type === "connected") {
        if (phase.type !== "booting") {
          reject("Invalid plugin initialization.");
          return;
        }
        phase = { type: "connected", page: phase.page };
        clearTimeout(deadline);
        status({ type: "ready" });
        return;
      }
      if (phase.type !== "connected") {
        reject("The plugin requested data before initialization.");
        return;
      }
      if (reading) {
        reject("The plugin already has a pending read.");
        return;
      }
      const page = phase.page;
      reading = (async () => {
        let reply;
        try {
          const result = await reads.read(message.request.cursor, owned);
          reply = { type: "page", page: result, cursor: message.request.cursor };
        } catch {
          reply = { type: "error", reason: "The plugin page read failed. Refresh to try again." };
        }
        if (!owned.aborted) await page.send({ type: "reply", reply });
      })()
        .catch(() => reject("The plugin channel could not publish its result."))
        .finally(() => {
          reading = undefined;
        });
    });
  });
  const startup = (async () => {
    const page = await allocation;
    owned.throwIfAborted();
    await openingSettled.promise;
    owned.throwIfAborted();
    const { html, initial } = await reads.load(owned);
    owned.throwIfAborted();
    phase = { type: "booting", page };
    await page.send({ type: "boot", html, initial, scheme });
  })().catch((error) => {
    reject(error instanceof Error ? error.message : "The plugin page could not be opened.");
  });
  signal.addEventListener("abort", retire, { once: true });
  if (owned.aborted) retire();
  else status({ type: "loading" });
  return close;
}
