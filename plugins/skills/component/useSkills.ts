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
  mutationSettlementIsUnknown,
  isErrorType,
  failureText as failure,
  type ComponentContext,
  useTranslator,
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
  type CommandState,
  type CommandIntent,
  type Notice,
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
  const t = useTranslator(CATALOGS, context.locale);
  const {
    runtime,
    workspace: capturedWorkspace,
    signal: componentSignal,
    track,
  } = context;
  const workspaceRef = capturedWorkspace?.ref;
  const workspace = useMemo(
    () => (workspaceRef ? runtime.workspace(workspaceRef) : undefined),
    [runtime, workspaceRef],
  );
  const [target, setTarget] = useState<Target>(
    workspace ? "available" : "library",
  );
  const focusSource = useRef({
    runtime,
    workspace,
    componentSignal,
    track,
    target,
  });
  useLayoutEffect(() => {
    focusSource.current = {
      runtime,
      workspace,
      componentSignal,
      track,
      target,
    };
  });
  const canRestoreFocus = () => {
    const current = focusSource.current;
    return (
      !componentSignal.aborted &&
      current.runtime === runtime &&
      current.workspace === workspace &&
      current.componentSignal === componentSignal &&
      current.track === track &&
      current.target === target
    );
  };
  const [search, setSearch] = useState("");
  const bodyViewport = useRef<HTMLDivElement>(null);
  const searchField = useRef<HTMLInputElement>(null);
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const [opened, setOpened] = useState<OpenedDocument>();
  const documentReading = useRef<AbortController>(undefined);
  const catalogReturn = useRef<CatalogReturn>(undefined);
  const pendingFocus = useRef<{
    target: "document" | "catalog";
    active: Element | null;
  }>(null);
  const [command, setCommand] = useState<CommandState>();
  const [review, setReview] = useState<Review>();
  const reviewElement = useRef<HTMLElement>(null);
  function closeReview() {
    const form = reviewElement.current;
    if (form && canRestoreFocus())
      handoffFocus(
        form,
        review?.opener?.isConnected ? review.opener : searchField.current,
      );
    setReview(undefined);
  }
  const [notice, setNotice] = useState<Notice>();
  const [observationError, setObservationError] = useState<string>();
  const refresh = useCallback(async () => {
    reading.current?.abort();
    if (target !== "library" && !workspace) {
      setRead({ loading: false });
      return;
    }
    const controller = new AbortController();
    reading.current = controller;
    const signal = AbortSignal.any([componentSignal, controller.signal]);
    setRead((previous) => ({ ...previous, loading: true, error: undefined }));
    try {
      let accepted: SkillSnapshot;
      switch (target) {
        case "library":
          accepted = {
            type: target,
            data: await track(() => runtime.skills.listLibrary(signal)),
          };
          break;
        case "available":
          accepted = {
            type: target,
            data: await track(() => workspace!.skills.listDiscovered(signal)),
          };
          break;
        case "review":
          accepted = {
            type: target,
            data: await track(() => workspace!.skills.listProposals(signal)),
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
  }, [runtime, componentSignal, track, workspace, target]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);
  const readDocument = useCallback(
    async (name: string) => {
      documentReading.current?.abort();
      const controller = new AbortController();
      documentReading.current = controller;
      const signal = AbortSignal.any([componentSignal, controller.signal]);
      setOpened(
        (previous) =>
          previous && { ...previous, loading: true, error: undefined },
      );
      try {
        const accepted = await track(() =>
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
    [componentSignal, track, workspace],
  );
  useEffect(() => () => documentReading.current?.abort(), []);
  const refreshOwners = useEffectEvent(() => {
    void refresh();
    if (opened) void readDocument(opened.name);
  });
  useEffect(() => {
    const controller = new AbortController();
    const signal = AbortSignal.any([componentSignal, controller.signal]);
    setObservationError(undefined);
    const observeChanges = async () => {
      try {
        const subscription = await runtime.runtimeEvents.subscribe(
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
    void track(() => observeChanges());
    return () => controller.abort();
  }, [runtime, componentSignal, track, workspace]);
  function navigate(next: Target) {
    if (command || componentSignal.aborted) return;
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
    if (bodyViewport.current) bodyViewport.current.scrollTop = 0;
  }
  function openDocument(name: string, opener: HTMLElement) {
    if (command || componentSignal.aborted) return;
    catalogReturn.current = {
      opener,
      scrollTop: bodyViewport.current?.scrollTop ?? 0,
    };
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
      active: bodyViewport.current?.ownerDocument.activeElement ?? null,
    };
    setOpened(undefined);
  }
  useLayoutEffect(() => {
    const requested = pendingFocus.current;
    pendingFocus.current = null;
    if (!requested) return;
    const viewport = bodyViewport.current;
    const active = viewport?.ownerDocument.activeElement;
    const focusIsUnclaimed =
      active === requested.active || active === active?.ownerDocument.body;
    if (requested.target === "document") {
      if (viewport) {
        viewport.scrollTop = 0;
        if (focusIsUnclaimed) viewport.focus({ preventScroll: true });
      }
    } else {
      const catalog = catalogReturn.current;
      if (viewport && catalog) viewport.scrollTop = catalog.scrollTop;
      const opener = catalog?.opener;
      if (focusIsUnclaimed)
        (opener?.isConnected ? opener : searchField.current)?.focus();
    }
  }, [opened]);
  async function observe(captured: Command) {
    try {
      await captured.promise;
      if (!componentSignal.aborted) {
        setCommand(undefined);
        setNotice({
          kind: "completed",
          intent: captured.intent,
        });
        void refresh();
      }
    } catch (error) {
      if (componentSignal.aborted) return;
      if (mutationSettlementIsUnknown(error)) {
        setCommand({
          phase: "uncertain",
          command: captured,
          error: failure(error),
        });
      } else {
        setCommand(undefined);
        setNotice(
          isErrorType(error, "revision_conflict")
            ? { kind: "revisionConflict", intent: captured.intent }
            : {
                kind: "failure",
                intent: captured.intent,
                detail: failure(error),
              },
        );
        void refresh();
      }
    }
  }
  function submit(make: () => MutationPromise<void>, intent: CommandIntent) {
    if (command || componentSignal.aborted) return;
    try {
      const captured = { promise: track(make), intent };
      closeReview();
      setCommand({ phase: "submitting", command: captured });
      setNotice(undefined);
      void observe(captured);
    } catch (error) {
      if (!componentSignal.aborted)
        setNotice({ kind: "failure", intent, detail: failure(error) });
    }
  }
  function confirmReview() {
    if (!review || !workspace) return;
    const { scope, name, revision } = review.proposal;
    submit(
      () =>
        review.decision === "approve"
          ? workspace.skills.approveProposal({ scope, name, revision })
          : workspace.skills.rejectProposal({ scope, name, revision }),
      { action: review.decision, name },
    );
  }
  function reconcile() {
    if (command?.phase !== "uncertain" || componentSignal.aborted) return;
    const original = command.command;
    try {
      const retry = {
        ...original,
        promise: track(() =>
          original.promise.retry({ signal: componentSignal }),
        ),
      };
      setCommand({ phase: "submitting", command: retry });
      void observe(retry);
    } catch (error) {
      if (!componentSignal.aborted)
        setCommand({
          phase: "uncertain",
          command: original,
          error: failure(error),
        });
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
    canRestoreFocus,
    t,
    bodyViewport,
    opened,
    review,
    setReview,
    closeReview,
    confirmReview,
    reviewElement,
    closeDocument,
    target,
    readDocument,
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
