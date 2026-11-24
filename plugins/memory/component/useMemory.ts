import { handoffFocus } from "@flame/ui";
import {
  useCallback,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  failureText,
  bindOperations,
  mutationSettlementIsUnknown,
  type ComponentContext,
  useTranslator,
} from "@flame/component-sdk";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";
import {
  type MutationName,
  type Scope,
  type Read,
  type Edit,
  type Command,
  type CommandIntent,
  type CommandSource,
  type CommandState,
  type Feedback,
  type Outcome,
} from "./types";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";
const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };
const IDLE: CommandState = { phase: "idle" };

function activeFeedback(state: CommandState): Feedback | undefined {
  switch (state.phase) {
    case "idle":
      return undefined;
    case "unknown":
      return {
        intent: state.command.intent,
        state: "unknown",
        failure: state.failure,
      };
    default:
      return { intent: state.command.intent, state: "submitting" };
  }
}
export function useMemory(context: ComponentContext) {
  const t = useTranslator(CATALOGS, context.locale);
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error("Memory requires its declared callable contribution");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      {
        scope: "installation",
        reference: context.binding.installationId,
      },
    );
  }, [context]);
  const [scope, setScope] = useState<Scope>("project");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [origin, setOrigin] = useState("all");
  const [pinned, setPinned] = useState(false);
  const [filtersOpen, setFiltersOpen] = useState(false);
  // A Session in a Project keeps project memory under that Project, never under
  // the folder it happens to run in.
  const project = scope === "project" ? context.project : undefined;
  const cwd =
    scope === "project" && !project ? context.workspace?.ref.path : "";
  const target = project ? project.id : cwd;
  const focusSource = useRef({
    runtime: context.runtime,
    binding: context.binding,
    track: context.track,
    signal: context.signal,
    scope,
    target,
  });
  useLayoutEffect(() => {
    focusSource.current = {
      runtime: context.runtime,
      binding: context.binding,
      track: context.track,
      signal: context.signal,
      scope,
      target,
    };
  });
  const canRestoreFocus = () => {
    const current = focusSource.current;
    return (
      !context.signal.aborted &&
      current.runtime === context.runtime &&
      current.binding === context.binding &&
      current.track === context.track &&
      current.signal === context.signal &&
      current.scope === scope &&
      current.target === target
    );
  };
  const [read, setRead] = useState<Read>({ loading: false });
  const current =
    read.accepted?.scope === scope && read.accepted.target === target
      ? read.accepted
      : undefined;
  const [edit, setEdit] = useState<Edit>();
  const searchElement = useRef<HTMLInputElement>(null);
  const editorElement = useRef<HTMLElement>(null);
  const editOpener = edit?.opener;
  const resolveOpener = useCallback(
    () =>
      editOpener?.isConnected && !editOpener.matches(":disabled")
        ? editOpener
        : searchElement.current,
    [editOpener],
  );
  function closeEditor() {
    const form = editorElement.current;
    if (form && canRestoreFocus()) handoffFocus(form, resolveOpener());
    setEdit(undefined);
  }
  const [command, setCommand] = useState<CommandState>(IDLE);
  const [outcomes, setOutcomes] = useState<{
    [source in CommandSource]?: Outcome;
  }>({});
  const pending =
    command.phase === "submitting" || command.phase === "checking";
  const unknown =
    command.phase === "unknown" || command.phase === "checking"
      ? command.command
      : undefined;
  function feedbackFor(source: CommandSource) {
    return command.phase !== "idle" && command.command.source === source
      ? activeFeedback(command)
      : outcomes[source];
  }
  function settle(source: CommandSource, outcome: Outcome | undefined) {
    setOutcomes((current) => ({ ...current, [source]: outcome }));
  }
  const reading = useRef<AbortController>(undefined);
  const lifetime = useRef<AbortController>(undefined);
  const recordResult = useRef<HTMLElement>(null);
  const summary = useRef<HTMLSpanElement>(null);
  const load = useCallback(
    async (owner: AbortSignal, cursors = [""], pageControl?: HTMLElement) => {
      reading.current?.abort();
      if (pageControl && summary.current)
        handoffFocus(pageControl, summary.current);
      if (target === undefined) {
        setRead((previous) => ({
          ...previous,
          loading: false,
          error: undefined,
        }));
        return;
      }
      const observer = new AbortController();
      reading.current = observer;
      const signal = AbortSignal.any([context.signal, owner, observer.signal]);
      if (signal.aborted) return;
      setRead((previous) => ({ ...previous, loading: true, error: undefined }));
      try {
        const page = await operations.query(
          "flame.memory/list.v3",
          {
            scope,
            cwd: cwd ?? "",
            ...(project ? { project: project.id } : {}),
            cursor: cursors.at(-1)!,
          },
          signal,
        );
        if (!signal.aborted) {
          setRead({
            loading: false,
            accepted: { page, cursors, scope, target },
          });
        }
      } catch (error) {
        if (!signal.aborted)
          setRead((previous) => ({
            ...previous,
            loading: false,
            error: failureText(error, t("error.read")),
          }));
      }
    },
    [context, operations, scope, cwd, project, target, t],
  );
  const refreshAccepted = useEffectEvent((signal: AbortSignal) => {
    void load(signal, current?.cursors);
  });
  useEffect(() => {
    const owner = new AbortController();
    lifetime.current = owner;
    return () => owner.abort();
  }, [
    context.runtime,
    context.binding,
    context.track,
    context.signal,
    scope,
    target,
  ]);
  useEffect(() => {
    // Revalidating a display context must not retire an accepted command's observer.
    const owner = lifetime.current;
    if (owner) refreshAccepted(AbortSignal.any([context.signal, owner.signal]));
  }, [context.signal, load]);
  useEffect(() => {
    const controller = new AbortController();
    const signal = AbortSignal.any([context.signal, controller.signal]);
    const watch = async () => {
      try {
        const subscription = await context.runtime.runtimeEvents.subscribe(
          { topics: ["plugins.dataChanged"] },
          signal,
        );
        if (signal.aborted) return;
        refreshAccepted(signal);
        for await (const event of subscription.events) {
          if (signal.aborted) break;
          if (event.type === "plugins.dataChanged" || event.type === "resync")
            refreshAccepted(signal);
        }
      } catch (error) {
        if (!signal.aborted)
          setRead((previous) => ({
            ...previous,
            error: failureText(error, t("error.observe")),
          }));
      }
    };
    void context.track(() => watch());
    return () => controller.abort();
  }, [context, load]);
  const locked = command.phase !== "idle" || context.signal.aborted;
  const activeFilters =
    Number(status !== "all") + Number(origin !== "all") + Number(pinned);
  function clearFilters() {
    setStatus("all");
    setOrigin("all");
    setPinned(false);
  }
  function beginEdit(next: Edit) {
    setFiltersOpen(false);
    settle("editor", undefined);
    setEdit(next);
  }
  function clearFeedback() {
    setOutcomes({});
  }
  function observerSignal(): AbortSignal | undefined {
    const owner = lifetime.current;
    if (!owner || owner.signal.aborted || context.signal.aborted) return;
    return AbortSignal.any([context.signal, owner.signal]);
  }
  // A retired display context drops its command; the new one starts idle
  // instead of staying locked behind a settlement it will never observe.
  async function observe(
    original: Command,
    owner: AbortSignal,
    phase: "submitting" | "checking",
  ) {
    setCommand({ phase, command: original });
    settle(original.source, undefined);
    try {
      await original.promise;
      if (owner.aborted) return;
      setCommand(IDLE);
      if (original.source === "editor") closeEditor();
      settle("record", { intent: original.intent, state: "confirmed" });
      void load(owner);
    } catch (error) {
      if (owner.aborted) return;
      if (mutationSettlementIsUnknown(error)) {
        setCommand({ phase: "unknown", command: original });
        return;
      }
      setCommand(IDLE);
      settle(original.source, {
        intent: original.intent,
        state: "refused",
        failure: { cause: error, fallback: "refused" },
      });
    } finally {
      if (owner.aborted)
        setCommand((current) =>
          current.phase !== "idle" && current.command === original
            ? IDLE
            : current,
        );
    }
  }
  function change<K extends MutationName>(
    method: K,
    input: MethodInputs[K],
    intent: CommandIntent,
    source: CommandSource = "record",
  ) {
    const owner = observerSignal();
    if (!owner || command.phase !== "idle" || target === undefined) return;
    try {
      const original = {
        promise: operations.command(method, input),
        source,
        intent,
      };
      void observe(original, owner, "submitting");
    } catch (error) {
      settle(source, {
        intent,
        state: "refused",
        failure: { cause: error, fallback: "submit" },
      });
    }
  }
  function reconcile() {
    const owner = observerSignal();
    if (!owner || command.phase !== "unknown") return;
    const original = command.command;
    try {
      const attempt = {
        ...original,
        promise: original.promise.retry({ signal: owner }),
      };
      void observe(attempt, owner, "checking");
    } catch (error) {
      setCommand({
        phase: "unknown",
        command: original,
        failure: { cause: error, fallback: "check" },
      });
    }
  }
  return {
    canRestoreFocus,
    t,
    edit,
    locked,
    setEdit,
    closeEditor,
    editorElement,
    scope,
    cwd,
    project,
    read,
    pending,
    observerSignal,
    load,
    beginEdit,
    setSearch,
    clearFilters,
    clearFeedback,
    setScope,
    searchElement,
    search,
    filtersOpen,
    setFiltersOpen,
    activeFilters,
    status,
    setStatus,
    origin,
    setOrigin,
    pinned,
    setPinned,
    resolveOpener,
    editorFeedback: feedbackFor("editor"),
    unknown,
    reconcile,
    change,
    current,
    summary,
    recordFeedback: feedbackFor("record"),
    recordResult,
  };
}
