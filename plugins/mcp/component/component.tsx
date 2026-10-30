import {
  useEffect,
  useEffectEvent,
  useId,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import {
  createTranslator,
  defineComponent,
  type ComponentContext,
} from "@flame/component-sdk";
import { Plus, Refresh, Server } from "@flame/icons/react";
import {
  Button,
  EmptyState,
  Icon,
  IconButton,
  RecordGroup,
  Toolbar,
} from "@flame/ui";
import { MCPSettings } from "./settings";
import { ServerRow } from "./serverRow";
import { ServerForm } from "./serverForm";
import { JsonImport } from "./jsonImport";
import { ReadStatus } from "./feedback";
import { serverKey } from "./model";
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
export const MCPServers = defineComponent((source) => {
  const settings = new MCPSettings(source);
  return function MCPSettingsView({ context }) {
    return <McpServersPane context={context} settings={settings} />;
  };
});
function McpServersPane({
  context,
  settings,
}: {
  context: ComponentContext;
  settings: MCPSettings;
}) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const [attempt, setAttempt] = useState(0);
  const observe = useEffectEvent(() => settings.observe(context));
  useEffect(() => observe(), [settings, context.signal, attempt]);
  const [adding, setAdding] = useState<HTMLElement | null>(null);
  const panelId = useId();
  const servers = state.servers.data;
  return (
    <div style={column}>
      <Toolbar label={t("title")}>
        <JsonImport context={context} settings={settings} t={t} />
        <Button
          variant="primary"
          size="sm"
          disabled={adding !== null}
          aria-expanded={adding !== null}
          aria-controls={panelId}
          onClick={(event) => setAdding(event.currentTarget)}
        >
          <Icon glyph={Plus} size="sm" />
          {t("mcp.add")}
        </Button>
        <IconButton
          icon={Refresh}
          aria-label={t("refresh")}
          pending={state.servers.phase === "loading"}
          onClick={() => setAttempt((value) => value + 1)}
        />
      </Toolbar>
      {adding && (
        <div id={panelId}>
          <ServerForm
            context={context}
            settings={settings}
            t={t}
            opener={adding}
            onDone={() => setAdding(null)}
            onCancel={() => setAdding(null)}
          />
        </div>
      )}
      {state.observationFailure && (
        <ReadStatus
          read={{ phase: "failed", error: state.observationFailure.error }}
          retry={() => setAttempt((n) => n + 1)}
          t={t}
        />
      )}
      <ReadStatus
        read={state.servers}
        retry={() => settings.readServers(context)}
        t={t}
      />
      {servers !== undefined &&
        (servers.length ? (
          <RecordGroup variant="list">
            {servers.map((server) => (
              <ServerRow
                key={serverKey(server.id)}
                context={context}
                settings={settings}
                server={server}
                t={t}
              />
            ))}
          </RecordGroup>
        ) : (
          !adding && <EmptyState icon={Server} title={t("mcp.empty")} />
        ))}
    </div>
  );
}
