import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { Button, HeadingLevel, Panel, PanelSection, PanelTitle, TextArea, vocab } from "@flame/ui";
import { AgentRow } from "@/ui/agent/index";
import { SystemMessage } from "@/ui/atoms/system-message";
import { useT } from "@/lib/i18n/index";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { rpcErrorText } from "@/lib/rpcErrors";
import { useAgent } from "@/features/agent/public/agent";
import {
  sessionDisplayTitle,
  useAgentSessions,
  type AgentSessionSummary,
} from "@/features/agent/public/session";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { useStartProjectSession } from "@/features/projects/application/startProjectSession";
import { selectWorkspaceChat } from "@/features/workspace/public/navigation";
import { useProjectsAccess } from "@/features/projects/application/projectsContext";
import { useProjectHomeTarget } from "@/features/projects/application/projectHome";
import { useProjects, type ProjectSummary } from "@/features/projects/application/projectQueries";
import { ProjectNameDialog } from "@/features/projects/ui/ProjectNameDialog";
import { color, space, type as typeStep } from "@flame/ui/tokens.stylex";

const styles = stylex.create({
  header: { display: "grid", gap: space.s3 },
  actions: { display: "flex", flexWrap: "wrap", gap: space.s2 },
  instructions: { display: "grid", gap: space.s2 },
  save: { display: "flex", justifyContent: "flex-end", gap: space.s2 },
  sessions: { display: "grid", gap: space.s0_5 },
  stamp: { color: color.fgFaint, fontVariantNumeric: "tabular-nums" },
});

const EMPTY_SESSIONS: AgentSessionSummary[] = [];

export function ProjectHome() {
  const t = useT();
  const access = useProjectsAccess();
  const projectId = useProjectHomeTarget(access.home);
  const projects = useProjects();
  const project = projects.data?.find((candidate) => candidate.id === projectId);
  if (!project)
    return (
      <p role="status">{projects.isPending ? t("common.loading") : t("projects.home.missing")}</p>
    );
  return <LoadedProjectHome key={project.id} project={project} />;
}

function LoadedProjectHome({ project }: { project: ProjectSummary }) {
  const t = useT();
  const access = useProjectsAccess();
  const agent = useAgent();
  const workspace = useWorkspace();
  const startSession = useStartProjectSession();
  const { data: sessions = EMPTY_SESSIONS } = useAgentSessions();
  const members = sessions
    .filter((session) => session.projectId === project.id)
    .sort((a, b) => (a.time === b.time ? 0 : a.time < b.time ? 1 : -1));
  const [renaming, setRenaming] = useState(false);
  const [draft, setDraft] = useState(project.instructions);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const dirty = draft.trim() !== project.instructions;

  const run = async (change: () => Promise<unknown>) => {
    setSaving(true);
    setError(null);
    try {
      await change();
    } catch (cause) {
      setError(rpcErrorText(cause) ?? t("projects.error.save"));
    } finally {
      setSaving(false);
    }
  };
  const open = (sessionId: string) => {
    agent.selectAgentSession(sessionId);
    selectWorkspaceChat(workspace);
  };

  return (
    <HeadingLevel value={1}>
      <Panel
        label={project.name}
        header={
          // The hosted frame already names the view, so the Project's own name is
          // the visible page title.
          <header {...stylex.props(styles.header)}>
            <PanelTitle size="lg">{project.name}</PanelTitle>
            <div {...stylex.props(styles.actions)}>
              <Button variant="primary" size="sm" onClick={() => startSession(project.id)}>
                {t("projects.home.newSession")}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setRenaming(true)}>
                {t("projects.rename.title")}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                pending={saving}
                onClick={() =>
                  void run(() => access.commands.update(project, { archived: !project.archived }))
                }
              >
                {project.archived ? t("projects.home.restore") : t("projects.home.archive")}
              </Button>
            </div>
          </header>
        }
      >
        {error && <SystemMessage variant="error">{error}</SystemMessage>}
        <PanelSection title={t("projects.home.instructions")}>
          <div {...stylex.props(styles.instructions)}>
            <TextArea
              autosize
              aria-label={t("projects.home.instructions")}
              placeholder={t("projects.home.instructionsPlaceholder")}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
            />
            {dirty && (
              <div {...stylex.props(styles.save)}>
                <Button variant="ghost" size="sm" onClick={() => setDraft(project.instructions)}>
                  {t("common.cancel")}
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  pending={saving}
                  onClick={() =>
                    void run(() => access.commands.update(project, { instructions: draft.trim() }))
                  }
                >
                  {t("projects.home.save")}
                </Button>
              </div>
            )}
          </div>
        </PanelSection>
        <PanelSection title={t("projects.home.sessions")}>
          {members.length === 0 ? (
            <p {...stylex.props(vocab.muted, typeStep.uiSm)}>{t("projects.home.noSessions")}</p>
          ) : (
            <div {...stylex.props(styles.sessions)}>
              {members.map((session) => (
                <AgentRow
                  key={session.id}
                  icon="chat"
                  onClick={() => open(session.id)}
                  trailing={
                    <span {...stylex.props(styles.stamp, typeStep.uiXs)}>
                      {formatRelative(session.time)}
                    </span>
                  }
                >
                  {sessionDisplayTitle(t, session.title)}
                </AgentRow>
              ))}
            </div>
          )}
        </PanelSection>
      </Panel>
      {renaming && <ProjectNameDialog project={project} onClose={() => setRenaming(false)} />}
    </HeadingLevel>
  );
}
