import { useEffect, useMemo } from "react";
import { A2uiSurface, type ReactCatalogComponent } from "@a2ui/react";
import {
  STRICT_VALIDATION,
  MessageProcessor,
  type ActionPayload,
  type SurfaceModel,
} from "@a2ui/web_core";
import {
  defineComponent,
  useTranslator,
  type ComponentContext, type PluginComponentProps,
  type Translate,
} from "@flame/component-sdk";
import { DocumentBody, RecordGroup, RecordTitle } from "@flame/ui";
import { A2UI_VERSION } from "../contracts/catalog";
import { Presentation } from "../contracts/document";
import { InteractionController } from "./interactionController";
import { InteractionFeedback } from "./interaction";
import { CatalogDisplay, displayCatalog } from "./renderingCatalog";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

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
      { version: A2UI_VERSION, validationConfig: STRICT_VALIDATION },
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
  return function GenUIResult({ context, locale }: PluginComponentProps) {
    const t = useTranslator(CATALOGS, locale);
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
    const disconnect = controller.connect();
    return () => {
      disconnect();
      unmount();
    };
  }, [entry, controller]);
  return (
    <CatalogDisplay value={{ controller, t }}>
      <DocumentBody>
        <RecordGroup aria-label={entry.presentation.snapshot().title}>
          <RecordTitle>{entry.presentation.snapshot().title}</RecordTitle>
          <InteractionFeedback controller={controller} t={t} />
          {entry.surfaces.map((surface) => (
            <A2uiSurface key={surface.id} surface={surface} />
          ))}
        </RecordGroup>
      </DocumentBody>
    </CatalogDisplay>
  );
}
