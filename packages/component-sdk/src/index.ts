import type { ClientTelemetry } from "./telemetry";
export { OperationFailure } from "@flame/runtime-client";
import {
  useCallback,
  useMemo,
  useState,
  type ComponentType,
  type ReactNode,
  type SetStateAction,
} from "react";
import type { FlameClient } from "@flame/runtime-client";
import type {
  AppliedChange,
  ResultShape,
  SafetyClass,
  Session,
  PluginOperationBinding,
  Target,
  WorkspaceRef,
  PluginPointReference,
  RuntimeSubscribeRequest,
} from "@flame/runtime-client/wire";
export type {
  ClientTelemetry,
  TelemetrySnapshot,
  TelemetryAttributes,
  SpanRow,
  MetricRow,
  LogRow,
} from "./telemetry";
export { mutationSettlementIsUnknown } from "@flame/runtime-client";
export { isErrorType } from "@flame/runtime-client";
export { scrollPositions, type ScrollPosition } from "./scroll";
export { failureText } from "./errorText";
export { CursorPager, type PageDirection } from "./cursorPager";
export { CommandActivity } from "./commandActivity";
export { bindOperations } from "./operations";

// The active Session's Project. Project-scoped plugins key their records on it
// rather than on the Session's folder.
export interface CapturedProject {
  readonly id: string;
  readonly name: string;
}

export interface WorkspaceContext {
  readonly ref: Readonly<WorkspaceRef>;
  readonly focus?: Readonly<{ path: string; revision: bigint }>;
}

export interface CapturedFile {
  readonly workspace?: Readonly<WorkspaceRef>;
  readonly path: string;
  readonly content: string;
  readonly startLine: number;
  readonly totalLines: number;
  readonly truncated: boolean;
}

export interface WorkspaceChangesActionInput {
  readonly changes: readonly {
    readonly path: string;
    readonly added?: number;
    readonly removed?: number;
    readonly binary?: boolean;
  }[];
  openView(origin?: HTMLElement): void;
}

export type RecordedFileChange = Readonly<AppliedChange>;

export interface CapturedToolSource {
  readonly modelName: string;
  readonly ref:
    | Readonly<{ type: "builtIn"; name: string }>
    | Readonly<{
        type: "mcp";
        name: string;
        serverName: string;
        installationId?: string;
      }>;
}

export interface CapturedToolResult {
  readonly source?: CapturedToolSource;
  readonly id: string;
  readonly runId: string;
  readonly name: string;
  readonly fn: string;
  readonly fnKind?: "path" | "prose";
  readonly args: string;
  readonly status: "running" | "ok" | "err" | "denied";
  readonly resultShape?: ResultShape;
  readonly changes?: readonly RecordedFileChange[];
  readonly hits?: number;
  readonly files?: number;
  readonly lines?: number;
  readonly exitCode?: number;
  readonly outputPage?: {
    readonly shellId?: string;
    readonly status?: string;
    readonly offset: number;
    readonly nextOffset: number;
    readonly hasMore: boolean;
    readonly dropped: boolean;
  };
  readonly result?: string;
  readonly command?: string;
  readonly error?: string;
  readonly operation?: string;
  readonly safetyClass?: SafetyClass;
  readonly range?: Readonly<{ start: number; end: number }>;
  readonly durationMillis?: number;
  readonly approvalDecision?: "approve" | "deny";
}

export interface CapturedMessage {
  readonly sessionId: string;
  readonly itemId: string;
  readonly runId?: string;
  readonly role: "user" | "assistant" | "system";
}

export type CapturedSession = Readonly<
  Pick<Session, "id" | "revision" | "title">
>;

export interface ToolInteractionInput {
  readonly action: string;
  readonly payloadJSON: string;
}

export type InteractionSubmission =
  | {
      readonly status: "accepted";
      readonly sessionId: string;
      readonly runId: string;
      readonly userItemId: string;
    }
  | {
      readonly status: "retired";
      readonly sessionId: string;
      readonly runId: string;
      readonly userItemId?: string;
    }
  | {
      readonly status: "unknown";
      readonly idempotencyKey: string;
      recover(): Promise<InteractionSubmission>;
    };

export interface CapturedToolInteraction {
  readonly source: Readonly<{ runId: string; itemId: string }>;
  submit(input: ToolInteractionInput): Promise<InteractionSubmission>;
}

// The Composer draft current when the context was captured. Appending only
// edits that draft; the user still owns sending it.
export interface ComposerReference {
  /** One line naming what the user pointed at. */
  readonly label: string;
  /** What the model receives: the exact object, version and position. */
  readonly text: string;
}

