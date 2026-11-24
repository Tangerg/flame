import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  bindOperations,
  mutationSettlementIsUnknown,
  OperationFailure,
  scrollPositions,
  failureText as failure,
  type ComponentContext,
  useTranslator,
} from "@flame/component-sdk";
import { type Model } from "@flame/runtime-client/wire";
import {
  CAPABILITY_JSON,
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
  type Page,
  type Draft,
  type Mutation,
  type Command,
  type CommandState,
  type Notice,
  type Read,
  type Edit,
  type ModelRead,
} from "./types";
export const enabled = (item: Schedule) => item.nextRunAt !== undefined;
export const modelKey = (provider: string, model: string) =>
  JSON.stringify([provider, model]);
export const selection = (model?: Model, reasoningEffort = "") => ({
  provider: model?.provider ?? "",
  model: model?.id ?? "",
  reasoningEffort,
});
export function useSchedules(context: ComponentContext) {
  const t = useTranslator(CATALOGS, context.locale);
  const dateTime = useMemo(
    () =>
      new Intl.DateTimeFormat(context.locale, {
        dateStyle: "medium",
        timeStyle: "short",
      }),
    [context.locale],
  );
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error("Schedules requires its own callable contribution");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      { scope: "installation", reference: context.binding.installationId },
    );
  }, [context]);
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
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
  const [edit, setEdit] = useState<Edit>();
  const [deleting, setDeleting] = useState<{
    item: Schedule;
  }>();
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
  const [command, setCommand] = useState<CommandState>();
  const accepted = useRef<{ page: Page; history: string[] }>(undefined);
  const [notice, setNotice] = useState<Notice>();
  const feedbackElement = useRef<HTMLElement>(null);
  const [liveError, setLiveError] = useState<string>();
  const arrival = useRef<HTMLElement>(undefined);
  const summary = useRef<HTMLSpanElement>(null);
  const refresh = useCallback(
    async (
      history = accepted.current?.history ?? [],
      pageControl?: HTMLElement,
    ) => {
      reading.current?.abort();
      arrival.current = undefined;
      const controller = new AbortController();
      reading.current = controller;
      const signal = AbortSignal.any([context.signal, controller.signal]);
      setRead((previous) => ({ ...previous, loading: true, error: undefined }));
      try {
        const page = await operations.query(
          "flame.schedules/list.v2",
          { cursor: history.at(-1) ?? "" },
          signal,
        );
        if (!signal.aborted) {
          accepted.current = { page, history };
          arrival.current = pageControl;
          setRead({ loading: false, accepted: accepted.current });
        }
      } catch (error) {
        if (!signal.aborted)
          setRead((previous) => ({
            ...previous,
            loading: false,
            error: failure(error),
          }));
      }
    },
    [context, operations],
  );
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);
  useLayoutEffect(() => {
    const control = arrival.current;
    arrival.current = undefined;
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
        const subscription = await context.runtime.runtimeEvents.subscribe(
          { topics: ["plugins.dataChanged"] },
          signal,
        );
        if (signal.aborted) return;
        void refresh();
        for await (const event of subscription.events) {
          if (signal.aborted) break;
          if (event.type === "plugins.dataChanged" || event.type === "resync")
            void refresh();
        }
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
  async function observe(captured: Command) {
    try {
      const output = await captured.promise;
      if (!context.signal.aborted) {
        if (edit?.basis || deleting) taskOpener.current = searchElement.current;
        setCommand(undefined);
        setEdit(undefined);
        setDeleting(undefined);
        setNotice(
          captured.method === "flame.schedules/runNow.v2"
            ? (output as MethodOutputs["flame.schedules/runNow.v2"])
            : t("notice.completed", { label: captured.label }),
        );
        void refresh();
      }
    } catch (error) {
      if (context.signal.aborted) return;
      if (mutationSettlementIsUnknown(error)) {
        setCommand({
          phase: "uncertain",
          command: captured,
          error: failure(error),
        });
      } else {
        setCommand(undefined);
        setNotice(
          error instanceof OperationFailure && error.code === "revision"
            ? t("notice.revision")
            : failure(error),
        );
        if (error instanceof OperationFailure && error.code === "revision") {
          taskOpener.current = searchElement.current;
          setEdit(undefined);
        }
        void refresh();
      }
    }
  }
  function submit<K extends Mutation>(
    method: K,
    input: MethodInputs[K],
    label: string,
  ) {
    if (command || context.signal.aborted) return;
    try {
      const captured = {
        method,
        promise: operations.command(method, input),
        label,
      };
      setCommand({ phase: "submitting", command: captured });
      setNotice(undefined);
      void observe(captured);
    } catch (error) {
      if (!context.signal.aborted) setNotice(failure(error));
    }
  }
  function reconcile() {
    if (command?.phase !== "uncertain" || context.signal.aborted) return;
    const original = command.command;
    try {
      const retry = {
        ...original,
        promise: original.promise.retry({ signal: context.signal }),
      };
      const feedback = feedbackElement.current;
      if (
        feedback &&
        !feedback.closest("[hidden], [inert]") &&
        (feedback.contains(feedback.ownerDocument.activeElement) ||
          feedback.ownerDocument.activeElement === feedback.ownerDocument.body)
      )
        feedback.focus({ preventScroll: true });
      setCommand({ phase: "submitting", command: retry });
      void observe(retry);
    } catch (error) {
      if (!context.signal.aborted)
        setCommand({
          phase: "uncertain",
          command: original,
          error: failure(error),
        });
    }
  }
  // Opening a task replaces any other one and remembers where focus returns.
  function beginTask(
    opener: HTMLElement,
    task: { edit: Edit } | { deleting: Schedule },
  ) {
    if (command) return;
    taskOpener.current = opener;
    setNotice(undefined);
    setEdit("edit" in task ? task.edit : undefined);
    setDeleting(
      "deleting" in task ? { item: structuredClone(task.deleting) } : undefined,
    );
  }
  function add(opener: HTMLElement, template?: (typeof templates)[number]) {
    beginTask(opener, {
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
    beginTask(opener, { deleting: item });
  }
  function deleteConfirmed() {
    if (deleting)
      submit(
        "flame.schedules/delete.v2",
        { id: deleting.item.id },
        t("command.delete"),
      );
  }
  function runNow(item: Schedule) {
    submit("flame.schedules/runNow.v2", { id: item.id }, t("command.runNow"));
  }
  function toggleEnabled(item: Schedule) {
    const next = !enabled(item);
    submit(
      "flame.schedules/update.v2",
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
    setEdit((previous) =>
      previous
        ? { ...previous, draft: { ...previous.draft, [key]: value } }
        : undefined,
    );
  }
  function save() {
    if (!edit || !canSave) return;
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
        "flame.schedules/update.v2",
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
      );
    else submit("flame.schedules/create.v2", draft, t("command.create"));
  }
  function cancelTask() {
    if (command) return;
    // Base UI resolves the target before sibling buttons are re-enabled.
    // Catalog reads, rather than the retiring editor, can keep them disabled.
    if ((edit?.basis || deleting) && (read.loading || read.error))
      taskOpener.current = searchElement.current;
    setEdit(undefined);
    setDeleting(undefined);
  }
  function filterPage(nextSearch: string, nextStatus = status) {
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
      (status === "all" ||
        (enabled(item) ? "enabled" : "disabled") === status) &&
      `${item.id} ${item.title} ${item.instructions} ${item.cron} ${item.cwd}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  const busy = Boolean(command);
  const canSave = Boolean(
    !busy &&
    edit?.draft.instructions.trim() &&
    edit.draft.cron.trim() &&
    edit.draft.timeZone.trim(),
  );
  const editorOpen = Boolean(edit || deleting);
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
      : "default";
  return {
    t,
    dateTime,
    notice,
    feedbackElement,
    command,
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
    deleting,
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
