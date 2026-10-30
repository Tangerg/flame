export interface Preferences {
  completionSound: boolean;
  systemNotifications: boolean;
}

const records = {
  completionSound: { key: "flame.completion-sound", initial: false },
  systemNotifications: { key: "flame.system-notifications", initial: true },
} as const;

function read(name: keyof Preferences): boolean {
  const record = records[name];
  const text = localStorage.getItem(record.key);
  if (text === null) return record.initial;
  let stored: unknown;
  try {
    stored = JSON.parse(text);
  } catch (error) {
    if (!(error instanceof SyntaxError)) throw error;
    console.warn(`[notifications] invalid ${record.key} preference`);
    return record.initial;
  }
  if (
    typeof stored === "object" &&
    stored !== null &&
    "version" in stored &&
    stored.version === 0 &&
    "state" in stored &&
    typeof stored.state === "object" &&
    stored.state !== null &&
    name in stored.state
  ) {
    const value = (stored.state as Record<string, unknown>)[name];
    if (typeof value === "boolean") return value;
  }
  console.warn(`[notifications] invalid ${record.key} preference`);
  return record.initial;
}

export function readPreferences(): Preferences {
  return {
    completionSound: read("completionSound"),
    systemNotifications: read("systemNotifications"),
  };
}

export function writePreference(name: keyof Preferences, value: boolean): void {
  if (typeof value !== "boolean")
    throw new Error("Notification preferences require boolean values.");
  localStorage.setItem(
    records[name].key,
    JSON.stringify({ state: { [name]: value }, version: 0 }),
  );
}
