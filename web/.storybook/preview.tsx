import type { Preview } from "@storybook/react-vite";
import "../../docs/design/app.css";
import "../src/stories/stories.css";
import { Framed, type Frame } from "../src/stories/Framed";
import { ThemeStamp, type Theme } from "../src/stories/ThemeStamp";

function isTheme(value: unknown): value is Theme {
  return value === "light" || value === "dark";
}

const preview: Preview = {
  initialGlobals: { theme: "light" },
  globalTypes: {
    theme: {
      description: "The surface's theme, stamped as data-theme on <html>",
      toolbar: {
        title: "Theme",
        icon: "circlehollow",
        items: ["light", "dark"],
        dynamicTitle: true,
      },
    },
  },
  parameters: {
    layout: "fullscreen",
    // Every story is also a test, and an accessibility violation fails it.
    a11y: { test: "error" },
  },
  decorators: [
    (Story, context) => {
      // A story that pins a theme wins over the toolbar.
      const pinned: unknown = context.parameters["theme"];
      const chosen: unknown = context.globals["theme"];
      const theme = [pinned, chosen].find(isTheme) ?? "light";
      const frame: Frame =
        context.parameters["frame"] === "screen" ? "screen" : "page";
      return (
        <ThemeStamp theme={theme}>
          <Framed frame={frame}>
            <Story />
          </Framed>
        </ThemeStamp>
      );
    },
  ],
};

export default preview;
