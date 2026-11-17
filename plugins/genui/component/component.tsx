import ReactMarkdown from "react-markdown";
import {
  Fragment,
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import {
  A2uiSurface,
  createComponentImplementation,
  type ReactCatalogComponent,
} from "@a2ui/react";
import {
  STRICT_VALIDATION,
  MessageProcessor,
  type ActionPayload,
  type SurfaceModel,
  type ResolvedChildRef,
} from "@a2ui/web_core";
import {
  TextApi,
  RowApi,
  ColumnApi,
  ListApi,
  CardApi,
  DividerApi,
  ButtonApi,
  TextFieldApi,
  CheckBoxApi,
  TabsApi,
} from "@a2ui/web_core/catalogs/basic/v1";
import {
  defineComponent,
  useTranslator,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import {
  Button,
  Checkbox,
  DocumentBody,
  ModalDialog,
  RecordGroup,
  RecordTitle,
  Tabs,
  TextArea,
  TextField,
} from "@flame/ui";
import { createCatalog, DialogApi } from "../contracts/catalog";
import { Presentation } from "../contracts/document";
import { InteractionController } from "./interactionController";
import { InteractionFeedback } from "./interaction";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };
const Display = createContext<{
  controller: InteractionController;
  t: Translate;
} | null>(null);
function useDisplay() {
  const value = useContext(Display);
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
const implementations: ReactCatalogComponent[] = [
  createComponentImplementation(TextApi, ({ props }) => (
    <span
      dir="auto"
      style={{
        whiteSpace: "pre-wrap",
        overflowWrap: "anywhere",
        flexGrow: props.weight,
        fontSize: props.variant === "caption" ? "var(--fs-ui-xs)" : undefined,
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
  )),
  createComponentImplementation(RowApi, ({ props, buildChild }) => (
    <div
      style={{
        display: "flex",
        flexWrap: "wrap",
        flexGrow: props.weight,
        justifyContent: justification[props.justify ?? "start"],
        alignItems: props.align,
        gap: "calc(var(--spacing) * 3)",
        minWidth: 0,
      }}
    >
      {children(props.children, buildChild)}
    </div>
  )),
  createComponentImplementation(ColumnApi, ({ props, buildChild }) => (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        flexGrow: props.weight,
        justifyContent: justification[props.justify ?? "start"],
        alignItems: props.align,
        gap: "calc(var(--spacing) * 3)",
        minWidth: 0,
      }}
    >
      {children(props.children, buildChild)}
    </div>
  )),
  createComponentImplementation(ListApi, ({ props, buildChild }) => (
    <div
      style={{
        display: "flex",
        flexDirection: props.direction === "horizontal" ? "row" : "column",
        flexGrow: props.weight,
        alignItems: props.align,
        gap: "calc(var(--spacing) * 2)",
        minWidth: 0,
      }}
    >
      {children(props.children, buildChild)}
    </div>
  )),
  createComponentImplementation(CardApi, ({ props, buildChild }) => (
    <RecordGroup style={{ flexGrow: props.weight }}>
      {buildChild(props.child)}
    </RecordGroup>
  )),
  createComponentImplementation(DividerApi, ({ props }) => (
    <hr
      aria-orientation={props.axis ?? "horizontal"}
      style={
        props.axis === "vertical"
          ? {
              alignSelf: "stretch",
              height: "auto",
              border: 0,
              borderInlineStart: "1px solid var(--color-divider)",
              flexGrow: props.weight,
            }
          : {
              width: "100%",
              border: 0,
              borderBlockStart: "1px solid var(--color-divider)",
              flexGrow: props.weight,
            }
      }
    />
  )),
  createComponentImplementation(ButtonApi, ({ props, buildChild }) => {
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
  }),
  createComponentImplementation(TextFieldApi, ({ props }) => {
    const id = ReactUseId();
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
  }),
  createComponentImplementation(CheckBoxApi, ({ props }) => {
    const id = ReactUseId();
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
  }),
  createComponentImplementation(TabsApi, ({ props, buildChild }) => {
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
  }),
  createComponentImplementation(DialogApi, ({ props, buildChild }) => {
    const [open, setOpen] = useState(false);
    const { t } = useDisplay();
    return (
      <>
        <Button onClick={() => setOpen(true)}>{text(props.label)}</Button>
        <ModalDialog
          open={open}
          onOpenChange={setOpen}
          title={text(props.label)}
          closeLabel={t("interaction.cancel")}
        >
          {buildChild(props.child)}
        </ModalDialog>
      </>
    );
  }),
];
const displayCatalog = createCatalog(implementations);

class CapturedSurface {
  readonly presentation: Presentation;
  readonly processor: MessageProcessor<ReactCatalogComponent>;
  readonly surfaces: readonly SurfaceModel<ReactCatalogComponent>[];
  readonly result: string;
  #action: ((action: ActionPayload) => Promise<void>) | undefined;
  constructor(result: string) {
    this.result = result;
    this.presentation = Presentation.read(result);
    this.processor = new MessageProcessor(
      [displayCatalog],
      (action) => {
        if (!this.#action) throw new Error("genui: interface is not mounted");
        return this.#action(action);
      },
      { version: "1.0", validationConfig: STRICT_VALIDATION },
    );
    this.processor.processMessages(this.presentation.messages());
    this.surfaces = [...this.processor.model.surfacesMap.values()];
  }
  mount(controller: InteractionController) {
    const action = (event: ActionPayload) => controller.execute(event);
    this.#action = action;
    return () => {
      if (this.#action === action) this.#action = undefined;
    };
  }
  writeOperation(id: string, value: unknown) {
    const path =
      "/data/operations/" + id.replaceAll("~", "~0").replaceAll("/", "~1");
    for (const surface of this.surfaces) surface.dataModel.set(path, value);
  }
  resetDraft(surfaceId: string) {
    const initial = this.presentation
      .messages()
      .find(
        (message) =>
          "createSurface" in message &&
          message.createSurface.surfaceId === surfaceId,
      );
    if (!initial || !("createSurface" in initial))
      throw new Error("genui: unknown surface");
    this.processor
      .getSurface(surfaceId)
      ?.dataModel.set(
        "/draft",
        structuredClone(initial.createSurface.dataModel?.draft ?? {}),
      );
  }
  dispose() {
    this.#action = undefined;
    this.processor.dispose();
  }
}

import { useId as ReactUseId } from "react";
export const GenUIResult = defineComponent((source) => {
  const captured = new Map<string, CapturedSurface>();
  source.signal.addEventListener(
    "abort",
    () => {
      captured.forEach((entry) => entry.dispose());
      captured.clear();
    },
    { once: true },
  );
  return function GenUIResult({ context }: { context: ComponentContext }) {
    const t = useTranslator(CATALOGS, context.locale);
    const tool = context.toolResult;
    if (
      source.signal.aborted ||
      !tool ||
      tool.status !== "ok" ||
      typeof tool.result !== "string"
    )
      throw new Error("genui: an accepted tool result is required");
    const key = JSON.stringify([tool.runId, tool.id]);
    let entry = captured.get(key);
    if (!entry) {
      entry = new CapturedSurface(tool.result);
      captured.set(key, entry);
    }
    if (entry.result !== tool.result)
      throw new Error("genui: captured result changed identity");
    return <CapturedDocument context={context} entry={entry} t={t} />;
  };
});
function CapturedDocument({
  context,
  entry,
  t,
}: {
  context: ComponentContext;
  entry: CapturedSurface;
  t: Translate;
}) {
  const controller = useMemo(
    () =>
      new InteractionController(context, entry.presentation, {
        writeOperation: (id, value) => entry.writeOperation(id, value),
        resetDraft: (id) => entry.resetDraft(id),
      }),
    [context, entry],
  );
  useEffect(() => {
    const unmount = entry.mount(controller);
    void controller.initialize();
    return unmount;
  }, [entry, controller]);
  return (
    <Display value={{ controller, t }}>
      <DocumentBody>
        <RecordGroup aria-label={entry.presentation.snapshot().title}>
          <RecordTitle>{entry.presentation.snapshot().title}</RecordTitle>
          <InteractionFeedback controller={controller} t={t} />
          {entry.surfaces.map((surface) => (
            <A2uiSurface key={surface.id} surface={surface} />
          ))}
        </RecordGroup>
      </DocumentBody>
    </Display>
  );
}
