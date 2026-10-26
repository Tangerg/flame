import { TrajectoryReport, runId } from "./report";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  failureText,
  createTranslator,
  mutationSettlementIsUnknown,
  type ComponentContext,
} from "@flame/component-sdk";
import type { MutationPromise } from "@flame/runtime-client";
import { Refresh } from "@flame/icons/react";
import type {
  Page,
  Session,
  TrajectoryEntry,
} from "@flame/runtime-client/wire";
import {
  Button,
  Disclosure,
  Facts,
  Panel,
  PanelHeader,
  PanelFooter,
  IconButton,
  Form,
  FormField,
  FormSection,
  FormActions,
  Metadata,
  SearchField,
  FilterBar,
  Select,
  Toolbar,
  TextField,
} from "@flame/ui";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

type AcceptedPage = {
  page: Page<TrajectoryEntry>;
  cursors: (string | undefined)[];
  includeDescendants: boolean;
};

type Read = {
  loading: boolean;
  accepted?: AcceptedPage;
  error?: string;
};

export function Trajectory({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const [kind, setKind] = useState("all");
  const [run, setRun] = useState("all");
  const [search, setSearch] = useState("");
  const sessionId = useMemo(() => {
    if (!context.sessionId)
      throw new Error("Session trajectory requires a Session context");
    return context.sessionId;
  }, [context]);
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const arrival = useRef<HTMLElement>(undefined);
  const summary = useRef<HTMLParagraphElement>(null);
  const searchElement = useRef<HTMLElement>(null);
  const load = useCallback(
    async (
      cursors: (string | undefined)[] = [undefined],
      capturedDescendants?: boolean,
      origin?: HTMLElement,
    ) => {
      reading.current?.abort();
      arrival.current = undefined;
      const controller = new AbortController();
      reading.current = controller;
      const signal = AbortSignal.any([context.signal, controller.signal]);
      setRead((previous) => ({ ...previous, loading: true, error: undefined }));
      try {
        const includeDescendants =
          capturedDescendants ??
          Boolean(
            (await context.track(context.runtime.runtime.discover(signal)))
              .capabilities.features?.subagents?.enabled,
          );
        signal.throwIfAborted();
        const page = await context.track(
          context.runtime.sessions.trajectory(
            {
              sessionId,
              ...(includeDescendants ? { includeDescendants: true } : {}),
              cursor: cursors.at(-1),
              limit: 100,
            },
            signal,
          ),
        );
        if (!signal.aborted) {
          arrival.current = origin;
          setRead({
            loading: false,
            accepted: { page, cursors, includeDescendants },
          });
        }
      } catch (error) {
        if (!signal.aborted)
          setRead((previous) => ({
            ...previous,
            loading: false,
            error: failureText(error, t("read.failed")),
          }));
      }
    },
    [context, sessionId, t],
  );
  useEffect(() => {
    void load();
    return () => reading.current?.abort();
  }, [load]);
  const accepted = read.accepted;
  useLayoutEffect(() => {
    const origin = arrival.current;
    arrival.current = undefined;
    const active = origin?.ownerDocument.activeElement;
    if (origin && (active === origin || active === origin.ownerDocument.body))
      summary.current?.focus();
  }, [accepted]);
  const rows = accepted?.page.data ?? [];
  const runOptions = [
    { value: "all", label: t("run.all") },
    ...Array.from(
      new Set([...rows.map(runId), ...(run === "all" ? [] : [run])]),
    ).map((id) => ({
      value: id,
      label: id || t("value.sessionOnly"),
    })),
  ];

  return (
    <Panel
      label={t("panel.title")}
      header={
        <PanelHeader
          size="md"
          title={t("panel.title")}
          context={
            <Metadata>
              {t("panel.session")} <code>{sessionId}</code>
            </Metadata>
          }
        />
      }
      controls={
        <FilterBar
          label={t("controls.label")}
          search={
            <SearchField
              ref={searchElement}
              size="lg"
              id="search"
              aria-label={t("search.label")}
              placeholder={t("search.placeholder")}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              onClear={() => setSearch("")}
              clearLabel={t("search.clear")}
            />
          }
          actions={
            <IconButton
              icon={Refresh}
              aria-label={t("refresh")}
              size="md"
              pending={read.loading}
              onClick={() =>
                void (accepted
                  ? load(accepted.cursors, accepted.includeDescendants)
                  : load())
              }
            />
          }
        >
          <Select
            size="lg"
            id="kind"
            label={t("kind.label")}
            value={kind}
            onValueChange={setKind}
            options={[
              { value: "all", label: t("kind.all") },
              { value: "run", label: t("kind.run") },
              { value: "model", label: t("kind.model") },
              { value: "endpoint", label: t("kind.endpoint") },
              { value: "toolCall", label: t("kind.toolCall") },
              { value: "message", label: t("kind.message") },
              { value: "attention", label: t("kind.attention") },
            ]}
          />
          <Select
            size="lg"
            id="run"
            label={t("run.label")}
            value={run}
            onValueChange={setRun}
            options={runOptions}
          />
        </FilterBar>
      }
    >
      {read.loading && <p role="status">{t("status.reading")}</p>}
      {read.error && (
        <p role="alert">{t("status.error", { error: read.error })}</p>
      )}
      {accepted && (
        <>
          <TrajectoryReport
            ref={summary}
            page={accepted.page}
            pageNumber={accepted.cursors.length}
            kind={kind}
            run={run}
            search={search}
            stale={Boolean(read.error)}
            onClearFilters={() => {
              setKind("all");
              setRun("all");
              setSearch("");
              searchElement.current?.focus();
            }}
            t={t}
            locale={context.locale}
          />
          {(accepted.cursors.length > 1 ||
            Boolean(accepted.page.nextCursor)) && (
            <PanelFooter>
              {" "}
              <Toolbar label={t("pages.label")}>
                <Button
                  pending={read.loading}
                  disabled={accepted.cursors.length === 1}
                  onClick={(event) =>
                    void load(
                      accepted.cursors.slice(0, -1),
                      accepted.includeDescendants,
                      event.currentTarget,
                    )
                  }
                >
                  {t("pages.newer")}
                </Button>
                <Button
                  pending={read.loading}
                  disabled={!accepted.page.nextCursor}
                  onClick={(event) =>
                    void load(
                      [...accepted.cursors, accepted.page.nextCursor!],
                      accepted.includeDescendants,
                      event.currentTarget,
                    )
                  }
                >
                  {t("pages.older")}
                </Button>
              </Toolbar>
            </PanelFooter>
          )}
        </>
      )}
    </Panel>
  );
}

export function RenameSession({ context }: { context: ComponentContext }) {
  const session = context.sessionTarget;
  if (!session) throw new Error("A captured Session action target is required");
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const [title, setTitle] = useState(session.title);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string>();
  const [unknown, setUnknown] = useState<MutationPromise<Session>>();
  const [saved, setSaved] = useState<Session>();
  const pending = useRef(false);
  const observe = async (command: MutationPromise<Session>) => {
    pending.current = true;
    setBusy(true);
    setFailure(undefined);
    try {
      const result = await context.track(command);
      if (context.signal.aborted) return;
      setSaved(result);
      setUnknown(undefined);
    } catch (error) {
      if (context.signal.aborted) return;
      setFailure(failureText(error, String(error)));
      setUnknown(mutationSettlementIsUnknown(error) ? command : undefined);
    } finally {
      pending.current = false;
      if (!context.signal.aborted) setBusy(false);
    }
  };
  return (
    <Form
      onSubmit={(event) => {
        event.preventDefault();
        if (pending.current || unknown || saved || context.signal.aborted)
          return;
        context.signal.throwIfAborted();
        void observe(
          context.runtime.sessions.update({
            sessionId: session.id,
            expectedRevision: session.revision,
            title,
          }),
        );
      }}
    >
      <FormSection title={t("rename.section")}>
        <FormField label={t("rename.titleLabel")}>
          <TextField
            aria-label={t("rename.titleLabel")}
            // oxlint-disable-next-line jsx-a11y/no-autofocus
            autoFocus
            value={title}
            pending={busy}
            readOnly={Boolean(unknown) || Boolean(saved)}
            onChange={(event) => setTitle(event.target.value)}
          />
        </FormField>
        {failure && (
          <p role="alert">{t("rename.failure", { message: failure })}</p>
        )}
        {unknown && (
          <>
            <p role="status">{t("rename.unconfirmed")}</p>
            <Button
              type="button"
              pending={busy}
              aria-busy={busy || undefined}
              onClick={() => {
                if (pending.current || context.signal.aborted) return;
                void observe(unknown.retry());
              }}
            >
              {busy ? (
                <span role="status">{t("rename.checking")}</span>
              ) : (
                t("rename.reconcile")
              )}
            </Button>
          </>
        )}
        {saved && (
          <p role="status">
            {t("rename.saved", {
              id: saved.id,
              revision: saved.revision,
              title: saved.title || t("rename.untitled"),
            })}
          </p>
        )}
        <Disclosure title={t("rename.captured")}>
          <Facts
            values={[
              [t("panel.session"), session.id],
              [t("rename.factRevision"), session.revision],
            ]}
          />
        </Disclosure>
      </FormSection>
      <FormActions label={t("rename.actions")}>
        <Button
          variant="primary"
          type="submit"
          pending={busy}
          focusableWhenDisabled
          aria-busy={(busy && !unknown) || undefined}
          disabled={
            Boolean(unknown) || Boolean(saved) || context.signal.aborted
          }
        >
          {busy && !unknown ? (
            <span role="status">{t("rename.saving")}</span>
          ) : (
            t("rename.save")
          )}
        </Button>
      </FormActions>
    </Form>
  );
}
