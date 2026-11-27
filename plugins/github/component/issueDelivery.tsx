import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import {
  bindOperations,
  failureText,
  useTranslator,
  watchRuntimeTopics,
  type ComponentContext,
  type PluginComponentProps,
} from "@flame/component-sdk";
import {
  Button,
  FormActions,
  Metadata,
  Panel,
  PanelSection,
  PanelTitle,
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
import { ProjectCheckEvidence } from "./reviewEvidence";

const catalogs = {
  en: {
    ...en,
    issueTitle: "Issue deliveries",
    issueQueue: "Queue",
    issueProject: "Open Issue deliveries from a project.",
    issueEmpty: "No eligible issues on this page.",
    issueRefresh: "Refresh",
    issueLatest: "Back to latest",
    issueOlder: "Older issues",
    issueOpenRun: "Open conversation",
    issueMissingRun: "The original conversation is no longer available.",
    issueCI: "GitHub checks",
    issueDraft: "Comment for approval",
    issueApprove: "Approve and publish",
    issueReceipt: "Publication receipt",
    issueApproved: "Approved {{at}}",
    issueRecover: "Recover original publication",
    "issueStatus.queued": "Queued",
    "issueStatus.running": "Run in progress",
    "issueStatus.checking": "Verifying",
    "issueStatus.ready": "Awaiting approval",
    "issueStatus.blocked": "Blocked",
    "issueStatus.published": "Published",
    "issueStatus.publicationRefused": "Publication refused",
    "issueStatus.publicationUnconfirmed": "Publication unconfirmed",
    "issueCI.passed": "Passed",
    "issueCI.failed": "Failed",
    "issueCI.pending": "Pending",
    "issueCI.incomplete": "Incomplete",
    "issueCI.missing": "Missing",
    "issueCI.unavailable": "Unavailable",
  },
  zh: {
    ...zh,
    issueTitle: "Issue 交付",
    issueQueue: "队列",
    issueProject: "请从项目中打开 Issue 交付。",
    issueEmpty: "这一页没有符合条件的 Issue。",
    issueRefresh: "刷新",
    issueLatest: "回到最新",
    issueOlder: "较早的 Issue",
    issueOpenRun: "打开会话",
    issueMissingRun: "原会话已不可用。",
    issueCI: "GitHub 检查",
    issueDraft: "待批准的评论",
    issueApprove: "批准并发布",
    issueReceipt: "发布回执",
    issueApproved: "批准时间 {{at}}",
    issueRecover: "找回原发布回执",
    "issueStatus.queued": "已排队",
    "issueStatus.running": "运行中",
    "issueStatus.checking": "验收中",
    "issueStatus.ready": "待批准",
    "issueStatus.blocked": "已阻止",
    "issueStatus.published": "已发布",
    "issueStatus.publicationRefused": "发布被拒绝",
    "issueStatus.publicationUnconfirmed": "发布结果待确认",
    "issueCI.passed": "已通过",
    "issueCI.failed": "失败",
    "issueCI.pending": "等待中",
    "issueCI.incomplete": "证据不完整",
    "issueCI.missing": "缺少证据",
    "issueCI.unavailable": "不可用",
  },
  "zh-TW": zhTW,
  de,
  es,
  fr,
  ja,
  ko,
};
const list = "flame.github/issue-deliveries.v1";
const publish = "flame.github/publish-issue-delivery.v1";
type Page = MethodOutputs[typeof list];
type Translation = ReturnType<typeof useTranslator>;

export function GitHubIssueDeliveries({
  context,
  locale,
}: PluginComponentProps) {
  const t = useTranslator(catalogs, locale);
  const [target, setTarget] = useState<string>();
  useEffect(() => context.views?.subscribeTarget(setTarget), [context.views]);
  const project = target || context.project?.id;
  return (
    <Panel
      label={t("issueTitle")}
      header={<PanelTitle>{t("issueTitle")}</PanelTitle>}
    >
      {project ? (
        <IssueQueue key={project} project={project} context={context} t={t} />
      ) : (
        <p role="status">{t("issueProject")}</p>
      )}
    </Panel>
  );
}

function IssueQueue({
  project,
  context,
  t,
}: {
  project: string;
  context: ComponentContext;
  t: Translation;
}) {
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error("Issue deliveries require their own contribution");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      {
        scope: "installation",
        reference: context.binding.installationId,
      },
    );
  }, [context]);
  const [before, setBefore] = useState<Page["next"]>();
  const [page, setPage] = useState<Page>();
  const [refresh, setRefresh] = useState(0);
  const [readError, setReadError] = useState<string>();
  const [actionError, setActionError] = useState<string>();
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  useSyncExternalStore(
    operations.subscribeCommands,
    operations.commandsVersion,
  );
  const originals = context.signal.aborted ? [] : operations.commands(publish);
  const busy =
    pending || originals.some((command) => command.state === "active");

  useEffect(() => {
    const watching = new AbortController();
    const lifetime = AbortSignal.any([watching.signal, context.signal]);
    let reading: AbortController | undefined;
    const read = () => {
      reading?.abort();
      reading = new AbortController();
      const signal = AbortSignal.any([reading.signal, lifetime]);
      setLoading(true);
      setReadError(undefined);
      void operations.query(list, { projectId: project, before }, signal).then(
        (result) => {
          if (signal.aborted) return;
          setPage(result);
          setLoading(false);
        },
        (error) => {
          if (signal.aborted) return;
          setPage(undefined);
          setReadError(failureText(error));
          setLoading(false);
        },
      );
    };
    window.addEventListener("focus", read, { signal: lifetime });
    void context
      .track(() =>
        watchRuntimeTopics(
          context.runtime,
          { topics: ["plugins.dataChanged"] },
          lifetime,
          read,
        ),
      )
      .catch(() => {
        if (!lifetime.aborted) read();
      });
    return () => {
      watching.abort();
      reading?.abort();
    };
  }, [context, operations, project, before, refresh]);

  async function perform(action: () => Promise<unknown>) {
    setPending(true);
    setActionError(undefined);
    try {
      await action();
    } catch (error) {
      if (!context.signal.aborted) setActionError(failureText(error));
    } finally {
      if (!context.signal.aborted) {
        setPending(false);
        setRefresh((revision) => revision + 1);
      }
    }
  }
  const go = (cursor: Page["next"]) => {
    setPage(undefined);
    setLoading(true);
    setBefore(cursor);
  };

  return (
    <>
      <PanelSection title={t("issueQueue")}>
        <FormActions label={t("issueTitle")}>
          <Button
            pending={loading}
            onClick={() => setRefresh((revision) => revision + 1)}
          >
            {t("issueRefresh")}
          </Button>
          {before !== undefined && (
            <Button onClick={() => go(undefined)}>{t("issueLatest")}</Button>
          )}
          {page?.next !== undefined && (
            <Button onClick={() => go(page.next)}>{t("issueOlder")}</Button>
          )}
        </FormActions>
        {readError && <p role="alert">{readError}</p>}
        {actionError && <p role="alert">{actionError}</p>}
        {page?.items.length === 0 && <p role="status">{t("issueEmpty")}</p>}
        {!!page?.items.length &&
          originals.map(
            (command) =>
              command.state === "recoverable" && (
                <Button
                  key={command.idempotencyKey}
                  disabled={busy || loading}
                  onClick={() => void perform(() => command.recover())}
                >
                  {t("issueRecover")}
                </Button>
              ),
          )}
      </PanelSection>
      {page?.items.map((item) => (
        <PanelSection key={item.id} title={item.title}>
          <Metadata>
            <a
              className="underline"
              href={`https://github.com/${item.repository}/issues/${item.number}`}
            >
              {item.repository} #{String(item.number)}
            </a>
            {" · "}
            {t(`issueStatus.${item.status}`)}
            {item.reason && ` · ${item.reason}`}
          </Metadata>
          <Metadata>
            {item.branch}
            {item.head && (
              <>
                {" "}
                · <code>{item.head}</code>
              </>
            )}
          </Metadata>
          {item.workspace && (
            <Metadata>
              <code className="break-all">{item.workspace}</code>
            </Metadata>
          )}
          {item.sessionId && context.openSession && (
            <Button
              size="sm"
              disabled={busy}
              onClick={() =>
                void perform(async () => {
                  if (
                    (await context.openSession!(item.sessionId!)) === "missing"
                  )
                    throw new Error(t("issueMissingRun"));
                })
              }
            >
              {t("issueOpenRun")}
            </Button>
          )}
          {item.project && (
            <ProjectCheckEvidence project={item.project} t={t} />
          )}
          {item.ci && (
            <details>
              <summary>
                {t("issueCI")} · {t(`issueCI.${item.ci.status}`)}
              </summary>
              {item.ci.checks.map((check) => (
                <Metadata key={String(check.id)}>
                  {check.name} · {check.conclusion || check.status}
                </Metadata>
              ))}
            </details>
          )}
          {item.draft && !item.receipt && (
            <details open={item.status === "ready"}>
              <summary>{t("issueDraft")}</summary>
              <pre className="whitespace-pre-wrap break-words">
                {item.draft}
              </pre>
            </details>
          )}
          {item.approvedAt && (
            <Metadata>{t("issueApproved", { at: item.approvedAt })}</Metadata>
          )}
          {item.receipt?.comment.url.startsWith("https://github.com/") && (
            <a className="underline" href={item.receipt.comment.url}>
              {t("issueReceipt")}
            </a>
          )}
          {item.status === "ready" && item.digest && (
            <Button
              variant="primary"
              disabled={busy || loading}
              onClick={() =>
                void perform(() =>
                  operations.command(publish, {
                    id: item.id,
                    digest: item.digest!,
                  }),
                )
              }
            >
              {t("issueApprove")}
            </Button>
          )}
        </PanelSection>
      ))}
    </>
  );
}
