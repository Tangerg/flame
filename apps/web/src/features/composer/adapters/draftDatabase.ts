import { z } from "zod";
import { type ComposerDraftText, type ComposerDraft } from "@/features/composer/domain/composer";

const DATABASE = "flame.composer";
const DRAFT_STORE = "drafts";
const PASTE_STORE = "pastes";
const STORES = [DRAFT_STORE, PASTE_STORE];
const pasteReference = z.object({ id: z.string().min(1), record: z.string().min(1) }).strict();
const draftRecord = z
  .object({
    endpoint: z.string(),
    session: z.string(),
    value: z.string(),
    pastes: z
      .array(pasteReference)
      .refine((pastes) => new Set(pastes.map((paste) => paste.id)).size === pastes.length),
  })
  .strict();
const pasteRecord = z
  .object({
    record: z.string().min(1),
    endpoint: z.string(),
    session: z.string(),
    id: z.string().min(1),
    text: z.string(),
  })
  .strict();
type StoredPaste = z.infer<typeof pasteRecord>;

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

function completion(transaction: IDBTransaction, failure: { cause?: unknown }): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve();
    transaction.onerror = () => {
      failure.cause ??= transaction.error;
    };
    transaction.onabort = () =>
      reject(
        failure.cause ??
          transaction.error ??
          new DOMException("draft transaction aborted", "AbortError"),
      );
  });
}

/** Only transaction completion confirms a revision. Request success can still be rolled back. */
export class DraftDatabase {
  private connection: Promise<IDBDatabase> | null = null;
  private closed = false;
  private readonly pastes = new Map<string, Map<string, readonly StoredPaste[]>>();

  private open(): Promise<IDBDatabase> {
    if (this.closed) return Promise.reject(new Error("draft database is closed"));
    if (!this.connection) {
      this.connection = new Promise<IDBDatabase>((resolve, reject) => {
        const request = indexedDB.open(DATABASE, 1);
        let refused = false;
        request.onupgradeneeded = () => {
          const db = request.result;
          db.createObjectStore(DRAFT_STORE, { keyPath: ["endpoint", "session"] }).createIndex(
            "endpoint",
            "endpoint",
          );
          db.createObjectStore(PASTE_STORE, { keyPath: "record" }).createIndex(
            "endpoint",
            "endpoint",
          );
        };
        request.onblocked = () => {
          refused = true;
          reject(new Error("draft storage is blocked by another window"));
        };
        request.onerror = () => reject(request.error);
        request.onsuccess = () => {
          const db = request.result;
          if (refused || this.closed) {
            db.close();
            reject(new Error("draft database is closed"));
            return;
          }
          db.onversionchange = () => {
            db.close();
            this.connection = null;
          };
          resolve(db);
        };
      }).catch((cause: unknown) => {
        this.connection = null;
        throw cause;
      });
    }
    return this.connection;
  }

  async read(endpoint: string): Promise<ReadonlyMap<string, ComposerDraftText>> {
    const db = await this.open();
    const transaction = db.transaction(STORES, "readonly");
    const done = completion(transaction, {});
    const [drafts, pastes] = await Promise.all([
      requestResult<unknown[]>(
        transaction.objectStore(DRAFT_STORE).index("endpoint").getAll(endpoint),
      ),
      requestResult<unknown[]>(
        transaction.objectStore(PASTE_STORE).index("endpoint").getAll(endpoint),
      ),
      done,
    ]);
    const records = new Map(
      pastes.map((value) => {
        const record = pasteRecord.parse(value);
        return [record.record, record];
      }),
    );
    const texts = new Map<string, ComposerDraftText>();
    const retained = new Map<string, readonly StoredPaste[]>();
    for (const value of drafts) {
      const draft = draftRecord.parse(value);
      const attached = draft.pastes.map((reference) => {
        const paste = records.get(reference.record);
        if (
          !paste ||
          paste.endpoint !== endpoint ||
          paste.session !== draft.session ||
          paste.id !== reference.id
        )
          throw new Error("saved draft has a missing or mismatched pasted text record");
        return paste;
      });
      retained.set(draft.session, attached);
      texts.set(draft.session, {
        value: draft.value,
        pastes: attached.map(({ id, text }) => ({ id, text })),
      });
    }
    this.pastes.set(endpoint, retained);
    return texts;
  }

  async write(endpoint: string, drafts: ReadonlyMap<string, ComposerDraft>): Promise<void> {
    const db = await this.open();
    const prepared = [...drafts].map(([session, draft]) => {
      const previous = this.pastes.get(endpoint)?.get(session) ?? [];
      const pastes = draft.pastes.map(
        ({ id, text }) =>
          previous.find((paste) => paste.id === id && paste.text === text) ?? {
            record: crypto.randomUUID(),
            endpoint,
            session,
            id,
            text,
          },
      );
      return {
        session,
        value: draft.value,
        pastes,
        added: pastes.filter((paste) => !previous.includes(paste)),
      };
    });
    const transaction = db.transaction(STORES, "readwrite");
    const failure: { cause?: unknown } = {};
    const done = completion(transaction, failure);
    const draftStore = transaction.objectStore(DRAFT_STORE);
    const pasteStore = transaction.objectStore(PASTE_STORE);
    const abort = (cause: unknown) => {
      failure.cause = cause;
      transaction.abort();
    };
    for (const { session, value, pastes, added } of prepared) {
      const request = draftStore.get([endpoint, session]);
      request.onsuccess = () => {
        try {
          const previous =
            request.result === undefined ? undefined : draftRecord.parse(request.result);
          for (let index = 0; index < pastes.length; index++) {
            const paste = pastes[index]!;
            if (
              !added.includes(paste) &&
              !previous?.pastes.some((reference) => reference.record === paste.record)
            ) {
              const fresh = { ...paste, record: crypto.randomUUID() };
              pastes[index] = fresh;
              added.push(fresh);
            }
          }
          for (const reference of previous?.pastes ?? []) {
            if (!pastes.some((paste) => paste.record === reference.record))
              pasteStore.delete(reference.record);
          }
          for (const paste of added) pasteStore.add(paste);
          if (value || pastes.length)
            draftStore.put({
              endpoint,
              session,
              value,
              pastes: pastes.map(({ id, record }) => ({ id, record })),
            });
          else draftStore.delete([endpoint, session]);
        } catch (cause) {
          abort(cause);
        }
      };
    }
    await done;
    let retained = this.pastes.get(endpoint);
    if (!retained) {
      retained = new Map();
      this.pastes.set(endpoint, retained);
    }
    for (const { session, pastes } of prepared) retained.set(session, pastes);
  }

  close(): void {
    this.closed = true;
    void this.connection?.then(
      (db) => db.close(),
      () => undefined,
    );
    this.pastes.clear();
  }
}
