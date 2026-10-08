import { z } from "zod";
import { validateWire } from "@flame/runtime-contract/validate";
import type { PluginCarrier, PluginPage } from "@/foundation/pluginCarrier";
import type { TrajectoryViewReads } from "../application/trajectoryView";

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

export async function mountTrajectoryFrame({
  container,
  reads,
  carrier,
  signal,
  fail,
}: {
  container: HTMLElement;
  reads: TrajectoryViewReads;
  carrier: PluginCarrier;
  signal: AbortSignal;
  fail: (reason: string) => void;
}): Promise<() => Promise<void>> {
  const lifetime = new AbortController();
  const owned = AbortSignal.any([signal, lifetime.signal]);
  const initialized = Promise.withResolvers<void>();
  const settled = initialized.promise.then(
    () => ({ error: undefined }),
    (error) => ({ error }),
  );
  let page: PluginPage | undefined;
  let reading: Promise<void> | undefined;
  let connected = false;
  let ready = false;
  let boot: (() => Promise<void>) | undefined;
  let closing: Promise<void> | undefined;
  const close = () =>
    (closing ??= (async () => {
      lifetime.abort();
      clearTimeout(deadline);
      signal.removeEventListener("abort", retire);
      initialized.reject(new Error("Plugin page retired."));
      await page?.close();
      await reading;
    })());
  const reject = (reason: string) => {
    if (owned.aborted) return;
    initialized.reject(new Error(reason));
    fail(reason);
    void close().catch(console.error);
  };
  const retire = () => {
    void close().catch(console.error);
  };
  const deadline = setTimeout(() => reject("The plugin did not initialize."), 10000);
  signal.addEventListener("abort", retire, { once: true });
  try {
    page = await carrier.open(container, owned, (value) => {
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
        if (ready) {
          reject("The plugin carrier repeated initialization.");
          return;
        }
        ready = true;
        void boot?.().catch((error) => reject(String(error)));
        return;
      }
      if (message.type === "connected") {
        if (!ready || connected) {
          reject("Invalid plugin initialization.");
          return;
        }
        connected = true;
        clearTimeout(deadline);
        initialized.resolve();
        return;
      }
      if (!connected) {
        reject("The plugin requested data before initialization.");
        return;
      }
      if (reading) {
        reject("The plugin already has a pending read.");
        return;
      }
      reading = (async () => {
        try {
          const result = await reads.read(message.request.cursor, owned);
          if (!owned.aborted)
            await page?.send({
              type: "reply",
              reply: { type: "page", page: result, cursor: message.request.cursor },
            });
        } catch {
          if (!owned.aborted)
            await page?.send({
              type: "reply",
              reply: { type: "error", reason: "The trajectory read failed. Refresh to try again." },
            });
        } finally {
          reading = undefined;
        }
      })();
      void reading.catch(() => reject("The plugin channel could not publish its result."));
    });
    if (owned.aborted) {
      await page.close();
      owned.throwIfAborted();
    }
    const { html, initial } = await reads.load(owned);
    boot = async () => {
      if (!owned.aborted) await page?.send({ type: "boot", html, initial });
    };
    if (ready) await boot();
    const result = await settled;
    if (result.error) throw result.error;
    return close;
  } catch (error) {
    await close();
    throw error;
  }
}
