import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import {
  bindOperations,
  failureText,
  watchRuntimeTopics,
  type ComponentContext,
  type PluginComponentProps,
} from "@flame/component-sdk";
import {
  Button,
  FormActions,
  FormField,
  Metadata,
  Panel,
  PanelSection,
  PanelTitle,
  Select,
  TextField,
} from "@flame/ui";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";

const board = "flame.github/team-board.v1";
const grant = "flame.github/team-grant.v1";
const revoke = "flame.github/team-revoke.v1";
const invite = "flame.github/team-invite.v1";
const deliveries = "flame.github/issue-deliveries.v1";

const english = {
  title: "Team and work intake",
  project: "Open Team and work intake from a project.",
  incoming: "Incoming work",
  empty: "No work items received",
  share: "Share an Issue delivery",
  member: "Team member",
  role: "Role",
  viewer: "Viewer",
  reviewer: "Reviewer",
  publisher: "Publisher",
  projectAccess: "Project task access",
  machineAccess: "Machine and verification access",
  delivery: "Original Issue delivery",
  pick: "Select a delivery",
  grant: "Grant access",
  access: "Team access",
  invite: "Show private invitation",
  link: "Private approval link",
  revoke: "Revoke access",
  revoked: "Revoked",
  audit: "Access audit",
  granted: "Granted access",
  publicationApproved: "Approved the original publication",
  recovery: "Recover original action",
  expires: "Access expires",
};
const chinese: typeof english = {
  title: "团队与工作收件箱",
  project: "请从项目打开团队与工作收件箱。",
  incoming: "收到的工作项",
  empty: "暂未收到工作项",
  share: "共享一条 Issue 交付",
  member: "团队成员",
  role: "角色",
  viewer: "查看者",
  reviewer: "审核者",
  publisher: "发布者",
  projectAccess: "访问项目任务",
  machineAccess: "访问机器与验收证据",
  delivery: "原 Issue 交付",
  pick: "选择交付记录",
  grant: "授予权限",
  access: "团队访问权限",
  invite: "显示私密邀请",
  link: "私密审批链接",
  revoke: "撤销权限",
  revoked: "已撤权",
  audit: "权限审计",
  granted: "已授权",
  publicationApproved: "已批准原发布",
  recovery: "恢复原操作",
  expires: "权限到期",
};

export function GitHubProjectTeam({ context, locale }: PluginComponentProps) {
  const t = locale.startsWith("zh") ? chinese : english;
  const [target, setTarget] = useState<string>();
  useEffect(() => context.views?.subscribeTarget(setTarget), [context.views]);
  const project = target || context.project?.id;
  return (
    <Panel label={t.title} header={<PanelTitle>{t.title}</PanelTitle>}>
      {project ? (
        <ProjectTeam key={project} project={project} context={context} t={t} />
      ) : (
        <p role="status">{t.project}</p>
      )}
    </Panel>
  );
}

