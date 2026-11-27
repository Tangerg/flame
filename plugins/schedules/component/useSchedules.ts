import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  type CommandActivity,
  mutationSettlementIsUnknown,
  OperationFailure,
  scrollPositions,
  failureText as failure,
  type ComponentContext,
  useTranslator,
  watchRuntimeTopics,
} from "@flame/component-sdk";
import { type Model } from "@flame/runtime-client/wire";
import {
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";
import { templates } from "./templates";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";
const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };
import {
  type Schedule,
  type Draft,
  type Command,
  type Notice,
  type Edit,
  type ModelRead,
} from "./types";
import { isConcealed } from "@flame/ui";
import { AUTHORING_LABELS, METHODS, isAuthoringMethod, type AuthoringMethod } from "./methods";
import { ScheduleCatalog } from "./scheduleCatalog";
import { bindScheduleOperations } from "./scheduleCommands";
export const enabled = (item: Schedule) => item.nextRunAt !== undefined;
export const modelKey = (provider: string, model: string) =>
  JSON.stringify([provider, model]);
export const selection = (model?: Model, reasoningEffort = "") => ({
  provider: model?.provider ?? "",
  model: model?.id ?? "",
  reasoningEffort,
});
// The schedules backend refuses a stale expected revision with this code.
const REVISION_CONFLICT = "revision";

// The unset model or reasoning choice: the Run uses the Runtime default.
export const DEFAULT_CHOICE = "default";

// "all" lifts the status filter; the others keep schedules in that state.
export const UNFILTERED = "all";
export const SCHEDULE_FILTERS = [UNFILTERED, "enabled", "disabled"] as const;
export type ScheduleFilter = (typeof SCHEDULE_FILTERS)[number];

type ScheduleTask =
  | { kind: "edit"; edit: Edit }
  | { kind: "delete"; schedule: Schedule };

type TaskSubmission = { task: ScheduleTask; originalKey?: string };

function targetsExistingSchedule(task: ScheduleTask): boolean {
  return task.kind === "delete" || task.edit.basis !== undefined;
}

