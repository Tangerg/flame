import * as stylex from "@stylexjs/stylex";
import { useRef, useState, type RefObject } from "react";
import { ModalDialog } from "@flame/ui";
import { AgentRow } from "@/ui/agent/index";
import { Icon } from "@/ui/index";
import { useT } from "@/lib/i18n/index";
import { useSetSessionProject } from "@/features/agent/public/session";
import { useProjects } from "@/features/projects/application/projectQueries";
import { space } from "@flame/ui/tokens.stylex";

const styles = stylex.create({
  list: { display: "grid", gap: space.s0_5 },
});

// The choice settles against the Session revision the row showed; Runtime
// refuses it while a Run could observe the change.
export function MoveToProjectDialog({
  session,
  finalFocus,
  onClose,
}: {
  session: { id: string; revision: number; projectId?: string };
  finalFocus?: RefObject<HTMLElement | null>;
  onClose: () => void;
}) {
  const t = useT();
  const projects = useProjects();
  const setProject = useSetSessionProject();
  const [busy, setBusy] = useState(false);
  const current = useRef<HTMLButtonElement>(null);
  const choose = async (projectId: string | null) => {
    if (busy) return;
    if (projectId === (session.projectId ?? null)) {
      onClose();
      return;
    }
    setBusy(true);
    await setProject(session.id, session.revision, projectId);
    onClose();
  };
  const choices = (projects.data ?? []).filter(
    (project) => !project.archived || project.id === session.projectId,
  );
  return (
    <ModalDialog
      open
      size="sm"
      title={t("projects.move.title")}
      initialFocus={current}
      finalFocus={finalFocus}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <div role="listbox" aria-label={t("projects.move.title")} {...stylex.props(styles.list)}>
        {choices.map((project) => (
          <AgentRow
            key={project.id}
            ref={project.id === session.projectId ? current : undefined}
            role="option"
            aria-selected={project.id === session.projectId}
            icon="blocks"
            disabled={busy}
            onClick={() => void choose(project.id)}
            trailing={
              project.id === session.projectId ? <Icon name="check" size="xs" /> : undefined
            }
          >
            {project.name}
          </AgentRow>
        ))}
        <AgentRow
          ref={session.projectId ? undefined : current}
          role="option"
          aria-selected={!session.projectId}
          icon="x"
          disabled={busy}
          onClick={() => void choose(null)}
          trailing={!session.projectId ? <Icon name="check" size="xs" /> : undefined}
        >
          {t("projects.move.none")}
        </AgentRow>
      </div>
    </ModalDialog>
  );
}
