import { useMemo } from "react";
import {
  createTranslator,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import {
  Badge,
  Callout,
  DocumentBody,
  HeadingLevel,
  Metadata,
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

function displayLibrary(
  t: Translate,
  context: ComponentContext,
  presentation: Presentation,
) {
  return createDisplayLibrary({
    Document: (view) => (
      <GenUIInteractionProvider
        context={context}
        presentation={presentation}
        t={t}
      >
        <DocumentView {...view} />
      </GenUIInteractionProvider>
    ),
    Stack: StackView,
    Grid: GridView,
    Card: CardView,
    Section: SectionView,
    Tabs: (view) => <TabsView {...view} t={t} />,
    Accordion: AccordionView,
    Disclosure: DisclosureView,
    Carousel: (view) => <CarouselView {...view} t={t} />,
    Popover: PopoverView,
    If: ({ props, renderNode }) =>
      props.when ? renderNode(props.children) : null,
    Text: ({ props }) => <RecordBody>{props.text}</RecordBody>,
    Markdown: (view) => <MarkdownView {...view} t={t} />,
    Code: (view) => <CodeView {...view} t={t} />,
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
    Image: (view) => <ImageView {...view} t={t} />,
    Metric: ({ props }) => (
      <Stats
        values={[
          [
            props.label,
            <>
              <bdi>{String(props.value)}</bdi>
              {props.detail !== undefined && (
                <Metadata>{props.detail}</Metadata>
              )}
            </>,
          ],
        ]}
      />
    ),
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
    Table: (view) => <TableView {...view} t={t} />,
    Chart: ({ props }) => (
      <ChartView
        {...props}
        dataLabel={t("chart.data", { chart: props.caption })}
        emptyLabel={t("chart.empty")}
      />
    ),
    Progress: ({ props }) => (
      <div>
        <span>{props.label}</span>
        <progress
          aria-label={props.label}
          value={props.value}
          max={props.max ?? 100}
        />
      </div>
    ),
    Divider: () => <hr />,
    Input: (view) => <InputView {...view} t={t} />,
    TextArea: (view) => <TextAreaView {...view} t={t} />,
    Select: (view) => <SelectView {...view} t={t} />,
    MultiSelect: (view) => <MultiSelectView {...view} t={t} />,
    RadioGroup: (view) => <RadioGroupView {...view} t={t} />,
    Checkbox: CheckboxView,
    Switch: SwitchView,
    Slider: SliderView,
    DatePicker: (view) => <DatePickerView {...view} t={t} />,
    Form: (view) => <FormView {...view} t={t} />,
    Button: ButtonView,
    Math: (view) => <MathView {...view} t={t} />,
    Timer: (view) => <TimerView {...view} t={t} />,
    Flashcard: (view) => <FlashcardView {...view} t={t} />,
    YouTube: YouTubeView,
    Map: (view) => <MapView {...view} t={t} />,
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
}

function failOnRenderErrors(errors: OpenUIError[]) {
  if (errors.length === 0) return;
  throw new Error(
    `genui: accepted document could not be rendered: ${errors.map((error) => `${error.code}: ${error.message}`).join("; ")}`,
  );
}

export function GenUIResult({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const tool = context.toolResult;
  const presentation = useMemo(() => {
    if (tool?.status !== "ok" || typeof tool.result !== "string") {
      throw new Error("genui: requires a successful captured tool result");
    }
    return Presentation.read(tool.result);
  }, [tool]);
  const library = useMemo(
    () => displayLibrary(t, context, presentation),
    [t, context, presentation],
  );
  const response = useMemo(
    () => presentation.openUI(library),
    [presentation, library],
  );
  return (
    <Renderer
      key={tool?.id}
      response={response}
      library={library}
      initialState={presentation.initialState()}
      isStreaming={false}
      onError={failOnRenderErrors}
      publishObservability={false}
    />
  );
}
