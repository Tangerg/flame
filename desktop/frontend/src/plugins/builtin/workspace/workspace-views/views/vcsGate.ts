import type { IconName } from "@/ui";
import { t } from "@/lib/i18n";

export const gitOffEmpty = (icon: IconName) => ({
  icon,
  title: t("vcs.gitNotAvailable"),
  sub: t("vcs.gitNotAvailableSub"),
});

export const notARepoEmpty = (icon: IconName) => ({
  icon,
  title: t("vcs.notARepo"),
  sub: t("vcs.notARepoSub"),
});
