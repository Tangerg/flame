import { type ReactNode } from "react";
import {
  Button,
  Checkbox,
  ModalDialog,
  FormActions,
  FormRow,
  FormSection,
  FormField,
  Select,
  TextArea,
  TextField,
} from "@flame/ui";
import { selection, modelKey, DEFAULT_CHOICE } from "./useSchedules";
import { type Edit } from "./types";
import { type ScheduleState, type Translate } from "./useSchedules";

// A model that is no longer offered stays selectable under its saved name, so
// opening an existing schedule never silently changes its model.
function modelOptions(
  t: Translate,
  models: ScheduleState["models"],
  selectedKey: string,
  selectedModel: ScheduleState["selectedModel"],
  saved: Edit["draft"]["modelSelection"],
) {
  const options = [
    { value: DEFAULT_CHOICE, label: t("model.default") },
    ...models
      .filter((model) => !model.deprecated)
      .map((model) => ({
        value: modelKey(model.provider, model.id),
        label: `${model.provider} / ${model.displayName ?? model.id}`,
      })),
  ];
  if (
    selectedKey !== DEFAULT_CHOICE &&
    (!selectedModel || selectedModel.deprecated)
  )
    options.push({
      value: selectedKey,
      label: t("model.existing", {
        provider: saved.provider,
        model: saved.model,
        state: selectedModel?.deprecated
          ? t("model.deprecated")
          : t("model.unavailable"),
      }),
    });
  return options;
}

function reasoningOptions(
  t: Translate,
  selectedModel: ScheduleState["selectedModel"],
  effort: string,
) {
  const levels = selectedModel?.capabilities?.reasoningLevels ?? [];
  const options = [
    { value: DEFAULT_CHOICE, label: t("reasoning.default") },
    ...levels.map((value) => ({ value, label: value })),
  ];
  if (effort && !levels.includes(effort))
    options.push({
      value: effort,
      label: t("reasoning.unavailable", { effort }),
    });
  return options;
}

export function ScheduleEditor({
  t,
  edit,
  feedback,
  busy,
  canSave,
  titleElement,
  resolveOpener,
  cancelTask,
  save,
  patch,
  selectedKey,
  models,
  selectedModel,
  currentModels,
  setModelRefresh,
}: Pick<
  ScheduleState,
  | "t"
  | "busy"
  | "canSave"
  | "titleElement"
  | "resolveOpener"
  | "cancelTask"
  | "save"
  | "patch"
  | "selectedKey"
  | "models"
  | "selectedModel"
  | "currentModels"
  | "setModelRefresh"
> & { edit: Edit; feedback: ReactNode }) {
  const { draft } = edit;
  return (
    <ModalDialog
      open
      title={
        edit.basis
          ? t("editor.edit.title", {
              title: edit.basis.title || t("editor.edit.untitled"),
            })
          : t("editor.new.title")
      }
      description={
        edit.basis
          ? t("editor.edit.description", {
              revision: String(edit.basis.revision),
            })
          : undefined
      }
      closeLabel={t("editor.close")}
      size="lg"
      initialFocus={titleElement}
      finalFocus={resolveOpener}
      dismissible={!busy}
      onOpenChange={(open) => {
        if (!open) cancelTask();
      }}
      onSubmit={(event) => {
        event.preventDefault();
        save();
      }}
      auxiliaryAction={feedback}
      footer={
        <>
          <Button variant="ghost" disabled={busy} onClick={cancelTask}>
            {t("cancel")}
          </Button>
          <Button variant="primary" type="submit" disabled={!canSave}>
            {t("command.save")}
          </Button>
        </>
      }
    >
      <FormField label={t("field.title")}>
        <TextField
          ref={titleElement}
          size="lg"
          aria-label={t("field.title")}
          value={draft.title}
          disabled={busy}
          onChange={(event) => patch("title", event.target.value)}
        />
      </FormField>
      <FormField label={t("field.instructions")}>
        <TextArea
          aria-label={t("field.instructions")}
          value={draft.instructions}
          disabled={busy}
          rows={4}
          onChange={(event) => patch("instructions", event.target.value)}
        />
      </FormField>
      <FormSection title={t("editor.section.timing.title")}>
        <FormRow>
          <FormField label={t("field.cron")}>
            <TextField
              size="lg"
              aria-label={t("field.cron")}
              placeholder="0 9 * * 1"
              value={draft.cron}
              disabled={busy}
              onChange={(event) => patch("cron", event.target.value)}
            />
          </FormField>
          <FormField label={t("field.timeZone")}>
            <TextField
              size="lg"
              aria-label={t("field.timeZone")}
              value={draft.timeZone}
              disabled={busy}
              onChange={(event) => patch("timeZone", event.target.value)}
            />
          </FormField>
          <FormField label={t("field.workspace")}>
            <TextField
              size="lg"
              aria-label={t("field.workspace")}
              placeholder={t("field.workspace.placeholder")}
              value={draft.cwd}
              disabled={busy}
              onChange={(event) => patch("cwd", event.target.value)}
            />
          </FormField>
        </FormRow>
        <Checkbox
          label={t("status.enabled")}
          checked={draft.enabled}
          disabled={busy}
          onCheckedChange={(value) => patch("enabled", value)}
        />
      </FormSection>
      <FormSection title={t("editor.section.execution.title")}>
        <FormRow>
          <FormField label={t("field.model")}>
            <Select
              size="lg"
              label={t("field.model")}
              value={selectedKey}
              disabled={busy}
              options={modelOptions(
                t,
                models,
                selectedKey,
                selectedModel,
                draft.modelSelection,
              )}
              onValueChange={(key) =>
                patch(
                  "modelSelection",
                  selection(
                    models.find(
                      (model) => modelKey(model.provider, model.id) === key,
                    ),
                  ),
                )
              }
            />
          </FormField>
          <FormField label={t("field.reasoning")}>
            <Select
              size="lg"
              label={t("field.reasoning")}
              value={draft.modelSelection.reasoningEffort || DEFAULT_CHOICE}
              disabled={busy || !draft.modelSelection.model}
              options={reasoningOptions(
                t,
                selectedModel,
                draft.modelSelection.reasoningEffort,
              )}
              onValueChange={(value) =>
                patch("modelSelection", {
                  ...draft.modelSelection,
                  reasoningEffort: value === DEFAULT_CHOICE ? "" : value,
                })
              }
            />
          </FormField>
        </FormRow>
        <FormActions
          label={t("models.refresh")}
          feedback={
            currentModels?.error && (
              <p role="alert">
                {t("models.error", { error: currentModels.error })}
              </p>
            )
          }
        >
          <Button
            pending={currentModels?.loading && !busy}
            aria-busy={currentModels?.loading}
            disabled={busy || currentModels?.loading}
            onClick={() => setModelRefresh((value) => value + 1)}
          >
            {currentModels?.loading
              ? t("models.refreshing")
              : t("models.refresh")}
          </Button>
        </FormActions>
      </FormSection>
    </ModalDialog>
  );
}
