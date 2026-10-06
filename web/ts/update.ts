// followBuild shows the update banner when the stream reports a build other
// than the one that served this page, and reloads on Refresh. It takes the
// stream's EventSource, since the build arrives on it.
export function followBuild(source: EventSource): void {
  const mine =
    document.querySelector<HTMLMetaElement>("meta[name='eyeball-build']")
      ?.content ?? "";
  source.addEventListener("build", (event) => {
    const holder = document.getElementById("update");
    if (
      holder &&
      event instanceof MessageEvent &&
      typeof event.data === "string"
    ) {
      holder.hidden = mine === "" || event.data === mine;
    }
  });
}

// refreshOnTap reloads the page from the banner's Refresh. Drafts are kept
// across a reload, so nothing typed is lost.
export function refreshOnTap(): void {
  document.addEventListener("click", (event) => {
    if (
      event.target instanceof Element &&
      event.target.closest("#update .refresh")
    ) {
      window.location.reload();
    }
  });
}
