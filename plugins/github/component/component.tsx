import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  bindOperations,
  failureText,
  mutationSettlementIsUnknown,
  useTranslator,
  watchRuntimeTopics,
  type ComponentContext,
} from "@flame/component-sdk";
import type { MutationPromise } from "@flame/runtime-client";
import { compareJSONNumbers } from "@frame/sdk/json";
import {
  Button,
  FormActions,
  FormField,
  Metadata,
  Panel,
  PanelSection,
  PanelTitle,
  RecordBody,
  RecordGroup,
  Select,
  TextArea,
  TextField,
} from "@flame/ui";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { de } from "./locales/de";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { ProjectCheckEvidence, ReviewPatch } from "./reviewEvidence";

const catalogs = { en, zh, "zh-TW": zhTW, de, es, fr, ja, ko };
const read = "flame.github/review-work.v1";
const update = "flame.github/update-review-work.v1";
const publish = "flame.github/submit-pull-request-review.v1";
type Target = MethodInputs[typeof read];
type Work = MethodOutputs[typeof read];
type Change = MethodInputs[typeof update];
type Finding = Work["findings"][number];
type Disposition = NonNullable<Change["decision"]>["disposition"];
type Translation = ReturnType<typeof useTranslator>;

function newerWork(current: Work | undefined, observed: Work): Work {
  return current && compareJSONNumbers(current.version, observed.version) > 0
    ? current
    : observed;
}

function locate(target: string): Target | undefined {
  const match =
    /^([\w.-]+\/[\w.-]+)#([1-9][0-9]*)(?:@([a-f0-9]+:[a-f0-9]+))?$/iu.exec(
      target,
    );
  if (!match) return;
  const number = Number(match[2]);
  if (Number.isSafeInteger(number))
    return { repository: match[1]!, number, revision: match[3] };
}

export function GitHubReviews({ context }: { context: ComponentContext }) {
  const t = useTranslator(catalogs, context.locale);
  const [address, setAddress] = useState("");
  const [target, setTarget] = useState<Target>();
  useEffect(
    () =>
      context.views?.subscribeTarget((value) => {
        const selected = locate(value);
        if (selected) {
          setAddress(value);
          setTarget(selected);
        }
      }),
    [context.views],
  );
  const selected = locate(address.trim());

  return (
    <Panel label={t("title")} header={<PanelTitle>{t("title")}</PanelTitle>}>
      <PanelSection title={t("open")}>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (selected) setTarget(selected);
          }}
        >
          <FormField label={t("address")}>
            <TextField
              value={address}
              placeholder="owner/repo#42"
              onChange={(event) => setAddress(event.target.value)}
            />
          </FormField>
          <FormActions label={t("open")}>
            <Button type="submit" disabled={!selected}>
              {t("open")}
            </Button>
          </FormActions>
        </form>
      </PanelSection>
      {target && (
        <Review
          key={JSON.stringify(target)}
          target={target}
          context={context}
          t={t}
        />
      )}
    </Panel>
  );
}

