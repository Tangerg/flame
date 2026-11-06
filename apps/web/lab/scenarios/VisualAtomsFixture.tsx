import { iconGlyph } from "@/ui/icons/index";
import { Refresh } from "@flame/icons/react";
import { useState, type ReactNode } from "react";
import * as stylex from "@stylexjs/stylex";
import { color, space, surface, type as typeStep } from "@flame/ui/tokens.stylex";
import { ChoiceList, ChoiceOption } from "@/ui/index";
import { Chip } from "@/ui/index";
import { Divider } from "@/ui/index";
import { FilePath } from "@/ui/index";
import { Gauge } from "@/ui/index";
import { Kbd } from "@/ui/index";
import { Loader } from "@/ui/index";
import { ProgressBar } from "@/ui/index";
import { SectionLabel } from "@/ui/index";
import { Segmented, Slider, Switch } from "@flame/ui";
import { SelectTrigger } from "@/ui/index";
import { SkeletonList } from "@/ui/index";
import { StatusDot } from "@/ui/index";
import { StepMark } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { Tag } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { Well } from "@/ui/index";
import {
  Panel,
  PanelHeader,
  FilterBar,
  FormField,
  FormRow,
  ListRow,
  Metadata,
  Select,
  Badge,
  Button,
  ActionMenu,
  Checkbox,
  EmptyState,
  IconButton,
  SearchField,
  TextArea,
  TextField,
  Toolbar,
  DiffStat,
} from "@flame/ui";
import { Icon } from "@/ui/icons/index";
const styles = stylex.create({
  page: {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fill, minmax(320px, 1fr))",
    alignContent: "start",
    gap: space.s6,
    height: "100vh",
    overflowY: "auto",
    padding: space.s6,
    backgroundColor: surface.canvas,
    color: color.fg,
  },
  specimen: { display: "flex", flexDirection: "column", gap: space.s2, minWidth: 0 },
  name: { color: color.fgFaint },
  row: { display: "flex", flexWrap: "wrap", alignItems: "center", gap: space.s2 },
  column: { display: "flex", flexDirection: "column", gap: space.s2 },
  fill: { width: "100%" },
});

function Specimen({ name, children }: { name: string; children: ReactNode }) {
  return (
    <section data-atom={name} {...stylex.props(styles.specimen)}>
      <div {...stylex.props(styles.name, typeStep.uiXs)}>{name}</div>
      {children}
    </section>
  );
}

const noop = () => {};

const BUTTON_VARIANTS = [
  { variant: "primary", label: "Primary" },
  { variant: "soft", label: "Soft" },
  { variant: "outline", label: "Outline" },
  { variant: "ghost", label: "Ghost" },
  { variant: "tonal", label: "Delete" },
] as const;

function SegmentedSpecimen() {
  const [value, setValue] = useState("trace");
  return (
    <Segmented
      value={value}
      onChange={setValue}
      ariaLabel="Diagnostics"
      options={[
        { value: "trace", label: "Trace" },
        { value: "metrics", label: "Metrics" },
        { value: "logs", label: "Logs" },
      ]}
    />
  );
}

function ChoiceSpecimen({ multiple }: { multiple: boolean }) {
  const values = ["alpha", "beta", "gamma"] as const;
  const [value, setValue] = useState<string[]>(["beta"]);
  return (
    <ChoiceList
      multiple={multiple}
      value={value}
      values={values}
      labelledBy="atoms-choice"
      numbered={!multiple}
      onValueChange={setValue}
    >
      {values.map((option, index) => (
        <ChoiceOption
          key={option}
          multiple={multiple}
          value={option}
          selected={value.includes(option)}
          ordinal={index + 1}
          label={option}
        >
          {option}
        </ChoiceOption>
      ))}
    </ChoiceList>
  );
}