export function useSchedules(
  context: ComponentContext,
  locale: string,
  observation: CommandActivity<Command>,
) {
  const t = useTranslator(CATALOGS, locale);
  const dateTime = useMemo(
    () =>
      new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "short",
      }),
    [locale],
  );
  const operations = useMemo(() => bindScheduleOperations(context), [context]);
  const catalog = useMemo(
    () => new ScheduleCatalog(
      (cursor, signal) => operations.query(METHODS.list, { cursor }, signal),
      context.signal,
    ),
    [context.signal, operations],
  );
  const read = useSyncExternalStore(catalog.subscribe, catalog.snapshot);
  const { refresh, openPage } = catalog;
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<ScheduleFilter>(UNFILTERED);
  const modelSource = useMemo(
    () => ({
      runtime: context.runtime,
      track: context.track,
      signal: context.signal,
    }),
    [context.runtime, context.track, context.signal],
  );
  const [modelRead, setModelRead] = useState<ModelRead>();
  const [modelRefresh, setModelRefresh] = useState(0);
  const currentModels =
    modelRead?.source === modelSource ? modelRead : undefined;
  const models = [...(currentModels?.catalogs.values() ?? [])].flat();
  const [task, setTask] = useState<ScheduleTask>();
  const edit = task?.kind === "edit" ? task.edit : undefined;
  const deleteTarget = task?.kind === "delete" ? task.schedule : undefined;
  const retainedTask = useRef<Required<TaskSubmission>>(undefined);
  const searchElement = useRef<HTMLInputElement>(null);
  const taskOpener = useRef<HTMLElement>(null);
  const titleElement = useRef<HTMLInputElement>(null);
  const deleteCancel = useRef<HTMLButtonElement>(null);
  const resolveOpener = useCallback(
    () =>
      taskOpener.current?.isConnected
        ? taskOpener.current
        : searchElement.current,
    [],
  );
  const command = useSyncExternalStore(
    observation.subscribe,
    observation.snapshot,
  );
  useSyncExternalStore(
    operations.subscribeCommands,
    operations.commandsVersion,
  );
  const original = context.signal.aborted
    ? undefined
    : operations.originalCommand();
  const pending = original?.state === "recoverable" ? original : undefined;
  const [notice, setNotice] = useState<Notice>();
  const feedbackElement = useRef<HTMLElement>(null);
  const [liveError, setLiveError] = useState<string>();
  const summary = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    void catalog.refresh();
    return catalog.stop;
  }, [catalog]);
  useLayoutEffect(() => {
    const control = read.accepted?.focusFrom;
    const active = control?.ownerDocument.activeElement;
    if (
      control &&
      (active === control || active === control.ownerDocument.body)
    )
      summary.current?.focus();
  }, [read.accepted]);
  useEffect(() => {
    const controller = new AbortController();
    const signal = AbortSignal.any([context.signal, controller.signal]);
    const watch = async () => {
      try {
        await watchRuntimeTopics(
          context.runtime,
          { topics: ["plugins.dataChanged"] },
          signal,
          () => void refresh(),
        );
      } catch (error) {
        if (!signal.aborted) setLiveError(failure(error));
      }
    };
    void context.track(() => watch());
    return () => controller.abort();
  }, [context, refresh]);
  useEffect(() => {
    const controller = new AbortController();
    const signal = AbortSignal.any([modelSource.signal, controller.signal]);
    setModelRead((previous) => ({
      source: modelSource,
      catalogs:
        previous?.source === modelSource ? previous.catalogs : new Map(),
      loading: true,
    }));
    const readModels = async () => {
      try {
        const page = await modelSource.runtime.providers.list(signal);
        if (signal.aborted) return;
        const providers = page.data.filter((provider) => provider.configured);
        const results = await Promise.allSettled(
          providers.map((provider) =>
            modelSource.runtime.models.list(provider.id, signal),
          ),
        );
        if (signal.aborted) return;
        setModelRead((previous) => {
          const catalogs = new Map<string, Model[]>();
          const errors: string[] = [];
          for (const [index, provider] of providers.entries()) {
            const result = results[index]!;
            if (result.status === "fulfilled")
              catalogs.set(provider.id, result.value.data);
            else {
              const retained =
                previous?.source === modelSource
                  ? previous.catalogs.get(provider.id)
                  : undefined;
              if (retained) catalogs.set(provider.id, retained);
              errors.push(`${provider.id}: ${failure(result.reason)}`);
            }
          }
          return {
            source: modelSource,
            catalogs,
            loading: false,
            error: errors.join("; ") || undefined,
          };
        });
      } catch (error) {
        if (!signal.aborted)
          setModelRead((previous) => ({
            source: modelSource,
            catalogs:
              previous?.source === modelSource ? previous.catalogs : new Map(),
            loading: false,
            error: failure(error),
          }));
      }
    };
    void modelSource.track(() => readModels());
    return () => controller.abort();
  }, [modelSource, modelRefresh]);
  function observe(
    command: Command,
    execute: () => Promise<MethodOutputs[keyof MethodOutputs]> | undefined,
    submission?: TaskSubmission,
  ) {
    if (context.signal.aborted) return;
    void observation.run(command, async () => {
      try {
        const attempt = execute();
        if (!attempt) return;
        setNotice(undefined);
        const output = await attempt;
        if (context.signal.aborted) return;
        if (submission)
          closeTask(submission.task, targetsExistingSchedule(submission.task));
        setNotice(
          "runId" in output
            ? output
            : t("notice.completed", { label: command.label }),
        );
        void refresh();
      } catch (error) {
        if (context.signal.aborted) return;
        const stale =
          error instanceof OperationFailure && error.code === REVISION_CONFLICT;
        setNotice(stale ? t("notice.revision") : failure(error));
        if (stale && submission) closeTask(submission.task, true);
        const unresolved = mutationSettlementIsUnknown(error);
        if (unresolved && submission) {
          const original = operations.originalCommand();
          if (
            original?.state === "recoverable" &&
            original.method === command.method &&
            (submission.originalKey === undefined ||
              original.idempotencyKey === submission.originalKey)
          ) {
            retainedTask.current = {
              originalKey: original.idempotencyKey,
              task: submission.task,
            };
          }
        }
        if (!unresolved) void refresh();
      }
    });
  }
  function submit<K extends AuthoringMethod>(
    method: K,
    input: MethodInputs[K],
    label: string,
    owner?: ScheduleTask,
  ) {
    observe(
      { method, label },
      () => operations.submit(method, input),
      owner && { task: owner },
    );
  }
  function reconcile(key: string) {
    if (context.signal.aborted) return;
    const original = operations.originalCommand();
    if (original?.state !== "recoverable" || original.idempotencyKey !== key)
      return;
    const { method } = original;
    if (!isAuthoringMethod(method)) return;
    const retained = retainedTask.current;
    observe(
      { method, label: t(AUTHORING_LABELS[method]) },
      () => {
        const feedback = feedbackElement.current;
        if (
          feedback &&
          !isConcealed(feedback) &&
          (feedback.contains(feedback.ownerDocument.activeElement) ||
            feedback.ownerDocument.activeElement === feedback.ownerDocument.body)
        )
          feedback.focus({ preventScroll: true });
        return original.recover();
      },
      retained?.originalKey === key ? retained : undefined,
    );
  }
  function beginTask(opener: HTMLElement, next: ScheduleTask) {
    if (busy) return;
    taskOpener.current = opener;
    setNotice(undefined);
    retainedTask.current = undefined;
    setTask(next);
  }
  function closeTask(owned: ScheduleTask, focusSearch: boolean) {
    if (owned !== task) return;
    if (focusSearch) taskOpener.current = searchElement.current;
    if (retainedTask.current?.task === owned) retainedTask.current = undefined;
    setTask((current) => current === owned ? undefined : current);
  }
  function add(opener: HTMLElement, template?: (typeof templates)[number]) {
    beginTask(opener, {
      kind: "edit",
      edit: {
        draft: {
          title: template ? t(`template.${template.id}.title`) : "",
          instructions: template
            ? t(`template.${template.id}.instructions`)
            : "",
          cron: template?.cron ?? "",
          timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
          cwd: context.workspace?.ref.path ?? "",
          modelSelection: selection(),
          enabled: true,
        },
      },
    });
  }
  function editSchedule(item: Schedule, opener: HTMLElement) {
    beginTask(opener, {
      kind: "edit",
      edit: {
        basis: structuredClone(item),
        draft: {
          title: item.title,
          instructions: item.instructions,
          cron: item.cron,
          timeZone: item.timeZone,
          cwd: item.cwd,
          modelSelection: structuredClone(item.modelSelection),
          enabled: enabled(item),
        },
      },
    });
  }
  function confirmDeletion(item: Schedule, opener: HTMLElement) {
    beginTask(opener, { kind: "delete", schedule: structuredClone(item) });
  }
  function deleteConfirmed() {
    if (task?.kind === "delete") {
      const { id, revision } = task.schedule;
      submit(
        METHODS.delete,
        { id, expectedRevision: revision },
        t("command.delete"),
        task,
      );
    }
  }
  function runNow(item: Schedule) {
    submit(METHODS.runNow, { id: item.id }, t("command.runNow"));
  }
  function toggleEnabled(item: Schedule) {
    const next = !enabled(item);
    submit(
      METHODS.update,
      {
        id: item.id,
        expectedRevision: item.revision,
        enabled: next,
        modelSelection: {},
      },
      next ? t("command.enable") : t("command.disable"),
    );
  }
  function patch<K extends keyof Draft>(key: K, value: Draft[K]) {
    setTask((current) =>
      current?.kind === "edit"
        ? {
            ...current,
            edit: { ...current.edit, draft: { ...current.edit.draft, [key]: value } },
          }
        : current,
    );
  }
  function save() {
    if (task?.kind !== "edit" || !canSave) return;
    const { edit } = task;
    const draft = {
      ...edit.draft,
      title: edit.draft.title.trim(),
      instructions: edit.draft.instructions.trim(),
      cron: edit.draft.cron.trim(),
      timeZone: edit.draft.timeZone.trim(),
      cwd: edit.draft.cwd.trim(),
    };
    if (edit.basis)
      submit(
        METHODS.update,
        {
          id: edit.basis.id,
          expectedRevision: edit.basis.revision,
          title: draft.title,
          instructions: draft.instructions,
          cron: draft.cron,
          timeZone: draft.timeZone,
          cwd: draft.cwd,
          enabled: draft.enabled,
          modelSelection: draft.modelSelection,
        },
        t("command.save"),
        task,
      );
    else submit(METHODS.create, draft, t("command.create"), task);
  }
  function cancelTask() {
    if (busy || !task) return;
    // Base UI resolves the target before sibling buttons are re-enabled.
    // Catalog reads, rather than the retiring editor, can keep them disabled.
    closeTask(
      task,
      targetsExistingSchedule(task) && Boolean(read.loading || read.error),
    );
  }
  function filterPage(nextSearch: string, nextStatus: ScheduleFilter = status) {
    if (nextSearch === search && nextStatus === status) return;
    setSearch(nextSearch);
    setStatus(nextStatus);
    for (const position of scrollPositions(searchElement.current))
      position.element.scrollTop = 0;
  }
  const page = read.accepted?.page;
  const rows = page?.rows ?? [];
  const matching = rows.filter(
    (item) =>
      (status === UNFILTERED ||
        (enabled(item) ? "enabled" : "disabled") === status) &&
      `${item.id} ${item.title} ${item.instructions} ${item.cron} ${item.cwd}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  const busy = command !== undefined || original !== undefined;
  const canSave = Boolean(
    !busy &&
    edit?.draft.instructions.trim() &&
    edit.draft.cron.trim() &&
    edit.draft.timeZone.trim(),
  );
  const editorOpen = task !== undefined;
  const actionsDisabled = busy || editorOpen;
  const disabled = actionsDisabled || read.loading || Boolean(read.error);
  const selectedModel = models.find(
    (model) =>
      model.provider === edit?.draft.modelSelection.provider &&
      model.id === edit?.draft.modelSelection.model,
  );
  const selectedKey =
    edit?.draft.modelSelection.provider && edit.draft.modelSelection.model
      ? modelKey(
          edit.draft.modelSelection.provider,
          edit.draft.modelSelection.model,
        )
      : DEFAULT_CHOICE;
  return {
    t,
    dateTime,
    notice,
    feedbackElement,
    command,
    pending,
    reconcile,
    searchElement,
    search,
    filterPage,
    editorOpen,
    actionsDisabled,
    add,
    status,
    read,
    busy,
    refresh,
    openPage,
    summary,
    liveError,
    edit,
    titleElement,
    resolveOpener,
    cancelTask,
    save,
    canSave,
    patch,
    selectedKey,
    models,
    selectedModel,
    currentModels,
    setModelRefresh,
    deleteTarget,
    deleteCancel,
    page,
    rows,
    matching,
    disabled,
    editSchedule,
    confirmDeletion,
    deleteConfirmed,
    runNow,
    toggleEnabled,
  };
}

export type ScheduleState = ReturnType<typeof useSchedules>;
export type Translate = ScheduleState["t"];