function Review({
  target,
  context,
  t,
}: {
  target: Target;
  context: ComponentContext;
  t: Translation;
}) {
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error("GitHub Reviews requires its own contribution");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      { scope: "installation", reference: context.binding.installationId },
    );
  }, [context]);
  const [state, setState] = useState<Work>();
  const [revision, setRevision] = useState(target.revision);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [readError, setReadError] = useState<string>();
  const [writeError, setWriteError] = useState<string>();
  const [message, setMessage] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [recover, setRecover] = useState<() => void>();
  const selected = state?.selected;
  const recheck = selected?.recheck;
  const stale = selected && selected.headSha !== state.currentHead;
  useEffect(() => {
    const watching = new AbortController();
    let reading: AbortController | undefined;
    let pinned = revision;
    const refresh = () => {
      reading?.abort();
      reading = new AbortController();
      const signal = reading.signal;
      setReadError(undefined);
      setLoading(true);
      void operations.query(read, { ...target, revision: pinned }, signal).then(
        (result) => {
          if (signal.aborted) return;
          pinned ??= result.selected?.id;
          setState((current) => newerWork(current, result));
          setLoading(false);
        },
        (error) => {
          if (signal.aborted) return;
          setReadError(failureText(error));
          setLoading(false);
        },
      );
    };
    void context
      .track(() =>
        watchRuntimeTopics(
          context.runtime,
          { topics: ["plugins.dataChanged"] },
          AbortSignal.any([watching.signal, context.signal]),
          refresh,
        ),
      )
      .catch((error) => {
        if (watching.signal.aborted) return;
        if (reading) setReadError(failureText(error));
        else refresh();
      });
    return () => {
      watching.abort();
      reading?.abort();
    };
  }, [context, operations, revision, target]);

  async function apply<T>(
    command: MutationPromise<T>,
    accept: (value: T) => void,
  ) {
    setBusy(true);
    setWriteError(undefined);
    setRecover(undefined);
    try {
      accept(await command);
    } catch (error) {
      setWriteError(failureText(error));
      if (mutationSettlementIsUnknown(error))
        setRecover(() => () => {
          void apply(command.retry(), accept);
        });
    } finally {
      setBusy(false);
    }
  }
  const change = (
    input: Pick<Change, "finding" | "decision" | "follow" | "revision">,
  ) => {
    if (!state || busy || loading || recover) return;
    void apply(
      operations.command(update, {
        ...target,
        ...input,
        expectedVersion: state.version,
      }),
      (value) => {
        setState((current) => newerWork(current, value));
        setRevision(value.selected?.id);
      },
    );
  };
  const pendingPublications = context.signal.aborted
    ? []
    : operations.pendingCommands(publish);
  const recoverPublication = async (
    original: (typeof pendingPublications)[number],
  ) => {
    setBusy(true);
    setWriteError(undefined);
    try {
      const receipt = await original.recover();
      setMessage(t("published", { id: String(receipt.id) }));
    } catch (error) {
      setWriteError(failureText(error));
    } finally {
      setBusy(false);
    }
  };
  const attach = (label: string, text: string) => {
    if (!context.composer?.addReference({ label, text }))
      setMessage(t("composerUnavailable"));
  };
  const link =
    selected &&
    context.views?.href(
      context.component.componentId,
      `${state.repository}#${state.number}@${selected.id}`,
    );

  return (
    <PanelSection
      title={selected?.title ?? `${target.repository} #${target.number}`}
    >
      {readError && <p role="alert">{readError}</p>}
      {writeError && <p role="alert">{writeError}</p>}
      {message && <p role="status">{message}</p>}
      {!state && !readError && <p role="status">{t("loading")}</p>}
      {state && (
        <>
          <FormActions label={t("revision")}>
            <Button
              disabled={busy || loading || !!recover}
              onClick={() => change({ revision: undefined })}
            >
              {t("inspect")}
            </Button>
            {selected && (
              <>
                <Button
                  disabled={busy || loading || !!recover}
                  onClick={() => change({ revision: selected.id })}
                >
                  {t("checksRefresh")}
                </Button>
                <Button
                  disabled={busy || loading || !!recover}
                  onClick={() =>
                    change({ revision: selected.id, follow: !state.following })
                  }
                >
                  {t(state.following ? "pauseFollow" : "follow")}
                </Button>
              </>
            )}
            {recover && (
              <Button disabled={busy} onClick={recover}>
                {t("recover")}
              </Button>
            )}
            {link && <a href={link}>{t("revisionLink")}</a>}
          </FormActions>
          {state.following && <Metadata>{t("following")}</Metadata>}
          {state.followError && <p role="alert">{state.followError}</p>}
          {selected && (
            <>
              <Select
                label={t("revision")}
                value={selected.id}
                disabled={busy || loading}
                onValueChange={setRevision}
                options={state.versions.map(({ id, baseSha, headSha }) => ({
                  value: id,
                  label: `${baseSha.slice(0, 12)} → ${headSha.slice(0, 12)}`,
                }))}
              />
              {stale && (
                <p role="status">
                  {t("stalePublish", { sha: state.currentHead.slice(0, 12) })}
                </p>
              )}
              {recheck && (
                <RecordGroup aria-label={t("localRecheck")}>
                  <Metadata>
                    {t("recheck." + recheck.status)}
                    {recheck.checkedHead && (
                      <>
                        {" "}
                        · <code>{recheck.checkedHead}</code>
                      </>
                    )}
                  </Metadata>
                  {recheck.diagnostic && (
                    <RecordBody>
                      <pre className="break-words whitespace-pre-wrap">
                        {recheck.diagnostic}
                      </pre>
                    </RecordBody>
                  )}
                </RecordGroup>
              )}
              {recheck?.project && (
                <ProjectCheckEvidence project={recheck.project} t={t} />
              )}
              <RecordGroup aria-label={t("checks")}>
                {state.checks.map((check) => (
                  <RecordBody key={String(check.id)}>
                    {check.name} · {check.conclusion ?? check.status}
                  </RecordBody>
                ))}
                {state.checksPartial && (
                  <Metadata>{t("checksPartial")}</Metadata>
                )}
              </RecordGroup>
              <FindingForm
                key={selected.id}
                t={t}
                disabled={busy || loading || !!recover}
                record={(finding) => change({ finding, revision: selected.id })}
              />
              <RecordGroup aria-label={t("findings")}>
                {state.findings.map((finding) => (
                  <FindingRow
                    key={finding.id}
                    finding={finding}
                    t={t}
                    disabled={busy || loading || !!recover}
                    decide={(disposition, reason) =>
                      change({
                        revision: selected.id,
                        decision: { finding: finding.id, disposition, reason },
                      })
                    }
                    ask={() => {
                      const tip =
                        finding.side === "LEFT"
                          ? selected.baseSha
                          : selected.headSha;
                      const repository =
                        finding.side === "LEFT"
                          ? state.repository
                          : selected.headRepository;
                      attach(
                        `${finding.path}:${finding.line} · ${tip.slice(0, 12)}`,
                        `Pull request ${state.repository} #${state.number}, reviewed revision ${selected.id}. Read ${repository} at commit ${tip}, file ${finding.path}, side ${finding.side}, line ${finding.line}. Finding: ${finding.summary}`,
                      );
                    }}
                  />
                ))}
              </RecordGroup>
              {(state.needsRecheck.length > 0 || recheck?.findingsPartial) && (
                <RecordGroup aria-label={t("recheck")}>
                  <Metadata>{t("recheck")}</Metadata>
                  {state.needsRecheck.map((finding) => {
                    const result = recheck?.findings.find(
                      (value) => value.finding === finding.id,
                    );
                    const sourceId =
                      result?.sourceRevision ?? recheck?.previousRevision;
                    const source = state.versions.find(
                      ({ id }) => id === sourceId,
                    );
                    const from =
                      finding.side === "LEFT"
                        ? source?.baseSha
                        : source?.headSha;
                    const to =
                      finding.side === "LEFT"
                        ? selected.baseSha
                        : selected.headSha;
                    const original = finding.decisions.at(-1);
                    return (
                      <FindingRow
                        key={selected.id + ":" + finding.id}
                        finding={{
                          ...finding,
                          decisions: result?.decisions ?? [],
                        }}
                        t={t}
                        disabled={busy || loading || !!recover}
                        resolve
                        decide={
                          result
                            ? (disposition, reason) =>
                                change({
                                  revision: selected.id,
                                  decision: {
                                    finding: finding.id,
                                    disposition,
                                    reason,
                                  },
                                })
                            : undefined
                        }
                        ask={() =>
                          attach(
                            `${finding.path} · ${to.slice(0, 12)}`,
                            [
                              `Recheck finding from pull request ${state.repository} #${state.number}, side ${finding.side}, file ${finding.path}, original line ${finding.line}.`,
                              `Original commit ${from ?? sourceId ?? "unavailable"}. Selected commit ${to}. Finding: ${finding.summary}`,
                              original &&
                                `Original decision: ${original.disposition}. ${original.reason}`,
                              result && `File comparison: ${result.status}.`,
                              result?.patch,
                              ...(recheck?.project?.checks ?? []).map(
                                ({ name, status }) => `${name}: ${status}`,
                              ),
                            ]
                              .filter(Boolean)
                              .join("\n"),
                          )
                        }
                      >
                        {original && (
                          <Metadata>
                            {t("originalDecision")} · {t(original.disposition)}{" "}
                            · {original.reason}
                          </Metadata>
                        )}
                        {result && (
                          <>
                            <Metadata>
                              {t("findingFile." + result.status)}
                              {from && (
                                <>
                                  {" "}
                                  ·{" "}
                                  <code>
                                    {from.slice(0, 12)} → {to.slice(0, 12)}
                                  </code>
                                </>
                              )}
                            </Metadata>
                            {result.patch && (
                              <ReviewPatch
                                patch={result.patch}
                                truncated={result.patchTruncated}
                                t={t}
                              />
                            )}
                          </>
                        )}
                      </FindingRow>
                    );
                  })}
                  {recheck?.findingsPartial && (
                    <Metadata>{t("findingsPartial")}</Metadata>
                  )}
                </RecordGroup>
              )}
              {!recover &&
                pendingPublications.map((original) => (
                  <Button
                    key={original.idempotencyKey}
                    disabled={busy}
                    onClick={() => void recoverPublication(original)}
                  >
                    {t("recover")}
                  </Button>
                ))}
              <PublishForm
                key={selected.id}
                t={t}
                body={drafts[selected.id] ?? ""}
                edit={(body) =>
                  setDrafts((current) => ({ ...current, [selected.id]: body }))
                }
                disabled={
                  busy ||
                  loading ||
                  !!readError ||
                  !!stale ||
                  !!recover ||
                  pendingPublications.length > 0
                }
                publishReview={(body) => {
                  void apply(
                    operations.command(publish, {
                      repository: state.repository,
                      number: state.number,
                      commitId: selected.headSha,
                      event: "COMMENT",
                      body,
                    }),
                    (receipt) => {
                      setMessage(t("published", { id: String(receipt.id) }));
                      setDrafts((current) =>
                        current[selected.id]?.trim() === body
                          ? { ...current, [selected.id]: "" }
                          : current,
                      );
                    },
                  );
                }}
              />
            </>
          )}
          <RecordGroup aria-label={t("discussion")}>
            {state.discussion.map((comment) => (
              <RecordGroup
                key={`${comment.id}:${comment.updatedAt}:${comment.receivedAtRevision}`}
              >
                <Metadata>
                  {comment.author} · {comment.updatedAt}
                </Metadata>
                <RecordBody>{comment.body}</RecordBody>
                <Button
                  onClick={() =>
                    attach(
                      `#${state.number} · ${comment.author}`,
                      `Continue review of ${state.repository} #${state.number} at ${comment.receivedAtRevision}. Discussion comment ${comment.id} observed at version ${comment.updatedAt}: ${comment.url}\n${comment.body}`,
                    )
                  }
                >
                  {t("continue")}
                </Button>
              </RecordGroup>
            ))}
          </RecordGroup>
        </>
      )}
    </PanelSection>
  );
}

