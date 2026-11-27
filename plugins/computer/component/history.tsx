import { type Translate } from "@flame/component-sdk";
import {
  Badge,
  Button,
  Callout,
  Disclosure,
  EmptyState,
  Metadata,
  PanelSection,
  RecordBody,
  RecordGroup,
  Toolbar,
} from "@flame/ui";
import { Outcome } from "./result";
import { type ComputerView, commandLabel } from "./controls";

export function ConsoleHistory({
  state,
  dateTime,
  t,
}: {
  state: Pick<
    ComputerView,
    | "history"
    | "result"
    | "loadHistory"
    | "inspect"
    | "closeInspection"
    | "page"
  >;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
}) {
  const hasPrevious = Boolean(state.history?.cursors.length);
  return (
    <PanelSection
      title={t("history.title")}
      actions={
        <Button
          disabled={state.history?.loading}
          onClick={() => void state.loadHistory()}
        >
          {t("history.refresh")}
        </Button>
      }
    >
      {state.history?.loading && <p role="status">{t("history.reading")}</p>}
      {state.history?.error && (
        <div role="alert">
          <RecordBody>{state.history.error}</RecordBody>
        </div>
      )}
      {state.history?.value && state.history.value.entries.length === 0 && (
        <EmptyState title={t("history.empty")} align="start" />
      )}
      {state.history?.value?.entries.map((entry) => {
        const result = state.result?.id === entry.id ? state.result : undefined;
        return (
          <RecordGroup key={entry.id}>
            <Metadata>
              {commandLabel(entry.method, t)} ·{" "}
              {dateTime.format(new Date(entry.admittedAt))}
            </Metadata>
            <Disclosure
              title={entry.id}
              open={result !== undefined}
              data-focused={result !== undefined}
              onOpenChange={(open) => {
                if (open) void state.inspect(entry.id);
                else state.closeInspection();
              }}
              actions={
                <>
                  {!entry.settled && (
                    <Badge tone="warning">{t("history.unknown")}</Badge>
                  )}
                  {result && (
                    <Button
                      focusableWhenDisabled
                      disabled={result.loading}
                      onClick={() => void state.inspect(entry.id)}
                    >
                      {t("result.refresh")}
                    </Button>
                  )}
                </>
              }
            >
              {result && (
                <RecordGroup
                  role="region"
                  aria-label={t("result.title")}
                  aria-busy={result.loading}
                >
                  {result.loading && (
                    <p role="status">{t("history.reading")}</p>
                  )}
                  {result.error && (
                    <div role="alert">
                      {result.value && (
                        <Callout kind="warning">{t("result.stale")}</Callout>
                      )}
                      <RecordBody>{result.error}</RecordBody>
                    </div>
                  )}
                  {result.value && (
                    <Outcome record={result.value} t={t} dateTime={dateTime} />
                  )}
                </RecordGroup>
              )}
            </Disclosure>
          </RecordGroup>
        );
      })}
      {(hasPrevious || state.history?.value?.next) && (
        <Toolbar label={t("history.pages")}>
          <Button
            disabled={!hasPrevious || state.history?.loading}
            onClick={() => void state.page("previous")}
          >
            {t("history.previous")}
          </Button>
          <Button
            disabled={!state.history?.value?.next || state.history.loading}
            onClick={() => void state.page("next")}
          >
            {t("history.next")}
          </Button>
        </Toolbar>
      )}
    </PanelSection>
  );
}
