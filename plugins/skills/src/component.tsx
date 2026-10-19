import {
  useCallback,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import { ArrowLeft, BookOpen, Refresh } from "@flame/icons/react";
import {
  mutationSettlementIsUnknown,
  isErrorType,
  type ComponentContext,
} from "@flame/plugin-sdk";
import type { MutationPromise } from "@flame/runtime-contract/client";
import type {
  ManagedSkill,
  Page,
  Skill,
  SkillDetail,
  SkillDiscovery,
  SkillProposal,
} from "@flame/runtime-contract/wire";
import {
  Button,
  Badge,
  Panel,
  PanelHeader,
  IconButton,
  PanelSection,
  RecordForm,
  FormSection,
  FormActions,
  DocumentBody,
  Disclosure,
  EmptyState,
  Facts,
  Icon,
  ListRow,
  Metadata,
  PanelTitle,
  RecordBody,
  RecordButton,
  RecordGroup,
  RecordHeading,
  RecordTitle,
  RecordSummary,
  SearchField,
  FilterBar,
  Segmented,
  Toolbar,
} from "@flame/ui";

type SkillSnapshot =
  | { type: "available"; data: SkillDiscovery }
  | { type: "library"; data: Page<ManagedSkill> }
  | { type: "review"; data: Page<SkillProposal> };
type SkillRecord = Skill | ManagedSkill | SkillProposal;
type Target = SkillSnapshot["type"];
type Read = { loading: boolean; accepted?: SkillSnapshot; error?: string };
type OpenedDocument = {
  name: string;
  loading: boolean;
  accepted?: SkillDetail;
  error?: string;
};
type ScrollPosition = { element: HTMLElement; top: number };
type CatalogReturn = { opener: HTMLElement; scroll: ScrollPosition[] };
type Command = {
  promise: MutationPromise<void>;
  label: string;
  uncertain: boolean;
  error?: string;
};
type Review = {
  proposal: SkillProposal;
  decision: "approve" | "reject";
  opener: HTMLElement;
};
const sections = [
  { value: "available", label: "Available Skills" },
  { value: "review", label: "Skill proposals" },
  { value: "library", label: "Personal Skill library" },
];
const failure = (error: unknown) =>
  error instanceof Error
    ? error.message
    : "The operation could not be completed.";

function scrollPositions(element: HTMLElement | null): ScrollPosition[] {
  const positions: ScrollPosition[] = [];
  for (
    let parent = element?.parentElement;
    parent;
    parent = parent.parentElement
  ) {
    if (/(auto|scroll)/.test(getComputedStyle(parent).overflowY))
      positions.push({ element: parent, top: parent.scrollTop });
  }
  return positions;
}

function SkillRow({
  item,
  review,
  focusAfterRemoval,
  read,
  children,
}: {
  item: SkillRecord;
  review: boolean;
  focusAfterRemoval: RefObject<HTMLElement | null>;
  read?: { disabled: boolean; onOpen: (origin: HTMLElement) => void };
  children?: ReactNode;
}) {
  const element = useRef<HTMLElement>(null);
  const ownedFocus = useRef(false);
  useLayoutEffect(() => {
    const row = element.current;
    if (!row) return;
    ownedFocus.current = false;
    return () => {
      ownedFocus.current = row.contains(row.ownerDocument.activeElement);
    };
  }, []);
  useEffect(() => {
    const row = element.current;
    if (!row) return;
    return () => {
      const next = focusAfterRemoval.current;
      if (
        !row.isConnected &&
        ownedFocus.current &&
        row.ownerDocument.activeElement === row.ownerDocument.body &&
        next?.isConnected
      )
        next.focus();
    };
  }, [focusAfterRemoval]);
  const summary = (
    <RecordSummary
      icon={BookOpen}
      title={item.name}
      description={item.description}
      status={
        <Badge tone={review ? "warning" : "neutral"}>
          {review
            ? "Proposal"
            : "lifecycle" in item
              ? item.lifecycle
              : "scope" in item
                ? item.scope
                : "Available"}
        </Badge>
      }
    />
  );
  if (read)
    return (
      <RecordButton
        ref={(node) => {
          element.current = node;
        }}
        disabled={read.disabled}
        onClick={(event) => read.onOpen(event.currentTarget)}
      >
        {summary}
      </RecordButton>
    );
  if ("lifecycle" in item)
    return (
      <ListRow ref={element}>
        <RecordHeading
          title={item.name}
          description={item.description}
          status={<Badge>{item.lifecycle}</Badge>}
          actions={children}
        />
      </ListRow>
    );
  return (
    <Disclosure ref={element} variant="row" actions={children} title={summary}>
      {"instructions" in item && (
        <FormSection title="Instructions">
          <RecordBody>{item.instructions}</RecordBody>
        </FormSection>
      )}
      <Facts
        values={[
          ["Scope", "scope" in item ? item.scope : undefined],
          ["Revision", "revision" in item ? item.revision : undefined],
          [
            "Installation",
            "installation" in item
              ? item.installation?.installationId
              : undefined,
          ],
          [
            "Release",
            "installation" in item ? item.installation?.digest : undefined,
          ],
          ["Origin", "origin" in item ? item.origin : undefined],
          [
            "Source Session",
            "sourceSession" in item ? item.sourceSession : undefined,
          ],
          [
            "Revises an existing Skill",
            review
              ? String("revises" in item && item.revises === true)
              : undefined,
          ],
        ]}
      />
    </Disclosure>
  );
}
export function Skills({ context }: { context: ComponentContext }) {
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
        setNotice(`${captured.label} completed.`);
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
            ? "The proposal changed. Read the latest revision before deciding again."
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
  const title = sections.find((section) => section.value === target)!.label;
  const skillDocument = opened?.accepted;

  return (
    <Panel
      ref={panel}
      label="Skills"
      onKeyDown={(event) => {
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          event.nativeEvent.isComposing ||
          (!opened && !review)
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        if (review) setReview(undefined);
        else closeDocument();
      }}
      header={
        <PanelHeader
          title="Skills"
          description={
            opened
              ? undefined
              : target === "library"
                ? "Reusable instructions you manage."
                : "Instructions and proposals for this workspace."
          }
        />
      }
      controls={
        opened ? (
          <>
            <Toolbar
              label="Skill document navigation"
              actions={
                <IconButton
                  icon={Refresh}
                  aria-label="Refresh document"
                  size="md"
                  disabled={opened.loading}
                  onClick={() => void readDocument(opened.name)}
                />
              }
            >
              <Button
                variant="ghost"
                size="sm"
                aria-label="Back to available Skills"
                onClick={closeDocument}
              >
                <Icon glyph={ArrowLeft} size="sm" />
                Available Skills
              </Button>
            </Toolbar>
            <Toolbar label="Skill document">
              <PanelTitle ref={heading} tabIndex={-1} size="sm" aria-level={2}>
                {opened.name}
              </PanelTitle>
              {skillDocument && <Badge>{skillDocument.scope}</Badge>}
            </Toolbar>
          </>
        ) : (
          <>
            <Toolbar label="Skill navigation">
              <Segmented
                ariaLabel="Catalog"
                value={target}
                options={sections}
                disabled={Boolean(command)}
                onChange={(value) => navigate(value as Target)}
              />
            </Toolbar>
            <FilterBar
              label="Skill controls"
              search={
                <SearchField
                  ref={searchField}
                  size="lg"
                  id="search"
                  aria-label="Search this catalog"
                  placeholder="Name or description"
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                />
              }
              actions={
                <IconButton
                  icon={Refresh}
                  aria-label="Refresh"
                  size="md"
                  disabled={read.loading || Boolean(command)}
                  onClick={() => void refresh()}
                />
              }
            />
          </>
        )
      }
    >
      {!opened && read.loading && <p role="status">Reading Skills…</p>}
      {!opened && read.error && (
        <p role="alert">
          Could not read {title.toLocaleLowerCase()}: {read.error}
          {snapshot && " The last accepted read is retained."}
        </p>
      )}
      {opened?.loading && <p role="status">Reading {opened.name}…</p>}
      {opened?.error && (
        <p role="alert">
          {skillDocument
            ? `Could not refresh ${opened.name}: ${opened.error}`
            : `Could not read ${opened.name}: ${opened.error}`}
        </p>
      )}
      {opened?.error && skillDocument && (
        <Metadata role="status">
          Showing the last accepted read of {opened.name}.
        </Metadata>
      )}
      {observationError && (
        <p role="alert">
          Live refresh is unavailable: {observationError}. Use Refresh to read
          the current owner.
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {command && (
        <section aria-label="Skill command status">
          <p role={command.uncertain ? "alert" : "status"}>
            {command.uncertain
              ? "The original command outcome is not confirmed. Reconcile it before issuing another command or changing catalogs."
              : `${command.label}…`}
          </p>
          {command.error && <RecordBody>{command.error}</RecordBody>}
          {command.uncertain && (
            <>
              <Metadata>
                Original command: {command.promise.idempotencyKey}
              </Metadata>
              <Button onClick={reconcile}>Reconcile original command</Button>
            </>
          )}
        </section>
      )}
      {review && (
        <RecordForm
          aria-label="Confirm proposal decision"
          opener={review.opener}
        >
          <RecordTitle>
            {review.decision === "approve" ? "Approve" : "Reject"} ·{" "}
            {review.proposal.name}
          </RecordTitle>
          <FormSection title="Proposed skill">
            <RecordBody>{review.proposal.description}</RecordBody>
            <RecordBody>{review.proposal.instructions}</RecordBody>
          </FormSection>
          <Disclosure title="Review target">
            <Facts
              values={[
                ["Workspace", workspace!.ref.path],
                ["Scope", review.proposal.scope],
                ["Revision", review.proposal.revision],
              ]}
            />
          </Disclosure>
          <FormActions label="Proposal confirmation">
            <Button
              variant="ghost"
              disabled={Boolean(command)}
              // oxlint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              onClick={() => setReview(undefined)}
            >
              Cancel
            </Button>
            <Button
              variant={review.decision === "approve" ? "primary" : "tonal"}
              tone={review.decision === "reject" ? "negative" : undefined}
              disabled={disabled}
              onClick={() => {
                const { scope, name, revision } = review.proposal;
                submit(
                  () =>
                    review.decision === "approve"
                      ? workspace!.skills.approveProposal({
                          scope,
                          name,
                          revision,
                        })
                      : workspace!.skills.rejectProposal({
                          scope,
                          name,
                          revision,
                        }),
                  `${review.decision === "approve" ? "Approve" : "Reject"} ${name}`,
                );
              }}
            >
              Confirm {review.decision}
            </Button>
          </FormActions>
        </RecordForm>
      )}
      {skillDocument && (
        <>
          {skillDocument.description && (
            <RecordBody>{skillDocument.description}</RecordBody>
          )}
          <section aria-label={`Instructions for ${skillDocument.name}`}>
            <DocumentBody>
              <RecordBody>{skillDocument.instructions}</RecordBody>
            </DocumentBody>
          </section>
          <Disclosure title="Source and revision">
            <Facts
              values={[
                ["Source path", skillDocument.path],
                ["Revision", skillDocument.revision],
                ["Installation", skillDocument.installation?.installationId],
                ["Release", skillDocument.installation?.digest],
              ]}
            />
          </Disclosure>
        </>
      )}
      <div hidden={Boolean(opened)}>
        <PanelSection
          title={
            <>
              {title} {snapshot && <Badge>{rows.length}</Badge>}
            </>
          }
          description={
            <>
              {target === "library"
                ? "User-managed Skills. Installation-provided Skills are managed with their package."
                : workspace
                  ? `Workspace: ${workspace.ref.path}`
                  : "Select a workspace to inspect available Skills and proposals."}
            </>
          }
        >
          <Metadata role="status" className="sr-only">
            {snapshot
              ? `${rows.length} ${snapshot.type === "review" ? "pending proposals" : snapshot.type === "library" ? "user-managed Skills" : "available Skills"}`
              : "No accepted read."}
          </Metadata>

          {snapshot?.type === "available" &&
            snapshot.data.diagnostics.map((diagnostic, index) => (
              <RecordBody key={index}>
                {diagnostic.name}: {diagnostic.detail}
              </RecordBody>
            ))}
          <RecordGroup variant="list" id="items" aria-label={title}>
            {matching.map((item) => (
              <SkillRow
                key={`${snapshot!.type}:${"scope" in item ? `${item.scope}:${item.name}` : item.name}`}
                item={item}
                review={snapshot!.type === "review"}
                focusAfterRemoval={searchField}
                read={
                  snapshot!.type === "available"
                    ? {
                        disabled: Boolean(command),
                        onOpen: (origin) => openDocument(item.name, origin),
                      }
                    : undefined
                }
              >
                {snapshot!.type === "library" && "lifecycle" in item && (
                  <Button
                    variant="soft"
                    size="md"
                    disabled={disabled}
                    onClick={() =>
                      submit(
                        () =>
                          item.lifecycle === "archived"
                            ? context.runtime.skills.restore(item.name)
                            : context.runtime.skills.archive(item.name),
                        `${item.lifecycle === "archived" ? "Restore" : "Archive"} ${item.name}`,
                      )
                    }
                  >
                    {item.lifecycle === "archived" ? "Restore" : "Archive"}
                  </Button>
                )}
                {snapshot!.type === "review" && "origin" in item && (
                  <Toolbar label={`Proposal actions for ${item.name}`}>
                    {(["reject", "approve"] as const).map((decision) => (
                      <Button
                        variant={decision === "approve" ? "outline" : "wash"}
                        tone={decision === "reject" ? "negative" : undefined}
                        size="md"
                        key={decision}
                        pending={disabled}
                        onClick={(event) =>
                          setReview({
                            proposal: structuredClone(item),
                            decision,
                            opener: event.currentTarget,
                          })
                        }
                      >
                        {decision === "approve" ? "Approve" : "Reject"}
                      </Button>
                    ))}
                  </Toolbar>
                )}
              </SkillRow>
            ))}
            {!matching.length && !read.loading && snapshot && (
              <EmptyState
                icon={BookOpen}
                size="compact"
                title={
                  rows.length
                    ? "No matching Skills in this catalog."
                    : "No Skills in this catalog."
                }
                sub={
                  rows.length
                    ? "Clear the search to show Skills."
                    : "Choose another catalog or refresh after adding a Skill."
                }
              />
            )}
          </RecordGroup>
        </PanelSection>
      </div>
    </Panel>
  );
}
