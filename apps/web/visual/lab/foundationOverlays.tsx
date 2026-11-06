import { useRef, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import {
  ActionMenu,
  Button,
  Disclosure,
  FormField,
  ModalDialog,
  Popover,
  RichTooltip,
  TextField,
  Tooltip,
} from "@flame/ui";
import { foundationMessages } from "./foundationMessages";
import { foundationStyles as styles } from "./foundationStyles";
import { foundationPlacement, foundationViews as views } from "./foundationVariants";
import type { LabExampleProps } from "./story";

export function TooltipExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const side = foundationPlacement(variant);
  return (
    <div {...stylex.props(styles.row)}>
      <Tooltip label={copy.helpDescription} side={side}>
        <Button variant="outline" onClick={() => observe("click", "tooltip")}>
          {copy.help}
        </Button>
      </Tooltip>
      <RichTooltip
        trigger={
          <Button variant="outline" onClick={() => observe("click", "rich-tooltip")}>
            {copy.details}
          </Button>
        }
        side={side}
      >
        <strong>{copy.title}</strong>
        <p>{copy.helpDescription}</p>
      </RichTooltip>
    </div>
  );
}

export function PopoverExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState(copy.draft);
  const change = (next: boolean) => {
    setOpen(next);
    observe("openChange", next);
  };
  return (
    <Popover.Root open={open} onOpenChange={change}>
      <Popover.Trigger render={<Button variant="outline" />}>{copy.details}</Popover.Trigger>
      <Popover.Content side={foundationPlacement(variant)} sideOffset={8} aria-label={copy.details}>
        <div {...stylex.props(styles.stack, styles.popup)}>
          <FormField label={copy.note}>
            <TextField
              value={value}
              onChange={(event) => {
                setValue(event.target.value);
                observe("valueChange.length", event.target.value.length);
              }}
            />
          </FormField>
          <Button onClick={() => change(false)}>{copy.close}</Button>
        </div>
      </Popover.Content>
    </Popover.Root>
  );
}

export function ModalExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(copy.draft);
  const [settlement, setSettlement] = useState<"saved" | "discarded" | null>(null);
  const opener = useRef<HTMLButtonElement>(null);
  const confirmation = variant === views.confirmation.id;
  const size = variant === views.sm.id || variant === views.lg.id ? variant : "md";
  const change = (next: boolean) => {
    setOpen(next);
    observe("openChange", next);
  };
  return (
    <div {...stylex.props(styles.stack)}>
      <Button ref={opener} variant="outline" onClick={() => change(true)}>
        {copy.openDialog}
      </Button>
      {settlement && <p role="status">{copy[settlement]}</p>}
      <ModalDialog
        open={open}
        onOpenChange={change}
        title={confirmation ? copy.confirmTitle : copy.title}
        description={confirmation ? copy.confirmDescription : copy.overlayDescription}
        closeLabel={copy.close}
        size={size}
        role={confirmation ? "alertdialog" : "dialog"}
        finalFocus={opener}
        footer={
          <>
            <Button onClick={() => change(false)}>{copy.cancel}</Button>
            <Button
              variant={confirmation ? "tonal" : "primary"}
              tone={confirmation ? "negative" : undefined}
              onClick={() => {
                if (confirmation) setDraft("");
                setSettlement(confirmation ? "discarded" : "saved");
                if (confirmation) observe("discard");
                else observe("save.length", draft.length);
                change(false);
              }}
            >
              {confirmation ? copy.discard : copy.save}
            </Button>
          </>
        }
      >
        <FormField label={copy.note}>
          <TextField
            value={draft}
            readOnly={confirmation}
            onChange={(event) => {
              setDraft(event.target.value);
              observe("valueChange.length", event.target.value.length);
            }}
          />
        </FormField>
      </ModalDialog>
    </div>
  );
}

export function ActionMenuExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  type Action = "open" | "duplicate" | "remove" | "unavailable";
  const [selected, setSelected] = useState<Action | null>(null);
  const select = (value: Action) => {
    setSelected(value);
    observe("select", value);
  };
  return (
    <div {...stylex.props(styles.stack)}>
      <ActionMenu
        label={copy.actions}
        triggerContent={copy.actions}
        disabled={variant === views.disabled.id}
        items={[
          { label: copy.open, onSelect: () => select("open") },
          { label: copy.duplicate, onSelect: () => select("duplicate") },
          { label: copy.remove, onSelect: () => select("remove"), tone: "negative" },
          { label: copy.unavailable, onSelect: () => select("unavailable"), disabled: true },
        ]}
      />
      {selected && (
        <p role="status">
          {copy.actionResult}: {copy[selected]}
        </p>
      )}
    </div>
  );
}

function DisclosureDraft({ locale, observe }: Pick<LabExampleProps, "locale" | "observe">) {
  const copy = foundationMessages[locale];
  const [draft, setDraft] = useState(copy.draft);
  return (
    <FormField label={copy.retainedDraft}>
      <TextField
        value={draft}
        onChange={(event) => {
          setDraft(event.target.value);
          observe("valueChange.length", event.target.value.length);
        }}
      />
    </FormField>
  );
}

export function DisclosureExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [open, setOpen] = useState(false);
  return (
    <Disclosure
      title={copy.disclosure}
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        observe("openChange", next);
      }}
      variant={variant === views.row.id ? "row" : "plain"}
      keepMounted={variant === views.retained.id}
    >
      <div {...stylex.props(styles.stack)}>
        <p>{copy.disclosureBody}</p>
        <DisclosureDraft locale={locale} observe={observe} />
      </div>
    </Disclosure>
  );
}