export function VisualAtomsFixture() {
  const [status, setStatus] = useState("pending");
  const [search, setSearch] = useState("");
  return (
    <main data-slot="atoms-gallery" {...stylex.props(styles.page)}>
      <Specimen name="shared-panel">
        <Panel
          label="Shared Memory controls"
          header={
            <PanelHeader
              title="Agent memory"
              description="Facts and preferences for future work."
            />
          }
          controls={
            <FilterBar
              label="Memory controls"
              search={
                <SearchField
                  size="lg"
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  aria-label="Search memory"
                  placeholder="Recorded text or ID"
                />
              }
              actions={
                <Toolbar label="Memory actions">
                  <IconButton icon={Refresh} size="md" aria-label="Refresh" onClick={noop} />
                  <Button variant="primary" size="lg" onClick={noop}>
                    <Icon name="plus" size="sm" /> Add memory
                  </Button>
                </Toolbar>
              }
            >
              <Select
                size="lg"
                label="Memory status"
                value={status}
                onValueChange={setStatus}
                options={[
                  { value: "pending", label: "Pending review" },
                  { value: "active", label: "Active" },
                ]}
              />
            </FilterBar>
          }
        >
          <ListRow>
            <p>Keep the durable fact with its owner.</p>
            <Metadata>Project · User authored · Pinned</Metadata>
          </ListRow>
        </Panel>
      </Specimen>
      <Specimen name="form-alignment">
        <FormRow>
          <FormField label="Short label">
            <TextField aria-label="Short form value" defaultValue="One line" />
          </FormField>
          <FormField label="A longer translated field label that wraps onto several lines">
            <TextField aria-label="Long form value" defaultValue="One line" />
          </FormField>
        </FormRow>
      </Specimen>
      <Specimen name="button">
        <div {...stylex.props(styles.column)}>
          {BUTTON_VARIANTS.map(({ variant, label }) => (
            <div key={variant} {...stylex.props(styles.row)}>
              <Button variant={variant} tone={variant === "tonal" ? "negative" : undefined}>
                {label}
              </Button>
              <Button
                variant={variant}
                tone={variant === "tonal" ? "negative" : undefined}
                disabled
              >
                {label} disabled
              </Button>
              <Button variant={variant} tone={variant === "tonal" ? "negative" : undefined} pending>
                {label} pending
              </Button>
            </div>
          ))}
        </div>
      </Specimen>
      <Specimen name="unavailable-controls">
        <TextButton disabled shape="row">
          Disabled text action
        </TextButton>
        <TextButton pending shape="row">
          Pending text action
        </TextButton>
        <Segmented
          disabled
          value="trace"
          onChange={noop}
          ariaLabel="Unavailable diagnostics"
          options={[
            { value: "trace", label: "Trace" },
            { value: "metrics", label: "Metrics" },
          ]}
        />
        <span id="atoms-disabled-choice">Unavailable choices</span>
        <ChoiceList
          disabled
          multiple={false}
          value={[]}
          values={["unavailable"]}
          labelledBy="atoms-disabled-choice"
          onValueChange={noop}
        >
          <ChoiceOption
            disabled
            multiple={false}
            value="unavailable"
            selected={false}
            label="Unavailable choice"
          >
            Unavailable choice
          </ChoiceOption>
        </ChoiceList>
      </Specimen>
      <Specimen name="icon-button">
        <div {...stylex.props(styles.row)}>
          <IconButton icon={iconGlyph("search")} size="xs" aria-label="Search" />
          <IconButton icon={iconGlyph("copy")} size="sm" aria-label="Copy" />
          <IconButton icon={iconGlyph("settings")} size="md" aria-label="Settings" />
          <IconButton icon={iconGlyph("bell")} size="md" badge={3} aria-label="Notifications" />
          <IconButton icon={iconGlyph("trash")} size="sm" disabled aria-label="Delete" />
        </div>
      </Specimen>
      <Specimen name="text-button">
        <div {...stylex.props(styles.row)}>
          <TextButton tone="muted">Muted</TextButton>
          <TextButton tone="faint">Faint</TextButton>
          <TextButton tone="accent">Accent</TextButton>
          <TextButton tone="negative">Negative</TextButton>
        </div>
      </Specimen>
      <Specimen name="switch">
        <div {...stylex.props(styles.row)}>
          <Switch checked={false} onCheckedChange={noop} ariaLabel="Off" />
          <Switch checked onCheckedChange={noop} ariaLabel="On" />
          <Switch checked={false} disabled onCheckedChange={noop} ariaLabel="Off disabled" />
          <Switch checked disabled onCheckedChange={noop} ariaLabel="On disabled" />
        </div>
      </Specimen>
      <Specimen name="checkbox">
        <div {...stylex.props(styles.column)}>
          <Checkbox checked={false} onCheckedChange={noop} label="Unchecked" />
          <Checkbox checked onCheckedChange={noop} label="Checked" />
          <Checkbox checked={false} disabled onCheckedChange={noop} label="Disabled" />
        </div>
      </Specimen>
      <Specimen name="segmented">
        <SegmentedSpecimen />
      </Specimen>
      <Specimen name="slider">
        <Slider value={40} onValueChange={noop} ariaLabel="Volume" />
      </Specimen>
      <Specimen name="choice-list-single">
        <span id="atoms-choice" hidden>
          Choice
        </span>
        <ChoiceSpecimen multiple={false} />
      </Specimen>
      <Specimen name="choice-list-multiple">
        <ChoiceSpecimen multiple />
      </Specimen>
      <Specimen name="text-field">
        <div {...stylex.props(styles.column)}>
          <TextField placeholder="Placeholder" aria-label="Empty" />
          <TextField defaultValue="Filled value" aria-label="Filled" />
          <TextField defaultValue="Invalid value" invalid aria-label="Invalid" />
          <TextField defaultValue="Disabled" disabled aria-label="Disabled" />
          <SearchField
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search…"
            aria-label="Search"
          />
          <TextArea placeholder="Write a note" aria-label="Note" />
        </div>
      </Specimen>
      <Specimen name="select-trigger">
        <SelectTrigger label="gpt-5 · high" leading={<Icon name="sparkle" size="sm" />} />
        <SelectTrigger label="Disabled selection" disabled />
        <SelectTrigger label="Pending selection" pending />
        <Select
          disabled
          label="Disabled status"
          value="pending"
          onValueChange={noop}
          options={[{ value: "pending", label: "Pending review" }]}
        />
      </Specimen>
      <Specimen name="action-menu">
        <ActionMenu
          label="Record actions"
          triggerContent="Record actions"
          items={[
            { label: "Open record", onSelect: noop },
            { label: "Edit record", onSelect: noop },
            { label: "Delete record", tone: "negative", disabled: true, onSelect: noop },
          ]}
        />
      </Specimen>
      <Specimen name="badge">
        <div {...stylex.props(styles.row)}>
          <Badge>Neutral</Badge>
          <Badge tone="accent">Accent</Badge>
          <Badge tone="success">Success</Badge>
          <Badge tone="warning">Warning</Badge>
          <Badge tone="negative">Negative</Badge>
          <Badge tone="info">Info</Badge>
        </div>
      </Specimen>
      <Specimen name="tag-chip-kbd">
        <div {...stylex.props(styles.row)}>
          <Tag>tag</Tag>
          <Tag ink="strong">strong</Tag>
          <Chip icon="file" onClose={noop} closeLabel="Remove">
            main.go
          </Chip>
          <Chip kind="attached" icon="image">
            screenshot.png
          </Chip>
          <Kbd>⌘</Kbd>
          <Kbd>K</Kbd>
        </div>
      </Specimen>
      <Specimen name="status">
        <div {...stylex.props(styles.row)}>
          <StatusDot tone="idle" />
          <StatusDot tone="running" />
          <StatusDot tone="waiting" />
          <StatusDot tone="ok" />
          <StatusDot tone="err" />
          <Gauge value={0.62} label="Context" />
          <DiffStat added={12} removed={4} />
        </div>
      </Specimen>
      <Specimen name="diff-stat">
        <DiffStat added={12} removed={4} />
        <DiffStat added={0} removed={0} />
        <DiffStat added={0} removed={undefined} />
        <DiffStat added={undefined} removed={undefined} />
        <DiffStat added={1234567} removed={7654321} />
      </Specimen>
      <Specimen name="progress">
        <div {...stylex.props(styles.column, styles.fill)}>
          <ProgressBar value={40} label="Bar" />
          <ProgressBar value={70} label="Row" weight="row" />
          <Loader text="Loading" />
        </div>
      </Specimen>
      <Specimen name="steps">
        <div {...stylex.props(styles.column)}>
          <StepMark state="done" label="Completed" />
          <StepMark state="active" label="In progress" />
          <StepMark state="pending" label="Pending" />
        </div>
      </Specimen>
      <Specimen name="system-message">
        <div {...stylex.props(styles.column)}>
          <SystemMessage variant="info">Information message.</SystemMessage>
          <SystemMessage variant="success">Saved.</SystemMessage>
          <SystemMessage variant="warning">Check this first.</SystemMessage>
          <SystemMessage variant="error">Something failed.</SystemMessage>
          <SystemMessage
            variant="warning"
            action={{ label: "Review installation details and retry the original operation" }}
          >
            {`The original command remains unresolved.\n${"installation_digest_".repeat(12)}`}
          </SystemMessage>
        </div>
      </Specimen>
      <Specimen name="section-divider">
        <div {...stylex.props(styles.column)}>
          <SectionLabel trailing={<Tag>3</Tag>}>Section</SectionLabel>
          <Divider>Divider</Divider>
          <Divider intent="accent" icon={<Icon name="check" size="xs" />}>
            Approved
          </Divider>
        </div>
      </Specimen>
      <Specimen name="well-path">
        <div {...stylex.props(styles.column)}>
          <Well>go test ./...</Well>
          <FilePath path="apps/web/src/ui/atoms/switch.tsx" />
          <Button size="sm" variant="ghost">
            Before long JSON
          </Button>
          <Well cap="md" wrap="pre">
            {JSON.stringify(
              { content: "A recorded tool result retains its original content. ".repeat(8) },
              null,
              2,
            )}
          </Well>
        </div>
      </Specimen>
      <Specimen name="empty-skeleton">
        <div {...stylex.props(styles.column)}>
          <EmptyState
            icon={iconGlyph("bell")}
            title="Nothing here"
            sub="New items appear here."
            size="compact"
          />
          <SkeletonList count={2} label="Loading" />
        </div>
      </Specimen>
    </main>
  );
}

