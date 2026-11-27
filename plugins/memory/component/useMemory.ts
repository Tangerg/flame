import { type MemoryFilterState } from "./useMemoryFilters";
import { handoffFocus } from "@flame/ui";
import {
  useCallback,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  failureText,
  mutationSettlementIsUnknown,
  type CommandActivity,
  type ComponentContext,
  useTranslator,
  watchRuntimeTopics,
} from "@flame/component-sdk";
import type { MethodInputs } from "../package/io.github.tangerg.flame/contracts/methods.generated";
import {
  type MemoryItem,
  type MutationName,
  type Scope,
  type Read,
  type Edit,
  type CommandPresentation,
  type CommandIntent,
  type CommandSource,
  type Failure,
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
import { METHODS } from "./methods";
import { bindMemoryOperations } from "./memoryOperations";
const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

export function useMemory(
  context: ComponentContext,
  locale: string,
  filters: MemoryFilterState,
  activity: CommandActivity<CommandPresentation>,
) {
  const t = useTranslator(CATALOGS, locale);
  const operations = useMemo(() => bindMemoryOperations(context), [context]);
  const [scope, setScope] = useState<Scope>("project");
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
  const editorElement = useRef<HTMLElement>(null);
  const editOpener = edit?.opener;
  const resolveOpener = useCallback(
    () =>
      editOpener?.isConnected && !editOpener.matches(":disabled")
        ? editOpener
        : filters.searchElement.current,
    [editOpener, filters.searchElement],
  );
  function closeEditor() {
    const form = editorElement.current;
    if (form && canRestoreFocus()) handoffFocus(form, resolveOpener());
    setEdit(undefined);
  }
  const activeCommand = useSyncExternalStore(
    activity.subscribe,
    activity.snapshot,
  );
  useSyncExternalStore(
    operations.subscribeCommands,
    operations.commandsVersion,
  );
  const journalCommand = context.signal.aborted
    ? undefined
    : operations.originalCommand();
  const original =
    journalCommand?.state === "recoverable" ? journalCommand : undefined;
  const [recoveryDetail, setRecoveryDetail] = useState<{
    key: string;
    presentation: CommandPresentation;
    failure: Failure;
  }>();
  const [outcomes, setOutcomes] = useState<{
    [source in CommandSource]?: Outcome;
  }>({});
  const pending =
    activeCommand !== undefined || journalCommand?.state === "active";
  const detail =
    original && recoveryDetail?.key === original.idempotencyKey
      ? recoveryDetail
      : undefined;
  const recoverySource =
    edit && detail?.presentation.source === "editor" ? "editor" : "record";
  function feedbackFor(source: CommandSource): Feedback | undefined {
    if (source === activeCommand?.source)
      return { intent: activeCommand.intent, state: "submitting" };
    if (original && source === recoverySource)
      return {
        intent: detail?.presentation.intent,
        state: "unknown",
        failure: detail?.failure,
      };
    return outcomes[source];
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
          METHODS.list,
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
        await watchRuntimeTopics(
          context.runtime,
          { topics: ["plugins.dataChanged"] },
          signal,
          () => refreshAccepted(signal),
        );
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
  const locked = pending || original !== undefined || context.signal.aborted;
  function beginEdit(next: Edit) {
    filters.setOpen(false);
    settle("editor", undefined);
    setEdit(next);
  }
  function clearFeedback() {
    setOutcomes({});
  }
  function changeScope(next: Scope) {
    if (locked || edit) return;
    filters.reset();
    clearFeedback();
    setScope(next);
  }
  function addItem(opener: HTMLElement) {
    beginEdit({ type: "add", content: "", opener });
  }
  function editItem(item: MemoryItem, opener: HTMLElement) {
    beginEdit({
      type: "update",
      item: structuredClone(item),
      content: item.content,
      opener,
    });
  }
  function removeItem(item: MemoryItem, opener: HTMLElement) {
    beginEdit({ type: "delete", item: structuredClone(item), opener });
  }
  function showPage(cursors?: string[], pageControl?: HTMLElement) {
    const owner = observerSignal();
    if (owner) void load(owner, cursors, pageControl);
  }
  function observerSignal(): AbortSignal | undefined {
    const owner = lifetime.current;
    if (!owner || owner.signal.aborted || context.signal.aborted) return;
    return AbortSignal.any([context.signal, owner.signal]);
  }
  function observe(
    presentation: CommandPresentation,
    execute: () => Promise<unknown> | undefined,
    checking = false,
  ) {
    const owner = observerSignal();
    if (!owner) return;
    const { source, intent } = presentation;
    void activity.run(presentation, async () => {
      let fallback: Failure["fallback"] = checking ? "check" : "submit";
      try {
        const attempt = execute();
        if (!attempt) return;
        settle(source, undefined);
        if (!checking) fallback = "refused";
        await attempt;
        if (owner.aborted) return;
        setRecoveryDetail(undefined);
        if (source === "editor") closeEditor();
        settle("record", { intent, state: "confirmed" });
        void load(owner);
      } catch (error) {
        if (owner.aborted) return;
        const failure = { cause: error, fallback };
        if (mutationSettlementIsUnknown(error)) {
          const retained = operations.originalCommand();
          if (retained?.state === "recoverable") {
            setRecoveryDetail({
              key: retained.idempotencyKey,
              presentation,
              failure,
            });
            return;
          }
        }
        setRecoveryDetail(undefined);
        settle(source, { intent, state: "refused", failure });
      }
    });
  }
  function change<K extends MutationName>(
    method: K,
    input: MethodInputs[K],
    intent: CommandIntent,
    source: CommandSource = "record",
  ) {
    if (locked || target === undefined) return;
    observe({ source, intent }, () => operations.submit(method, input));
  }
  function saveEdit() {
    if (!edit || edit.type === "delete") return;
    const content = edit.content.trim();
    if (edit.type === "add")
      change(
        METHODS.add,
        {
          scope,
          cwd: cwd ?? "",
          ...(project ? { project: project.id } : {}),
          content,
        },
        { action: "add", scope },
        "editor",
      );
    else
      change(
        METHODS.update,
        { id: edit.item.id, expectedRevision: edit.item.revision, content },
        { action: "edit", id: edit.item.id },
        "editor",
      );
  }
  function deleteEdited() {
    if (edit?.type !== "delete") return;
    change(
      METHODS.delete,
      { id: edit.item.id, expectedRevision: edit.item.revision },
      { action: "delete", id: edit.item.id },
      "editor",
    );
  }
  function reconcile(key: string) {
    if (context.signal.aborted) return;
    const retained = operations.originalCommand();
    if (retained?.state !== "recoverable" || retained.idempotencyKey !== key)
      return;
    observe(
      { source: recoverySource, intent: detail?.presentation.intent },
      retained.recover,
      true,
    );
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
    showPage,
    addItem,
    editItem,
    removeItem,
    saveEdit,
    deleteEdited,
    changeScope,
    resolveOpener,
    editorFeedback: feedbackFor("editor"),
    editorRecovery: recoverySource === "editor" ? original : undefined,
    recordRecovery: recoverySource === "record" ? original : undefined,
    reconcile,
    change,
    current,
    summary,
    recordFeedback: feedbackFor("record"),
    recordResult,
  };
}
