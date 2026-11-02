import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { draws } from "../app/routes";

const meta = {
  title: "Screen/Routes",
  parameters: { frame: "screen", sheet: false },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * Which addresses the app draws. A link and a notification's tap both ask
 * `draws`, and the server answers the shell for the same list, which a Go test
 * holds it to.
 */
export const Addresses: Story = {
  render: () => <p>Which addresses the app draws.</p>,
  play: async () => {
    const drawn = [
      "/",
      "/reviews/4",
      "/reviews/4/rounds/2",
      "/projects/3",
      "/projects/3/files/internal/a.go?copy=2",
      "/reviews/4/files/",
      "/reviews/4/files/docs/a%20b%23c.md?view=source",
      "/reviews/4/rounds/2/files/a.txt?against=base",
    ];
    for (const address of drawn) {
      await expect(draws(address)).toBe(true);
    }
    const loaded = [
      "/reviews/4/raw/1/a.png",
      "/api/",
      "/api/projects/3",
      "/events",
      "/static/app.css",
      "https://example.com/projects/3",
    ];
    for (const address of loaded) {
      await expect(draws(address)).toBe(false);
    }
  },
};
