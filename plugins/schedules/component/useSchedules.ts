import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  bindOperations,
  createTranslator,
  mutationSettlementIsUnknown,
  OperationFailure,
  failureText as failure,
  type ComponentContext,
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
  type Read,
  type Edit,
  type ModelRead,
} from "./types";
export const hasTime = (value: string) => value !== "0001-01-01T00:00:00Z";
export const enabled = (item: Schedule) => hasTime(item.NextRunAt);
export const selection = (model?: Model, reasoningEffort = "") => ({
  provider: model?.provider ?? "",
  model: model?.id ?? "",
  reasoningEffort,
});
export function useSchedules(context: ComponentContext) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
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
  const [modelRead, setModelRead] = useState<ModelRead>();
  const [modelRefresh, setModelRefresh] = useState(0);
  const currentModels = modelRead?.context === context ? modelRead : undefined;
  const models = [...(currentModels?.catalogs.values() ?? [])].flat();
  const [edit, setEdit] = useState<Edit>();
  const [deleting, setDeleting] = useState<{
    item: Schedule;
  }>();
  const searchElement = useRef<HTMLElement>(null);
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
  const [command, setCommand] = useState<Command>();
  const pending = useRef<Command>(undefined);
  const accepted = useRef<{ page: Page; history: string[] }>(undefined);
  const [notice, setNotice] = useState<string>();
  const [liveError, setLiveError] = useState<string>();
  const refresh = useCallback(
    async (history = accepted.current?.history ?? []) => {
      reading.current?.abort();
      const controller = new AbortController();
      reading.current = controller;
      const signal = AbortSignal.any([context.signal, controller.signal]);
      setRead((previous) => ({ ...previous, loading: true, error: undefined }));
      try {
        const page = await operations.query(
          "flame.schedules/list.v1",
          { Cursor: history.at(-1) ?? "" },
          signal,
        );
        if (!signal.aborted) {
          accepted.current = { page, history };
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
    void context.track(watch());
    return () => controller.abort();
  }, [context, refresh]);
  useEffect(() => {
    const controller = new AbortController();
    const signal = AbortSignal.any([context.signal, controller.signal]);
    setModelRead((previous) => ({
      context,
      catalogs: previous?.context === context ? previous.catalogs : new Map(),
      loading: true,
    }));
    const readModels = async () => {
      try {
        const page = await context.runtime.providers.list(signal);
        if (signal.aborted) return;
        const providers = page.data.filter((provider) => provider.configured);
        const results = await Promise.allSettled(
          providers.map((provider) =>
            context.runtime.models.list(provider.id, signal),
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
                previous?.context === context
                  ? previous.catalogs.get(provider.id)
                  : undefined;
              if (retained) catalogs.set(provider.id, retained);
              errors.push(`${provider.id}: ${failure(result.reason)}`);
            }
          }
          return {
            context,
            catalogs,
            loading: false,
            error: errors.join("; ") || undefined,
          };
        });
      } catch (error) {
        if (!signal.aborted)
          setModelRead((previous) => ({
            context,
            catalogs:
              previous?.context === context ? previous.catalogs : new Map(),
            loading: false,
            error: failure(error),
          }));
      }
    };
    void context.track(readModels());
    return () => controller.abort();
  }, [context, modelRefresh]);
  async function observe(captured: Command) {
    try {
      const output = await context.track(captured.promise);
      pending.current = undefined;
      if (!context.signal.aborted) {
        if (edit?.basis || deleting) taskOpener.current = searchElement.current;
        setCommand(undefined);
        setEdit(undefined);
        setDeleting(undefined);
        setNotice(
          output &&
            typeof output === "object" &&
            "RunID" in output &&
            "SessionID" in output
            ? t("notice.accepted", {
                run: String(output.RunID),
                session: String(output.SessionID),
              })
            : t("notice.completed", { label: captured.label }),
        );
        void refresh();
      }
    } catch (error) {
      if (context.signal.aborted) return;
      if (mutationSettlementIsUnknown(error)) {
        const uncertain = {
          ...captured,
          uncertain: true,
          error: failure(error),
        };
        pending.current = uncertain;
        setCommand(uncertain);
      } else {
        pending.current = undefined;
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
    if (pending.current || context.signal.aborted) return;
    try {
      const captured = {
        promise: operations.command(method, input),
        label,
        uncertain: false,
      };
      pending.current = captured;
      setCommand(captured);
      setNotice(undefined);
      void observe(captured);
    } catch (error) {
      if (!context.signal.aborted) setNotice(failure(error));
    }
  }
  function reconcile() {
    const original = pending.current;
    if (!original?.uncertain || context.signal.aborted) return;
    try {
      const retry = {
        ...original,
        promise: original.promise.retry({ signal: context.signal }),
        uncertain: false,
        error: undefined,
      };
      pending.current = retry;
      setCommand(retry);
      void observe(retry);
    } catch (error) {
      if (!context.signal.aborted)
        setCommand({ ...original, error: failure(error) });
    }
  }
  function add(opener: HTMLElement, template?: (typeof templates)[number]) {
    if (pending.current) return;
    taskOpener.current = opener;
    setNotice(undefined);
    setEdit({
      draft: {
        Title: template ? t(`template.${template.id}.title`) : "",
        Instructions: template ? t(`template.${template.id}.instructions`) : "",
        Cron: template?.cron ?? "",
        CWD: context.workspace?.ref.path ?? "",
        ModelSelection: selection(),
        Enabled: true,
      },
    });
    setDeleting(undefined);
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
      Title: edit.draft.Title.trim(),
      Instructions: edit.draft.Instructions.trim(),
      Cron: edit.draft.Cron.trim(),
      CWD: edit.draft.CWD.trim(),
    };
    if (edit.basis)
      submit(
        "flame.schedules/update.v1",
        {
          ID: edit.basis.ID,
          ExpectedRevision: edit.basis.Revision,
          Title: draft.Title,
          Instructions: draft.Instructions,
          Cron: draft.Cron,
          CWD: draft.CWD,
          Enabled: draft.Enabled,
          ModelSelection: {
            Provider: draft.ModelSelection.provider,
            Model: draft.ModelSelection.model,
            ReasoningEffort: draft.ModelSelection.reasoningEffort,
          },
        },
        t("command.save"),
      );
    else submit("flame.schedules/create.v1", draft, t("command.create"));
  }
  function cancelTask() {
    if (pending.current) return;
    // Base UI resolves the target before sibling buttons are re-enabled.
    // Catalog reads, rather than the retiring editor, can keep them disabled.
    if ((edit?.basis || deleting) && (read.loading || read.error))
      taskOpener.current = searchElement.current;
    setEdit(undefined);
    setDeleting(undefined);
  }
  const page = read.accepted?.page;
  const rows = page?.Rows ?? [];
  const matching = rows.filter(
    (item) =>
      (status === "all" ||
        (enabled(item) ? "enabled" : "disabled") === status) &&
      `${item.ID} ${item.Title} ${item.Instructions} ${item.Cron} ${item.CWD}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  const busy = Boolean(command);
  const canSave = Boolean(
    !busy && edit?.draft.Instructions.trim() && edit.draft.Cron.trim(),
  );
  const editorOpen = Boolean(edit || deleting);
  const actionsDisabled = busy || editorOpen;
  const disabled = actionsDisabled || read.loading || Boolean(read.error);
  const selectedModel = models.find(
    (model) =>
      model.provider === edit?.draft.ModelSelection.provider &&
      model.id === edit?.draft.ModelSelection.model,
  );
  const selectedKey =
    edit?.draft.ModelSelection.provider && edit.draft.ModelSelection.model
      ? JSON.stringify([
          edit.draft.ModelSelection.provider,
          edit.draft.ModelSelection.model,
        ])
      : "default";
  return {
    t,
    dateTime,
    notice,
    command,
    reconcile,
    searchElement,
    search,
    setSearch,
    editorOpen,
    actionsDisabled,
    add,
    status,
    setStatus,
    read,
    busy,
    refresh,
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
    submit,
    page,
    rows,
    matching,
    disabled,
    taskOpener,
    setNotice,
    setEdit,
    setDeleting,
  };
}
