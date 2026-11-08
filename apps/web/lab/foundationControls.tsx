import { useId, useState, type ComponentProps } from "react";
import * as stylex from "@stylexjs/stylex";
import { Bell } from "@flame/icons/react";
import {
  Button,
  Checkbox,
  FormField,
  IconButton,
  SearchField,
  Select,
  Switch,
  TextArea,
  TextField,
} from "@flame/ui";
import { foundationMessages } from "./foundationMessages";
import { foundationStyles as styles } from "./foundationStyles";
import {
  foundationButtonVariants,
  foundationControlSizes,
  foundationViews as views,
} from "./foundationVariants";
import type { LabExampleProps } from "./story";

const CONTROL_STATES = [views.editable, views.disabled, views.pending] as const;
const ICON_SIZES = [...foundationControlSizes, "xl"] as const satisfies readonly ComponentProps<
  typeof IconButton
>["size"][];
const FIELD_VARIANTS = ["boxed", "bare", "inline"] as const satisfies readonly ComponentProps<
  typeof TextField
>["variant"][];
const FIELD_SIZES = ["sm", "md", "lg"] as const satisfies readonly ComponentProps<
  typeof TextField
>["size"][];
const AREA_VARIANTS = [...FIELD_VARIANTS, "well"] as const satisfies readonly ComponentProps<
  typeof TextArea
>["variant"][];
const UNREAD_NOTIFICATIONS = 3;

export function ButtonExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  if (variant === views.sizes.id) {
    return (
      <div {...stylex.props(styles.row, styles.comparison)}>
        {foundationControlSizes.map((size) => (
          <div key={size} {...stylex.props(styles.pair)}>
            <span {...stylex.props(styles.label)}>{size}</span>
            <Button size={size} variant="primary" onClick={() => observe("click", size)}>
              {copy.continue}
            </Button>
          </div>
        ))}
      </div>
    );
  }
  if (variant === views.states.id) {
    return (
      <div {...stylex.props(styles.row, styles.comparison)}>
        {CONTROL_STATES.map((state) => (
          <div key={state.id} {...stylex.props(styles.pair)}>
            <span {...stylex.props(styles.label)}>{state.title[locale]}</span>
            <Button
              variant="primary"
              disabled={state.id === views.disabled.id}
              pending={state.id === views.pending.id}
              onClick={() => observe("click", state.id)}
            >
              {copy.continue}
            </Button>
          </div>
        ))}
      </div>
    );
  }
  return (
    <div {...stylex.props(styles.row, styles.comparison)}>
      {foundationButtonVariants.map((appearance) => (
        <div key={appearance} {...stylex.props(styles.pair)}>
          <span {...stylex.props(styles.label)}>{appearance}</span>
          <div
            {...stylex.props(
              (appearance === "media" || appearance === "mediaTray") && styles.media,
            )}
          >
            <Button variant={appearance} onClick={() => observe("click", appearance)}>
              {copy.continue}
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}

export function IconButtonExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [unread, setUnread] = useState(UNREAD_NOTIFICATIONS);
  const activate = (detail: string) => {
    setUnread((current) => (current === 0 ? UNREAD_NOTIFICATIONS : 0));
    observe("click", detail);
  };
  if (variant === views.states.id) {
    return (
      <div {...stylex.props(styles.row, styles.comparison)}>
        {CONTROL_STATES.map((state) => (
          <div key={state.id} {...stylex.props(styles.pair)}>
            <span {...stylex.props(styles.label)}>{state.title[locale]}</span>
            <IconButton
              icon={Bell}
              aria-label={copy.notifications}
              disabled={state.id === views.disabled.id}
              pending={state.id === views.pending.id}
              badge={unread}
              onClick={() => activate(state.id)}
            />
          </div>
        ))}
      </div>
    );
  }
  return (
    <div {...stylex.props(styles.row, styles.comparison)}>
      {ICON_SIZES.map((size) => (
        <div key={size} {...stylex.props(styles.pair)}>
          <span {...stylex.props(styles.label)}>{size}</span>
          <IconButton
            icon={Bell}
            size={size}
            aria-label={copy.notifications}
            badge={unread}
            onClick={() => activate(size)}
          />
        </div>
      ))}
    </div>
  );
}

export function TextFieldExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [value, setValue] = useState(variant === views.states.id ? "" : copy.draft);
  const feedback = useId();
  const change = (next: string) => {
    setValue(next);
    observe("valueChange.length", next.length);
  };
  if (variant === views.sizes.id) {
    return (
      <div {...stylex.props(styles.stack)}>
        {FIELD_SIZES.map((size) => (
          <FormField key={size} label={size}>
            <TextField size={size} value={value} onChange={(event) => change(event.target.value)} />
          </FormField>
        ))}
      </div>
    );
  }
  if (variant === views.states.id) {
    const invalid = value.trim().length === 0;
    return (
      <div {...stylex.props(styles.stack)}>
        <FormField label={copy.name}>
          <TextField
            value={value}
            invalid={invalid}
            aria-describedby={invalid ? feedback : undefined}
            onChange={(event) => change(event.target.value)}
          />
        </FormField>
        {invalid && <p id={feedback}>{copy.invalid}</p>}
        {CONTROL_STATES.map((state) => (
          <FormField key={state.id} label={state.title[locale]}>
            <TextField
              value={value}
              disabled={state.id === views.disabled.id}
              pending={state.id === views.pending.id}
              onChange={(event) => change(event.target.value)}
            />
          </FormField>
        ))}
      </div>
    );
  }
  return (
    <div {...stylex.props(styles.stack)}>
      {FIELD_VARIANTS.map((appearance) => (
        <FormField key={appearance} label={appearance}>
          <TextField
            variant={appearance}
            value={value}
            onChange={(event) => change(event.target.value)}
          />
        </FormField>
      ))}
    </div>
  );
}

export function TextAreaExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [value, setValue] = useState(copy.draft);
  const change = (next: string) => {
    setValue(next);
    observe("valueChange.length", next.length);
  };
  if (variant === views.autosize.id) {
    return (
      <FormField label={copy.note}>
        <TextArea
          autosize
          rows={1}
          value={value}
          placeholder={copy.notePlaceholder}
          styles={styles.area}
          onChange={(event) => change(event.target.value)}
        />
      </FormField>
    );
  }
  if (variant === views.states.id) {
    return (
      <div {...stylex.props(styles.stack)}>
        {CONTROL_STATES.map((state) => (
          <FormField key={state.id} label={state.title[locale]}>
            <TextArea
              rows={2}
              value={value}
              disabled={state.id === views.disabled.id}
              pending={state.id === views.pending.id}
              onChange={(event) => change(event.target.value)}
            />
          </FormField>
        ))}
      </div>
    );
  }
  return (
    <div {...stylex.props(styles.stack)}>
      {AREA_VARIANTS.map((appearance) => (
        <FormField key={appearance} label={appearance}>
          <TextArea
            variant={appearance}
            rows={2}
            value={value}
            onChange={(event) => change(event.target.value)}
          />
        </FormField>
      ))}
    </div>
  );
}

