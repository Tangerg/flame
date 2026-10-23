import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { type BlockStatus, type QuestionItem } from "@/features/agent/public/contentBlock";
import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ChangeEvent,
  type CompositionEvent,
  type KeyboardEvent,
} from "react";
import { ChoiceList, ChoiceOption } from "@/ui/index";
import { Divider } from "@/ui/index";
import { Icon } from "@/ui/index";
import { Surface } from "@/ui/index";
import { Badge, Button, IconButton, TextArea, TextField } from "@flame/ui";
import { AgentActivityDisclosure } from "@/ui/agent/index";
import { useT } from "@/lib/i18n/index";
import {
  clearQuestionAnswer,
  createQuestionDraft,
  questionAnswerText,
  questionDraftComplete,
  setQuestionOptions,
  setQuestionText,
  type QuestionDraft,
} from "@/features/agent/public/messagePresentation";
import {
  questionCardSettledView,
  useQuestionCardActions,
} from "@/features/chat/message/application/questionCardModel";
import { useRuntimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import { useCompositionCommitGuard } from "@/features/composer/public/composition";
import {
  color,
  corner,
  leading,
  motion,
  radius,
  space,
  surface,
  type as typeStep,
  weight,
} from "@flame/ui/tokens.stylex";
import { chatStyles as ct } from "@/ui/agent/chatStyles";
import { messageStyles as ms } from "@/features/chat/message/ui/messageStyles";
import { vocab } from "@flame/ui";
const qc = stylex.create({
  request: { display: "flex", minHeight: 0, flexDirection: "column" },
  reading: { minHeight: 0, overflowY: "auto", overscrollBehavior: "contain" },
  askBody: { paddingTop: space.s1, paddingBottom: space.s0_5 },
  settledLine: {
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1,
  },
  settledList: { display: "flex", flexDirection: "column", gap: space.s3 },
  settledItem: { display: "flex", flexDirection: "column", gap: space.s1 },
  settledAsk: { whiteSpace: "pre-wrap", lineHeight: leading.body, color: color.fgMuted },
  settledAnswer: {
    whiteSpace: "pre-wrap",
    overflowWrap: "break-word",
    lineHeight: leading.body,
    color: color.fgFaint,
  },
  pager: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1,
    color: color.fgFaint,
  },
  pageCount: { minWidth: space.s10, textAlign: "center", fontVariantNumeric: "tabular-nums" },
  choices: {
    display: "flex",
    flexDirection: "column",
    gap: space.s1,
    paddingInline: space.s2,
    paddingTop: space.s1,
    paddingBottom: 0,
  },
  optionLine: {
    display: "flex",
    flexWrap: "wrap",
    minWidth: 0,
    flex: 1,
    alignItems: "baseline",
    columnGap: space.s2,
    rowGap: space.s0_5,
  },
  optionLabel: {
    minWidth: 0,
    maxWidth: "100%",
    overflowWrap: "anywhere",
    fontWeight: weight.medium,
    color: color.fg,
  },
  optionDescription: { flexBasis: "100%", minWidth: 0, overflowWrap: "anywhere" },
  field: { paddingInline: space.s2, paddingBlock: space.s1_5 },
  textArea: { maxHeight: "calc(var(--spacing) * 40)" },
  freeRow: {
    display: "flex",
    minHeight: space.s8,
    borderRadius: radius.field,
    alignItems: "center",
    gap: space.s2,
    backgroundColor: { default: null, ":focus-within": surface.hover },
    paddingInline: space.s2,
    paddingBlock: space.s1_5,
    transitionProperty: "background-color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  freeMark: {
    display: "grid",
    height: space.s5,
    width: space.s5,
    flexShrink: 0,
    placeItems: "center",
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
  },
  freeMarkOn: { borderColor: color.fg, backgroundColor: color.fg, color: surface.canvas },
  freeMarkOff: {
    borderColor: surface.field,
    backgroundColor: surface.surface2,
    color: color.fgMuted,
  },
  freeField: { height: space.s5, padding: 0, lineHeight: leading.body },
  footerShrink: { flexShrink: 0 },
});

