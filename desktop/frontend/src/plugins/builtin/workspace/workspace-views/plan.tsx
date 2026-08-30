import { EmptyState } from "@/ui";
import { useT } from "@/lib/i18n";
import { planSubtext, usePlanView } from "@/plugins/builtin/workspace/application/planViewModel";
import { PlanList } from "./views/PlanList";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

function PlanTab() {
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

function PlanTabBadge() {
  const view = usePlanView();
  if (view.total === 0) return null;
  return `${view.done}/${view.total}`;
}

export const planView = defineWorkspaceView({
  id: "plan",
  title: "workspace.view.title.plan",
  icon: "list",
  badge: PlanTabBadge,
  order: 120,
  splittable: true,
  component: PlanTab,
});
