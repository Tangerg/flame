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
  type Feedback,
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
  const cwd = scope === "project" ? context.workspace?.ref.path : "";
  const [read, setRead] = useState<Read>({ loading: false });
  const current =
    read.accepted?.scope === scope && read.accepted.cwd === cwd
      ? read.accepted
      : undefined;
  const [edit, setEdit] = useState<Edit>();
  const searchElement = useRef<HTMLElement>(null);
  const editOpener = edit?.opener;
  const resolveOpener = useCallback(
    () =>
      editOpener?.isConnected &&
      !editOpener.matches(":disabled, [aria-disabled='true']")
        ? editOpener
        : searchElement.current,
    [editOpener],
  );
  const [pending, setPending] = useState(false);
  const [unknown, setUnknown] = useState<Command>();
  const [editorFeedback, setEditorFeedback] = useState<Feedback>();
  const [recordFeedback, setRecordFeedback] = useState<Feedback>();
  const reading = useRef<AbortController>(undefined);
  const lifetime = useRef<AbortController>(undefined);
  const command = useRef<Command>(undefined);
  const observing = useRef(false);
  const arrival = useRef<HTMLElement>(undefined);
  const initiator = useRef<HTMLElement>(undefined);
  const recordResult = useRef<HTMLElement>(null);
  const summary = useRef<HTMLSpanElement>(null);
  const load = useCallback(
    async (owner: AbortSignal, cursors = [""], pageControl?: HTMLElement) => {
      reading.current?.abort();
      arrival.current = undefined;
      if (cwd === undefined) {
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
          "flame.memory/list.v2",
          {
            Scope: scope,
            CWD: cwd,
            Cursor: cursors.at(-1)!,
          },
          signal,
        );
        if (!signal.aborted) {
          arrival.current = pageControl;
          setRead({ loading: false, accepted: { page, cursors, scope, cwd } });
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
    [context, operations, scope, cwd, t],
  );
  const refreshAccepted = useEffectEvent((signal: AbortSignal) => {
    void load(signal, current?.cursors);
  });
  useEffect(() => {
    const owner = new AbortController();
    lifetime.current = owner;
    const signal = AbortSignal.any([context.signal, owner.signal]);
    refreshAccepted(signal);
    return () => owner.abort();
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
  useLayoutEffect(() => {
    const control = arrival.current;
    arrival.current = undefined;
    const active = control?.ownerDocument.activeElement;
    if (
      control &&
      (active === control || active === control.ownerDocument.body)
    )
      summary.current?.focus();
  }, [current]);
  const locked = pending || Boolean(unknown) || context.signal.aborted;
  const activeFilters =
    Number(status !== "all") + Number(origin !== "all") + Number(pinned);
  function clearFilters() {
    setStatus("all");
    setOrigin("all");
    setPinned(false);
  }
  function beginEdit(next: Edit) {
    setFiltersOpen(false);
    setEditorFeedback(undefined);
    setEdit(next);
  }
  function report(
    original: Command,
    state: Feedback["state"],
    detail?: string,
  ) {
    const update =
      original.source === "editor" ? setEditorFeedback : setRecordFeedback;
    update({ label: original.label, state, detail });
  }
  function observerSignal(): AbortSignal | undefined {
    const owner = lifetime.current;
    if (!owner || owner.signal.aborted || context.signal.aborted) return;
    return AbortSignal.any([context.signal, owner.signal]);
  }
  function handOff() {
    const control = initiator.current;
    initiator.current = undefined;
    // Runs before the accepted read can replace the row; a later focus choice keeps the document.
    if (control && control === control.ownerDocument.activeElement)
      recordResult.current?.focus();
  }
  async function observe(original: Command, owner: AbortSignal) {
    if (observing.current) return;
    observing.current = true;
    setPending(true);
    report(original, "submitting");
    try {
      await original.promise;
      if (owner.aborted) return;
      handOff();
      command.current = undefined;
      setUnknown(undefined);
      if (original.source === "editor") {
        setEdit(undefined);
        setEditorFeedback(undefined);
      }
      setRecordFeedback({ label: original.label, state: "confirmed" });
      void load(owner);
    } catch (error) {
      if (owner.aborted) return;
      handOff();
      if (mutationSettlementIsUnknown(error)) {
        setUnknown(() => original);
        report(original, "unknown");
      } else {
        command.current = undefined;
        setUnknown(undefined);
        report(original, "refused", failureText(error, t("error.refused")));
      }
    } finally {
      observing.current = false;
      if (!owner.aborted) setPending(false);
    }
  }
  function change<K extends MutationName>(
    method: K,
    input: MethodInputs[K],
    label: string,
    source: Command["source"] = "record",
    control?: HTMLElement,
  ) {
    const owner = observerSignal();
    if (!owner || command.current || observing.current || cwd === undefined)
      return;
    try {
      const original = {
        promise: operations.command(method, input),
        source,
        label,
      };
      command.current = original;
      initiator.current = control;
      void observe(original, owner);
    } catch (error) {
      const update =
        source === "editor" ? setEditorFeedback : setRecordFeedback;
      update({
        label,
        state: "refused",
        detail: failureText(error, t("error.submit")),
      });
    }
  }
  function reconcile() {
    const owner = observerSignal();
    if (!owner || !unknown || observing.current) return;
    try {
      const attempt = {
        ...unknown,
        promise: unknown.promise.retry({ signal: owner }),
      };
      command.current = attempt;
      void observe(attempt, owner);
    } catch (error) {
      report(unknown, "unknown", failureText(error, t("error.check")));
    }
  }
  return {
    t,
    edit,
    locked,
    setEdit,
    scope,
    cwd,
    read,
    pending,
    observerSignal,
    load,
    beginEdit,
    command,
    observing,
    setSearch,
    clearFilters,
    setEditorFeedback,
    setRecordFeedback,
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
    editorFeedback,
    unknown,
    reconcile,
    change,
    current,
    summary,
    recordFeedback,
    recordResult,
    initiator,
  };
}
