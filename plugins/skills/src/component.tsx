import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { BookOpen, Refresh } from "@flame/icons/react";
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
  Disclosure,
  EmptyState,
  Facts,
  Metadata,
  RecordBody,
  RecordGroup,
  RecordTitle,
  RecordSummary,
  SearchField,
  FilterBar,
  Segmented,
  Toolbar,
} from "@flame/ui";

type SkillSnapshot =
  | { type: "available"; data: SkillDiscovery }
  | { type: "detail"; data: SkillDetail }
  | { type: "library"; data: Page<ManagedSkill> }
  | { type: "review"; data: Page<SkillProposal> };
type SkillRecord = Skill | SkillDetail | ManagedSkill | SkillProposal;
type Target = "available" | "review" | "library" | "detail";
type Read = { loading: boolean; accepted?: SkillSnapshot; error?: string };
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

function SkillRow({
  item,
  review,
  expanded,
  children,
}: {
  item: SkillRecord;
  review: boolean;
  expanded: boolean;
  children?: ReactNode;
}) {
  return (
    <Disclosure
      variant="row"
      defaultOpen={expanded}
      actions={children}
      title={
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
      }
    >
      {"instructions" in item && (
        <FormSection title="Instructions">
          <RecordBody>{item.instructions}</RecordBody>
        </FormSection>
      )}
      <Facts
        values={[
          ["Scope", "scope" in item ? item.scope : undefined],
          ["Lifecycle", "lifecycle" in item ? item.lifecycle : undefined],
          ["Revision", "revision" in item ? item.revision : undefined],
          ["Source path", "path" in item ? item.path : undefined],
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
  const [name, setName] = useState<string>();
  const [search, setSearch] = useState("");
  const [read, setRead] = useState<Read>({ loading: true });
  const reading = useRef<AbortController>(undefined);
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
        case "detail":
          accepted = {
            type: target,
            data: await context.track(
              workspace!.skills.getDiscovered(name!, signal),
            ),
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
  }, [context, workspace, target, name]);
  useEffect(() => {
    void refresh();
    return () => reading.current?.abort();
  }, [refresh]);

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
        void refresh();
        for await (const event of subscription.events) {
          if (signal.aborted) break;
          if (
            event.type === "skills.changed" ||
            event.type === "plugins.changed" ||
            event.type === "files.changed" ||
            event.type === "resync"
          )
            void refresh();
        }
      } catch (error) {
        if (!signal.aborted) setObservationError(failure(error));
      }
    };
    void context.track(observeChanges());
    return () => controller.abort();
  }, [context, workspace, refresh]);

  function navigate(next: Target, selected?: string) {
    if (pending.current || context.signal.aborted) return;
    if (next === target && selected === name) {
      void refresh();
      return;
    }
    reading.current?.abort();
    setRead({ loading: true });
    setTarget(next);
    setName(selected);
    setReview(undefined);
    setNotice(undefined);
    setSearch("");
  }
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
      : snapshot?.type === "detail"
        ? [snapshot.data]
        : (snapshot?.data.data ?? []);
  const matching = rows.filter((item) =>
    `${item.name} ${item.description ?? ""} ${"scope" in item ? item.scope : "lifecycle" in item ? item.lifecycle : ""}`
      .toLocaleLowerCase()
      .includes(search.toLocaleLowerCase()),
  );
  const disabled = Boolean(command) || read.loading || Boolean(read.error);
  const title =
    target === "detail"
      ? "Skill document"
      : sections.find((section) => section.value === target)!.label;

  return (
    <Panel
      label="Skills"
      header={
        <PanelHeader
          title="Skills"
          description={
            target === "library"
              ? "Reusable instructions you manage."
              : "Instructions and proposals for this workspace."
          }
        />
      }
      controls={
        <>
          <Toolbar label="Skill navigation">
            <Segmented
              ariaLabel="Catalog"
              value={target === "detail" ? "available" : target}
              options={sections}
              disabled={Boolean(command)}
              onChange={(value) => navigate(value as Target)}
            />
            {target === "detail" && (
              <Button
                disabled={Boolean(command)}
                onClick={() => navigate("available")}
              >
                Back to available Skills
              </Button>
            )}
          </Toolbar>
          <FilterBar
            label="Skill controls"
            search={
              <SearchField
                size="lg"
                id="search"
                aria-label="Search this catalog"
                placeholder="Name or description"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            }
          >
            <IconButton
              icon={Refresh}
              aria-label="Refresh"
              size="md"
              disabled={read.loading || Boolean(command)}
              onClick={() => void refresh()}
            />
          </FilterBar>
        </>
      }
    >
      {read.loading && <p role="status">Reading Skills…</p>}
      {read.error && (
        <p role="alert">{read.error} The last accepted read is retained.</p>
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
            ? `${rows.length} ${snapshot.type === "review" ? "pending proposals" : snapshot.type === "library" ? "user-managed Skills" : snapshot.type === "detail" ? "resolved document" : "available Skills"}`
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
              expanded={snapshot!.type === "detail"}
            >
              {snapshot!.type === "available" && (
                <Button
                  variant="soft"
                  size="md"
                  disabled={disabled}
                  onClick={() => navigate("detail", item.name)}
                >
                  Read instructions
                </Button>
              )}
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
                      disabled={disabled}
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
          {!matching.length && !read.loading && (
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
    </Panel>
  );
}
