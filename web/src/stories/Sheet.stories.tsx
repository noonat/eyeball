import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { all } from "./catalog";
import { sections } from "./sections";
import {
  Sheet,
  entries,
  missing,
  specimenName,
  specimenTitle,
  storyHref,
  unplaced,
} from "./Sheet";
import * as specimens from "./Specimens.stories";
import { storyNameFromExport, toId } from "storybook/internal/csf";

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

    // A Full screen button per section and theme, each opening a story that
    // exists: a section whose story is missing fails here, not on the phone.
    const links = canvas.getAllByRole("link", { name: /full screen$/ });
    await expect(links).toHaveLength(sections.length * 2);
    // Storybook's indexer needs a literal title, so the two are checked here.
    await expect(specimens.default.title).toBe(specimenTitle);
    for (const section of sections) {
      const name = specimenName(section.id);
      await expect(Object.keys(specimens)).toContain(name);
      const id = toId(specimenTitle, storyNameFromExport(name));
      for (const theme of ["light", "dark"] as const) {
        const link = canvas.getByRole("link", {
          name: `${section.title}, ${theme}, full screen`,
        });
        await expect(link.getAttribute("href")).toBe(storyHref(id, theme));
      }
    }

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
