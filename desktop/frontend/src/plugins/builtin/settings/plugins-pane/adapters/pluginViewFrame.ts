import { z } from "zod";
import { validateWire } from "@flame/runtime-contract/validate";
import type { PluginCarrier, PluginPage } from "@/foundation/pluginCarrier";
import type { PluginViewReads, PluginViewStatus, PluginViewStage } from "../application/pluginView";
import type { Scheme } from "@/lib/appearance";
import { reportViewFailure } from "./pluginViewDiagnostics";

const incoming = z.discriminatedUnion("type", [
  z.strictObject({ type: z.literal("ready") }),
  z.strictObject({ type: z.literal("connected") }),
  z.strictObject({ type: z.literal("failure"), reason: z.string().max(1024) }),
  z.strictObject({
    type: z.literal("request"),
    request: z.discriminatedUnion("type", [
      z.strictObject({
        type: z.literal("read"),
        cursor: z
          .string()
          .refine((cursor) => validateWire("PageQuery", { cursor }).length === 0)
          .optional(),
      }),
      z.strictObject({ type: z.literal("failure"), reason: z.string().max(1024) }),
    ]),
  }),
]);
type ReadRequest = { type: "read"; cursor?: string };
type FrameConfig<T> = {
  container: HTMLElement;
  reads: PluginViewReads<T>;
  carrier: PluginCarrier;
  signal: AbortSignal;
  status: (value: PluginViewStatus) => void;
  scheme: Scheme;
};
const reasons = {
  carrier: "The plugin carrier could not be opened.",
  resource: "The plugin resource could not be read.",
  initial: "The plugin's initial data could not be read.",
  publication: "The plugin channel could not publish its result.",
};

class PluginViewFrame<T> {
  private readonly lifetime = new AbortController();
  private readonly owned: AbortSignal;
  private readonly qualification = Promise.withResolvers<void>();
  private readonly allocation: Promise<PluginPage>;
  private readonly startup: Promise<void>;
  private phase:
    | { type: "opening" | "qualified" | "retired" }
    | { type: "booting" | "connected"; page: PluginPage } = { type: "opening" };
  // Delivery can precede acknowledgement; one successor may wait behind that publication.
  private activeRead: { type: "reading" } | { type: "publishing"; next?: ReadRequest } | undefined;
  private reading: Promise<void> | undefined;
  private closing: Promise<void> | undefined;
  private deadline: ReturnType<typeof setTimeout> | undefined;

  constructor(private readonly config: FrameConfig<T>) {
    this.owned = AbortSignal.any([config.signal, this.lifetime.signal]);
    this.allocation = Promise.resolve().then(() => {
      this.owned.throwIfAborted();
      return config.carrier.open(config.container, this.owned, this.receive);
    });
    this.startup = Promise.resolve()
      .then(() => this.start())
      .catch((error) => this.fail("carrier", reasons.carrier, error));
    config.signal.addEventListener("abort", this.retire, { once: true });
    if (this.owned.aborted) this.retire();
    else {
      this.deadline = setTimeout(
        () => this.fail("carrier", "The plugin carrier did not initialize."),
        10000,
      );
      config.status({ type: "loading" });
    }
  }

  close = (): Promise<void> => (this.closing ??= this.shutdown());
  private retire = (): void => {
    void this.close().catch((error) =>
      reportViewFailure(this.config.reads.binding, "retirement", error),
    );
  };

  private async shutdown(): Promise<void> {
    this.phase = { type: "retired" };
    this.lifetime.abort();
    clearTimeout(this.deadline);
    this.config.signal.removeEventListener("abort", this.retire);
    this.qualification.resolve();
    const results = await Promise.allSettled([
      this.allocation.then(
        (page) => page.close(),
        () => undefined,
      ),
      this.startup,
      this.reading,
    ]);
    const errors = results.flatMap((result) =>
      result.status === "rejected" ? [result.reason] : [],
    );
    if (errors.length) throw new AggregateError(errors, "The plugin page could not be retired.");
  }