function ProjectTeam({
  project,
  context,
  t,
}: {
  project: string;
  context: ComponentContext;
  t: typeof english;
}) {
  const operations = useMemo(() => {
    if (!context.binding)
      throw new Error("Team requires the GitHub contribution");
    return bindOperations<MethodInputs, MethodOutputs>(
      context,
      CAPABILITY_JSON,
      { scope: "installation", reference: context.binding.installationId },
    );
  }, [context]);
  const [snapshot, setSnapshot] = useState<MethodOutputs[typeof board]>();
  const [issues, setIssues] = useState<
    MethodOutputs[typeof deliveries]["items"]
  >([]);
  const [member, setMember] = useState("");
  const [role, setRole] = useState("viewer");
  const [delivery, setDelivery] = useState("");
  const [projectAccess, setProjectAccess] = useState(true);
  const [machineAccess, setMachineAccess] = useState(false);
  const [link, setLink] = useState<{ id: string; url: string }>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [revision, refresh] = useState(0);
  useSyncExternalStore(
    operations.subscribeCommands,
    operations.commandsVersion,
  );
  const commands = [
    ...operations.commands(grant),
    ...operations.commands(revoke),
  ];
  const unconfirmed = commands.find((command) => command.state !== "active");

  useEffect(() => {
    const effect = new AbortController();
    const lifetime = AbortSignal.any([effect.signal, context.signal]);
    let request: AbortController | undefined;
    const observe = () => {
      request?.abort();
      request = new AbortController();
      const signal = AbortSignal.any([request.signal, lifetime]);
      void operations.query(board, { projectId: project }, signal).then(
        (result) => {
          if (signal.aborted) return;
          setSnapshot(result);
          setLink((shown) =>
            result.grants.some(
              (item) => item.id === shown?.id && !item.revokedAt,
            )
              ? shown
              : undefined,
          );
          if (!result.sharing) return;
          void operations
            .query(deliveries, { projectId: project }, signal)
            .then(
              (available) => {
                if (!signal.aborted) setIssues(available.items);
              },
              (cause: unknown) => {
                if (!signal.aborted) setError(failureText(cause));
              },
            );
        },
        (cause: unknown) => {
          if (!signal.aborted) setError(failureText(cause));
        },
      );
    };
    observe();
    void context
      .track(() =>
        watchRuntimeTopics(
          context.runtime,
          { topics: ["plugins.dataChanged"] },
          lifetime,
          observe,
        ),
      )
      .catch((cause: unknown) => {
        if (!lifetime.aborted) setError(failureText(cause));
      });
    return () => {
      effect.abort();
      request?.abort();
    };
  }, [context, operations, project, revision]);

  const perform = async (act: () => Promise<unknown>) => {
    setError("");
    setBusy(true);
    try {
      await act();
      refresh((current) => current + 1);
    } catch (cause) {
      setError(failureText(cause));
    } finally {
      setBusy(false);
    }
  };
  const share = () => {
    void perform(async () => {
      const original = await operations.command(grant, {
        projectId: project,
        deliveryId: delivery,
        member,
        role,
        projectAccess,
        machineAccess,
      });
      const value = await operations.query(
        invite,
        { projectId: project, id: original.id },
        context.signal,
      );
      setLink({ id: original.id, url: value.url });
      setMember("");
    });
  };
  return (
    <>
      {error && <p role="alert">{error}</p>}
      {unconfirmed && (
        <Button
          onClick={() => void perform(() => unconfirmed.recover())}
          disabled={busy}
        >
          {t.recovery}
        </Button>
      )}
      <PanelSection title={t.incoming}>
        {snapshot?.sources.length
          ? snapshot.sources.map((item) => (
              <div key={item.kind + ":" + item.id}>
                <Metadata>
                  {item.kind === "linear" ? "Linear" : "Slack"}
                </Metadata>
                <a
                  href={item.url}
                  target="_blank"
                  rel="noreferrer"
                  className="underline"
                >
                  {item.title}
                </a>
              </div>
            ))
          : snapshot && <Metadata>{t.empty}</Metadata>}
      </PanelSection>
      {snapshot?.sharing && (
        <>
          <PanelSection title={t.share}>
            <FormField label={t.member}>
              <TextField
                value={member}
                onChange={(event) => setMember(event.target.value)}
                maxLength={120}
              />
            </FormField>
            <FormField label={t.delivery}>
              <Select
                value={delivery}
                onValueChange={setDelivery}
                options={[
                  { value: "", label: t.pick },
                  ...issues.map((item) => ({
                    value: item.id,
                    label: item.title,
                  })),
                ]}
              />
            </FormField>
            <FormField label={t.role}>
              <Select
                value={role}
                onValueChange={setRole}
                options={[
                  { value: "viewer", label: t.viewer },
                  { value: "reviewer", label: t.reviewer },
                  { value: "publisher", label: t.publisher },
                ]}
              />
            </FormField>
            <label>
              <input
                type="checkbox"
                checked={projectAccess}
                onChange={(event) => setProjectAccess(event.target.checked)}
              />{" "}
              {t.projectAccess}
            </label>
            <label>
              <input
                type="checkbox"
                checked={machineAccess}
                onChange={(event) => setMachineAccess(event.target.checked)}
              />{" "}
              {t.machineAccess}
            </label>
            <FormActions label={t.grant}>
              <Button
                onClick={share}
                disabled={
                  busy || Boolean(unconfirmed) || !member.trim() || !delivery
                }
              >
                {t.grant}
              </Button>
            </FormActions>
            {link && (
              <FormField label={t.link}>
                <TextField
                  value={link.url}
                  readOnly
                  onFocus={(event) => event.target.select()}
                />
              </FormField>
            )}
          </PanelSection>
          <PanelSection title={t.access}>
            {snapshot.grants.map((item) => (
              <div key={item.id}>
                <Metadata>
                  {item.member} ·{" "}
                  {item.role === "publisher"
                    ? t.publisher
                    : item.role === "reviewer"
                      ? t.reviewer
                      : t.viewer}
                </Metadata>
                {item.revokedAt ? (
                  <Metadata>{t.revoked}</Metadata>
                ) : (
                  <>
                    <Metadata>
                      {t.expires} {new Date(item.expiresAt).toLocaleString()}
                    </Metadata>
                    <FormActions label={item.member}>
                      <Button
                        variant="ghost"
                        disabled={busy}
                        onClick={() =>
                          void perform(async () => {
                            const value = await operations.query(
                              invite,
                              { projectId: project, id: item.id },
                              context.signal,
                            );
                            setLink({ id: item.id, url: value.url });
                          })
                        }
                      >
                        {t.invite}
                      </Button>
                      <Button
                        variant="ghost"
                        disabled={busy || Boolean(unconfirmed)}
                        onClick={() =>
                          void perform(async () => {
                            await operations.command(revoke, {
                              projectId: project,
                              id: item.id,
                            });
                            if (link?.id === item.id) setLink(undefined);
                          })
                        }
                      >
                        {t.revoke}
                      </Button>
                    </FormActions>
                  </>
                )}
              </div>
            ))}
          </PanelSection>
          <PanelSection title={t.audit}>
            {snapshot.audit.map((event) => (
              <Metadata key={event.grantId + event.action + event.at}>
                {snapshot.grants.find((item) => item.id === event.grantId)
                  ?.member ?? event.grantId}
                {" · "}
                {event.action === "publicationApproved"
                  ? t.publicationApproved
                  : event.action === "revoked"
                    ? t.revoked
                    : t.granted}
                {" · "}
                {new Date(event.at).toLocaleString()}
              </Metadata>
            ))}
          </PanelSection>
        </>
      )}
    </>
  );
}
