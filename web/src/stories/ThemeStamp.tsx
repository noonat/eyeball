import { useLayoutEffect, type ReactNode } from "react";

export type Theme = "light" | "dark";

interface ThemeStampProps {
  readonly theme: Theme;
  readonly children: ReactNode;
}

/** Stamps data-theme on <html>, which is where the real surface stamps it. */
export function ThemeStamp({ theme, children }: ThemeStampProps) {
  useLayoutEffect(() => {
    document.documentElement.dataset["theme"] = theme;
  }, [theme]);
  return children;
}
