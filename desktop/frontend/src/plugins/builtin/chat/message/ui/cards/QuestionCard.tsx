import * as stylex from "@stylexjs/stylex";
import type { BlockStatus, QuestionItem } from "@/plugins/sdk/types/contentBlock";
import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ChangeEvent,
  type CompositionEvent,
  type KeyboardEvent,
} from "react";
import {
  Badge,
  Button,
  ChoiceList,
  ChoiceOption,
  Icon,
  IconButton,
  Surface,
  TextArea,
  TextField,
} from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { HitlSettledRow } from "./HitlCard";
import { useT } from "@/lib/i18n";
import {
  clearQuestionAnswer,
  createQuestionDraft,
  questionAnswerText,
  questionDraftComplete,
  setQuestionOptions,
  setQuestionText,
  type QuestionDraft,
} from "@/plugins/builtin/agent/public/messagePresentation";
import {
  questionCardSettledView,
  useQuestionCardActions,
} from "../../application/questionCardModel";
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import { composerCompositionKeyIntent } from "@/plugins/builtin/chat/composer/public/composition";
import {
  color,
  corner,
  leading,
  motion,
  space,
  surface,
  type as typeStep,
} from "@/styles/tokens.stylex";
import { chatStyles as ct } from "../../../chatStyles";
import { messageStyles as ms } from "../messageStyles";
import { vocab } from "@/ui";

const qc = stylex.create({
  /** The ask's body sits close under its prompt: this is a question, not a section. */
  askBody: { paddingTop: space.s1, paddingBottom: space.s0_5 },
  settledLine: {
    display: "flex",
    minWidth: 0,
    alignItems: "center",
    gap: space.s1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
  settledList: { display: "flex", flexDirection: "column", gap: space.s3 },
  settledItem: { display: "flex", flexDirection: "column", gap: space.s1 },
  // A settled pair reads as a record, so both halves take the tight line box a log does.
  settledAsk: { whiteSpace: "pre-wrap", lineHeight: "1rem", color: color.fgMuted },
  settledAnswer: {
    whiteSpace: "pre-wrap",
    overflowWrap: "break-word",
    lineHeight: "1rem",
    color: color.fgFaint,
  },
  // The card takes focus so the keyboard can answer it, and the highlighted OPTION is the
  // indicator — the same reason a menu popup opts out of the ring.
  pager: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s1,
    color: color.fgFaint,
  },
  // A measure the count cannot outgrow, so the arrows beside it do not shift as it counts.
  pageCount: { minWidth: space.s10, textAlign: "center", fontVariantNumeric: "tabular-nums" },
  choices: {
    display: "flex",
    flexDirection: "column",
    gap: space.s1,
    paddingInline: space.s2,
    paddingTop: space.s1,
    paddingBottom: space.s2,
  },
  optionLine: { display: "flex", minWidth: 0, flex: 1, alignItems: "baseline", gap: space.s2 },
  // The label may take half the row and no more: the hint beside it has to be readable too.
  optionLabel: {
    minWidth: 0,
    maxWidth: "50%",
    flexShrink: 0,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    fontWeight: 500,
    color: color.fg,
  },
  field: { paddingInline: space.s2, paddingBlock: space.s1_5 },
  textArea: { maxHeight: "calc(var(--spacing) * 40)" },
  freeRow: {
    display: "flex",
    minHeight: space.s8,
    alignItems: "center",
    gap: space.s2,
    backgroundColor: { default: null, ":focus-within": surface.hover },
    paddingInline: space.s2,
    paddingBlock: space.s1_5,
    transitionProperty: "background-color",
    transitionDuration: motion.color,
  },
  freeMark: {
    display: "grid",
    height: space.s5,
    width: space.s5,
    flexShrink: 0,
    placeItems: "center",
    borderWidth: "1px",
    borderStyle: "solid",
  },
  // Filled once the reader has chosen to type rather than pick: the mark answers like an option.
  freeMarkOn: { borderColor: color.fg, backgroundColor: color.fg, color: surface.canvas },
  freeMarkOff: {
    borderColor: surface.field,
    backgroundColor: surface.surface2,
    color: color.fgMuted,
  },
  freeField: { height: space.s5, padding: 0, lineHeight: leading.body },
  footer: {
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: space.s2,
    paddingInline: space.s2,
    paddingBlock: space.s1,
  },
});

interface Props {
  status: BlockStatus;
  runId?: string;
  itemId?: string;
  questions: QuestionItem[];
  answered?: boolean;
  answers?: string[][];
}

const RECOMMENDED_SUFFIX = " (Recommended)";

