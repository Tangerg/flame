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
  mutationSettlementIsUnknown,
  isErrorType,
  scrollPositions,
  createTranslator,
  failureText as failure,
  type ComponentContext,
} from "@flame/component-sdk";
import { type MutationPromise } from "@flame/runtime-client";

import {
  type SkillSnapshot,
  type SkillRecord,
  type Target,
  type Read,
  type OpenedDocument,
  type CatalogReturn,
  type Command,
  type Review,
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
export const SECTION_VALUES = [
  "available",
  "review",
  "library",
] as const satisfies readonly Target[];
export function useSkills(context: ComponentContext) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const workspace = useMemo(
    () =>
      context.workspace
        ? context.runtime.workspace(context.workspace.ref)
        : undefined,
    [context],
  );
  const [target, setTarget] = useState<Target>(
    workspace ? "available" : "library",
  );
  const [search, setSearch] = useState("");
  const panel = useRef<HTMLElement>(null);
  const searchField = useRef<HTMLElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const [opened, setOpened] = useState<OpenedDocument>();
  const documentReading = useRef<AbortController>(undefined);
  const catalogReturn = useRef<CatalogReturn>(undefined);
  const pendingFocus = useRef<{
    target: "document" | "catalog";
    active: Element | null;
  }>(null);
  const [command, setCommand] = useState<Command>();
  const pending = useRef<Command>(undefined);
  const [review, setReview] = useState<Review>();
  const [notice, setNotice] = useState<string>();
  const [observationError, setObservationError] = useState<string>();
  const refresh = useCallback(async () => {
    reading.current?.abort();
    if (target !== "library" && !workspace) {
      setRead({ loading: false });
      return;
    }
    const controller = new AbortController();
    reading.current = controller;
    const signal = AbortSignal.any([context.signal, controller.signal]);
    setRead((previous) => ({ ...previous, loading: true, error: undefined }));
    try {
      let accepted: SkillSnapshot;
      switch (target) {
        case "library":
          accepted = {
            type: target,
            data: await context.track(
              context.runtime.skills.listLibrary(signal),
            ),
          };
          break;
        case "available":
          accepted = {
            type: target,
            data: await context.track(workspace!.skills.listDiscovered(signal)),
          };
          break;
        case "review":
          accepted = {
            type: target,
            data: await context.track(workspace!.skills.listProposals(signal)),
          };
          break;
      }
      if (!signal.aborted) setRead({ loading: false, accepted });
    } catch (error) {
      if (!signal.aborted)
        setRead((previous) => ({
          ...previous,
          loading: false,
          error: failure(error),
        }));
    }
  }, [context, workspace, target]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);
  const readDocument = useCallback(
    async (name: string) => {
      documentReading.current?.abort();
      const controller = new AbortController();
      documentReading.current = controller;
      const signal = AbortSignal.any([context.signal, controller.signal]);
      setOpened(
        (previous) =>
          previous && { ...previous, loading: true, error: undefined },
      );
      try {
        const accepted = await context.track(
          workspace!.skills.getDiscovered(name, signal),
        );
        if (!signal.aborted)
          setOpened(
            (previous) => previous && { ...previous, loading: false, accepted },
          );
      } catch (error) {
        if (!signal.aborted)
          setOpened(
            (previous) =>
              previous && {
                ...previous,
                loading: false,
                error: failure(error),
              },
          );
      }
    },
    [context, workspace],
  );
  useEffect(() => () => documentReading.current?.abort(), []);
  const refreshOwners = useEffectEvent(() => {
    void refresh();
    if (opened) void readDocument(opened.name);
  });
  useEffect(() => {
    const controller = new AbortController();
    const signal = AbortSignal.any([context.signal, controller.signal]);
    setObservationError(undefined);
    const observeChanges = async () => {
      try {
        const subscription = await context.runtime.runtimeEvents.subscribe(
          {
            topics: [
              "skills.changed",
              "plugins.changed",
              ...(workspace ? ["files.changed" as const] : []),
            ],
            ...(workspace
              ? {
                  watches: [
                    { watchId: "skills-workspace", workspace: workspace.ref },
                  ],
                }
              : {}),
          },
          signal,
        );
        if (signal.aborted) return;
        refreshOwners();
        for await (const event of subscription.events) {
          if (signal.aborted) break;
          if (
            event.type === "skills.changed" ||
            event.type === "plugins.changed" ||
            event.type === "files.changed" ||
            event.type === "resync"
          )
            refreshOwners();
        }
      } catch (error) {
        if (!signal.aborted) setObservationError(failure(error));
      }
    };
    void context.track(observeChanges());
    return () => controller.abort();
  }, [context, workspace]);
  function navigate(next: Target) {
    if (pending.current || context.signal.aborted) return;
    if (next === target) {
      void refresh();
      return;
    }
    reading.current?.abort();
    setRead({ loading: true });
    setTarget(next);
    setReview(undefined);
    setNotice(undefined);
    setSearch("");
  }
  function searchCatalog(next: string) {
    if (next === search) return;
    setSearch(next);
    for (const position of scrollPositions(panel.current))
      position.element.scrollTop = 0;
  }
  function openDocument(name: string, opener: HTMLElement) {
    if (pending.current || context.signal.aborted) return;
    catalogReturn.current = { opener, scroll: scrollPositions(panel.current) };
    pendingFocus.current = {
      target: "document",
      active: opener.ownerDocument.activeElement,
    };
    setOpened({ name, loading: true });
    setReview(undefined);
    setNotice(undefined);
    void readDocument(name);
  }
  function closeDocument() {
    documentReading.current?.abort();
    pendingFocus.current = {
      target: "catalog",
      active: panel.current?.ownerDocument.activeElement ?? null,
    };
    setOpened(undefined);
  }
  useLayoutEffect(() => {
    const requested = pendingFocus.current;
    pendingFocus.current = null;
    if (!requested) return;
    const active = panel.current?.ownerDocument.activeElement;
    const focusIsUnclaimed =
      active === requested.active || active === active?.ownerDocument.body;
    if (requested.target === "document") {
      for (const position of scrollPositions(panel.current))
        position.element.scrollTop = 0;
      if (focusIsUnclaimed) heading.current?.focus();
    } else {
      for (const position of catalogReturn.current?.scroll ?? [])
        position.element.scrollTop = position.top;
      const opener = catalogReturn.current?.opener;
      if (focusIsUnclaimed)
        (opener?.isConnected ? opener : searchField.current)?.focus();
    }
  }, [opened]);
  async function observe(captured: Command) {
    try {
      await context.track(captured.promise);
      pending.current = undefined;
      if (!context.signal.aborted) {
        setCommand(undefined);
        setNotice(t("notice.completed", { label: captured.label }));
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
          isErrorType(error, "revision_conflict")
            ? t("notice.revisionConflict")
            : failure(error),
        );
        void refresh();
      }
    }
  }
  function submit(make: () => MutationPromise<void>, label: string) {
    if (pending.current || context.signal.aborted) return;
    try {
      const captured = { promise: make(), label, uncertain: false };
      pending.current = captured;
      setCommand(captured);
      setReview(undefined);
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
  const snapshot = read.accepted;
  const rows: SkillRecord[] =
    snapshot?.type === "available"
      ? snapshot.data.skills
      : (snapshot?.data.data ?? []);
  const matching = rows.filter((item) =>
    `${item.name} ${item.description ?? ""} ${"scope" in item ? item.scope : "lifecycle" in item ? item.lifecycle : ""}`
      .toLocaleLowerCase()
      .includes(search.toLocaleLowerCase()),
  );
  const disabled = Boolean(command) || read.loading || Boolean(read.error);
  const title = t(`section.${target}`);
  const skillDocument = opened?.accepted;
  return {
    t,
    panel,
    opened,
    review,
    setReview,
    closeDocument,
    target,
    readDocument,
    heading,
    skillDocument,
    command,
    navigate,
    searchField,
    search,
    searchCatalog,
    read,
    refresh,
    title,
    snapshot,
    observationError,
    notice,
    reconcile,
    workspace,
    disabled,
    submit,
    rows,
    matching,
    openDocument,
  };
}
