import { useState, type ReactNode } from "react";
import * as stylex from "@stylexjs/stylex";
import { color, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import {
  Badge,
  Button,
  Checkbox,
  ChoiceList,
  ChoiceOption,
  Chip,
  DiffStat,
  Divider,
  EmptyState,
  FilePath,
  Gauge,
  IconButton,
  Kbd,
  Loader,
  PillButton,
  ProgressBar,
  SearchField,
  SectionLabel,
  Segmented,
  SelectTrigger,
  SkeletonList,
  Slider,
  StatusDot,
  StepMark,
  Switch,
  SystemMessage,
  Tag,
  TextArea,
  TextButton,
  TextField,
  Well,
} from "@/ui";
import { Icon } from "@/ui/icons";

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
  return (
    <main data-slot="atoms-gallery" {...stylex.props(styles.page)}>
      <Specimen name="button">
        <div {...stylex.props(styles.row)}>
          <Button variant="primary">Primary</Button>
          <Button variant="soft">Soft</Button>
          <Button variant="outline">Outline</Button>
          <Button variant="ghost">Ghost</Button>
          <Button variant="tonal" tone="negative">
            Delete
          </Button>
          <Button variant="primary" disabled>
            Disabled
          </Button>
        </div>
      </Specimen>
      <Specimen name="icon-button">
        <div {...stylex.props(styles.row)}>
          <IconButton icon="search" size="xs" aria-label="Search" />
          <IconButton icon="copy" size="sm" aria-label="Copy" />
          <IconButton icon="settings" size="md" aria-label="Settings" />
          <IconButton icon="bell" size="md" badge={3} aria-label="Notifications" />
          <IconButton icon="trash" size="sm" disabled aria-label="Delete" />
        </div>
      </Specimen>
      <Specimen name="pill-button">
        <div {...stylex.props(styles.row)}>
          <PillButton variant="outlined">Outlined</PillButton>
          <PillButton variant="solid">Solid</PillButton>
          <PillButton variant="danger">Danger</PillButton>
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
          <SearchField placeholder="Search…" aria-label="Search" />
          <TextArea placeholder="Write a note" aria-label="Note" />
        </div>
      </Specimen>
      <Specimen name="select-trigger">
        <SelectTrigger label="gpt-5 · high" leading={<Icon name="sparkle" size="sm" />} />
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
      <Specimen name="progress">
        <div {...stylex.props(styles.column, styles.fill)}>
          <ProgressBar value={40} label="Bar" />
          <ProgressBar value={70} label="Row" weight="row" />
          <Loader text="Loading" />
        </div>
      </Specimen>
      <Specimen name="steps">
        <div {...stylex.props(styles.column)}>
          <StepMark state="done" />
          <StepMark state="active" />
          <StepMark state="pending" />
        </div>
      </Specimen>
      <Specimen name="system-message">
        <div {...stylex.props(styles.column)}>
          <SystemMessage variant="info">Information message.</SystemMessage>
          <SystemMessage variant="success">Saved.</SystemMessage>
          <SystemMessage variant="warning">Check this first.</SystemMessage>
          <SystemMessage variant="error">Something failed.</SystemMessage>
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
          <FilePath path="desktop/frontend/src/ui/atoms/switch.tsx" />
        </div>
      </Specimen>
      <Specimen name="empty-skeleton">
        <div {...stylex.props(styles.column)}>
          <EmptyState
            icon="bell"
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
