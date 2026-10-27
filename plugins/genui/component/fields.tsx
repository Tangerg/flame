import { useId, type ReactNode } from "react";
import {
  Button,
  Checkbox,
  Form,
  FormActions,
  FormField,
  FormGroup,
  Metadata,
  RecordTitle,
  Select,
  Slider,
  Switch,
  TextArea,
  TextField,
} from "@flame/ui";
import type { Translate } from "@flame/component-sdk";
import type { ComponentRenderProps } from "@openuidev/react-lang";
import type { DisplayProps } from "../contracts/catalog";
import { validateField, type ValidationIssue } from "../contracts/document";
import { useGenUIInteraction } from "./interaction";
import { moveChoice } from "./layout";
import { useLocalValue } from "./state";

type Translated = { t: Translate };

function FieldFrame({
  field,
  issue,
  errorId,
  t,
  children,
}: Translated & {
  field: string;
  issue?: ValidationIssue;
  errorId: string;
  children: ReactNode;
}) {
  return (
    <div data-genui-field={field} data-genui-invalid={issue ? "" : undefined}>
      {children}
      {issue && (
        <Metadata id={errorId} role="status">
          {t(
            `validation.${issue.code}`,
            issue.limit === undefined ? undefined : { limit: issue.limit },
          )}
        </Metadata>
      )}
    </div>
  );
}

export function InputView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Input"]> & Translated) {
  const [value, setValue] = useLocalValue<string>(props.field);
  const { blocked } = useGenUIInteraction();
  const errorId = useId();
  const issue = validateField(value, props.rules, props.type ?? "text");
  return (
    <FieldFrame field={props.field} issue={issue} errorId={errorId} t={t}>
      <FormField label={props.label}>
        <TextField
          name={props.field}
          type={props.type === "email" ? "email" : "text"}
          inputMode={props.type === "number" ? "decimal" : undefined}
          value={value}
          onChange={(event) => !blocked && setValue(event.currentTarget.value)}
          placeholder={props.placeholder}
          required={props.rules?.required}
          minLength={props.rules?.minLength}
          maxLength={props.rules?.maxLength}
          invalid={issue !== undefined}
          aria-describedby={issue ? errorId : undefined}
          pending={blocked}
        />
      </FormField>
    </FieldFrame>
  );
}

export function TextAreaView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["TextArea"]> & Translated) {
  const [value, setValue] = useLocalValue<string>(props.field);
  const { blocked } = useGenUIInteraction();
  const errorId = useId();
  const issue = validateField(value, props.rules, "text");
  return (
    <FieldFrame field={props.field} issue={issue} errorId={errorId} t={t}>
      <FormField label={props.label}>
        <TextArea
          name={props.field}
          value={value}
          rows={props.rows}
          onChange={(event) => !blocked && setValue(event.currentTarget.value)}
          placeholder={props.placeholder}
          required={props.rules?.required}
          minLength={props.rules?.minLength}
          maxLength={props.rules?.maxLength}
          invalid={issue !== undefined}
          aria-describedby={issue ? errorId : undefined}
          pending={blocked}
        />
      </FormField>
    </FieldFrame>
  );
}

export function SelectView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Select"]> & Translated) {
  const [value, setValue] = useLocalValue<string>(props.field);
  const { blocked } = useGenUIInteraction();
  const errorId = useId();
  const issue = validateField(value, props.rules);
  return (
    <FieldFrame field={props.field} issue={issue} errorId={errorId} t={t}>
      <FormGroup label={props.label}>
        <Select
          label={props.label}
          value={value || null}
          options={props.options}
          onValueChange={(next) => !blocked && setValue(next)}
          disabled={blocked}
        />
      </FormGroup>
    </FieldFrame>
  );
}

export function MultiSelectView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["MultiSelect"]> & Translated) {
  const [values, setValues] = useLocalValue<string[]>(props.field);
  const { blocked } = useGenUIInteraction();
  const errorId = useId();
  const issue = validateField(values, props.rules);
  return (
    <FieldFrame field={props.field} issue={issue} errorId={errorId} t={t}>
      <FormGroup label={props.label}>
        {props.options.map((option) => (
          <Checkbox
            key={option.value}
            label={option.label}
            checked={values.includes(option.value)}
            disabled={blocked}
            onCheckedChange={(checked) => {
              if (!blocked) {
                setValues(
                  checked
                    ? [...values, option.value]
                    : values.filter((value) => value !== option.value),
                );
              }
            }}
          />
        ))}
      </FormGroup>
    </FieldFrame>
  );
}

