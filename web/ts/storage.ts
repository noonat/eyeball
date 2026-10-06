// The page's per-device memory: where a reader was and what they had not yet
// sent. Every call is wrapped, so a page whose storage throws, as a private
// window's can, still works and only forgets.

const prefix = "eyeball:";
const tokenKey = "eyeball:token";
// An entry older than this is dropped on load. Thirty days is a guess.
const maxAge = 30 * 24 * 60 * 60 * 1000;

interface Entry {
  readonly at: number;
  readonly value: unknown;
}

function isEntry(v: unknown): v is Entry {
  return (
    typeof v === "object" &&
    v !== null &&
    "at" in v &&
    typeof v.at === "number" &&
    "value" in v
  );
}

export function remember(key: string, value: unknown): void {
  try {
    window.localStorage.setItem(
      prefix + key,
      JSON.stringify({ at: Date.now(), value }),
    );
  } catch {
    // Storage that refuses a write forgets this value, and nothing else breaks.
  }
}

export function recall(key: string): unknown {
  try {
    const raw = window.localStorage.getItem(prefix + key);
    if (raw === null) {
      return null;
    }
    const entry: unknown = JSON.parse(raw);
    return isEntry(entry) ? entry.value : null;
  } catch {
    return null;
  }
}

export function forget(key: string): void {
  try {
    window.localStorage.removeItem(prefix + key);
  } catch {
    // Nothing to do: a value that cannot be removed cannot be read either.
  }
}

// prune drops entries older than maxAge. The token is valid for as long as its
// run of serve lasts, however old, and is not an entry.
export function prune(): void {
  try {
    const now = Date.now();
    for (let i = window.localStorage.length - 1; i >= 0; i--) {
      const key = window.localStorage.key(i);
      if (key === null || !key.startsWith(prefix) || key === tokenKey) {
        continue;
      }
      const raw = window.localStorage.getItem(key);
      const entry: unknown = raw === null ? null : JSON.parse(raw);
      if (!isEntry(entry) || now - entry.at > maxAge) {
        window.localStorage.removeItem(key);
      }
    }
  } catch {
    // A store that cannot be read cannot be pruned either.
  }
}
