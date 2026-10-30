// pageBuild is the build that served this page, from the head every page and the
// React shell write.
export function pageBuild(): string {
  return (
    document.querySelector<HTMLMetaElement>("meta[name='eyeball-build']")
      ?.content ?? ""
  );
}

// newerBuild says whether the daemon runs a build other than this page's. A
// page with no build of its own offers nothing, since it cannot tell.
export function newerBuild(mine: string, newest: string): boolean {
  return mine !== "" && newest !== "" && newest !== mine;
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