export function VisualInputContractFixture() {
  const initial = "A short note";
  const [value, setValue] = useState(initial);
  const [search, setSearch] = useState("match");
  const [visible, setVisible] = useState(false);
  const [narrow, setNarrow] = useState(false);
  const [fontSize, setFontSize] = useState(14);
  const [pending, setPending] = useState(true);
  const [submitted, setSubmitted] = useState(0);
  return (
    <main {...stylex.props(styles.page)}>
      <section
        {...stylex.props(styles.column)}
        style={{ width: narrow ? 180 : 440, maxWidth: "100%" }}
      >
        <Button variant="outline" onClick={() => setVisible(!visible)}>
          {visible ? "Hide notes" : "Show notes"}
        </Button>
        <Button variant="outline" onClick={() => setNarrow(!narrow)}>
          Toggle note width
        </Button>
        <Button
          variant="outline"
          onClick={() => setFontSize((current) => (current === 14 ? 24 : 14))}
        >
          Toggle note font
        </Button>
        <form onReset={() => setValue(initial)}>
          <div hidden={!visible}>
            <TextArea
              autosize
              rows={1}
              aria-label="Controlled note"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              style={{ maxHeight: 160, fontSize }}
            />
            <TextArea
              autosize
              rows={1}
              aria-label="Uncontrolled note"
              defaultValue={initial}
              style={{ maxHeight: 160 }}
            />
          </div>
          <Button variant="outline" type="reset">
            Reset notes
          </Button>
        </form>
        <SearchField
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          onClear={() => setSearch("")}
          aria-label="Editable search"
          clearLabel="Clear editable search"
        />
        {(["disabled", "readOnly", "pending"] as const).map((state) => (
          <SearchField
            key={state}
            value="fixed"
            {...{ [state]: true }}
            onClear={() => {
              throw new Error("Immutable search cleared");
            }}
            aria-label={`${state} search`}
            clearLabel={`Clear ${state} search`}
          />
        ))}
      </section>
      <section {...stylex.props(styles.column)}>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            setSubmitted((count) => count + 1);
          }}
        >
          <FormField label="Submission note">
            <TextField defaultValue="Keep this draft" />
          </FormField>
          <Button type="submit" pending={pending}>
            Save submission note
          </Button>
        </form>
        <Button onClick={() => setPending(!pending)}>
          {pending ? "Finish pending submission" : "Mark submission pending"}
        </Button>
        <p role="status">Submissions: {submitted}</p>
        <SegmentedSpecimen />
        {(["boxed", "bare", "inline", "well"] as const).map((variant) => (
          <FormField key={variant} label={`${variant} invalid note`}>
            <TextArea
              variant={variant}
              invalid
              defaultValue="Keep the validation mark while focused"
            />
          </FormField>
        ))}
      </section>
    </main>
  );
}
