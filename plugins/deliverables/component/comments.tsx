import { useState } from "react";
import type { Translate } from "@flame/component-sdk";
import {
  Button,
  Disclosure,
  FormField,
  Metadata,
  RecordBody,
  RecordGroup,
  RecordSummary,
  Segmented,
  TextArea,
  Toolbar,
} from "@flame/ui";
import { METHODS } from "../contracts/methods";
import { formatTimestamp } from "./archive";
import { selectionPreamble, type LiveState, type useHandoff } from "./live";
import type { DocumentRead, Thread } from "./types";

export type Anchor = Thread["anchor"];

// What a new comment is anchored to: the editor's selected object and its
// text now, or the whole document.
export type CommentTarget = { object: string; quote: string; label: string };

export function quoteOf(text: string): string {
  return text.length > 500 ? `${text.slice(0, 499)}…` : text;
}

function Reference({
  thread,
  label,
  document,
  handoff,
  t,
}: {
  thread: Thread;
  label: string;
  document: DocumentRead;
  handoff: ReturnType<typeof useHandoff>;
  t: Translate;
}) {
  return (
    <Button
      onClick={() =>
        handoff.hand({
          label: `${t("comments.title")} · ${label}`,
          text: [
            ...selectionPreamble(document),
            `Comment thread: ${thread.id}`,
            `Anchored to revision ${thread.anchor.revision}${thread.anchor.object ? `, object ${thread.anchor.object}` : ""}`,
            ...(thread.anchor.quote
              ? [`Quoted then: ${JSON.stringify(thread.anchor.quote)}`]
              : []),
            `Comment: ${thread.messages[0]!.body}`,
            "Read the thread with the Deliverables threads tool, make the change it asks for, and answer with reply naming the revision that addresses it.",
          ].join("\n"),
        })
      }
    >
      {handoff.label}
    </Button>
  );
}

function ThreadEntry({
  state,
  document,
  thread,
  label,
  onLocate,
  handoff,
  dateTime,
  t,
}: {
  state: LiveState;
  document: DocumentRead;
  thread: Thread;
  label: string;
  onLocate?: () => void;
  handoff: ReturnType<typeof useHandoff>;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
}) {
  const [reply, setReply] = useState("");
  const target = { id: thread.documentId, thread: thread.id };
  const first = thread.messages[0]!;
  return (
    <Disclosure
      variant="row"
      title={
        <RecordSummary
          title={label}
          description={first.body}
          status={
            thread.status === "resolved" && (
              <Metadata>{t("comments.resolved")}</Metadata>
            )
          }
        />
      }
    >
      {thread.anchor.quote && (
        <Metadata>
          {t("comments.quote")} {thread.anchor.quote}
        </Metadata>
      )}
      {thread.messages.map((message) => (
        <RecordGroup key={message.id}>
          <Metadata>
            {t(`comments.author.${message.author.origin}`)} ·{" "}
            {formatTimestamp(message.createdAt, dateTime)}
          </Metadata>
          <RecordBody>{message.body}</RecordBody>
          {message.revision && (
            <Button
              onClick={() =>
                state.open({
                  id: thread.documentId,
                  revision: message.revision!,
                })
              }
            >
              {t("comments.revision", { id: message.revision.slice(0, 12) })}
            </Button>
          )}
        </RecordGroup>
      ))}
      <FormField label={t("comments.reply")}>
        <TextArea
          rows={2}
          value={reply}
          onChange={(event) => setReply(event.target.value)}
        />
      </FormField>
      <Toolbar label={t("comments.actions")}>
        <Button
          variant="primary"
          disabled={state.blocked || !reply.trim()}
          onClick={() =>
            void state
              .run(METHODS.reply, { ...target, body: reply.trim() })
              .then((value) => {
                if (value) setReply("");
              })
          }
        >
          {t("comments.reply")}
        </Button>
        <Button
          disabled={state.blocked}
          onClick={() =>
            void state.run(
              thread.status === "open" ? METHODS.resolve : METHODS.reopen,
              target,
            )
          }
        >
          {t(thread.status === "open" ? "comments.resolve" : "comments.reopen")}
        </Button>
        {onLocate && <Button onClick={onLocate}>{t("comments.locate")}</Button>}
        <Reference
          thread={thread}
          label={label}
          document={document}
          handoff={handoff}
          t={t}
        />
      </Toolbar>
    </Disclosure>
  );
}

// Comments anchor to the revision the editor shows. A thread whose object is
// not in that revision keeps its original quote and says so.
export function Comments({
  state,
  document,
  selection,
  describe,
  onLocate,
  handoff,
  dateTime,
  t,
}: {
  state: LiveState;
  document: DocumentRead;
  selection?: CommentTarget;
  // The anchored object as the shown revision names it, or nothing when that
  // revision no longer has it.
  describe(anchor: Anchor): string | undefined;
  onLocate(object: string): void;
  handoff: ReturnType<typeof useHandoff>;
  dateTime: Intl.DateTimeFormat;
  t: Translate;
}) {
  const [body, setBody] = useState("");
  const [all, setAll] = useState(false);
  const threads = state.threads?.value?.entries ?? [];
  const open = threads.filter((thread) => thread.status === "open").length;
  const shown = all
    ? threads
    : threads.filter((thread) => thread.status === "open");
  const add = () => {
    const captured = body.trim();
    void state
      .run(METHODS.comment, {
        id: document.document.id,
        revision: document.revision.id,
        ...(selection
          ? { object: selection.object, quote: quoteOf(selection.quote) }
          : {}),
        body: captured,
      })
      .then((value) => {
        if (value) setBody("");
      });
  };
  return (
    <Disclosure
      title={
        open ? t("comments.titleOpen", { count: open }) : t("comments.title")
      }
    >
      <FormField
        label={
          selection
            ? t("comments.on", { target: selection.label })
            : t("comments.onDocument")
        }
      >
        <TextArea
          rows={2}
          value={body}
          onChange={(event) => setBody(event.target.value)}
        />
      </FormField>
      <Button disabled={state.blocked || !body.trim()} onClick={add}>
        {t("comments.add")}
      </Button>
      {threads.length > 0 && (
        <Segmented
          ariaLabel={t("comments.filter")}
          value={all ? "all" : "open"}
          options={[
            { value: "open", label: t("comments.open") },
            { value: "all", label: t("comments.all") },
          ]}
          onChange={(value) => setAll(value === "all")}
        />
      )}
      {shown.length ? (
        <RecordGroup variant="list" aria-label={t("comments.title")}>
          {shown.map((thread) => {
            const object = thread.anchor.object;
            const named = object ? describe(thread.anchor) : undefined;
            const label = object
              ? (named ?? t("comments.gone"))
              : t("comments.document");
            return (
              <ThreadEntry
                key={thread.id}
                state={state}
                document={document}
                thread={thread}
                label={label}
                onLocate={named ? () => onLocate(object!) : undefined}
                handoff={handoff}
                dateTime={dateTime}
                t={t}
              />
            );
          })}
        </RecordGroup>
      ) : (
        <Metadata>
          {t(threads.length ? "comments.noneOpen" : "comments.none")}
        </Metadata>
      )}
    </Disclosure>
  );
}
