import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const repoRoot = fileURLToPath(new URL("..", import.meta.url));

// Storybook and Vitest both read this file, so one list of pre-bundled
// packages serves the dev server, the build and the tests.
export default defineConfig({
  plugins: [react()],
  // The Vitest add-on's setup file imports React, and without these the dev
  // server finds it late and fails with "does not provide an export named
  // 'default'".
  optimizeDeps: {
    include: [
      "react",
      "react-dom",
      "react-dom/client",
      "react/jsx-runtime",
      "react/jsx-dev-runtime",
    ],
  },
  // app.css and its fonts live in docs/design, above this directory.
  server: { fs: { allow: [repoRoot] } },
});
