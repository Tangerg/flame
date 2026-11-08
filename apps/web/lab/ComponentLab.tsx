import * as stylex from "@stylexjs/stylex";
import { labStyles as styles } from "./labStyles";
import { previewStyles } from "./previewStyles";
import { useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import {
  Button,
  FormField,
  Icon,
  IconButton,
  SearchField,
  Segmented,
  Select,
  Switch,
} from "@flame/ui";
import { ArrowUpRight, ChevronLeft, ChevronRight, Copy, Refresh, Trash } from "@flame/icons/react";
import { catalog } from "./catalog";
import type { LabEnvironment } from "./environment";
import { messages } from "./messages";
import { ObservationLog } from "./observations";
import { GalleryPreview, PreviewFrame } from "./PreviewFrame";
import { PreviewCanvas } from "./PreviewCanvas";
import {
  LAB_PATH,
  LAB_FONT_SIZES,
  previewURL,
  previewWidth,
  readSelection,
  selectStory,
  selectionSearch,
  VIEWPORTS,
  type LabSelection,
} from "./selection";
import { localize, type LabPanel, type LabStory } from "./story";

const GALLERY_PAGE_SIZE = 6;
const COMPARISON_GAP = 32;
const BOARD_CAPTION_HEIGHT = 36;

export function ComponentLab({
  initial,
  environment,
}: {
  initial: LabSelection;
  environment: LabEnvironment;
}) {
  const [selection, setSelection] = useState(initial);
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const [generation, setGeneration] = useState(0);
  const [notice, setNotice] = useState("");
  const [log] = useState(() => new ObservationLog());
  const entries = useSyncExternalStore(log.subscribe, log.snapshot);
  const main = useRef<HTMLElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const galleryScroll = useRef<HTMLDivElement>(null);
  const handoff = useRef<Element | null>(null);
  const galleryReturn = useRef<{ context: string; top: number } | null>(null);
  const story = catalog.story(selection.storyId);
  const panel = story.panel;
  const t = (key: keyof typeof messages) => localize(messages[key], selection.locale);
  const filtered = catalog.search(panel, query, selection.locale);
  const currentPage = Math.min(
    page,
    Math.max(0, Math.ceil(filtered.length / GALLERY_PAGE_SIZE) - 1),
  );
  const visible = filtered.slice(
    currentPage * GALLERY_PAGE_SIZE,
    (currentPage + 1) * GALLERY_PAGE_SIZE,
  );
  const inspecting = selection.mode === "inspect";
  const galleryContext = `${panel}\n${query}\n${currentPage}`;
  const width = previewWidth(story, selection.viewport);
  const opposite = selection.theme === "light" ? "dark" : "light";
  const { theme, locale, motion, fontSize } = selection;

  useEffect(() => {
    window.history.replaceState(null, "", `${LAB_PATH}?${selectionSearch(selection, catalog)}`);
  }, [selection]);

  useEffect(() => {
    const restore = () => setSelection(readSelection(window.location.search, catalog));
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, []);

  useEffect(() => {
    void environment
      .update({ theme, locale, motion, fontSize })
      .catch((error: unknown) => setNotice(String(error)));
  }, [environment, theme, locale, motion, fontSize]);

  useLayoutEffect(() => {
    if (galleryReturn.current?.context !== galleryContext) galleryReturn.current = null;
    const node = galleryScroll.current;
    if (!node) return;
    node.scrollTop = galleryReturn.current?.top ?? 0;
    galleryReturn.current = null;
  }, [inspecting, galleryContext]);

  useLayoutEffect(() => {
    const opener = handoff.current;
    handoff.current = null;
    if (!opener) return;
    const focused = document.activeElement;
    if (focused !== opener && focused !== null && focused !== document.body) return;
    main.current?.focus({ preventScroll: true });
    heading.current?.scrollIntoView({ block: "nearest" });
  }, [inspecting, galleryContext]);

  function rememberGallery() {
    const node = galleryScroll.current;
    if (node) galleryReturn.current = { context: galleryContext, top: node.scrollTop };
  }

  function choose(next: LabStory, opener?: Element) {
    rememberGallery();
    handoff.current = opener ?? null;
    setSelection((previous) => ({ ...selectStory(previous, next), mode: "inspect" }));
    setNotice("");
  }

  function turnPage(next: number, opener: Element) {
    handoff.current = opener;
    setPage(next);
  }

  function choosePanel(next: LabPanel) {
    galleryReturn.current = null;
    setSelection((previous) => ({
      ...selectStory(previous, catalog.first(next)),
      mode: "gallery",
    }));
    setQuery("");
    setPage(0);
    setNotice("");
  }

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(
        new URL(`${LAB_PATH}?${selectionSearch(selection, catalog)}`, window.location.origin).href,
      );
      setNotice(t("copied"));
    } catch {
      setNotice(t("clipboardFailed"));
    }
  }

  const sourceURL = `https://github.com/Tangerg/flame/blob/main/${story.source}`;
  const independentURL = previewURL(story, selection, `standalone:${story.id}`);
  const time = new Intl.DateTimeFormat(selection.locale, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });

  return (
    <div data-lab-shell {...stylex.props(styles.shell, inspecting && styles.inspectShell)}>
      <a href="#lab-main" {...stylex.props(styles.skipLink)}>
        {t("skipToPreview")}
      </a>
      <header {...stylex.props(styles.header)}>
        <a href={LAB_PATH} {...stylex.props(styles.brand)} aria-label={`Flame ${t("title")}`}>
          <strong>Flame</strong>
          <span {...stylex.props(styles.brandSubtitle)}>{t("title")}</span>
        </a>
        <div {...stylex.props(styles.headerActions)}>
          <Segmented
            ariaLabel={t("theme")}
            value={selection.theme}
            onChange={(theme) => setSelection((previous) => ({ ...previous, theme }))}
            options={[
              { value: "light", label: t("light") },
              { value: "dark", label: t("dark") },
            ]}
          />
          <Select
            label={t("language")}
            size="sm"
            value={selection.locale}
            onValueChange={(locale) => {
              if (locale === "zh-CN" || locale === "en")
                setSelection((previous) => ({ ...previous, locale }));
            }}
            options={[
              { value: "zh-CN", label: t("chinese") },
              { value: "en", label: t("english") },
            ]}
          />
          <Button
            variant="ghost"
            size="sm"
            aria-label={t("copyLink")}
            onClick={() => void copyLink()}
          >
            <Icon glyph={Copy} size="sm" aria-hidden="true" />
            <span {...stylex.props(styles.copyLabel)}>{t("copyLink")}</span>
          </Button>
        </div>
      </header>

      <aside {...stylex.props(styles.sidebar)} aria-label={t("catalog")}>
        <div {...stylex.props(styles.panelSwitch)} role="group" aria-label={t("catalog")}>
          {(["foundation", "business"] as const).map((candidate) => (
            <Button
              key={candidate}
              styles={styles.panelButton}
              variant={panel === candidate ? "soft" : "ghost"}
              active={panel === candidate}
              aria-pressed={panel === candidate}
              onClick={() => choosePanel(candidate)}
            >
              {t(candidate)}
            </Button>
          ))}
        </div>
        <SearchField
          aria-label={t("search")}
          placeholder={t("searchPlaceholder")}
          size="sm"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setPage(0);
          }}
          onClear={() => {
            setQuery("");
            setPage(0);
          }}
          clearLabel={t("clearSearch")}
        />
        <div {...stylex.props(styles.compactCatalog)}>
          <Select
            label={t("catalog")}
            value={filtered.some((entry) => entry.id === story.id) ? story.id : null}
            options={filtered.map((entry) => ({
              value: entry.id,
              label: localize(entry.title, selection.locale),
            }))}
            onValueChange={(id) => {
              if (id) choose(catalog.story(id));
            }}
          />
        </div>
        <nav {...stylex.props(styles.storyList)} aria-label={t("catalog")}>
          {filtered.map((entry, index) => {
            const group = localize(entry.group, selection.locale);
            const precedingGroup =
              index > 0 ? localize(filtered[index - 1]!.group, selection.locale) : null;
            return (
              <div key={entry.id}>
                {group !== precedingGroup && (
                  <h3 {...stylex.props(styles.groupLabel, index === 0 && styles.firstGroupLabel)}>
                    {group}
                  </h3>
                )}
                <Button
                  variant="ghost"
                  active={story.id === entry.id}
                  aria-current={story.id === entry.id ? "true" : undefined}
                  styles={[styles.storyLink, story.id === entry.id && styles.storySelected]}
                  onClick={() => choose(entry)}
                >
                  <span {...stylex.props(styles.storyTitle)}>
                    {localize(entry.title, selection.locale)}
                  </span>
                </Button>
              </div>
            );
          })}
          {filtered.length === 0 && <p {...stylex.props(styles.muted)}>{t("noResults")}</p>}
        </nav>
      </aside>

      <main
        id="lab-main"
        ref={main}
        tabIndex={-1}
        data-focus-inset=""
        aria-labelledby="lab-view-title"
        {...stylex.props(styles.main)}
      >
        <div {...stylex.props(styles.mainHeading)}>
          <h1 id="lab-view-title" ref={heading} {...stylex.props(styles.heading)}>
            {selection.mode === "gallery" ? t(panel) : localize(story.title, selection.locale)}
          </h1>
          <Segmented
            ariaLabel={t("viewMode")}
            value={selection.mode}
            onChange={(mode) => {
              if (mode === "inspect") rememberGallery();
              setSelection((previous) => ({ ...previous, mode }));
            }}
            options={[
              { value: "gallery", label: t("gallery") },
              { value: "inspect", label: t("inspect") },
            ]}
          />
        </div>
        {selection.mode === "gallery" ? (
          <div ref={galleryScroll} {...stylex.props(styles.galleryScroll)}>
            <div {...stylex.props(styles.gallery)}>
              {visible.map((entry) => {
                const previewSelection =
                  entry.id === story.id ? selection : selectStory(selection, entry);
                return (
                  <article
                    {...stylex.props(styles.card)}
                    key={entry.id}
                    data-selected={entry.id === story.id || undefined}
                  >
                    <div {...stylex.props(styles.cardHeading)}>
                      <div>
                        <h2 {...stylex.props(styles.cardTitle)}>
                          {localize(entry.title, selection.locale)}
                        </h2>
                      </div>
                      <IconButton
                        icon={ArrowUpRight}
                        aria-label={`${t("select")} · ${localize(entry.title, selection.locale)}`}
                        onClick={(event) => choose(entry, event.currentTarget)}
                      />
                    </div>
                    <GalleryPreview
                      story={entry}
                      selection={previewSelection}
                      width={entry.width}
                      generation={generation}
                      log={log}
                    />
                  </article>
                );
              })}
            </div>
            {filtered.length === 0 && (
              <div {...stylex.props(styles.empty)}>
                <h2 {...stylex.props(styles.emptyHeading)}>{t("noResults")}</h2>
                <p {...stylex.props(styles.muted)}>{t("noResultsHint")}</p>
              </div>
            )}
            {filtered.length > GALLERY_PAGE_SIZE && (
              <nav {...stylex.props(styles.pagination)} aria-label={t("pagination")}>
                <IconButton
                  icon={ChevronLeft}
                  aria-label={t("previous")}
                  disabled={currentPage === 0}
                  onClick={(event) => turnPage(currentPage - 1, event.currentTarget)}
                />
                <span>
                  {currentPage + 1} / {Math.ceil(filtered.length / GALLERY_PAGE_SIZE)}
                </span>
                <IconButton
                  icon={ChevronRight}
                  aria-label={t("next")}
                  disabled={(currentPage + 1) * GALLERY_PAGE_SIZE >= filtered.length}
                  onClick={(event) => turnPage(currentPage + 1, event.currentTarget)}
                />
              </nav>
            )}
          </div>
        ) : (
          <PreviewCanvas
            key={panel}
            panel={panel}
            locale={selection.locale}
            content={{
              width: selection.compare ? width * 2 + COMPARISON_GAP : width,
              height: story.height + BOARD_CAPTION_HEIGHT,
            }}
          >
            <div {...stylex.props(previewStyles.boards)} style={{ gap: COMPARISON_GAP }}>
              <section
                data-lab-board
                {...stylex.props(previewStyles.board)}
                aria-label={t(selection.theme)}
              >
                <div
                  {...stylex.props(previewStyles.boardCaption)}
                  style={{ height: BOARD_CAPTION_HEIGHT }}
                >
                  {width} × {story.height}
                  <span {...stylex.props(previewStyles.boardTheme)}>{t(selection.theme)}</span>
                </div>
                <PreviewFrame
                  story={story}
                  selection={selection}
                  width={width}
                  generation={generation}
                  log={log}
                />
              </section>
              {selection.compare && (
                <section
                  data-lab-board
                  {...stylex.props(previewStyles.board)}
                  aria-label={t(opposite)}
                >
                  <div
                    {...stylex.props(previewStyles.boardCaption)}
                    style={{ height: BOARD_CAPTION_HEIGHT }}
                  >
                    {width} × {story.height}
                    <span {...stylex.props(previewStyles.boardTheme)}>{t(opposite)}</span>
                  </div>
                  <PreviewFrame
                    story={story}
                    selection={{ ...selection, theme: opposite }}
                    width={width}
                    generation={generation}
                    log={log}
                  />
                </section>
              )}
            </div>
          </PreviewCanvas>
        )}
      </main>

      {inspecting && (
        <aside {...stylex.props(styles.inspector)} aria-label={t("input")}>
          <section {...stylex.props(styles.inspectorSection)}>
            <div {...stylex.props(styles.sectionHeading)}>
              <h2 {...stylex.props(styles.inspectorHeading, styles.sectionTitle)}>{t("input")}</h2>
              <IconButton
                icon={Refresh}
                aria-label={t("resetPreview")}
                onClick={() => setGeneration((value) => value + 1)}
              />
            </div>
            <FormField label={t("variants")}>
              <Select
                label={t("variants")}
                value={selection.variantId}
                options={story.variants.map((variant) => ({
                  value: variant.id,
                  label: localize(variant.title, selection.locale),
                }))}
                onValueChange={(variantId) => {
                  if (story.variants.some((variant) => variant.id === variantId))
                    setSelection((previous) => ({ ...previous, variantId, mode: "inspect" }));
                }}
              />
            </FormField>
            <FormField label={t("viewport")}>
              <Select
                label={t("viewport")}
                value={selection.viewport}
                options={VIEWPORTS.map(({ id }) => ({ value: id, label: t(id) }))}
                onValueChange={(value) => {
                  const viewport = VIEWPORTS.find(({ id }) => id === value)?.id;
                  if (viewport)
                    setSelection((previous) => ({ ...previous, viewport, mode: "inspect" }));
                }}
              />
            </FormField>
            <FormField label={t("fontSize")}>
              <Select
                label={t("fontSize")}
                value={String(selection.fontSize)}
                options={LAB_FONT_SIZES.map((size) => ({
                  value: String(size),
                  label: `${size} px`,
                }))}
                onValueChange={(value) => {
                  if (value && LAB_FONT_SIZES.includes(Number(value)))
                    setSelection((previous) => ({ ...previous, fontSize: Number(value) }));
                }}
              />
            </FormField>
            <FormField label={t("motion")}>
              <Select
                label={t("motion")}
                value={selection.motion}
                options={[
                  { value: "off", label: t("motionOff") },
                  { value: "full", label: t("motionFull") },
                ]}
                onValueChange={(motion) => {
                  if (motion === "off" || motion === "full")
                    setSelection((previous) => ({ ...previous, motion }));
                }}
              />
            </FormField>
            <label {...stylex.props(styles.toggleRow)}>
              <span>{t("compare")}</span>
              <Switch
                ariaLabel={t("compare")}
                checked={selection.compare}
                onCheckedChange={(compare) =>
                  setSelection((previous) => ({ ...previous, compare, mode: "inspect" }))
                }
              />
            </label>
            <a
              {...stylex.props(styles.textLink)}
              href={independentURL}
              target="_blank"
              rel="noreferrer"
            >
              {t("openPreview")}
              <Icon glyph={ArrowUpRight} size="sm" aria-hidden="true" />
            </a>
          </section>
          <section data-lab-source {...stylex.props(styles.inspectorSection, styles.source)}>
            <a
              {...stylex.props(styles.sourceLink)}
              href={sourceURL}
              target="_blank"
              rel="noreferrer"
              title={story.source}
            >
              {t("openSource")}
              <Icon glyph={ArrowUpRight} size="sm" aria-hidden="true" />
            </a>
          </section>
          <section {...stylex.props(styles.observations)}>
            <div {...stylex.props(styles.sectionHeading)}>
              <h2 {...stylex.props(styles.inspectorHeading, styles.sectionTitle)}>
                {t("observations")}
                <span {...stylex.props(styles.count)}>{entries.length}</span>
              </h2>
              <IconButton
                icon={Trash}
                aria-label={t("clearEvents")}
                disabled={!entries.length}
                onClick={() => log.clear()}
              />
            </div>
            {!entries.length && <p {...stylex.props(styles.noEvents)}>{t("noEvents")}</p>}
            <ol {...stylex.props(styles.events)} aria-label={t("observations")}>
              {[...entries].reverse().map((entry) => (
                <li {...stylex.props(styles.event)} key={entry.sequence} data-kind={entry.kind}>
                  <div {...stylex.props(styles.eventMeta)}>
                    <span>{entry.instanceId.split(":")[0]}</span>
                    <time>{time.format(entry.receivedAt)}</time>
                  </div>
                  <strong
                    {...stylex.props(styles.eventName, entry.kind === "error" && styles.errorInk)}
                  >
                    {entry.name}
                  </strong>
                  {entry.detail && <pre {...stylex.props(styles.eventDetail)}>{entry.detail}</pre>}
                </li>
              ))}
            </ol>
          </section>
        </aside>
      )}
      <div {...stylex.props(styles.notice, !notice && styles.noNotice)} role="status">
        {notice}
        {notice && (
          <Button variant="ghost" size="sm" aria-label={t("dismiss")} onClick={() => setNotice("")}>
            ×
          </Button>
        )}
      </div>
    </div>
  );
}
