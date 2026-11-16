import {
  Button,
  ContentVisibility,
  Disclosure,
  DocumentBody,
  Metadata,
  PanelSection,
  Popover,
  RecordForm,
  RecordGroup,
  RecordTitle,
  Toolbar,
  Tabs,
} from "@flame/ui";
import type { ComponentRenderProps } from "@openuidev/react-lang";
import type { Translate } from "@flame/component-sdk";
import type { DisplayProps } from "../contracts/catalog";
import { useLocalValue } from "./state";

const GAP = {
  small: "calc(var(--spacing) * 2)",
  medium: "calc(var(--spacing) * 4)",
  large: "calc(var(--spacing) * 6)",
};

export function StackView({
  props,
  renderNode,
}: ComponentRenderProps<DisplayProps["Stack"]>) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: props.direction === "horizontal" ? "row" : "column",
        flexWrap: props.direction === "horizontal" ? "wrap" : undefined,
        gap: GAP[props.gap ?? "medium"],
        minWidth: 0,
      }}
    >
      {renderNode(props.children)}
    </div>
  );
}

export function GridView({
  props,
  renderNode,
}: ComponentRenderProps<DisplayProps["Grid"]>) {
  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: `repeat(auto-fit, minmax(min(100%, max(12rem, calc((100% - ${props.columns - 1} * var(--spacing) * 4) / ${props.columns}))), 1fr))`,
        gap: GAP.medium,
        minWidth: 0,
      }}
    >
      {renderNode(props.children)}
    </div>
  );
}

export function CardView({
  props,
  renderNode,
}: ComponentRenderProps<DisplayProps["Card"]>) {
  return (
    <RecordForm aria-label={props.title}>
      {props.title && <RecordTitle>{props.title}</RecordTitle>}
      {props.description && <Metadata>{props.description}</Metadata>}
      {renderNode(props.children)}
    </RecordForm>
  );
}

export function SectionView({
  props,
  renderNode,
}: ComponentRenderProps<DisplayProps["Section"]>) {
  return (
    <PanelSection title={props.title}>
      <RecordGroup>{renderNode(props.children)}</RecordGroup>
    </PanelSection>
  );
}

export function TabsView({
  props,
  renderNode,
  t,
}: ComponentRenderProps<DisplayProps["Tabs"]> & { t: Translate }) {
  const [selected, select] = useLocalValue<string>(props.field);
  return (
    <Tabs
      value={selected}
      onValueChange={select}
      ariaLabel={t("tabs.label")}
      items={props.items.map((item, index) => ({
        ...item,
        content: renderNode(props.children[index]),
      }))}
    />
  );
}

export function AccordionView({
  props,
  renderNode,
}: ComponentRenderProps<DisplayProps["Accordion"]>) {
  const [selected, select] = useLocalValue<string[]>(props.field);
  return (
    <RecordGroup variant="list">
      {props.items.map((item, index) => (
        <Disclosure
          key={item.value}
          title={item.label}
          open={selected.includes(item.value)}
          onOpenChange={(open) =>
            select(
              open
                ? [...selected, item.value]
                : selected.filter((value) => value !== item.value),
            )
          }
          variant="row"
          keepMounted
        >
          <ContentVisibility visible={selected.includes(item.value)}>
            {renderNode(props.children[index])}
          </ContentVisibility>
        </Disclosure>
      ))}
    </RecordGroup>
  );
}

export function DisclosureView({
  props,
  renderNode,
}: ComponentRenderProps<DisplayProps["Disclosure"]>) {
  const [open, setOpen] = useLocalValue<boolean>(props.field);
  return (
    <Disclosure
      title={props.title}
      open={open}
      onOpenChange={setOpen}
      keepMounted
    >
      <ContentVisibility visible={open}>
        {renderNode(props.children)}
      </ContentVisibility>
    </Disclosure>
  );
}

export function CarouselView({
  props,
  renderNode,
  t,
}: ComponentRenderProps<DisplayProps["Carousel"]> & { t: Translate }) {
  const [selected, select] = useLocalValue<number>(props.field);
  const current = Math.min(Math.max(0, selected), props.children.length - 1);
  const label = t("carousel.label");
  return (
    <RecordGroup
      aria-label={label}
      aria-roledescription={t("carousel.description")}
    >
      <Toolbar label={label}>
        <Button
          disabled={current === 0}
          focusableWhenDisabled
          onClick={() => select(current - 1)}
        >
          {t("previous")}
        </Button>
        <Metadata role="status">
          {t("page", { current: current + 1, total: props.children.length })}
        </Metadata>
        <Button
          disabled={current + 1 === props.children.length}
          focusableWhenDisabled
          onClick={() => select(current + 1)}
        >
          {t("next")}
        </Button>
      </Toolbar>
      {props.children.map((child, index) => (
        <div key={index} hidden={current !== index}>
          <ContentVisibility visible={current === index}>
            {renderNode(child)}
          </ContentVisibility>
        </div>
      ))}
    </RecordGroup>
  );
}

export function PopoverView({
  props,
  renderNode,
}: ComponentRenderProps<DisplayProps["Popover"]>) {
  const [open, setOpen] = useLocalValue<boolean>(props.field);
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger render={<Button />}>{props.label}</Popover.Trigger>
      <Popover.Content
        aria-label={props.label}
        align="start"
        sideOffset={8}
        style={{
          padding: "calc(var(--spacing) * 4)",
          width: "min(24rem, var(--available-width))",
          maxHeight: "var(--available-height)",
          overflowY: "auto",
          overscrollBehavior: "contain",
        }}
      >
        {/* The portal escapes the report's DocumentBody reading grammar. */}
        <DocumentBody>
          <div
            style={{
              display: "flex",
              flexDirection: "column",
              gap: GAP.medium,
              minWidth: 0,
            }}
          >
            <ContentVisibility visible={open}>
              {renderNode(props.children)}
            </ContentVisibility>
          </div>
        </DocumentBody>
      </Popover.Content>
    </Popover.Root>
  );
}
