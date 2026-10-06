// What more than one part of the page needs to ask: whether the connection is
// down, and whether anything typed is not yet stored.

let offline = false;

export function isOffline(): boolean {
  return offline;
}

// setOffline records the connection's state and tells the page, so controls
// that need the daemon can say why they are unavailable.
export function setOffline(down: boolean): void {
  if (offline === down) {
    return;
  }
  offline = down;
  document.dispatchEvent(new Event("eyeball:connection"));
}

// unsavedText says whether any composer or edit form holds text not yet
// stored, and with note set, whether the verdict's note does too. The note is
// not a comment, so it holds back a reload but not a verdict.
export function unsavedText(note: boolean): boolean {
  // An unsent card holds text the store does not, so a reload waits for it
  // too, until drafts are kept across one.
  if (note && document.querySelector(".comment.unsent") !== null) {
    return true;
  }
  const fields = note
    ? "form.composer textarea, #bar textarea"
    : "form.composer textarea";
  for (const field of document.querySelectorAll<HTMLTextAreaElement>(fields)) {
    if (field.value.trim() !== "") {
      return true;
    }
  }
  return false;
}