export function SearchExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [value, setValue] = useState("");
  const change = (next: string) => {
    setValue(next);
    observe("queryChange.length", next.length);
  };
  const examples = [copy.trace, copy.metrics, copy.logs].filter((label) =>
    label.toLocaleLowerCase(locale).includes(value.toLocaleLowerCase(locale)),
  );
  return (
    <div {...stylex.props(styles.stack)}>
      <SearchField
        value={value}
        onChange={(event) => change(event.target.value)}
        onClear={() => change("")}
        clearLabel={copy.clear}
        aria-label={copy.search}
        placeholder={copy.searchPlaceholder}
        disabled={variant === views.disabled.id}
        pending={variant === views.pending.id}
      />
      {examples.length ? (
        <ul {...stylex.props(styles.list)}>
          {examples.map((label) => (
            <li key={label}>{label}</li>
          ))}
        </ul>
      ) : (
        <p role="status">{copy.noMatches}</p>
      )}
    </div>
  );
}

export function CheckboxExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [checked, setChecked] = useState(false);
  return (
    <Checkbox
      checked={checked}
      disabled={variant === views.disabled.id}
      label={copy.includeDetails}
      onCheckedChange={(next) => {
        setChecked(next);
        observe("checkedChange", next);
      }}
    />
  );
}

export function SwitchExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [checked, setChecked] = useState(false);
  return (
    <div {...stylex.props(styles.row)}>
      <span>{copy.enableNotifications}</span>
      <Switch
        checked={checked}
        disabled={variant === views.disabled.id}
        ariaLabel={copy.enableNotifications}
        onCheckedChange={(next) => {
          setChecked(next);
          observe("checkedChange", next);
        }}
      />
    </div>
  );
}

export function SelectExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [value, setValue] = useState<string | null>(null);
  const feedback = useId();
  const invalid = variant === views.invalid.id && value === null;
  const options = [
    { value: "ready", label: copy.ready },
    { value: "review", label: copy.review },
    { value: "archived", label: copy.archived, disabled: true },
  ];
  const change = (next: string) => {
    setValue(next);
    observe("valueChange", next);
  };
  if (variant === views.sizes.id)
    return (
      <div {...stylex.props(styles.stack)}>
        {FIELD_SIZES.map((size) => (
          <div key={size} {...stylex.props(styles.pair)}>
            <span {...stylex.props(styles.label)}>{size}</span>
            <Select
              value={value}
              onValueChange={change}
              options={options}
              label={`${copy.status} · ${size}`}
              size={size}
            />
          </div>
        ))}
      </div>
    );
  return (
    <div {...stylex.props(styles.stack)}>
      <Select
        value={value}
        onValueChange={change}
        options={options}
        label={copy.status}
        disabled={variant === views.disabled.id}
        invalid={invalid}
        aria-describedby={invalid ? feedback : undefined}
      />
      {invalid && <p id={feedback}>{copy.selectRequired}</p>}
    </div>
  );
}
