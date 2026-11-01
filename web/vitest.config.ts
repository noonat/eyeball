import { fileURLToPath } from "node:url";
import { storybookTest } from "@storybook/addon-vitest/vitest-plugin";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config.ts";

const configDir = fileURLToPath(new URL(".storybook", import.meta.url));

// Chromium is the system's, never one Playwright downloaded. A machine that
// keeps it elsewhere sets EYEBALL_CHROMIUM.
const executablePath = process.env["EYEBALL_CHROMIUM"] ?? "/usr/bin/chromium";

// A function, since Vitest names each instance after its project and a shared
// object would carry the first project's name into the second.
function browser() {
  return {
    enabled: true,
    headless: true,
    provider: playwright({ launchOptions: { executablePath } }),
    instances: [{ browser: "chromium" as const }],
  };
}

// A story's own play function is its interaction test, and the accessibility
// add-on audits each. The second project holds the checks that have no screen:
// they run in the same Chromium, since they read markup with its DOM parser.
export default defineConfig({
  test: {
    projects: [
      mergeConfig(viteConfig, {
        plugins: [storybookTest({ configDir })],
        test: { name: "storybook", browser: browser() },
      }),
      mergeConfig(viteConfig, {
        test: {
          name: "checks",
          include: ["src/**/*.test.ts"],
          browser: browser(),
        },
      }),
    ],
  },
});
