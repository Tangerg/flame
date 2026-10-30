import {
  useEffect,
  useEffectEvent,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import {
  createTranslator,
  defineComponent,
  type ComponentContext,
} from "@flame/component-sdk";
import { Refresh, Server } from "@flame/icons/react";
import {
  EmptyState,
  IconButton,
  PanelSection,
  RecordBody,
  RecordGroup,
  Toolbar,
} from "@flame/ui";
import { ProviderSettings } from "./settings";
import { ProviderRow } from "./providerRow";
import { RoleSection } from "./roles";
import { ReadFailure } from "./feedback";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };
const column = {
  display: "grid",
  minWidth: 0,
  gap: "var(--layout-field-gap)",
} as const;
export const Providers = defineComponent((source) => {
  const settings = new ProviderSettings(source);
  return function ProviderSettingsView({ context }) {
    return <ProvidersPane context={context} settings={settings} />;
  };
});

function ProvidersPane({
  context,
  settings,
}: {
  context: ComponentContext;
  settings: ProviderSettings;
}) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const [attempt, setAttempt] = useState(0);
  const observe = useEffectEvent(() => settings.observe(context));
  useEffect(() => observe(), [settings, context.signal, attempt]);
  const providers = state.providers.data;
  const unset =
    providers !== undefined &&
    !providers.some((provider) => provider.configured);
  const [onboarding, setOnboarding] = useState({ settings, cameUnset: false });
  if (onboarding.settings !== settings)
    setOnboarding({ settings, cameUnset: unset });
  else if (unset && !onboarding.cameUnset)
    setOnboarding({ settings, cameUnset: true });
  return (
    <div style={column}>
      <Toolbar label={t("title")}>
        <IconButton
          icon={Refresh}
          aria-label={t("refresh")}
          pending={state.providers.phase === "loading"}
          onClick={() => setAttempt((value) => value + 1)}
        />
      </Toolbar>
      {unset && <RecordBody>{t("providers.setup.steps")}</RecordBody>}
      {onboarding.settings === settings &&
        onboarding.cameUnset &&
        providers !== undefined &&
        !unset && <p role="status">{t("providers.setup.done")}</p>}
      {state.observationFailure !== undefined && (
        <ReadFailure
          error={state.observationFailure.error}
          t={t}
          retry={() => setAttempt((value) => value + 1)}
        />
      )}
      {state.providers.phase === "failed" && (
        <ReadFailure
          error={state.providers.error}
          retained={providers !== undefined}
          t={t}
          retry={() => settings.refreshRead(context, "providers")}
        />
      )}
      {providers === undefined ? (
        state.providers.phase === "loading" && (
          <p role="status">{t("reading")}</p>
        )
      ) : providers.length ? (
        <RecordGroup variant="list">
          {providers.map((provider) => (
            <ProviderRow
              key={provider.id}
              context={context}
              settings={settings}
              provider={provider}
              t={t}
            />
          ))}
        </RecordGroup>
      ) : (
        <EmptyState
          icon={Server}
          title={t("providers.empty")}
          sub={t("providers.empty.sub")}
        />
      )}
      <PanelSection title={t("providers.roles.heading")}>
        <RecordGroup variant="list">
          <RoleSection
            kind="utility"
            context={context}
            settings={settings}
            t={t}
          />
          <RoleSection
            kind="embedding"
            context={context}
            settings={settings}
            t={t}
          />
        </RecordGroup>
      </PanelSection>
    </div>
  );
}
