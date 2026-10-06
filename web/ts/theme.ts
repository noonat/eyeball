// app.css switches on data-theme alone, so the device's preference has to be
// copied onto the root element, and copied again when it changes.
export function followTheme(): void {
  const dark = window.matchMedia("(prefers-color-scheme: dark)");
  const apply = (): void => {
    document.documentElement.dataset["theme"] = dark.matches ? "dark" : "light";
  };
  apply();
  dark.addEventListener("change", apply);
}
