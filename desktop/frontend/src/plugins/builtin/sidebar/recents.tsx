import * as stylex from "@stylexjs/stylex";
import { SectionLabel } from "@/ui";
import { SessionList } from "./ui/SessionList";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndex,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { definePlugin } from "@/plugins/sdk";
import { space } from "@/styles/tokens.stylex";

const rc = stylex.create({
  label: { paddingInline: space.s2, paddingTop: 0, paddingBottom: space.s2 },
});

function RecentsSection() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();

  if (!workIndex.recents?.length) return null;

  return (
    <>
      <SectionLabel className={stylex.props(rc.label).className}>
        {t("workIndex.section.recent")}
      </SectionLabel>
      <SessionList
        sessions={workIndex.recents}
        actions={actions}
        activeSessionId={workIndex.activeSessionId}
      />
    </>
  );
}

export const sidebarRecents = definePlugin({
  name: "flame.builtin.sidebar-recents",
  setup(ctx) {
    contributeWorkIndexItem(ctx, {
      id: "recents",
      order: 10,
      component: RecentsSection,
    });
  },
});
