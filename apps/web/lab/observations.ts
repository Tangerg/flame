import { z } from "zod";
import type { ObservationValue } from "./story";

export const OBSERVATION_CHANNEL = "flame.component-lab.observation";
export const OBSERVATION_VERSION = 1;
export const OBSERVATION_LIMITS = {
  instanceIdLength: 128,
  nameLength: 120,
  detailLength: 512,
  entries: 200,
} as const;
export const OBSERVATION_NAMES = {
  ready: "preview.ready",
  windowError: "window.error",
  unhandledRejection: "window.unhandledrejection",
  reactError: "react.error",
} as const;

const instanceIdSchema = z
  .string()
  .min(1)
  .max(OBSERVATION_LIMITS.instanceIdLength)
  .regex(/^[a-zA-Z0-9][a-zA-Z0-9._:-]*$/);
const messageSchema = z.strictObject({
  channel: z.literal(OBSERVATION_CHANNEL),
  version: z.literal(OBSERVATION_VERSION),
  kind: z.enum(["ready", "event", "error"]),
  instanceId: instanceIdSchema,
  name: z
    .string()
    .min(1)
    .max(OBSERVATION_LIMITS.nameLength)
    .refine((value) => value.trim() === value && !/\p{Cc}/u.test(value)),
  detail: z.string().max(OBSERVATION_LIMITS.detailLength),
});

export type ObservationMessage = Readonly<z.infer<typeof messageSchema>>;
export type ObservationEntry = ObservationMessage & {
  readonly sequence: number;
  readonly receivedAt: number;
};

// The receiver must also bind the message to its current iframe source, origin and instance.
export function parseObservationMessage(value: unknown): ObservationMessage | null {
  const result = messageSchema.safeParse(value);
  return result.success ? Object.freeze(result.data) : null;
}

export class ObservationLog {
  #entries: readonly ObservationEntry[] = Object.freeze([]);
  #sequence = 0;
  readonly #listeners = new Set<() => void>();

  readonly snapshot = (): readonly ObservationEntry[] => this.#entries;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  append(value: unknown, receivedAt: number): boolean {
    const message = parseObservationMessage(value);
    if (!message || !Number.isFinite(receivedAt) || receivedAt < 0) return false;
    const entry = Object.freeze({ ...message, receivedAt, sequence: ++this.#sequence });
    this.#entries = Object.freeze([
      ...this.#entries.slice(-(OBSERVATION_LIMITS.entries - 1)),
      entry,
    ]);
    this.#notify();
    return true;
  }

  clear(): void {
    if (this.#entries.length === 0) return;
    this.#entries = Object.freeze([]);
    this.#notify();
  }

  #notify(): void {
    for (const listener of this.#listeners) listener();
  }
}

const sensitiveField =
  /password|passphrase|passcode|secret|token|credential|api[\s_-]?key|authorization|cookie|one-time-code/i;
const redactedDetail = "[redacted]";
const interactiveTarget =
  "button, a[href], input, select, textarea, [role='button'], [role='checkbox'], [role='radio'], [role='switch'], [role='tab'], [role='menuitem'], [role='option'], [contenteditable='true']";

function boundedText(value: string, limit: number): string {
  return value.length <= limit ? value : `${value.slice(0, limit - 1)}…`;
}

function observationName(value: string): string {
  return boundedText(value.replace(/\p{Cc}/gu, " ").trim(), OBSERVATION_LIMITS.nameLength);
}

function sensitive(element: Element): boolean {
  return ["type", "name", "id", "autocomplete", "aria-label"].some((attribute) =>
    sensitiveField.test(element.getAttribute(attribute) ?? ""),
  );
}

function targetElement(event: Event): Element | null {
  const target = event.composedPath()[0] ?? event.target;
  if (!(target instanceof Node)) return null;
  const element = target instanceof Element ? target : target.parentElement;
  return element?.closest(interactiveTarget) ?? null;
}

function targetName(element: Element): string {
  if (sensitive(element)) return `${element.localName} [sensitive]`;
  const label = element.getAttribute("aria-label");
  if (label) return observationName(label);
  const labelledBy = element.getAttribute("aria-labelledby");
  if (labelledBy) {
    const firstLabel = labelledBy.trim().split(/\s+/)[0];
    const text = firstLabel ? element.ownerDocument.getElementById(firstLabel)?.textContent : null;
    if (text?.trim()) return observationName(text);
  }
  if (element.matches("input, select, textarea")) {
    const control = element as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;
    const text = control.labels?.[0]?.textContent;
    return observationName(text?.trim() || control.name || control.id || element.localName);
  }
  return observationName(
    element.textContent?.trim() || element.getAttribute("role") || element.localName,
  );
}

