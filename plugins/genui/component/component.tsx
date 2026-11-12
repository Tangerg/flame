import { createContext, Fragment, useContext, useMemo, useState } from "react";
import {
  type ComponentContext,
  type Translate,
  useTranslator,
} from "@flame/component-sdk";
import {
  Badge,
  Callout,
  DocumentBody,
  HeadingLevel,
  Metadata,
  ProgressBar,
  RecordBody,
  RecordGroup,
  RecordTitle,
  Stats,
} from "@flame/ui";
import { type OpenUIError } from "@openuidev/lang-core";
import { Renderer, type ComponentRenderProps } from "@openuidev/react-lang";
import { createDisplayLibrary, type DisplayProps } from "../contracts/catalog";
import { Presentation, isSafeURL } from "../contracts/document";
import { GenUIInteractionProvider, useGenUIInteraction } from "./interaction";
import { SandboxView } from "./sandbox";
import { ChartView } from "./chart";
import { TableView } from "./table";
import { MapView } from "./map";
import {
  AccordionView,
  CardView,
  CarouselView,
  DisclosureView,
  GridView,
  PopoverView,
  SectionView,
  StackView,
  TabsView,
} from "./layout";
import {
  ButtonView,
  CheckboxView,
  DatePickerView,
  FormView,
  InputView,
  MultiSelectView,
  RadioGroupView,
  SelectView,
  SliderView,
  SwitchView,
  TextAreaView,
} from "./fields";
import {
  CodeView,
  FlashcardView,
  ImageView,
  MarkdownView,
  MathView,
  SvgView,
  TimerView,
  YouTubeView,
} from "./media";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };
const CALLOUT_KIND = {
  neutral: "note",
  info: "note",
  accent: "important",
  success: "tip",
  warning: "warning",
  negative: "caution",
} as const;

const DisplayContext = createContext<{
  context: ComponentContext;
  presentation: Presentation;
  t: Translate;
  locale: string;
} | null>(null);

function useDisplay() {
  const display = useContext(DisplayContext);
  if (!display)
    throw new Error("genui: display requires its captured document");
  return display;
}

function DocumentView({
  props,
  renderNode,
}: ComponentRenderProps<DisplayProps["Document"]>) {
  const { feedback } = useGenUIInteraction();
  return (
    <DocumentBody>
      <RecordGroup aria-label={props.title} dir="auto">
        <RecordTitle>{props.title}</RecordTitle>
        {feedback}
        <HeadingLevel>{renderNode(props.children)}</HeadingLevel>
      </RecordGroup>
    </DocumentBody>
  );
}

