import { z } from "zod";
import { type ComposerDraftText, type ComposerDraft } from "@/features/composer/domain/composer";

const DATABASE = "flame.composer";
const STORES = ["drafts", "pastes", "endpoints"];
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
const endpointRecord = z.object({ endpoint: z.string(), imported: z.boolean() }).strict();
type StoredPaste = z.infer<typeof pasteRecord>;

export interface DraftRead {
  drafts: ReadonlyMap<string, ComposerDraftText>;
  initialized: boolean;
  imported: boolean;
}

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
          db.createObjectStore("drafts", { keyPath: ["endpoint", "session"] }).createIndex(
            "endpoint",
            "endpoint",
          );
          db.createObjectStore("pastes", { keyPath: "record" }).createIndex("endpoint", "endpoint");
          db.createObjectStore("endpoints", { keyPath: "endpoint" });
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

  async read(endpoint: string): Promise<DraftRead> {
    const db = await this.open();
    const transaction = db.transaction(STORES, "readonly");
    const done = completion(transaction, {});
    const values = await Promise.all([
      requestResult<unknown[]>(
        transaction.objectStore("drafts").index("endpoint").getAll(endpoint),
      ),
      requestResult<unknown[]>(
        transaction.objectStore("pastes").index("endpoint").getAll(endpoint),
      ),
      requestResult<unknown>(transaction.objectStore("endpoints").get(endpoint)),
      done,
    ]);
    const [drafts, pastes, marker] = values;
    const metadata = marker === undefined ? undefined : endpointRecord.parse(marker);
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
    return {
      drafts: texts,
      initialized: metadata !== undefined || drafts.length > 0,
      imported: metadata?.imported ?? false,
    };
  }

  async write(
    endpoint: string,
    drafts: ReadonlyMap<string, ComposerDraft>,
    importing?: () => boolean,
  ): Promise<void> {
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
    const draftStore = transaction.objectStore("drafts");
    const pasteStore = transaction.objectStore("pastes");
    const metadataStore = transaction.objectStore("endpoints");
    const abort = (cause: unknown) => {
      failure.cause = cause;
      transaction.abort();
    };
    const write = () => {
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
      const marker = metadataStore.get(endpoint);
      marker.onsuccess = () => {
        try {
          const previous =
            marker.result === undefined ? undefined : endpointRecord.parse(marker.result);
          metadataStore.put({
            endpoint,
            imported: Boolean(importing) || previous?.imported === true,
          });
        } catch (cause) {
          abort(cause);
        }
      };
    };
    if (importing) {
      const marker = metadataStore.get(endpoint);
      const count = draftStore.index("endpoint").count(endpoint);
      let ready = 0;
      const check = () => {
        if (++ready !== 2) return;
        if (marker.result !== undefined || count.result !== 0 || !importing()) {
          abort(
            new Error(
              "existing drafts can only be imported into fresh draft storage before editing",
            ),
          );
          return;
        }
        write();
      };
      marker.onsuccess = check;
      count.onsuccess = check;
    } else write();
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