export interface CapturedComposer {
  // False when another Session's draft has replaced the captured one.
  addReference(reference: ComposerReference): boolean;
}

export interface SaveFileRequest {
  /** Suggested basename only; the host owns the actual destination. */
  readonly filename: string;
  readonly mime: string;
  /** The host captures these exact bytes synchronously when saving is admitted. */
  readonly bytes: Uint8Array;
}

/** Browser download dispatch is distinct from a completed native write. */
export type FileSaveResult = "requested" | "saved" | "canceled";

export interface ClientCommandInvocation {
  readonly sessionId?: string;
  readonly signal: AbortSignal;
}

export interface ClientCommand {
  readonly id: string;
  readonly label: string;
  readonly combo?: string;
  run(invocation: ClientCommandInvocation): void | Promise<void>;
}

export interface RootRunSettlement {
  readonly sessionId: string;
  readonly status: "needsInput" | "finished" | "error" | "canceled";
  readonly errorMessage: string | null;
}

import type { NotificationAuthorization } from "./notificationAuthorization.generated";
export type { NotificationAuthorization } from "./notificationAuthorization.generated";

export interface ComponentNotification {
  readonly id: string;
  readonly sessionId: string;
  readonly title: string;
  readonly body: string;
}

export interface SystemNotificationChannel {
  readonly applicationName: string;
  authorization(): NotificationAuthorization;
  subscribe(listener: () => void): () => void;
  refreshAuthorization(): Promise<NotificationAuthorization>;
  requestAuthorization(): Promise<NotificationAuthorization>;
  send(notification: ComponentNotification): Promise<boolean>;
}

export type ComponentSessionOpenOutcome = "opened" | "missing" | "superseded";

export interface ComponentContext {
  readonly component: Readonly<ComponentReference>;
  readonly file?: Readonly<CapturedFile>;
  readonly toolResult?: Readonly<CapturedToolResult>;
  readonly message?: CapturedMessage;
  readonly interaction?: CapturedToolInteraction;
  readonly composer?: CapturedComposer;
  readonly runtime: FlameClient;
  readonly telemetry: ClientTelemetry;
  readonly workspace?: Readonly<WorkspaceContext>;
  readonly project?: Readonly<CapturedProject>;
  readonly sessionId?: string;
  readonly sessionTarget?: CapturedSession;
  readonly binding?: Readonly<PluginOperationBinding>;
  readonly openSession?: (
    sessionId: string,
  ) => Promise<ComponentSessionOpenOutcome>;
  readonly views?: ComponentViews;
  // Only clientEffect placements register global commands. IDs belong to the
  // installed component. Dispatch captures the current
  // Session before asynchronous work; withdrawal refuses new dispatches.
  registerCommand(command: ClientCommand): () => void;
  // Client effects observe the Host's settled root-run facts. The captured
  // lifetime joins callbacks and refuses new observations after withdrawal.
  subscribeRootRunSettlements(
    listener: (
      settlement: RootRunSettlement,
      signal: AbortSignal,
    ) => void | Promise<void>,
  ): () => void;
  readonly notifications: SystemNotificationChannel;
  // Accepted saves are joined by this captured lifetime. They do not change any
  // plugin delivery, revision or approval fact; the host only owns file transfer.
  saveFile(request: SaveFileRequest): Promise<FileSaveResult>;
  readonly signal: AbortSignal;
  // Reserve work before starting it; preserve the original command promise and
  // its retry identity. The factory is invoked synchronously after admission.
  track<P extends Promise<unknown>>(operation: (signal: AbortSignal) => P): P;
  compose(composition: ComponentComposition): ReactNode;
}

// A plugin opens and links its own workspace views. A target addresses an
// object inside a view in terms only that view's component interprets; the
// host owns the URL that carries it.
export interface ComponentViews {
  open(componentId: string, target?: string, origin?: HTMLElement): void;
  href(componentId: string, target?: string): string;
  openRelated?(
    dependency: string,
    installationId: string,
    componentId: string,
    target?: string,
    origin?: HTMLElement,
  ): Promise<void>;
  // Each target this component's view is shown with, the current one first.
  subscribeTarget(listener: (target: string) => void): () => void;
}

export interface ComponentReference {
  readonly installationId: string;
  readonly digest: string;
  readonly componentId: string;
}

export interface ComponentComposition {
  readonly contract: string;
  readonly target: Target;
  readonly attributes: unknown;
  readonly order?: readonly PluginPointReference[];
  readonly input?: unknown;
  readonly children?: ReactNode;
}