function FindingForm({
  t,
  disabled,
  record,
}: {
  t: Translation;
  disabled: boolean;
  record(finding: NonNullable<Change["finding"]>): void;
}) {
  const [path, setPath] = useState("");
  const [line, setLine] = useState("");
  const [summary, setSummary] = useState("");
  const [side, setSide] = useState<"LEFT" | "RIGHT">("RIGHT");
  const number = /^[1-9][0-9]*$/u.test(line.trim()) ? Number(line) : NaN;
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (path.trim() && Number.isSafeInteger(number) && summary.trim())
          record({
            path: path.trim(),
            side,
            line: number,
            summary: summary.trim(),
          });
      }}
    >
      <FormField label={t("path")}>
        <TextField
          value={path}
          onChange={(event) => setPath(event.target.value)}
        />
      </FormField>
      <FormField label={t("line")}>
        <TextField
          inputMode="numeric"
          value={line}
          onChange={(event) => setLine(event.target.value)}
        />
      </FormField>
      <Select
        label={t("side")}
        value={side}
        onValueChange={setSide}
        options={[
          { value: "LEFT", label: t("base") },
          { value: "RIGHT", label: t("head") },
        ]}
      />
      <FormField label={t("summary")}>
        <TextArea
          autosize
          value={summary}
          onChange={(event) => setSummary(event.target.value)}
        />
      </FormField>
      <FormActions label={t("record")}>
        <Button
          type="submit"
          disabled={
            disabled ||
            !path.trim() ||
            !Number.isSafeInteger(number) ||
            !summary.trim()
          }
        >
          {t("record")}
        </Button>
      </FormActions>
    </form>
  );
}