// OpenUI renders these functions as component types, so display updates must not replace them.
const displayLibrary = createDisplayLibrary({
  Document: (view) => {
    const { context, presentation, t } = useDisplay();
    return (
      <GenUIInteractionProvider
        context={context}
        presentation={presentation}
        t={t}
      >
        <DocumentView {...view} />
      </GenUIInteractionProvider>
    );
  },
  Stack: StackView,
  Grid: GridView,
  Card: CardView,
  Section: SectionView,
  Tabs: (view) => <TabsView {...view} t={useDisplay().t} />,
  Accordion: AccordionView,
  Disclosure: DisclosureView,
  Carousel: (view) => <CarouselView {...view} t={useDisplay().t} />,
  Popover: PopoverView,
  If: ({ props, renderNode }) =>
    props.when ? renderNode(props.children) : null,
  Text: ({ props }) => <RecordBody>{props.text}</RecordBody>,
  Markdown: (view) => <MarkdownView {...view} t={useDisplay().t} />,
  Code: (view) => <CodeView {...view} t={useDisplay().t} />,
  Badge: ({ props }) => <Badge tone={props.tone}>{props.text}</Badge>,
  Callout: ({ props }) => (
    <Callout kind={CALLOUT_KIND[props.tone ?? "info"]}>
      {props.title && <strong>{props.title}</strong>}
      <RecordBody>{props.text}</RecordBody>
    </Callout>
  ),
  Link: ({ props }) => {
    if (!isSafeURL(props.href, "link"))
      throw new Error("genui: invalid link destination");
    return (
      <a href={props.href} target="_blank" rel="noreferrer noopener">
        {props.label}
      </a>
    );
  },
  Image: (view) => <ImageView {...view} t={useDisplay().t} />,
  Metric: ({ props }) => {
    const value = String(props.value);
    const digits = /^[0-9]+$/.test(value);
    return (
      <Stats
        values={[
          [
            props.label,
            <>
              <bdi
                style={
                  digits ? { display: "block", textWrap: "balance" } : undefined
                }
              >
                {digits
                  ? [...value].map((digit, index) => (
                      <Fragment key={index}>
                        {index > 0 && <wbr />}
                        {digit}
                      </Fragment>
                    ))
                  : value}
              </bdi>
              {props.detail !== undefined && (
                <Metadata>{props.detail}</Metadata>
              )}
            </>,
          ],
        ]}
      />
    );
  },
  List: ({ props }) => {
    const Tag = props.ordered ? "ol" : "ul";
    return (
      <Tag>
        {props.items.map((item, index) => (
          <li key={index} dir="auto">
            {item}
          </li>
        ))}
      </Tag>
    );
  },
  Table: (view) => <TableView {...view} t={useDisplay().t} />,
  Chart: ({ props }) => {
    const { t } = useDisplay();
    return (
      <ChartView
        {...props}
        dataLabel={t("chart.data", { chart: props.caption })}
        emptyLabel={t("chart.empty")}
      />
    );
  },
  Progress: ({ props }) => (
    <ProgressBar
      label={props.label}
      value={props.value}
      max={props.max}
      locale={useDisplay().locale}
      labelVisibility="visible"
    />
  ),
  Divider: () => <hr />,
  Input: (view) => <InputView {...view} t={useDisplay().t} />,
  TextArea: (view) => <TextAreaView {...view} t={useDisplay().t} />,
  Select: (view) => <SelectView {...view} t={useDisplay().t} />,
  MultiSelect: (view) => <MultiSelectView {...view} t={useDisplay().t} />,
  RadioGroup: (view) => <RadioGroupView {...view} t={useDisplay().t} />,
  Checkbox: CheckboxView,
  Switch: SwitchView,
  Slider: SliderView,
  DatePicker: (view) => <DatePickerView {...view} t={useDisplay().t} />,
  Form: (view) => <FormView {...view} t={useDisplay().t} />,
  Button: ButtonView,
  Math: (view) => <MathView {...view} t={useDisplay().t} />,
  Timer: (view) => <TimerView {...view} t={useDisplay().t} />,
  Flashcard: (view) => <FlashcardView {...view} t={useDisplay().t} />,
  YouTube: (view) => <YouTubeView {...view} t={useDisplay().t} />,
  Map: (view) => <MapView {...view} t={useDisplay().t} />,
  HtmlView: ({ props }) => <SandboxView {...props} />,
  Svg: SvgView,
  G: ({ props, renderNode }) => (
    <g transform={props.transform}>{renderNode(props.children)}</g>
  ),
  Circle: ({ props }) => <circle {...props} />,
  Ellipse: ({ props }) => <ellipse {...props} />,
  Rect: ({ props }) => <rect {...props} />,
  Line: ({ props }) => <line {...props} />,
  Path: ({ props }) => <path {...props} />,
  Polyline: ({ props }) => <polyline {...props} />,
  Polygon: ({ props }) => <polygon {...props} />,
  SvgText: ({ props }) => (
    <text x={props.x} y={props.y} fill={props.fill} fontSize={props.fontSize}>
      {props.text}
    </text>
  ),
});

function failOnRenderErrors(errors: OpenUIError[]) {
  if (errors.length === 0) return;
  throw new Error(
    `genui: accepted document could not be rendered: ${errors.map((error) => `${error.code}: ${error.message}`).join("; ")}`,
  );
}

function CapturedDocument({
  context: initialContext,
  locale,
}: {
  context: ComponentContext;
  locale: string;
}) {
  const [captured] = useState(() => {
    const tool = initialContext.toolResult;
    if (tool?.status !== "ok" || typeof tool.result !== "string") {
      throw new Error("genui: requires a successful captured tool result");
    }
    const presentation = Presentation.read(tool.result);
    return {
      context: initialContext,
      presentation,
      response: presentation.openUI(displayLibrary),
    };
  });
  const t = useTranslator(CATALOGS, locale);
  const display = useMemo(
    () => ({
      context: captured.context,
      presentation: captured.presentation,
      t,
      locale,
    }),
    [captured, t, locale],
  );
  return (
    <DisplayContext value={display}>
      <Renderer
        response={captured.response}
        library={displayLibrary}
        initialState={captured.presentation.initialState()}
        isStreaming={false}
        onError={failOnRenderErrors}
        publishObservability={false}
      />
    </DisplayContext>
  );
}

export function GenUIResult({ context }: { context: ComponentContext }) {
  const { signal, toolResult } = context;
  const instance = useMemo(() => crypto.randomUUID(), [signal, toolResult]);
  return (
    <CapturedDocument
      key={instance}
      context={context}
      locale={context.locale}
    />
  );
}
