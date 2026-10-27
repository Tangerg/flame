import { useEffect, useId, useRef, type KeyboardEvent } from "react";
import {
  Button,
  ContentVisibility,
  Disclosure,
  Metadata,
  PanelSection,
  RecordForm,
  RecordGroup,
  RecordTitle,
  Toolbar,
  useContentVisible,
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

export function moveChoice(
  event: KeyboardEvent<HTMLButtonElement>,
  choose: (index: number) => void,
) {
  const options = Array.from(
    event.currentTarget.parentElement!.querySelectorAll<HTMLButtonElement>(
      "button",
    ),
  );
  const current = options.indexOf(event.currentTarget);
  let index: number;
  switch (event.key) {
    case "ArrowRight":
    case "ArrowDown":
      index = (current + 1) % options.length;
      break;
    case "ArrowLeft":
    case "ArrowUp":
      index = (current + options.length - 1) % options.length;
      break;
    case "Home":
      index = 0;
      break;
    case "End":
      index = options.length - 1;
      break;
    default:
      return;
  }
  event.preventDefault();
  choose(index);
  options[index]?.focus();
}

export function TabsView({
  props,
  renderNode,
  t,
}: ComponentRenderProps<DisplayProps["Tabs"]> & { t: Translate }) {
  const [selected, select] = useLocalValue<string>(props.field);
  const id = useId();
  return (
    <RecordGroup>
      <div
        role="tablist"
        aria-label={t("tabs.label")}
        style={{ display: "flex", flexWrap: "wrap" }}
      >
        {props.items.map((item, index) => (
          <Button
            key={item.value}
            id={`${id}-tab-${index}`}
            role="tab"
            aria-selected={selected === item.value}
            aria-controls={`${id}-panel-${index}`}
            tabIndex={selected === item.value ? 0 : -1}
            variant="soft"
            active={selected === item.value}
            onClick={() => select(item.value)}
            onKeyDown={(event) =>
              moveChoice(event, (next) => select(props.items[next]!.value))
            }
          >
            {item.label}
          </Button>
        ))}
      </div>
      {props.items.map((item, index) => (
        <div
          key={item.value}
          id={`${id}-panel-${index}`}
          role="tabpanel"
          aria-labelledby={`${id}-tab-${index}`}
          hidden={selected !== item.value}
          tabIndex={0}
        >
          <ContentVisibility visible={selected === item.value}>
            {renderNode(props.children[index])}
          </ContentVisibility>
        </div>
      ))}
    </RecordGroup>
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
        <Button disabled={current === 0} onClick={() => select(current - 1)}>
          {t("previous")}
        </Button>
        <Metadata role="status">
          {t("page", { current: current + 1, total: props.children.length })}
        </Metadata>
        <Button
          disabled={current + 1 === props.children.length}
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
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const visible = useContentVisible();
  useEffect(() => {
    if (!open || !visible) return;
    const first = panel.current?.querySelector<HTMLElement>(
      "button:not(:disabled), input:not(:disabled), textarea:not(:disabled), select:not(:disabled), a[href], [tabindex='0']",
    );
    (first ?? panel.current)?.focus();
    const closeOutside = (event: PointerEvent) => {
      if (event.target instanceof Node && !root.current?.contains(event.target))
        setOpen(false);
    };
    document.addEventListener("pointerdown", closeOutside);
    return () => document.removeEventListener("pointerdown", closeOutside);
  }, [open, visible, setOpen]);
  return (
    <div
      ref={root}
      style={{ position: "relative", minWidth: 0 }}
      onKeyDown={(event) => {
        if (event.key === "Escape" && open) {
          event.preventDefault();
          setOpen(false);
          trigger.current?.focus();
        }
      }}
    >
      <Button
        ref={trigger}
        aria-expanded={open}
        aria-controls={id}
        aria-haspopup="dialog"
        onClick={() => setOpen(!open)}
      >
        {props.label}
      </Button>
      {open && visible && (
        <RecordForm
          ref={panel}
          id={id}
          role="dialog"
          aria-label={props.label}
          tabIndex={-1}
          style={{
            position: "absolute",
            zIndex: 1,
            width: "max-content",
            maxWidth: "100%",
          }}
        >
          {renderNode(props.children)}
        </RecordForm>
      )}
    </div>
  );
}
