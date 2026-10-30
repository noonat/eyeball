import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Sheet, entries, storyHref } from "./Sheet";

// Every story the components declare, so a new state joins the sheet without
// anyone remembering to list it.
const modules = import.meta.glob<Record<string, unknown>>(
  ["./*.stories.tsx", "!./Sheet.stories.tsx", "../components/*.stories.tsx"],
  { eager: true },
);
const all = entries(modules);

const meta = {
  title: "Foundations/Sheet",
  parameters: { frame: "screen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** All the states, for judging on the phone. */
export const States: Story = {
  render: () => <Sheet entries={all} />,
  play: async ({ canvasElement }) => {
    const links = within(canvasElement).getAllByRole("link", {
      name: "Full screen",
    });
    // Two links for each state, one per theme.
    await expect(links).toHaveLength(all.length * 2);
    // The production build adds __namedExportsOrder to every story module, and
    // dev mode does not, so a module carrying it is built here.
    const fabricated = entries({
      "./X.stories.tsx": {
        default: { title: "Foundations/X" },
        Only: { render: () => <p>only</p> },
        __namedExportsOrder: ["Only"],
      },
    });
    await expect(fabricated).toHaveLength(1);
    const first = all[0];
    await expect(first).toBeDefined();
    if (first !== undefined) {
      await expect(links[0]?.getAttribute("href")).toBe(
        storyHref(first.id, "light"),
      );
    }
  },
};