export interface ComponentValue<Props = unknown> {
  readonly component: string;
  readonly props?: Props;
}

export interface PluginComponentProps<Input = unknown, Props = unknown> {
  context: ComponentContext;
  locale: string;
  input?: Input;
  contribution?: Props;
  children?: ReactNode;
}

export type PluginComponent<Input = unknown, Props = unknown> = ComponentType<
  PluginComponentProps<Input, Props>
>;

export interface ComponentSource {
  readonly runtimeIdentity: object;
  readonly component: Readonly<ComponentReference>;
  readonly signal: AbortSignal;
}

export interface ComponentDefinition {
  readonly kind: "flame.component.source";
  create(source: ComponentSource): PluginComponent;
}

export function defineComponent(
  create: ComponentDefinition["create"],
): ComponentDefinition {
  return Object.freeze({ kind: "flame.component.source", create });
}

export function isComponentDefinition(
  value: unknown,
): value is ComponentDefinition {
  return (
    typeof value === "object" &&
    value !== null &&
    "kind" in value &&
    value.kind === "flame.component.source" &&
    "create" in value &&
    typeof value.create === "function"
  );
}

export type Catalog = Readonly<Record<string, string>>;

export type Translate = (
  key: string,
  params?: Readonly<Record<string, string | number>>,
) => string;

// Mirrors the host i18next conventions: flat dotted keys, {{name}}
// interpolation with numbers in the locale's digits (pass a string to keep a
// value verbatim), _one/_other plural suffixes via Intl.PluralRules on
// `count`, with en as reference and fallback.
export function createTranslator(
  catalogs: Readonly<Record<string, Catalog>>,
  locale: string,
): Translate {
  const base = catalogs.en ?? {};
  const active = catalogs[locale] ?? catalogs[locale.split("-")[0]!] ?? base;
  const plural =
    typeof Intl !== "undefined" && "PluralRules" in Intl
      ? new Intl.PluralRules(locale)
      : undefined;
  const numbers =
    typeof Intl !== "undefined" ? new Intl.NumberFormat(locale) : undefined;
  return (key, params) => {
    let lookup = key;
    if (params && typeof params.count === "number") {
      const category = plural ? plural.select(params.count) : "other";
      const suffixed = `${key}_${category}`;
      lookup =
        active[suffixed] !== undefined || base[suffixed] !== undefined
          ? suffixed
          : `${key}_other`;
    }
    const template =
      active[lookup] ?? base[lookup] ?? active[key] ?? base[key] ?? key;
    return params
      ? template.replace(/\{\{(\w+)\}\}/g, (whole, name: string) => {
          if (!(name in params)) return whole;
          const value = params[name];
          return typeof value === "number" && numbers
            ? numbers.format(value)
            : String(value);
        })
      : template;
  };
}

export function useTranslator(
  catalogs: Readonly<Record<string, Catalog>>,
  locale: string,
): Translate {
  return useMemo(() => createTranslator(catalogs, locale), [catalogs, locale]);
}

// A retired callback must not overwrite state already produced by the next capture.
export function useContextState<T>(
  context: ComponentContext,
): readonly [T | undefined, (next: SetStateAction<T | undefined>) => void] {
  const { signal } = context;
  const [state, setState] = useState({ signal, value: undefined as T | undefined });
  const set = useCallback(
    (next: SetStateAction<T | undefined>) =>
      setState((previous) => {
        if (previous.signal !== signal || signal.aborted) return previous;
        return {
          signal,
          value:
            typeof next === "function"
              ? (next as (current: T | undefined) => T | undefined)(previous.value)
              : next,
        };
      }),
    [signal],
  );
  if (state.signal !== signal) {
    setState({ signal, value: undefined });
    return [undefined, set];
  }
  return [state.value, set];
}

// Calls changed once the subscription is live, so a reader catches up on what
// happened before it subscribed, and again on every event of the subscribed
// topics or a resync. Rejects when the observation fails.
export async function watchRuntimeTopics(
  runtime: Pick<FlameClient, "runtimeEvents">,
  request: RuntimeSubscribeRequest,
  signal: AbortSignal,
  changed: () => void,
): Promise<void> {
  const subscription = await runtime.runtimeEvents.subscribe(request, signal);
  if (signal.aborted) return;
  changed();
  const topics = new Set<string>(request.topics);
  for await (const event of subscription.events) {
    if (signal.aborted) return;
    if (event.type === "resync" || topics.has(event.type)) changed();
  }
}
