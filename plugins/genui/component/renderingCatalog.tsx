import * as token from "@flame/ui/tokens.stylex";
import ReactMarkdown from "react-markdown";
import {
  Fragment,
  createContext,
  useContext,
  useId,
  useState,
  useSyncExternalStore,
} from "react";
import {
  createComponentImplementation,
  type ReactCatalogComponent,
} from "@a2ui/react";
import { type ComponentApi, type ResolvedChildRef } from "@a2ui/web_core";
import { type Translate } from "@flame/component-sdk";
import {
  Button,
  Checkbox,
  ModalDialog,
  RecordGroup,
  Tabs,
  TextArea,
  TextField,
} from "@flame/ui";
import { createCatalog, trustedComponentApis } from "../contracts/catalog";
import { type InteractionController } from "./interactionController";

export const CatalogDisplay = createContext<{
  controller: InteractionController;
  t: Translate;
} | null>(null);
function useDisplay() {
  const value = useContext(CatalogDisplay);
  if (!value) throw new Error("genui: missing captured interface");
  useSyncExternalStore(
    value.controller.subscribe,
    value.controller.snapshot,
    value.controller.snapshot,
  );
  return value;
}
function text(value: unknown): string {
  if (typeof value !== "string")
    throw new Error("genui: expected a resolved string");
  return value;
}
function children(
  refs: readonly (string | ResolvedChildRef)[],
  build: (id: string, basePath?: string) => React.ReactNode,
) {
  return refs.map((ref) => (
    <Fragment
      key={
        typeof ref === "string" ? ref : JSON.stringify([ref.id, ref.basePath])
      }
    >
      {typeof ref === "string" ? build(ref) : build(ref.id, ref.basePath)}
    </Fragment>
  ));
}
const inlineElements = ["p", "strong", "em", "code", "del", "br"];
function InlineParagraph({ children }: { children?: React.ReactNode }) {
  return <>{children}</>;
}
const inlineComponents = { p: InlineParagraph };
const justification = {
  start: "flex-start",
  end: "flex-end",
  center: "center",
  spaceAround: "space-around",
  spaceBetween: "space-between",
  spaceEvenly: "space-evenly",
  stretch: "stretch",
} as const;
type ReactRenderers<Components extends Record<string, ComponentApi>> = {
  [Name in keyof Components]: Parameters<
    typeof createComponentImplementation<Components[Name]>
  >[1];
};

function createReactCatalog<Components extends Record<string, ComponentApi>>(
  components: Components,
  renderers: ReactRenderers<Components>,
) {
  const implementations: ReactCatalogComponent[] = [];
  for (const name in components) {
    implementations.push(
      createComponentImplementation(components[name], renderers[name]),
    );
  }
  return createCatalog(implementations);
}

