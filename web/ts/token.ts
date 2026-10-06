import { setHeader, verbOf } from "./events";

// The loopback listener's token, handed over in the printed address's fragment
// and kept per origin. A browser sends a fragment to no server and puts it in
// no Referer, so the token never reaches a request line or a log.
const key = "eyeball:token";
const header = "X-Eyeball-Token";

// takeToken stores a token the address carries, then removes the fragment from
// the address bar and the history. Only the token= form is read, so a heading's
// fragment on a file page is left alone.
export function takeToken(): void {
  const match = /^#token=([A-Z2-7]+)$/.exec(window.location.hash);
  if (match?.[1] === undefined) {
    return;
  }
  try {
    window.localStorage.setItem(key, match[1]);
  } catch {
    // Storage that throws leaves the page read-only on loopback, which the
    // refusal on its first write explains.
  }
  const { pathname, search } = window.location;
  window.history.replaceState(null, "", pathname + search);
}

// tokenHeaders is the header a write made outside htmx carries, empty where
// no token is stored.
export function tokenHeaders(): Record<string, string> {
  try {
    const token = window.localStorage.getItem(key);
    return token === null ? {} : { [header]: token };
  } catch {
    return {};
  }
}

// sendToken adds the stored token to every write. The tailnet's origin never
// stores one, so nothing is added there.
export function sendToken(): void {
  document.addEventListener("htmx:configRequest", (event) => {
    if (verbOf(event) === "get") {
      return;
    }
    let token: string | null = null;
    try {
      token = window.localStorage.getItem(key);
    } catch {
      return;
    }
    if (token !== null) {
      setHeader(event, header, token);
    }
  });
}