export function RadioGroupView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["RadioGroup"]> & Translated) {
  const [value, setValue] = useLocalValue<string>(props.field);
  const { blocked } = useGenUIInteraction();
  const errorId = useId();
  const issue = validateField(value, props.rules);
  return (
    <FieldFrame field={props.field} issue={issue} errorId={errorId} t={t}>
      <FormGroup label={props.label}>
        <div
          role="radiogroup"
          aria-label={props.label}
          aria-invalid={issue !== undefined}
          aria-describedby={issue ? errorId : undefined}
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: "calc(var(--spacing) * 2)",
          }}
        >
          {props.options.map((option, index) => (
            <Button
              key={option.value}
              role="radio"
              aria-checked={value === option.value}
              tabIndex={
                value === option.value || (!value && index === 0) ? 0 : -1
              }
              active={value === option.value}
              variant="outline"
              disabled={blocked}
              onClick={() => setValue(option.value)}
              onKeyDown={(event) => {
                if (!blocked)
                  moveChoice(event, (next) =>
                    setValue(props.options[next]!.value),
                  );
              }}
            >
              {option.label}
            </Button>
          ))}
        </div>
      </FormGroup>
    </FieldFrame>
  );
}

export function CheckboxView({
  props,
}: ComponentRenderProps<DisplayProps["Checkbox"]>) {
  const [value, setValue] = useLocalValue<boolean>(props.field);
  const { blocked } = useGenUIInteraction();
  return (
    <div data-genui-field={props.field}>
      <Checkbox
        label={props.label}
        checked={value}
        onCheckedChange={setValue}
        disabled={blocked}
      />
    </div>
  );
}

export function SwitchView({
  props,
}: ComponentRenderProps<DisplayProps["Switch"]>) {
  const [value, setValue] = useLocalValue<boolean>(props.field);
  const { blocked } = useGenUIInteraction();
  return (
    <div data-genui-field={props.field}>
      <FormGroup label={props.label}>
        <Switch
          ariaLabel={props.label}
          checked={value}
          onCheckedChange={setValue}
          disabled={blocked}
        />
      </FormGroup>
    </div>
  );
}

export function SliderView({
  props,
}: ComponentRenderProps<DisplayProps["Slider"]>) {
  const [value, setValue] = useLocalValue<number>(props.field);
  const { blocked } = useGenUIInteraction();
  return (
    <div data-genui-field={props.field}>
      <FormGroup label={props.label}>
        <div inert={blocked || undefined} aria-disabled={blocked || undefined}>
          <Slider
            ariaLabel={props.label}
            value={value}
            min={props.min}
            max={props.max}
            step={props.step}
            onValueChange={(next) => !blocked && setValue(next)}
          />
        </div>
        <output aria-label={props.label}>{String(value)}</output>
      </FormGroup>
    </div>
  );
}

export function DatePickerView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["DatePicker"]> & Translated) {
  const [value, setValue] = useLocalValue<string>(props.field);
  const { blocked } = useGenUIInteraction();
  const errorId = useId();
  const issue = validateField(value, undefined, "date", props);
  return (
    <FieldFrame field={props.field} issue={issue} errorId={errorId} t={t}>
      <FormField label={props.label}>
        <TextField
          type="date"
          name={props.field}
          value={value}
          min={props.min}
          max={props.max}
          invalid={issue !== undefined}
          aria-describedby={issue ? errorId : undefined}
          onChange={(event) => !blocked && setValue(event.currentTarget.value)}
          pending={blocked}
        />
      </FormField>
    </FieldFrame>
  );
}

export function FormView({
  props,
  renderNode,
  t,
}: ComponentRenderProps<DisplayProps["Form"]> & Translated) {
  const interaction = useGenUIInteraction();
  return (
    <Form
      aria-label={props.title ?? props.submitLabel}
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        if (interaction.blocked) return;
        void interaction.execute(props.action);
      }}
      onReset={(event) => {
        event.preventDefault();
        if (interaction.blocked) return;
        interaction.resetForm(props.action);
      }}
    >
      {props.title && <RecordTitle>{props.title}</RecordTitle>}
      {renderNode(props.children)}
      <FormActions label={t("form.actions")}>
        <Button type="submit" variant="primary" disabled={interaction.blocked}>
          {props.submitLabel}
        </Button>
        {props.resetLabel && (
          <Button type="reset" disabled={interaction.blocked}>
            {props.resetLabel}
          </Button>
        )}
      </FormActions>
    </Form>
  );
}

export function ButtonView({
  props,
}: ComponentRenderProps<DisplayProps["Button"]>) {
  const interaction = useGenUIInteraction();
  return (
    <Button
      variant="outline"
      tone={props.tone}
      disabled={interaction.blocked || props.disabled}
      onClick={() => void interaction.execute(props.action)}
    >
      {props.label}
    </Button>
  );
}
