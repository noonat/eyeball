import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.tsx"],
  addons: ["@storybook/addon-a11y", "@storybook/addon-vitest"],
  framework: "@storybook/react-vite",
  // The files the fixture views point at, served at the addresses the views
  // hold, so a frame, an image or a player on a sheet shows real content.
  staticDirs: ["../src/fixtures/_raw"],
};

export default config;
