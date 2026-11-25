import { useMemo, useState } from "react";
import type { Translate } from "@flame/component-sdk";
import {
  Button,
  Callout,
  Checkbox,
  Disclosure,
  Metadata,
  RecordBody,
  RecordGroup,
  Toolbar,
} from "@flame/ui";
import { parseJSON, projectNumbers } from "@frame/sdk/json";
import { METHODS } from "../contracts/methods";
import { formatTimestamp } from "./archive";
import { edited, type Draft } from "./live-draft";
import type { LiveEditor, LiveState } from "./live";
import type { Suggestion } from "./types";

// Adopted edits join the draft against the revision the suggestion was made
// on, replacing the draft's own edit of the same target; a draft on another
// revision has to be saved or discarded first.
function merge<Operation>(
  current: Draft<Operation> | undefined,
  base: string,
  adopted: Operation[],
): Draft<Operation> {
  if (!current?.operations.length)
    return { base, operations: adopted, undo: [] };
  const key = (operation: Operation) => {
    const { type, id } = operation as { type: string; id?: string };
    return id === undefined ? undefined : `${type} ${id}`;
  };
  const replaced = new Set(adopted.map(key));
  return edited(current, [
    ...current.operations.filter((operation) => {
      const target = key(operation);
      return target === undefined || !replaced.has(target);
    }),
    ...adopted,
  ]);
}

function SuggestionCard<T, Operation>({
  state,
  live,
  suggestion,
  describe,
  dateTime,
  t,
}: {
  state: LiveState;
  live: LiveEditor<T, Operation>;
  suggestion: Suggestion;
  describe(operation: Operation): string;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
}) {
  const operations = useMemo(
    () =>
      (
        projectNumbers(parseJSON(suggestion.operationsJSON)) as {
          operations: Operation[];
        }
      ).operations,
    [suggestion.operationsJSON],
  );
  const [chosen, setChosen] = useState<ReadonlySet<number>>(
    () => new Set(operations.keys()),
  );
  const draft = live.draft;
  const elsewhere =
    !!draft?.operations.length && draft.base !== suggestion.revision;
  const settle = (outcome: "adopted" | "dismissed") =>
    state.run(METHODS.settle, {
      id: suggestion.documentId,
      suggestion: suggestion.id,
      outcome,
    });
  return (
    <RecordGroup>
      <Metadata>
        {t(`comments.author.${suggestion.author.origin}`)} ·{" "}
        {formatTimestamp(suggestion.createdAt, dateTime)} ·{" "}
        {t("suggestions.revision", { id: suggestion.revision.slice(0, 12) })}
      </Metadata>
      {suggestion.note && <RecordBody>{suggestion.note}</RecordBody>}
      {operations.map((operation, index) => (
        <Checkbox
          key={index}
          checked={chosen.has(index)}
          onCheckedChange={(checked) => {
            const next = new Set(chosen);
            if (checked) next.add(index);
            else next.delete(index);
            setChosen(next);
          }}
          label={describe(operation)}
        />
      ))}
      {elsewhere && <Callout kind="note">{t("suggestions.elsewhere")}</Callout>}
      <Toolbar label={t("suggestions.actions")}>
        <Button
          variant="primary"
          disabled={state.blocked || elsewhere || !chosen.size}
          onClick={() => {
            const adopted = operations.filter((_, index) => chosen.has(index));
            const applied = live.setDraft((current) =>
              current?.operations.length && current.base !== suggestion.revision
                ? current
                : merge(current, suggestion.revision, adopted),
            );
            if (applied) void settle("adopted");
          }}
        >
          {t("suggestions.adopt")}
        </Button>
        <Button
          disabled={state.blocked}
          onClick={() => void settle("dismissed")}
        >
          {t("suggestions.dismiss")}
        </Button>
      </Toolbar>
    </RecordGroup>
  );
}

// Pending suggestions for the document. Adopting fills the local draft, which
// the person still reviews and saves; a suggestion on an earlier revision
// reaches the head through the draft's own reconciliation.
export function Suggestions<T, Operation>({
  state,
  live,
  describe,
  dateTime,
  t,
}: {
  state: LiveState;
  live: LiveEditor<T, Operation>;
  describe(operation: Operation): string;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
}) {
  const pending =
    state.suggestions?.value?.entries.filter(
      (entry) =>
        entry.status === "pending" && entry.documentId === live.documentId,
    ) ?? [];
  if (!pending.length) return null;
  return (
    <Disclosure
      title={t("suggestions.title", { count: pending.length })}
      defaultOpen
    >
      {pending.map((suggestion) => (
        <SuggestionCard
          key={suggestion.id}
          state={state}
          live={live}
          suggestion={suggestion}
          describe={describe}
          dateTime={dateTime}
          t={t}
        />
      ))}
    </Disclosure>
  );
}