interface Props {
  sessionId: string;
  status: BlockStatus;
  /** The Run to resume; present only while this question awaits an answer. */
  resumeRunId?: string;
  itemId?: string;
  questions: QuestionItem[];
  answered?: boolean;
  answers?: string[][];
}

const RECOMMENDED_SUFFIX = " (Recommended)";

export function QuestionCard({
  sessionId,
  status,
  resumeRunId,
  itemId,
  questions,
  answered,
  answers,
}: Props) {
  const t = useT();
  const questionCardId = useId();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const [draft, setDraft] = useState<QuestionDraft>(() => createQuestionDraft(questions));
  const [questionIndex, setQuestionIndex] = useState(0);
  const [settledOpen, setSettledOpen] = useState(false);
  const requestRef = useRef<HTMLDivElement>(null);
  const focusQuestionOnChange = useRef(false);
  const activeQuestionRef = useRef<HTMLDivElement>(null);
  const composition = useCompositionCommitGuard();
  const actions = useQuestionCardActions({ sessionId, resumeRunId, itemId, questions, draft });
  const activeIndex = Math.min(questionIndex, Math.max(questions.length - 1, 0));
  const activeQuestion = questions[activeIndex];
  const activeDraft = draft[activeIndex] ?? { selected: [], text: "" };
  const activeQuestionComplete = activeQuestion
    ? questionDraftComplete([activeQuestion], [activeDraft])
    : false;
  const isLastQuestion = activeIndex >= questions.length - 1;

  useLayoutEffect(() => {
    if (!focusQuestionOnChange.current) {
      requestRef.current?.focus();
      return;
    }
    focusQuestionOnChange.current = false;
    activeQuestionRef.current
      ?.querySelector<HTMLElement>('[role="radio"], [role="checkbox"], textarea, input')
      ?.focus();
  }, [activeIndex]);

  const navigateQuestion = (nextIndex: number) => {
    const bounded = Math.min(Math.max(nextIndex, 0), Math.max(questions.length - 1, 0));
    if (bounded === activeIndex) return;
    composition.reset();
    focusQuestionOnChange.current = true;
    setQuestionIndex(bounded);
  };

  const settled = questionCardSettledView({
    status,
    resumeRunId,
    answered,
    pending: actions.pending,
    questions,
    draft,
    answers,
  });

  if (settled.settled) {
    const shown = settled.answers;
    if (!shown)
      return (
        <Divider icon={<Icon name="x" size="xs" />}>{t("question.settled.dismissed")}</Divider>
      );
    const countLabel = t("question.settled.question", { count: questions.length });
    return (
      <AgentActivityDisclosure
        icon="question"
        shell="line"
        open={settledOpen}
        onToggle={() => setSettledOpen((open) => !open)}
        label={
          <span {...stylex.props(qc.settledLine, vocab.truncate)}>
            <span {...stylex.props(vocab.muted)}>{t("question.settled.asked")}</span>
            <span {...stylex.props(vocab.faint)}>{countLabel}</span>
          </span>
        }
        contentClassName={stylex.props(qc.askBody).className}
      >
        <div {...stylex.props(qc.settledList)}>
          {questions.map((question, index) => (
            <div key={index} {...stylex.props(qc.settledItem)}>
              <div {...stylex.props(qc.settledAsk, typeStep.uiSm)}>{question.prompt}</div>
              <div data-settled-answer="" {...stylex.props(qc.settledAnswer, typeStep.uiSm)}>
                {questionAnswerText(shown, index) || t("question.settled.noAnswer")}
              </div>
            </div>
          ))}
        </div>
      </AgentActivityDisclosure>
    );
  }

  const submitOrAdvance = (nextDraft: QuestionDraft) => {
    setDraft(nextDraft);
    if (isLastQuestion) {
      if (runtimeAvailable && !actions.disabled) actions.submit(nextDraft);
      return;
    }
    navigateQuestion(activeIndex + 1);
  };

  const selectOptions = (
    question: Extract<QuestionItem, { type: "choice" }>,
    selected: string[],
  ) => {
    if (!runtimeAvailable || actions.pending) return;
    const nextDraft = setQuestionOptions(draft, activeIndex, question, selected);
    if (question.multiple) {
      setDraft(nextDraft);
      return;
    }
    submitOrAdvance(nextDraft);
  };

  const skipCurrent = () => {
    if (!runtimeAvailable || actions.pending) return;
    submitOrAdvance(clearQuestionAnswer(draft, activeIndex));
  };

  const advanceCurrent = () => {
    if (!runtimeAvailable || actions.pending) return;
    submitOrAdvance(draft);
  };

  const handleCompositionStart = () => {
    composition.start();
  };

  const handleAnswerChange = (event: ChangeEvent<HTMLTextAreaElement | HTMLInputElement>) => {
    if (!activeQuestion) return;
    const value = event.currentTarget.value;
    const nativeComposing = (event.nativeEvent as { isComposing?: boolean }).isComposing === true;
    composition.noteChange(nativeComposing);
    setDraft((previous) => setQuestionText(previous, activeIndex, activeQuestion, value));
  };

  const handleCompositionEnd = (
    event: CompositionEvent<HTMLTextAreaElement | HTMLInputElement>,
  ) => {
    composition.end();
    if (!activeQuestion) return;
    const value = event.currentTarget.value;
    setDraft((previous) => setQuestionText(previous, activeIndex, activeQuestion, value));
  };

  const handleAnswerKeyDown = (event: KeyboardEvent<HTMLTextAreaElement | HTMLInputElement>) => {
    const compositionIntent = composition.keyIntent(event.nativeEvent);
    if (compositionIntent !== null) {
      if (compositionIntent === "committed-enter") event.preventDefault();
      return;
    }
    if (event.key !== "Enter" || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey) {
      return;
    }
    event.preventDefault();
    if (activeQuestionComplete) advanceCurrent();
    else skipCurrent();
  };

  if (!activeQuestion) return null;

  const promptId = `${questionCardId}-prompt-${activeIndex}`;
  const isSingleChoice = activeQuestion.type === "choice" && !activeQuestion.multiple;
  const explicitFreeform = activeDraft.text.trim().length > 0;
  const explicitMultiChoice =
    activeQuestion.type === "choice" && activeQuestion.multiple && activeDraft.selected.length > 0;
  const actionSkips = isSingleChoice || (!explicitFreeform && !explicitMultiChoice);

  return (
    <Surface
      ref={requestRef}
      variant="prompt"
      inset="none"
      tabIndex={-1}
      data-slot="question-request-surface"
      data-chrome-focus
      className={stylex.props(ms.cardClip, qc.request).className}
    >
      <div {...stylex.props(qc.reading)}>
        <div {...stylex.props(ms.cardHeadTight)}>
          <h3 id={promptId} className={stylex.props(ms.cardPromptFlush, typeStep.uiMd).className}>
            {activeQuestion.prompt}
          </h3>
          {questions.length > 1 && (
            <div {...stylex.props(qc.pager, typeStep.uiXs)}>
              <IconButton
                icon={iconGlyph("chevron-left")}
                size="xs"
                quiet
                disabled={activeIndex === 0}
                focusableWhenDisabled
                aria-label={t("question.action.previous")}
                onClick={() => navigateQuestion(activeIndex - 1)}
              />
              <span {...stylex.props(qc.pageCount)}>
                {t("question.progress", { current: activeIndex + 1, total: questions.length })}
              </span>
              <IconButton
                icon={iconGlyph("chevron-right")}
                size="xs"
                quiet
                disabled={isLastQuestion}
                focusableWhenDisabled
                aria-label={t("question.action.next")}
                onClick={() => navigateQuestion(activeIndex + 1)}
              />
            </div>
          )}
        </div>

        <div ref={activeQuestionRef} {...stylex.props(qc.choices)}>
          {activeQuestion.type === "choice" && (
            <ChoiceList
              multiple={activeQuestion.multiple}
              value={activeDraft.selected}
              values={activeQuestion.options.map((option) => option.label)}
              labelledBy={promptId}
              disabled={!runtimeAvailable}
              numbered
              pending={actions.pending}
              onValueChange={(selected) => selectOptions(activeQuestion, selected)}
            >
              {activeQuestion.options.map((option, optionIndex) => {
                const selected = activeDraft.selected.includes(option.label);
                const recommended = option.label.endsWith(RECOMMENDED_SUFFIX);
                const label = recommended
                  ? option.label.slice(0, -RECOMMENDED_SUFFIX.length)
                  : option.label;
                return (
                  <ChoiceOption
                    key={option.label}
                    multiple={activeQuestion.multiple}
                    value={option.label}
                    selected={selected}
                    ordinal={optionIndex + 1}
                    label={option.label}
                    description={option.description}
                    disabled={!runtimeAvailable}
                    pending={actions.pending}
                    onReselect={() => selectOptions(activeQuestion, [option.label])}
                  >
                    <span {...stylex.props(qc.optionLine)}>
                      <span {...stylex.props(qc.optionLabel, typeStep.uiMd)}>{label}</span>
                      {recommended && <Badge>{t("question.recommended")}</Badge>}
                      {option.description && (
                        <span
                          className={
                            stylex.props(
                              qc.optionDescription,
                              ct.bodyLeading,
                              vocab.muted,
                              typeStep.uiSm,
                            ).className
                          }
                        >
                          {option.description}
                        </span>
                      )}
                    </span>
                  </ChoiceOption>
                );
              })}
            </ChoiceList>
          )}

          {activeQuestion.type === "text" && (
            <div {...stylex.props(qc.field)}>
              <TextArea
                size="sm"
                rows={4}
                value={activeDraft.text}
                aria-label={activeQuestion.prompt}
                placeholder={t("question.freetext.placeholder")}
                disabled={!runtimeAvailable}
                pending={actions.pending}
                onChange={handleAnswerChange}
                onCompositionStart={handleCompositionStart}
                onCompositionEnd={handleCompositionEnd}
                onKeyDown={handleAnswerKeyDown}
                onKeyUp={() => composition.clear()}
                onBlur={() => composition.clear()}
                className={stylex.props(qc.textArea).className}
              />
            </div>
          )}

          {activeQuestion.type === "choice" && activeQuestion.allowCustom && (
            <div {...stylex.props(qc.freeRow)}>
              <span
                aria-hidden
                {...stylex.props(
                  qc.freeMark,
                  corner.pill,
                  explicitFreeform ? qc.freeMarkOn : qc.freeMarkOff,
                )}
              >
                <Icon name="edit" size="xs" />
              </span>
              <TextField
                variant="bare"
                value={activeDraft.text}
                aria-label={activeQuestion.prompt}
                placeholder={t("question.freetext.placeholder")}
                disabled={!runtimeAvailable}
                pending={actions.pending}
                onChange={handleAnswerChange}
                onCompositionStart={handleCompositionStart}
                onCompositionEnd={handleCompositionEnd}
                onKeyDown={handleAnswerKeyDown}
                onKeyUp={() => composition.clear()}
                onBlur={() => composition.clear()}
                styles={qc.freeField}
              />
            </div>
          )}
        </div>
      </div>
      <div {...stylex.props(ms.cardActions, qc.footerShrink)}>
        <Button
          variant={actionSkips ? "outline" : "primary"}
          size="sm"
          disabled={actions.disabled || !runtimeAvailable}
          onClick={actionSkips ? skipCurrent : advanceCurrent}
        >
          {t(actionSkips ? "question.action.skip" : "question.action.advance")}
        </Button>
      </div>
    </Surface>
  );
}