export const displayCatalog = createReactCatalog(trustedComponentApis, {
  text: ({ props }) => (
    <span
      dir="auto"
      style={{
        whiteSpace: "pre-wrap",
        overflowWrap: "anywhere",
        flexGrow: props.weight,
        fontSize: props.variant === "caption" ? token.fontSize.uiXs : undefined,
      }}
    >
      <ReactMarkdown
        skipHtml
        unwrapDisallowed
        allowedElements={inlineElements}
        components={inlineComponents}
      >
        {text(props.text)}
      </ReactMarkdown>
    </span>
  ),
  row: ({ props, buildChild }) => (
    <div
      style={{
        display: "flex",
        flexWrap: "wrap",
        flexGrow: props.weight,
        justifyContent: justification[props.justify ?? "start"],
        alignItems: props.align,
        gap: token.space.s3,
        minWidth: 0,
      }}
    >
      {children(props.children, buildChild)}
    </div>
  ),
  column: ({ props, buildChild }) => (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        flexGrow: props.weight,
        justifyContent: justification[props.justify ?? "start"],
        alignItems: props.align,
        gap: token.space.s3,
        minWidth: 0,
      }}
    >
      {children(props.children, buildChild)}
    </div>
  ),
  list: ({ props, buildChild }) => (
    <div
      style={{
        display: "flex",
        flexDirection: props.direction === "horizontal" ? "row" : "column",
        flexGrow: props.weight,
        alignItems: props.align,
        gap: token.space.s2,
        minWidth: 0,
      }}
    >
      {children(props.children, buildChild)}
    </div>
  ),
  card: ({ props, buildChild }) => (
    <RecordGroup style={{ flexGrow: props.weight }}>
      {buildChild(props.child)}
    </RecordGroup>
  ),
  divider: ({ props }) => (
    <hr
      aria-orientation={props.axis ?? "horizontal"}
      style={
        props.axis === "vertical"
          ? {
              alignSelf: "stretch",
              height: "auto",
              border: 0,
              borderInlineStart: `1px solid ${token.surface.divider}`,
              flexGrow: props.weight,
            }
          : {
              width: "100%",
              border: 0,
              borderBlockStart: `1px solid ${token.surface.divider}`,
              flexGrow: props.weight,
            }
      }
    />
  ),
  button: ({ props, buildChild }) => {
    const { controller } = useDisplay();
    return (
      <Button
        variant={
          props.variant === "primary"
            ? "primary"
            : props.variant === "borderless"
              ? "ghost"
              : "soft"
        }
        disabled={controller.blocked || props.isValid === false}
        style={{ flexGrow: props.weight }}
        onClick={() => {
          void props.action();
        }}
      >
        {buildChild(props.child)}
      </Button>
    );
  },
  textField: ({ props }) => {
    const id = useId();
    const field = {
      value: text(props.value ?? ""),
      placeholder:
        props.placeholder === undefined ? undefined : text(props.placeholder),
      "aria-label": text(props.label),
      "aria-invalid": props.isValid === false,
      "aria-describedby": props.validationErrors?.length ? id : undefined,
    };
    return (
      <label>
        <span>{text(props.label)}</span>
        {props.variant === "longText" ? (
          <TextArea
            {...field}
            onChange={(event) => props.setValue(event.target.value)}
          />
        ) : (
          <TextField
            {...field}
            type={
              props.variant === "obscured"
                ? "password"
                : props.variant === "number"
                  ? "number"
                  : "text"
            }
            onChange={(event) => props.setValue(event.target.value)}
          />
        )}
        <span
          id={id}
          role={props.validationErrors?.length ? "alert" : undefined}
        >
          {props.validationErrors?.join(" ")}
        </span>
      </label>
    );
  },
  checkbox: ({ props }) => {
    const id = useId();
    const errors = props.validationErrors ?? [];
    return (
      <div style={{ flexGrow: props.weight }}>
        <Checkbox
          label={text(props.label)}
          checked={props.value === true}
          onCheckedChange={props.setValue}
          aria-invalid={props.isValid === false}
          aria-describedby={errors.length ? id : undefined}
        />
        <span id={id} role={errors.length ? "alert" : undefined}>
          {errors.join(" ")}
        </span>
      </div>
    );
  },
  tabs: ({ props, buildChild }) => {
    const [value, setValue] = useState<string | number>(0);
    const { t } = useDisplay();
    return (
      <Tabs
        ariaLabel={t("tabs.label")}
        value={value}
        onValueChange={setValue}
        items={props.tabs.map((tab, index) => ({
          value: index,
          label: text(tab.title),
          content: buildChild(tab.child),
        }))}
      />
    );
  },
  dialog: ({ props, buildChild }) => {
    const [open, setOpen] = useState(false);
    const { t } = useDisplay();
    return (
      <>
        <Button onClick={() => setOpen(true)}>{text(props.label)}</Button>
        <ModalDialog
          open={open}
          onOpenChange={setOpen}
          title={text(props.label)}
          closeLabel={t("dialog.close")}
        >
          {buildChild(props.child)}
        </ModalDialog>
      </>
    );
  },
});
