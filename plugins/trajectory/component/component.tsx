import { TrajectoryReport, runId } from "./report";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  failureText,
  mutationSettlementIsUnknown,
  type ComponentContext,
  useTranslator,
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
  RecordBody,
  SearchField,
  FilterBar,
  Select,
  Toolbar,
  TextField,
} from "@flame/ui";
import { catalogs } from "./catalogs";
export { SessionExports } from "./exports";

type AcceptedPage = {
  page: Page<TrajectoryEntry>;
  cursors: (string | undefined)[];
  includeDescendants: boolean;
};

type Read = {
  loading: boolean;
  accepted?: AcceptedPage;
  error?: { cause: unknown };
};

export function Trajectory({ context }: { context: ComponentContext }) {
  const { runtime, track, signal: componentSignal, sessionId } = context;
  if (!sessionId)
    throw new Error("Session trajectory requires a Session context");
  const t = useTranslator(catalogs, context.locale);
  const [kind, setKind] = useState("all");
  const [run, setRun] = useState("all");
  const [search, setSearch] = useState("");
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
  const arrival = useRef<HTMLElement>(undefined);
  const summary = useRef<HTMLParagraphElement>(null);
  const searchElement = useRef<HTMLInputElement>(null);
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
      const signal = AbortSignal.any([componentSignal, controller.signal]);
      setRead((previous) => ({ ...previous, loading: true, error: undefined }));
      try {
        const includeDescendants =
          capturedDescendants ??
          Boolean(
            (await track(() => runtime.runtime.discover(signal))).capabilities
              .features?.childRuns?.enabled,
          );
        signal.throwIfAborted();
        const page = await track(() =>
          runtime.sessions.trajectory(
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
            error: { cause: error },
          }));
      }
    },
    [runtime, track, componentSignal, sessionId],
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
        <div role="alert">
          <RecordBody>
            {t("status.error", {
              error: failureText(read.error.cause, t("read.failed")),
            })}
          </RecordBody>
        </div>
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

// A rename is edited, saved once, or held unconfirmed until a check through
// its original idempotency key settles it.
type Rename =
  | { phase: "editing"; failure?: string }
  | { phase: "saving" }
  | { phase: "unknown"; original: MutationPromise<Session>; failure: string }
  | { phase: "checking"; original: MutationPromise<Session> }
  | { phase: "saved"; session: Session };

export function RenameSession({ context }: { context: ComponentContext }) {
  const session = context.sessionTarget;
  if (!session) throw new Error("A captured Session action target is required");
  const t = useTranslator(catalogs, context.locale);
  const [title, setTitle] = useState(session.title);
  const [rename, setRename] = useState<Rename>({ phase: "editing" });
  const busy = rename.phase === "saving" || rename.phase === "checking";
  const unconfirmed = rename.phase === "unknown" || rename.phase === "checking";
  const failure =
    rename.phase === "editing" || rename.phase === "unknown"
      ? rename.failure
      : undefined;
  const observe = async (
    command: MutationPromise<Session>,
    phase: "saving" | "checking",
  ) => {
    setRename(phase === "checking" ? { phase, original: command } : { phase });
    try {
      const session = await command;
      if (context.signal.aborted) return;
      setRename({ phase: "saved", session });
    } catch (error) {
      if (context.signal.aborted) return;
      const failure = failureText(error, String(error));
      setRename(
        mutationSettlementIsUnknown(error)
          ? { phase: "unknown", original: command, failure }
          : { phase: "editing", failure },
      );
    }
  };
  return (
    <Form
      onSubmit={(event) => {
        event.preventDefault();
        if (rename.phase !== "editing" || context.signal.aborted) return;
        void observe(
          context.track(() =>
            context.runtime.sessions.update({
              sessionId: session.id,
              expectedRevision: session.revision,
              title,
            }),
          ),
          "saving",
        );
      }}
    >
      <FormSection title={t("rename.section")}>
        <FormField label={t("rename.titleLabel")}>
          <TextField
            aria-label={t("rename.titleLabel")}
            autoFocus
            value={title}
            pending={busy}
            readOnly={rename.phase !== "editing"}
            onChange={(event) => setTitle(event.target.value)}
          />
        </FormField>
        {failure && (
          <p role="alert">{t("rename.failure", { message: failure })}</p>
        )}
        {unconfirmed && (
          <>
            <p role="status">{t("rename.unconfirmed")}</p>
            <Button
              type="button"
              pending={busy}
              aria-busy={busy || undefined}
              onClick={() => {
                if (rename.phase !== "unknown" || context.signal.aborted)
                  return;
                const { original } = rename;
                void observe(
                  context.track(() => original.retry()),
                  "checking",
                );
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
        {rename.phase === "saved" && (
          <p role="status">
            {t("rename.saved", {
              id: rename.session.id,
              revision: rename.session.revision,
              title: rename.session.title || t("rename.untitled"),
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
          aria-busy={rename.phase === "saving" || undefined}
          disabled={rename.phase !== "editing" || context.signal.aborted}
        >
          {rename.phase === "saving" ? (
            <span role="status">{t("rename.saving")}</span>
          ) : (
            t("rename.save")
          )}
        </Button>
      </FormActions>
    </Form>
  );
}
