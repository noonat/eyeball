import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Sheet, entries, missing, storyHref, unplaced } from "./Sheet";
import { sections } from "./sections";

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

/** Every primitive's states, together, for judging on the phone. */
export const States: Story = {
  render: () => <Sheet entries={all} sections={sections} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // A section that names a story that does not exist draws nothing for it,
    // and a story no section places never reaches the sheet.
    await expect(missing(all, sections)).toEqual([]);
    await expect(unplaced(all, sections)).toEqual([]);

    // A link per state and theme.
    const links = canvas.getAllByRole("link", { name: /full screen$/ });
    const shown = all.filter((entry) => entry.onSheet);
    await expect(links).toHaveLength(shown.length * 2);
    await expect(links[0]?.getAttribute("href")).toBe(
      storyHref("foundations-palette--tokens", "light"),
    );

    // Each panel stamps its own theme, whatever the page around it is.
    for (const theme of ["light", "dark"]) {
      const slabs = canvasElement.querySelectorAll(
        `.slab[data-theme="${theme}"]`,
      );
      await expect(slabs.length).toBeGreaterThan(0);
    }
    await expect(canvasElement.querySelectorAll(".phone")).toHaveLength(0);

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
  },
};
