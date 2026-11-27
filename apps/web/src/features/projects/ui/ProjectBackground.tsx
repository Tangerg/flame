import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Button,
  Checkbox,
  FormActions,
  FormField,
  FormRow,
  Metadata,
  PanelSection,
  TextArea,
  TextField,
} from "@flame/ui";
import { mutationSettlementIsUnknown, type MutationPromise } from "@flame/runtime-client";
import { type ProjectBackgroundPolicy } from "@flame/runtime-client/wire";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { useWorkspaceViews } from "@/features/workspace/public/views";
import { useProjectsAccess } from "@/features/projects/application/projectsContext";
import { type ProjectSummary } from "@/features/projects/application/projectQueries";
import { labelText, useT } from "@/lib/i18n/index";
import { rpcErrorText } from "@/lib/rpcErrors";
import { SystemMessage } from "@/ui/atoms/system-message";

export function ProjectBackground({
  project,
  openSession,
}: {
  project: ProjectSummary;
  openSession(sessionId: string): void;
}) {
  const t = useT();
  const access = useProjectsAccess();
  const workspace = useWorkspace();
  const views = useWorkspaceViews();
  const review = views.find((view) => view.material === "project-pr-reviews");
  const deliveries = views.find((view) => view.material === "project-issue-deliveries");
  const activity = useQuery({
    queryKey: ["project-background", project.id],
    queryFn: ({ signal }) => access.background.get(project.id, signal),
    refetchInterval: 5_000,
  });
  const [draft, edit] = useState<ProjectBackgroundPolicy>();
  const [refs, editRefs] = useState("");
  const [recover, setRecover] = useState<MutationPromise<ProjectBackgroundPolicy>>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const data = activity.data;
  async function settle(command: MutationPromise<ProjectBackgroundPolicy>) {
    setPending(true);
    setError("");
    try {
      await command;
      setRecover(undefined);
      edit(undefined);
      await activity.refetch();
    } catch (cause) {
      setError(rpcErrorText(cause) ?? t("projects.error.save"));
      setRecover(mutationSettlementIsUnknown(cause) ? command : undefined);
    } finally {
      setPending(false);
    }
  }

  return (
    <PanelSection title={t("projects.background.title")}>
      {activity.error && (
        <SystemMessage variant="error">
          {rpcErrorText(activity.error) ?? t("projects.background.loadError")}
        </SystemMessage>
      )}
      {error && <SystemMessage variant="error">{error}</SystemMessage>}
      {deliveries && (
        <Button
          size="sm"
          onClick={(event) =>
            workspace.navigation.openViewInDock(deliveries.id, event.currentTarget, project.id)
          }
        >
          {labelText(deliveries.title)}
        </Button>
      )}
      {data && (
        <>
          <Metadata>
            {t("projects.background.usage", {
              active: data.active,
              maxActive: data.policy.maxActive,
              daily: data.admitted,
              maxDaily: data.policy.maxDailyRuns,
            })}
          </Metadata>
          {data.policy.paused && <p>{t("projects.background.paused")}</p>}
          {draft ? (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                void settle(
                  access.background.setPolicy({
                    projectId: project.id,
                    policy: {
                      ...draft,
                      pullRequests: refs
                        .split("\n")
                        .map((ref) => ref.trim())
                        .filter(Boolean),
                    },
                  }),
                );
              }}
            >
              <Checkbox
                checked={draft.paused}
                onCheckedChange={(paused) => edit({ ...draft, paused })}
                disabled={pending || !!recover}
                label={t("projects.background.pause")}
              />
              <FormRow>
                <FormField label={t("projects.background.maxActive")}>
                  <TextField
                    type="number"
                    min={1}
                    max={32}
                    required
                    value={draft.maxActive}
                    onChange={(event) => edit({ ...draft, maxActive: Number(event.target.value) })}
                    disabled={pending || !!recover}
                  />
                </FormField>
                <FormField label={t("projects.background.maxDaily")}>
                  <TextField
                    type="number"
                    min={1}
                    max={1000}
                    required
                    value={draft.maxDailyRuns}
                    onChange={(event) =>
                      edit({ ...draft, maxDailyRuns: Number(event.target.value) })
                    }
                    disabled={pending || !!recover}
                  />
                </FormField>
              </FormRow>
              <FormField label={t("projects.background.prs")}>
                <TextArea
                  value={refs}
                  onChange={(event) => editRefs(event.target.value)}
                  placeholder="owner/repo#42"
                  disabled={pending || !!recover}
                />
              </FormField>
              <FormActions label={t("projects.background.settings")}>
                {recover ? (
                  <Button
                    type="button"
                    pending={pending}
                    onClick={() => void settle(recover.retry())}
                  >
                    {t("common.retry")}
                  </Button>
                ) : (
                  <>
                    <Button type="button" onClick={() => edit(undefined)} disabled={pending}>
                      {t("common.cancel")}
                    </Button>
                    <Button type="submit" pending={pending}>
                      {t("projects.home.save")}
                    </Button>
                  </>
                )}
              </FormActions>
            </form>
          ) : (
            <Button
              size="sm"
              onClick={() => {
                edit(data.policy);
                editRefs(data.policy.pullRequests.join("\n"));
              }}
            >
              {t("projects.background.settings")}
            </Button>
          )}
          <Metadata>{t("projects.background.projectId", { id: project.id })}</Metadata>
          {(data.policy.pullRequests.length > 0 || data.recent.length > 0) && (
            <ul>
              {data.policy.pullRequests.map((ref) => (
                <li key={ref}>
                  {review ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={(event) =>
                        workspace.navigation.openViewInDock(review.id, event.currentTarget, ref)
                      }
                    >
                      {ref}
                    </Button>
                  ) : (
                    ref
                  )}
                  {review && <PullRequestState reference={ref} />}
                </li>
              ))}
              {data.recent.map((entry) => (
                <li key={entry.runId}>
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={entry.status === "retired"}
                    onClick={() => openSession(entry.sessionId)}
                  >
                    {entry.source.title}
                  </Button>
                  <Metadata>
                    {entry.source.kind} · {entry.source.version} · {entry.outcome || entry.status}
                  </Metadata>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </PanelSection>
  );
}

function PullRequestState({ reference }: { reference: string }) {
  const t = useT();
  const access = useProjectsAccess();
  const state = useQuery({
    queryKey: ["project-background-pull-request", reference],
    queryFn: ({ signal }) => access.readPullRequest(reference, signal),
    enabled: false,
  });
  const summary = state.data;
  return (
    <>
      <Button
        size="sm"
        variant="ghost"
        pending={state.isFetching}
        onClick={() => void state.refetch()}
      >
        {t("projects.background.refreshPR")}
      </Button>
      {summary && (
        <Metadata>
          {summary.title} · {summary.head.slice(0, 12)}
          {summary.recheck && ` · ${summary.recheck}`}
          {summary.pendingFindings > 0 &&
            ` · ${t("projects.background.pendingFindings", { count: summary.pendingFindings })}`}
        </Metadata>
      )}
      {state.error && <p role="status">{rpcErrorText(state.error) ?? state.error.message}</p>}
    </>
  );
}