function FindingRow({
  finding,
  disabled,
  decide,
  ask,
  t,
  resolve = false,
  children,
}: {
  finding: Finding;
  disabled: boolean;
  decide?(disposition: Disposition, reason: string): void;
  ask(): void;
  t: Translation;
  resolve?: boolean;
  children?: ReactNode;
}) {
  const [reason, setReason] = useState("");
  const decision = finding.decisions.at(-1);
  return (
    <RecordGroup>
      <Metadata>
        {finding.path}:{String(finding.line)} ·{" "}
        {t(finding.side === "LEFT" ? "base" : "head")}
      </Metadata>
      <RecordBody>{finding.summary}</RecordBody>
      {children}
      {decision && (
        <Metadata>
          {t(decision.disposition)} · {decision.reason}
        </Metadata>
      )}
      {decide && (
        <FormField label={t("reason")}>
          <TextField
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          />
        </FormField>
      )}
      <FormActions label={t("findings")}>
        {decide && (
          <>
            <Button
              disabled={disabled || !reason.trim()}
              onClick={() => decide("accepted", reason.trim())}
            >
              {t(resolve ? "confirmFinding" : "accept")}
            </Button>
            {resolve && (
              <Button
                disabled={disabled || !reason.trim()}
                onClick={() => decide("resolved", reason.trim())}
              >
                {t("resolveFinding")}
              </Button>
            )}
            <Button
              disabled={disabled || !reason.trim()}
              onClick={() => decide("deferred", reason.trim())}
            >
              {t("defer")}
            </Button>
          </>
        )}
        <Button onClick={ask}>{t("ask")}</Button>
      </FormActions>
    </RecordGroup>
  );
}

function PublishForm({
  t,
  body,
  edit,
  disabled,
  publishReview,
}: {
  t: Translation;
  body: string;
  edit(body: string): void;
  disabled: boolean;
  publishReview(body: string): void;
}) {
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (!disabled && body.trim()) publishReview(body.trim());
      }}
    >
      <FormField label={t("publishComment")}>
        <TextArea
          autosize
          value={body}
          onChange={(event) => edit(event.target.value)}
        />
      </FormField>
      <FormActions label={t("publish")}>
        <Button type="submit" disabled={disabled || !body.trim()}>
          {t("publish")}
        </Button>
      </FormActions>
    </form>
  );
}
