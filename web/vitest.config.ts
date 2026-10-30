import { fileURLToPath } from "node:url";
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config.ts";

const configDir = fileURLToPath(new URL(".storybook", import.meta.url));

// Chromium is the system's, never one Playwright downloaded. A machine that
// keeps it elsewhere sets EYEBALL_CHROMIUM.
const executablePath = process.env["EYEBALL_CHROMIUM"] ?? "/usr/bin/chromium";

// The stories are the only tests, so there is one project. A story's own play
// function is its interaction test, and the accessibility add-on audits each.
export default defineConfig({
  test: {
    projects: [
      mergeConfig(viteConfig, {
        plugins: [storybookTest({ configDir })],
        test: {
          name: "storybook",
          browser: {
            enabled: true,
            headless: true,
            provider: playwright({ launchOptions: { executablePath } }),
            instances: [{ browser: "chromium" as const }],
          },
        },
      }),
    ],
  },
});
