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
  bindOperations,
  mutationSettlementIsUnknown,
  useTranslator,
} from "@flame/component-sdk";
import { scrollPositions } from "@flame/component-sdk";
import { METHODS } from "../contracts/methods";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";
import {
  failure,
  makeFormatters,
  type Action,
  type ArchiveRead,
  type ArchiveReturn,
  type BoundContext,
  type Header,
  type OpenedReport,
  type Outcome,
  type Period,
} from "./model";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

// One archive owns its reads, commands and the focus each transition returns to;
// command outcomes decide against the committed view, not the one they started in.
export function useReportArchive(context: BoundContext) {
  const locale = context.locale;
  const t = useTranslator(CATALOGS, locale);
  const fmt = useMemo(() => makeFormatters(locale, t), [locale, t]);
  const operations = useMemo(
    () =>
      bindOperations<MethodInputs, MethodOutputs>(context, CAPABILITY_JSON, {
        scope: "installation",
        reference: context.binding.installationId,
      }),
    [context],
  );
  const [archive, setArchive] = useState<ArchiveRead>({ loading: true });
  const [opened, setOpened] = useState<OpenedReport>();
  const [removing, setRemoving] = useState<Header>();
  const [period, setPeriod] = useState<Period>("daily");
  const [action, setAction] = useState<Action>();
  const busy = action !== undefined;
  const [outcome, setOutcome] = useState<Outcome>();
  const lifetime = useRef<AbortController>(undefined);
  const inspecting = useRef<AbortController>(undefined);
  const listing = useRef<AbortController>(undefined);
  const panel = useRef<HTMLElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const refreshButton = useRef<HTMLButtonElement>(null);
  const saveButton = useRef<HTMLButtonElement>(null);
  const backButton = useRef<HTMLButtonElement>(null);
  const removeButton = useRef<HTMLButtonElement>(null);
  const cancelRemoval = useRef<HTMLButtonElement>(null);
  // Command outcomes settle after awaits; they decide against the committed
  // view rather than the view captured when the command started.
  const shown = useRef<OpenedReport>(undefined);
  const archiveReturn = useRef<ArchiveReturn>(undefined);
  const pendingFocus = useRef<{
    target: "report" | "archive";
    active: Element | null;
  }>(null);
  // A confirmed recovery unmounts its initiating control after the commit.
  const recoveryReturn = useRef<Element | null>(null);
  const unknown = [
    ...operations.pendingCommands(METHODS.capture),
    ...operations.pendingCommands(METHODS.delete),
  ];

  const refresh = useCallback(
    async (owner: AbortSignal): Promise<Header[] | undefined> => {
      listing.current?.abort();
      const request = new AbortController();
      listing.current = request;
      const signal = AbortSignal.any([owner, request.signal]);
      setArchive((previous) => ({
        ...previous,
        loading: true,
        error: undefined,
      }));
      try {
        const { reports } = await operations.query(METHODS.list, {}, signal);
        if (signal.aborted) return;
        setArchive({ loading: false, accepted: reports });
        return reports;
      } catch (error) {
        if (!signal.aborted)
          setArchive((previous) => ({
            ...previous,
            loading: false,
            error: failure(error),
          }));
      }
    },
    [operations],
  );

  const refreshFromEffect = useEffectEvent((owner: AbortSignal) => {
    void refresh(owner);
  });

  useEffect(() => {
    const owner = new AbortController();
    lifetime.current = owner;
    refreshFromEffect(AbortSignal.any([owner.signal, context.signal]));
    return () => {
      owner.abort();
      inspecting.current?.abort();
      listing.current?.abort();
    };
  }, [context.signal]);

  useLayoutEffect(() => {
    shown.current = opened;
  }, [opened]);

  const openedId = opened?.header.id;
  useLayoutEffect(() => {
    const requested = pendingFocus.current;
    pendingFocus.current = null;
    if (!requested) return;
    const active = panel.current?.ownerDocument.activeElement;
    const focusIsUnclaimed =
      active === requested.active || active === active?.ownerDocument.body;
    if (requested.target === "report") {
      for (const position of scrollPositions(panel.current))
        position.element.scrollTop = 0;
      if (focusIsUnclaimed) heading.current?.focus();
      return;
    }
    const restore = archiveReturn.current;
    archiveReturn.current = undefined;
    for (const position of restore?.scroll ?? [])
      position.element.scrollTop = position.top;
    if (focusIsUnclaimed)
      (restore?.opener?.isConnected
        ? restore.opener
        : refreshButton.current
      )?.focus();
  }, [openedId]);

  function ownerSignal() {
    const owner = lifetime.current;
    if (!owner) throw new Error("Report component lifetime has not started");
    return AbortSignal.any([context.signal, owner.signal]);
  }

  function activeElement() {
    return panel.current?.ownerDocument.activeElement ?? null;
  }

  async function read(id: string) {
    inspecting.current?.abort();
    const request = new AbortController();
    inspecting.current = request;
    const signal = AbortSignal.any([ownerSignal(), request.signal]);
    setOpened((previous) =>
      previous?.header.id === id
        ? { ...previous, loading: true, error: undefined }
        : previous,
    );
    try {
      const accepted = await operations.query(METHODS.get, { id }, signal);
      if (!signal.aborted)
        setOpened((previous) =>
          previous?.header.id === id
            ? { ...previous, loading: false, accepted }
            : previous,
        );
    } catch (error) {
      if (!signal.aborted)
        setOpened((previous) =>
          previous?.header.id === id
            ? { ...previous, loading: false, error: failure(error) }
            : previous,
        );
    }
  }

  function open(
    header: Header,
    opener: HTMLElement | undefined,
    active: Element | null,
  ) {
    if (!shown.current)
      archiveReturn.current = {
        opener,
        scroll: scrollPositions(panel.current),
      };
    pendingFocus.current = { target: "report", active };
    setOpened({ header: structuredClone(header), loading: true });
    void read(header.id);
  }

  // A removed report's row is gone, so focus returns to the archive instead.
  function leave(active: Element | null, returnTo: "opener" | "archive") {
    inspecting.current?.abort();
    if (returnTo === "archive" && archiveReturn.current)
      archiveReturn.current.opener = undefined;
    pendingFocus.current = { target: "archive", active };
    setOpened(undefined);
  }

  async function observe(
    started: Action,
    command: Promise<Header | boolean>,
    removed?: Header,
  ) {
    const signal = ownerSignal();
    const active = activeElement();
    setAction(started);
    setOutcome(undefined);
    try {
      const result = await command;
      if (signal.aborted) return;
      if (started.kind === "check") recoveryReturn.current = active;
      if (typeof result !== "boolean") {
        const current = activeElement();
        // A later completion opens the saved report only while the user
        // remains where the command started.
        if (current === active || current === current?.ownerDocument.body) {
          setOutcome({ kind: "saved" });
          open(result, undefined, active);
        } else setOutcome({ kind: "savedArchived", header: result });
        void refresh(signal);
        return;
      }
      setOutcome({ kind: removed ? "removed" : "removalConfirmed" });
      const accepted = await refresh(signal);
      if (signal.aborted) return;
      // A recovered removal reports success without its original target. Only
      // the captured target or fresh canonical membership can retire the open
      // report; an unread archive keeps it.
      const current = shown.current;
      if (
        current &&
        (current.header.id === removed?.id ||
          (accepted && !accepted.some(({ id }) => id === current.header.id)))
      )
        leave(active, "archive");
    } catch (error) {
      if (!signal.aborted)
        setOutcome(
          mutationSettlementIsUnknown(error)
            ? { kind: "unknown" }
            : { kind: "archiveFailed", error: failure(error) },
        );
    } finally {
      if (!signal.aborted) {
        setAction(undefined);
        setRemoving(undefined);
      }
    }
  }

  function save() {
    if (busy || unknown.length) return;
    try {
      void observe(
        { kind: "capture" },
        operations.command(METHODS.capture, { period }),
      );
    } catch (error) {
      setOutcome({ kind: "captureRejected", error: failure(error) });
    }
  }

  function requestRemoval() {
    if (!opened || busy || unknown.length) return;
    setOutcome(undefined);
    setRemoving(structuredClone(opened.header));
  }

  function confirmRemoval() {
    if (!removing || busy || unknown.length) return;
    try {
      void observe(
        { kind: "removal" },
        operations.command(METHODS.delete, { id: removing.id }),
        removing,
      );
    } catch (error) {
      setRemoving(undefined);
      setOutcome({ kind: "removalRejected", error: failure(error) });
    }
  }

  function dismissRemoval() {
    if (!busy) setRemoving(undefined);
  }

  const removalReturn = useCallback(() => {
    for (const element of [removeButton.current, backButton.current])
      if (element?.isConnected && !element.disabled) return element;
    return refreshButton.current;
  }, []);

  useLayoutEffect(() => {
    const initiator = recoveryReturn.current;
    if (action || !initiator) return;
    recoveryReturn.current = null;
    const active = panel.current?.ownerDocument.activeElement;
    if (
      !initiator.isConnected &&
      (active === initiator || active === active?.ownerDocument.body)
    )
      (shown.current ? removalReturn() : saveButton.current)?.focus();
  }, [action, removalReturn]);

  function check(command: (typeof unknown)[number]) {
    if (busy) return;
    void observe(
      { kind: "check", key: command.idempotencyKey },
      command.recover(ownerSignal()),
    );
  }

  async function retire(command: (typeof unknown)[number]) {
    if (busy) return;
    const signal = ownerSignal();
    const active = activeElement();
    setAction({ kind: "retire", key: command.idempotencyKey });
    try {
      await command.retire(signal);
      if (signal.aborted) return;
      recoveryReturn.current = active;
      setOutcome({ kind: "retired" });
    } catch (error) {
      if (!signal.aborted)
        setOutcome({ kind: "retireFailed", error: failure(error) });
    } finally {
      if (!signal.aborted) setAction(undefined);
    }
  }

  return {
    t,
    locale,
    fmt,
    busy,
    action,
    outcome,
    refs: {
      panel,
      heading,
      refreshButton,
      saveButton,
      backButton,
      removeButton,
      cancelRemoval,
    },
    catalog: {
      read: archive,
      period,
      choosePeriod: setPeriod,
      refresh: () => void refresh(ownerSignal()),
      save,
      open: (header: Header, opener: HTMLElement) => {
        setOutcome(undefined);
        open(header, opener, activeElement());
      },
    },
    report: {
      opened,
      refresh: (id: string) => void read(id),
      back: () => leave(activeElement(), "opener"),
    },
    removal: {
      target: removing,
      request: requestRemoval,
      confirm: confirmRemoval,
      dismiss: dismissRemoval,
      focusReturn: removalReturn,
    },
    recovery: {
      commands: unknown,
      check,
      retire: (command: (typeof unknown)[number]) => void retire(command),
    },
  };
}

export type ReportArchive = ReturnType<typeof useReportArchive>;