// Text drafts and credential-shaped controls must never become an inspector's second data store.
function controlDetail(element: Element): string {
  if (sensitive(element)) return redactedDetail;
  if (element.localName === "input") {
    const input = element as HTMLInputElement;
    if (input.type === "checkbox" || input.type === "radio") return `checked: ${input.checked}`;
    if (input.type === "file") return `files: ${input.files?.length ?? 0}`;
    return `text length: ${input.value.length}`;
  }
  if (element.localName === "textarea") {
    return `text length: ${(element as HTMLTextAreaElement).value.length}`;
  }
  if (element.localName === "select") {
    return `selected index: ${(element as HTMLSelectElement).selectedIndex}`;
  }
  const checked = element.getAttribute("aria-checked");
  if (checked === "true" || checked === "false" || checked === "mixed")
    return `checked: ${checked}`;
  if (element.getAttribute("contenteditable") === "true") {
    return `text length: ${element.textContent?.length ?? 0}`;
  }
  return element.localName;
}

function valueDetail(value: ObservationValue | undefined): string | null {
  if (value === undefined) return "";
  if (value === null) return "null";
  if (typeof value === "string") return boundedText(value, OBSERVATION_LIMITS.detailLength);
  if (typeof value === "boolean") return String(value);
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  return null;
}

function errorDetail(value: unknown): string {
  if (value instanceof Error) return boundedText(value.message, OBSERVATION_LIMITS.detailLength);
  if (typeof value === "string") return boundedText(value, OBSERVATION_LIMITS.detailLength);
  return "non-text rejection";
}

class PreviewObservations {
  #state: "observing" | "ready" | "failed" | "disposed" = "observing";

  constructor(
    private readonly view: Window,
    private readonly document: Document,
    private readonly parent: Window,
    private readonly origin: string,
    private readonly instanceId: string,
  ) {
    document.addEventListener("click", this.#onInteraction, true);
    document.addEventListener("change", this.#onInteraction, true);
    view.addEventListener("error", this.#onError);
    view.addEventListener("unhandledrejection", this.#onRejection);
    view.addEventListener("pagehide", this.dispose);
  }

  readonly observe = (name: string, value?: ObservationValue): void => {
    const detail = valueDetail(value);
    if (detail === null) return;
    this.#send(
      name === OBSERVATION_NAMES.reactError ? "error" : "event",
      name,
      sensitiveField.test(name) ? redactedDetail : detail,
    );
  };

  readonly ready = (): void => {
    if (this.#state !== "observing") return;
    this.#state = "ready";
    this.document.documentElement.dataset.labReady = "true";
    this.#send("ready", OBSERVATION_NAMES.ready, "");
  };

  readonly dispose = (): void => {
    if (this.#state === "disposed") return;
    this.#state = "disposed";
    delete this.document.documentElement.dataset.labReady;
    this.document.removeEventListener("click", this.#onInteraction, true);
    this.document.removeEventListener("change", this.#onInteraction, true);
    this.view.removeEventListener("error", this.#onError);
    this.view.removeEventListener("unhandledrejection", this.#onRejection);
    this.view.removeEventListener("pagehide", this.dispose);
  };

  readonly #onInteraction = (event: Event): void => {
    const element = targetElement(event);
    if (element)
      this.#send("event", `${event.type}: ${targetName(element)}`, controlDetail(element));
  };

  readonly #onError = (event: ErrorEvent): void => {
    this.#send("error", OBSERVATION_NAMES.windowError, errorDetail(event.message));
  };

  readonly #onRejection = (event: PromiseRejectionEvent): void => {
    this.#send("error", OBSERVATION_NAMES.unhandledRejection, errorDetail(event.reason));
  };

  #send(kind: ObservationMessage["kind"], name: string, detail: string): void {
    if (this.#state === "disposed") return;
    if (kind === "error") {
      this.#state = "failed";
      delete this.document.documentElement.dataset.labReady;
    }
    if (this.parent === this.view) return;
    const normalizedName = observationName(name);
    if (!normalizedName) return;
    const message: ObservationMessage = {
      channel: OBSERVATION_CHANNEL,
      version: OBSERVATION_VERSION,
      kind,
      instanceId: this.instanceId,
      name: normalizedName,
      detail,
    };
    this.parent.postMessage(message, this.origin);
  }
}

export function installPreviewObservations(
  view: Window,
  document: Document,
  options: { readonly instanceId: string },
): PreviewObservations {
  const instance = instanceIdSchema.safeParse(options.instanceId);
  if (!instance.success) throw new Error("invalid preview observation instance");
  if (document.defaultView !== view)
    throw new Error("preview observation document does not match its window");
  const origin = view.location.origin;
  if (view.location.protocol !== "http:" && view.location.protocol !== "https:") {
    throw new Error("preview observations require an HTTP origin");
  }
  const observations = new PreviewObservations(view, document, view.parent, origin, instance.data);
  import.meta.hot?.dispose(observations.dispose);
  return observations;
}
