import { useId, useRef, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import {
  Badge,
  Button,
  Checkbox,
  Facts,
  FilterBar,
  Form,
  FormActions,
  FormField,
  FormGroup,
  FormRow,
  FormSection,
  ListRow,
  Metadata,
  Panel,
  PanelHeader,
  PanelHost,
  RecordBody,
  RecordButton,
  RecordGroup,
  RecordHeading,
  RecordSummary,
  SearchField,
  TextArea,
  TextField,
} from "@flame/ui";
import { foundationMessages } from "./foundationMessages";
import { foundationStyles as styles } from "./foundationStyles";
import { foundationViews as views } from "./foundationVariants";
import type { LabExampleProps } from "./story";

export function PanelExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [search, setSearch] = useState("");
  const examples = [copy.trace, copy.metrics, copy.logs].filter((title) =>
    title.toLocaleLowerCase(locale).includes(search.toLocaleLowerCase(locale)),
  );
  const change = (next: string) => {
    setSearch(next);
    observe("queryChange.length", next.length);
  };
  const panel = (
    <Panel
      label={copy.library}
      header={<PanelHeader title={copy.library} description={copy.libraryDescription} />}
      controls={
        <FilterBar
          label={copy.filters}
          search={
            <SearchField
              value={search}
              onChange={(event) => change(event.target.value)}
              onClear={() => change("")}
              clearLabel={copy.clear}
              aria-label={copy.search}
              placeholder={copy.searchPlaceholder}
            />
          }
        />
      }
    >
      <RecordGroup variant="list">
        {examples.map((title) => (
          <ListRow key={title}>
            <RecordHeading title={title} status={<Badge tone="info">{copy.local}</Badge>} />
            <Metadata>{copy.recordDescription}</Metadata>
          </ListRow>
        ))}
      </RecordGroup>
      {!examples.length && <p role="status">{copy.noMatches}</p>}
    </Panel>
  );
  return (
    <div {...stylex.props(styles.scroll)}>
      {variant === views.standalone.id ? (
        panel
      ) : (
        <PanelHost density={variant === views.compact.id ? "compact" : "page"}>{panel}</PanelHost>
      )}
    </div>
  );
}

export function FormExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const nameInput = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState(copy.draft);
  const [reviewed, setReviewed] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [saved, setSaved] = useState(false);
  const feedback = useId();
  const nameValid = name.trim().length > 0;
  const invalid = !nameValid && (submitted || variant === views.invalid.id);
  return (
    <Form
      onSubmit={(event) => {
        event.preventDefault();
        setSubmitted(true);
        setSaved(nameValid);
        if (!nameValid) {
          nameInput.current?.focus({ preventScroll: true });
          nameInput.current?.labels?.[0]?.scrollIntoView({ block: "center", inline: "nearest" });
        }
        observe(nameValid ? "submit.length" : "validation", nameValid ? name.length : "name");
      }}
    >
      <FormSection title={copy.formTitle} description={copy.formDescription}>
        <FormRow>
          <FormField label={copy.name}>
            <TextField
              ref={nameInput}
              value={name}
              invalid={invalid}
              aria-label={copy.name}
              aria-describedby={invalid ? feedback : undefined}
              onChange={(event) => {
                setName(event.target.value);
                setSaved(false);
                observe("nameChange.length", event.target.value.length);
              }}
            />
            {invalid && (
              <span id={feedback} role="alert">
                {copy.invalid}
              </span>
            )}
          </FormField>
          <FormGroup label={copy.review}>
            <Checkbox
              checked={reviewed}
              label={copy.accept}
              className={stylex.props(styles.fieldChoice).className}
              onCheckedChange={(next) => {
                setReviewed(next);
                setSaved(false);
                observe("reviewedChange", next);
              }}
            />
          </FormGroup>
        </FormRow>
        <FormField label={copy.description}>
          <TextArea
            rows={3}
            value={description}
            onChange={(event) => {
              setDescription(event.target.value);
              setSaved(false);
              observe("descriptionChange.length", event.target.value.length);
            }}
          />
        </FormField>
      </FormSection>
      <FormActions
        label={copy.formActions}
        feedback={saved ? <p role="status">{copy.saved}</p> : undefined}
      >
        <Button
          type="button"
          onClick={() => {
            setName("");
            setDescription(copy.draft);
            setReviewed(false);
            setSubmitted(false);
            setSaved(false);
            observe("reset");
          }}
        >
          {copy.reset}
        </Button>
        <Button variant="primary" type="submit">
          {copy.save}
        </Button>
      </FormActions>
    </Form>
  );
}

export function RecordsExample({ variant, locale, observe }: LabExampleProps) {
  const copy = foundationMessages[locale];
  const [selected, setSelected] = useState<number | null>(null);
  const titles = [copy.trace, copy.metrics, copy.logs];
  return (
    <div {...stylex.props(styles.stack)}>
      <RecordGroup variant={variant === views.cards.id ? "cards" : "list"}>
        {titles.map((title, index) => (
          <ListRow key={index} variant={variant === views.cards.id ? "card" : "row"}>
            <RecordButton
              aria-current={selected === index ? "true" : undefined}
              onClick={() => {
                setSelected(index);
                observe("select", index);
              }}
            >
              <RecordSummary
                title={title}
                description={copy.recordDescription}
                status={<Badge tone="success">{copy.ready}</Badge>}
              />
            </RecordButton>
          </ListRow>
        ))}
      </RecordGroup>
      {selected !== null && (
        <section aria-label={copy.recordOpened}>
          <RecordHeading title={titles[selected]} description={copy.recordDescription} />
          <Facts
            values={[
              [copy.owner, copy.ownerValue],
              [copy.scope, copy.scopeValue],
            ]}
          />
          <RecordBody>{copy.recordBody}</RecordBody>
        </section>
      )}
    </div>
  );
}