  private fail(stage: PluginViewStage, reason: string, cause: unknown = new Error(reason)): void {
    if (this.owned.aborted) return;
    reportViewFailure(this.config.reads.binding, stage, cause);
    this.config.status({ type: "failure", stage, reason });
    this.retire();
  }

  private receive = (value: unknown): void => {
    if (this.owned.aborted) return;
    const parsed = incoming.safeParse(value);
    if (!parsed.success) {
      this.fail("protocol", "The plugin requested an unsupported operation.");
      return;
    }
    const message = parsed.data;
    switch (message.type) {
      case "failure":
        this.fail("carrier", message.reason);
        return;
      case "ready":
        if (this.phase.type !== "opening") {
          this.fail("protocol", "The plugin carrier repeated initialization.");
          return;
        }
        this.phase = { type: "qualified" };
        this.qualification.resolve();
        return;
      case "connected":
        if (this.phase.type !== "booting") {
          this.fail("protocol", "Invalid plugin initialization.");
          return;
        }
        this.phase = { type: "connected", page: this.phase.page };
        clearTimeout(this.deadline);
        this.config.status({ type: "ready" });
        return;
      case "request":
        if (this.phase.type !== "connected") {
          this.fail("protocol", "The plugin requested data before initialization.");
          return;
        }
        if (message.request.type === "failure") {
          this.fail("guest", message.request.reason);
          return;
        }
        this.admitRead(message.request, this.phase.page);
    }
  };

  private admitRead(request: ReadRequest, page: PluginPage): void {
    if (this.activeRead) {
      if (this.activeRead.type === "publishing" && !this.activeRead.next) {
        this.activeRead.next = request;
        return;
      }
      this.fail("protocol", "The plugin already has a pending read.");
      return;
    }
    this.activeRead = { type: "reading" };
    const job = this.processReads(request, page)
      .catch((error) => this.fail("publication", reasons.publication, error))
      .finally(() => {
        if (this.reading === job) {
          this.reading = undefined;
          this.activeRead = undefined;
        }
      });
    this.reading = job;
  }

  private async processReads(request: ReadRequest, page: PluginPage): Promise<void> {
    let current: ReadRequest | undefined = request;
    while (current && !this.owned.aborted) {
      this.activeRead = { type: "reading" };
      let reply;
      try {
        reply = {
          type: "data",
          data: await this.config.reads.read(current.cursor, this.owned),
          cursor: current.cursor,
        };
      } catch (error) {
        if (this.owned.aborted) return;
        reportViewFailure(this.config.reads.binding, "read", error);
        reply = { type: "error", reason: "The plugin page read failed. Refresh to try again." };
      }
      if (this.owned.aborted) return;
      const publication: { type: "publishing"; next?: ReadRequest } = { type: "publishing" };
      this.activeRead = publication;
      await page.send({ type: "reply", reply });
      current = publication.next;
    }
    this.activeRead = undefined;
  }

  private async stage<R>(stage: keyof typeof reasons, operation: () => Promise<R>): Promise<R> {
    try {
      return await operation();
    } catch (error) {
      this.fail(stage, reasons[stage], error);
      throw error;
    }
  }

  private async start(): Promise<void> {
    const page = await this.allocation;
    this.owned.throwIfAborted();
    await this.qualification.promise;
    this.owned.throwIfAborted();
    clearTimeout(this.deadline);
    const tasks = [
      this.stage("resource", () => this.config.reads.resource(this.owned)),
      this.stage("initial", () => this.config.reads.initial(this.owned)),
    ] as const;
    let loaded: [string, T];
    try {
      loaded = await Promise.all(tasks);
    } catch (error) {
      await Promise.allSettled(tasks);
      throw error;
    }
    this.owned.throwIfAborted();
    this.phase = { type: "booting", page };
    await this.stage("publication", () =>
      page.send({ type: "boot", html: loaded[0], initial: loaded[1], scheme: this.config.scheme }),
    );
    if (this.phase.type === "booting" && !this.owned.aborted)
      this.deadline = setTimeout(
        () => this.fail("handshake", "The plugin did not initialize."),
        10000,
      );
  }
}

export function mountPluginViewFrame<T>(config: FrameConfig<T>): () => Promise<void> {
  return new PluginViewFrame(config).close;
}
