import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import {
  createTranslator,
  failureText,
  type ComponentContext,
} from "@flame/component-sdk";
import {
  Panel,
  PanelHeader,
  RecordGroup,
  ListRow,
  RecordBody,
  Switch,
} from "@flame/ui";
import { catalogs } from "./catalogs";
import { announceSettlement } from "./policy";
import {
  readPreferences,
  writePreference,
  type Preferences,
} from "./preferences";

export function CompletionNotifications({
  context,
}: {
  context: ComponentContext;
}) {
  const announce = useEffectEvent(
    (
      settlement: Parameters<typeof announceSettlement>[1],
      signal: AbortSignal,
    ) => announceSettlement(context, settlement, signal),
  );
  const subscribe = context.subscribeRootRunSettlements;
  useEffect(() => subscribe(announce), [subscribe]);
  return null;
}

export function NotificationSettings({
  context,
}: {
  context: ComponentContext;
}) {
  const t = useMemo(
    () => createTranslator(catalogs, context.locale),
    [context.locale],
  );
  const channel = context.notifications;
  const authorization = useSyncExternalStore(
    channel.subscribe,
    channel.authorization,
  );
  const [preferences, setPreferences] = useState(readPreferences);
  const [error, setError] = useState<string>();
  const [requesting, setRequesting] = useState(false);
  const failed = useCallback(
    (failure: unknown) => {
      if (!context.signal.aborted)
        setError(failureText(failure, t("settings.failed")));
    },
    [context.signal, t],
  );
  useEffect(() => {
    void channel.refreshAuthorization().catch(failed);
  }, [channel, failed]);
  const change = (name: keyof Preferences, value: boolean) => {
    try {
      context.signal.throwIfAborted();
      writePreference(name, value);
      setPreferences((current) => ({ ...current, [name]: value }));
      setError(undefined);
      return true;
    } catch (failure) {
      failed(failure);
      return false;
    }
  };
  const system = async (value: boolean) => {
    if (requesting) return;
    if (!change("systemNotifications", value) || !value) return;
    setRequesting(true);
    try {
      await channel.requestAuthorization();
    } catch (failure) {
      failed(failure);
    } finally {
      if (!context.signal.aborted) setRequesting(false);
    }
  };
  const blocked = authorization === "denied" || authorization === "unsupported";
  return (
    <Panel label={t("panel.title")}>
      <PanelHeader title={t("panel.title")} />
      {error && (
        <div role="alert">
          <RecordBody>{error}</RecordBody>
        </div>
      )}
      <RecordGroup variant="list">
        <ListRow style={{ display: "flex", alignItems: "center", gap: "1rem" }}>
          <div style={{ flex: 1 }}>
            <strong>{t("settings.notifications")}</strong>
            <RecordBody>
              {t(`settings.notifications.authorization.${authorization}`)}
            </RecordBody>
          </div>
          <Switch
            checked={preferences.systemNotifications && !blocked}
            disabled={blocked || requesting}
            ariaLabel={t("settings.notifications")}
            onCheckedChange={(value) => {
              void system(value);
            }}
          />
        </ListRow>
        <ListRow style={{ display: "flex", alignItems: "center", gap: "1rem" }}>
          <div style={{ flex: 1 }}>
            <strong>{t("settings.completionSound")}</strong>
            <RecordBody>{t("settings.completionSound.sub")}</RecordBody>
          </div>
          <Switch
            checked={preferences.completionSound}
            ariaLabel={t("settings.completionSound")}
            onCheckedChange={(value) => change("completionSound", value)}
          />
        </ListRow>
      </RecordGroup>
    </Panel>
  );
}