export function QuestionCard({ status, runId, itemId, questions, answered, answers }: Props) {
  const t = useT();
  const questionCardId = useId();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const [draft, setDraft] = useState<QuestionDraft>(() => createQuestionDraft(questions));
  const [questionIndex, setQuestionIndex] = useState(0);
  const [settledOpen, setSettledOpen] = useState(false);
  const requestRef = useRef<HTMLDivElement>(null);
  const focusQuestionOnChange = useRef(false);
  const activeQuestionRef = useRef<HTMLDivElement>(null);
  const composingRef = useRef(false);
  const compositionCommitPendingRef = useRef(false);
  const actions = useQuestionCardActions({ runId, itemId, status, questions, draft });
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
    composingRef.current = false;
    compositionCommitPendingRef.current = false;
    focusQuestionOnChange.current = true;
    setQuestionIndex(bounded);
  };

  const settled = questionCardSettledView({
    status,
    answered,
    pending: actions.pending,
    questions,
    draft,
    answers,
  });

  if (settled.settled) {
    const shown = settled.answers;
    if (!shown) return <HitlSettledRow label={t("question.settled.dismissed")} />;
    const countLabel = t("question.settled.question", { count: questions.length });
    return (
      <AgentActivityDisclosure
        icon="question"
        shell="line"
        open={settledOpen}
        onToggle={() => setSettledOpen((open) => !open)}
        label={
          <span {...stylex.props(qc.settledLine)}>
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
    composingRef.current = true;
    compositionCommitPendingRef.current = false;
  };

  const handleAnswerChange = (event: ChangeEvent<HTMLTextAreaElement | HTMLInputElement>) => {
    if (!activeQuestion) return;
    const value = event.currentTarget.value;
    const nativeComposing = (event.nativeEvent as { isComposing?: boolean }).isComposing === true;
    if (composingRef.current && !nativeComposing) {
      composingRef.current = false;
      compositionCommitPendingRef.current = true;
    }
    setDraft((previous) => setQuestionText(previous, activeIndex, activeQuestion, value));
  };

  const handleCompositionEnd = (
    event: CompositionEvent<HTMLTextAreaElement | HTMLInputElement>,
  ) => {
    composingRef.current = false;
    compositionCommitPendingRef.current = true;
    if (!activeQuestion) return;
    const value = event.currentTarget.value;
    setDraft((previous) => setQuestionText(previous, activeIndex, activeQuestion, value));
  };

  const handleAnswerKeyDown = (event: KeyboardEvent<HTMLTextAreaElement | HTMLInputElement>) => {
    const compositionIntent = composerCompositionKeyIntent(
      event.nativeEvent,
      composingRef.current,
      compositionCommitPendingRef.current,
    );
    compositionCommitPendingRef.current = false;
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
      // Focused programmatically when a question arrives, so the prompt is read and the next
      // Tab reaches the first option — the same reason a menu popup takes focus. NOT a tab
      // stop: as one it was a stop that showed nothing, since a card opting out of the ring
      // has no row state to stand in for it.
      tabIndex={-1}
      data-slot="question-request-surface"
      data-chrome-focus
      className={stylex.props(ms.cardClip).className}
    >
      <div {...stylex.props(ms.cardHeadTight)}>
        <h3 id={promptId} className={stylex.props(ms.cardPromptFlush, typeStep.uiMd).className}>
          {activeQuestion.prompt}
        </h3>
        {questions.length > 1 && (
          <div {...stylex.props(qc.pager, typeStep.uiXs)}>
            <IconButton
              icon="chevron-left"
              size="xs"
              quiet
              disabled={activeIndex === 0}
              title={t("question.action.previous")}
              onClick={() => navigateQuestion(activeIndex - 1)}
            />
            <span {...stylex.props(qc.pageCount)}>
              {t("question.progress", { current: activeIndex + 1, total: questions.length })}
            </span>
            <IconButton
              icon="chevron-right"
              size="xs"
              quiet
              disabled={isLastQuestion}
              title={t("question.action.next")}
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
            disabled={!runtimeAvailable || actions.pending}
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
                  disabled={!runtimeAvailable || actions.pending}
                  onReselect={() => selectOptions(activeQuestion, [option.label])}
                >
                  <span {...stylex.props(qc.optionLine)}>
                    <span {...stylex.props(qc.optionLabel, typeStep.uiMd)}>{label}</span>
                    {recommended && <Badge>{t("question.recommended")}</Badge>}
                    {option.description && (
                      <span
                        title={option.description}
                        className={
                          stylex.props(
                            vocab.fill,
                            vocab.truncate,
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
              font="sans"
              size="sm"
              rows={4}
              value={activeDraft.text}
              aria-label={activeQuestion.prompt}
              placeholder={t("question.freetext.placeholder")}
              disabled={!runtimeAvailable || actions.pending}
              onChange={handleAnswerChange}
              onCompositionStart={handleCompositionStart}
              onCompositionEnd={handleCompositionEnd}
              onKeyDown={handleAnswerKeyDown}
              onBlur={() => {
                compositionCommitPendingRef.current = false;
              }}
              className={stylex.props(qc.textArea).className}
            />
          </div>
        )}

        {activeQuestion.type === "choice" && activeQuestion.allowCustom && (
          <div {...stylex.props(qc.freeRow, corner.pill)}>
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
              font="sans"
              value={activeDraft.text}
              aria-label={activeQuestion.prompt}
              placeholder={t("question.freetext.placeholder")}
              disabled={!runtimeAvailable || actions.pending}
              onChange={handleAnswerChange}
              onCompositionStart={handleCompositionStart}
              onCompositionEnd={handleCompositionEnd}
              onKeyDown={handleAnswerKeyDown}
              onBlur={() => {
                compositionCommitPendingRef.current = false;
              }}
              className={stylex.props(qc.freeField, typeStep.uiMd).className}
            />
          </div>
        )}

        <div {...stylex.props(qc.footer)}>
          <Button
            variant={actionSkips ? "outline" : "primary"}
            size="sm"
            disabled={actions.disabled || !runtimeAvailable}
            onClick={actionSkips ? skipCurrent : advanceCurrent}
          >
            {t(actionSkips ? "question.action.skip" : "question.action.advance")}
          </Button>
        </div>
      </div>
    </Surface>
  );
}
