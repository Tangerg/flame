import { EmptyState } from "@/ui";
import { useT } from "@/lib/i18n";
import { planSubtext, usePlanView } from "@/plugins/builtin/workspace/application/planViewModel";
import { PlanList } from "./views/PlanList";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";

export function PlanTab() {
  const t = useT();
  const view = usePlanView();

  return (
    <WorkspaceViewLayout icon="list" titleStrong title="plan.title" sub={planSubtext(t, view)}>
      {view.state === "unavailable" ? (
        <EmptyState
          icon="list"
          title={t("plan.unavailable.title")}
          sub={t("plan.unavailable.sub")}
        />
      ) : view.state === "empty" ? (
        <EmptyState icon="list" title={t("plan.empty.title")} sub={t("plan.empty.sub")} />
      ) : (
        <PlanList steps={view.steps} />
      )}
    </WorkspaceViewLayout>
  );
}
