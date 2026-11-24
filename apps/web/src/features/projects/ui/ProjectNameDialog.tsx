import { useState } from "react";
import { InputDialog } from "@/ui/index";
import { useT } from "@/lib/i18n/index";
import { rpcErrorText } from "@/lib/rpcErrors";
import { useProjectsAccess } from "@/features/projects/application/projectsContext";
import { type ProjectSummary } from "@/features/projects/application/projectQueries";

// Creates a Project when none is given, otherwise renames that exact revision.
export function ProjectNameDialog({
  project,
  onClose,
  onSaved,
}: {
  project?: ProjectSummary;
  onClose: () => void;
  onSaved?: (project: ProjectSummary) => void;
}) {
  const t = useT();
  const access = useProjectsAccess();
  const [name, setName] = useState(project?.name ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const saved = project
        ? await access.commands.update(project, { name: name.trim() })
        : await access.commands.create(name.trim());
      onSaved?.(saved);
      onClose();
    } catch (cause) {
      setBusy(false);
      setError(rpcErrorText(cause) ?? t("projects.error.save"));
    }
  };
  return (
    <InputDialog
      title={project ? t("projects.rename.title") : t("projects.create.title")}
      label={t("projects.name")}
      value={name}
      busy={busy}
      error={error}
      onChange={(value) => {
        setName(value);
        setError(null);
      }}
      onConfirm={() => void submit()}
      onClose={onClose}
      confirmLabel={project ? t("projects.rename.confirm") : t("projects.create.confirm")}
      cancelLabel={t("common.cancel")}
    />
  );
}
