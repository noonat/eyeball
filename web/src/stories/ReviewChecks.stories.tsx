import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import type { FileBody, Rendering, Review } from "../api/types";
import { ReviewScreen } from "../components/ReviewScreen";
import { reviewApproved, reviewCode } from "../fixtures";

// States the committed template pages hold no copy of, so each asserts what it
// draws and none is compared with markup. They are not on the sheets: the sheets
// are the approved pages, and nothing here changes them.
const meta = {
  title: "Review/Checks",
  component: ReviewScreen,
  parameters: {
    frame: "screen",
    sheet: false,
    a11y: {
      config: {
        rules: [
          { id: "nested-interactive", selector: "summary", enabled: false },
        ],
      },
    },
  },
} satisfies Meta<typeof ReviewScreen>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The code review with every body replaced by a kind that has no rows. */
function withoutRows(kind: FileBody): Review {
  return {
    ...reviewCode,
    cards: (reviewCode.cards ?? []).map((card) => ({
      ...card,
      body: card.body === null ? null : { ...card.body, kind, rows: null },
    })),
  };
}

/** The lines a card shows in place of its rows, in the order of its cards. */
function lines(root: HTMLElement): readonly (string | null)[] {
  return [...root.querySelectorAll(".file .toobig")].map((n) => n.textContent);
}

/** A file past the diff limit shows one line, with no rows. */
export const TooLarge: Story = {
  args: { review: withoutRows("too_large") },
  play: async ({ canvasElement }) => {
    const shown = lines(canvasElement);
    await expect(shown.length).toBeGreaterThan(0);
    await expect(new Set(shown)).toEqual(
      new Set(["Too large to show as a diff."]),
    );
  },
};

/** A binary file shows one line, with no rows. */
export const Binary: Story = {
  args: { review: withoutRows("binary") },
  play: async ({ canvasElement }) => {
    const shown = lines(canvasElement);
    await expect(shown.length).toBeGreaterThan(0);
    await expect(new Set(shown)).toEqual(
      new Set(["Binary file, not shown as a diff."]),
    );
  },
};

/** A round decided with changes requested, which no approved page holds. */
export const ChangesRequested: Story = {
  args: {
    review: {
      ...reviewApproved,
      decided:
        reviewApproved.decided === null
          ? null
          : {
              ...reviewApproved.decided,
              approved: false,
              pill: "Changes requested 1h ago",
              sentence: "The verdict went out just now.",
            },
    },
  },
  play: async ({ canvasElement }) => {
    const notices = [...canvasElement.querySelectorAll(".notice")].map(
      (n) => n.textContent,
    );
    await expect(notices).toContain(
      "Changes requested. The verdict went out just now.",
    );
    await expect(
      canvasElement.querySelector(".pill.changes")?.textContent,
    ).toBe("Changes requested 1h ago");
  },
};

/** The code review with every body replaced by one rendered as `rendered`. */
function withRendered(rendered: Rendering): Review {
  return {
    ...reviewCode,
    cards: (reviewCode.cards ?? []).map((card) => ({
      ...card,
      body:
        card.body === null
          ? null
          : { ...card.body, kind: "rendered", rows: null, rendered },
    })),
  };
}

const noFrame = {
  sides: null,
  frame: "",
  sandboxed: false,
  title: "",
  kind: "",
  size: "",
  facts: null,
} satisfies Rendering;

/** An image or a player in a card sits in a box of its own. */
export const RenderedBox: Story = {
  args: {
    review: withRendered({
      ...noFrame,
      sides: [
        {
          label: "After",
          tint: "now",
          caption: "4 × 5 · 10 B",
          src: "/reviews/1/raw/1/icon.png",
          element: "img",
        },
      ],
    }),
  },
  play: async ({ canvasElement }) => {
    const boxes = canvasElement.querySelectorAll(
      ".file > .rendered > .beforeafter",
    );
    await expect(boxes.length).toBeGreaterThan(0);
  },
};

/**
 * A file the surface does not show, such as a binary from git, is one line in
 * its card naming it, with its size and a link to load it anyway.
 */
export const RenderedFacts: Story = {
  args: {
    review: withRendered({
      ...noFrame,
      facts: {
        kind: "Binary file",
        size: "8 B",
        hint: "Not shown as text.",
        load: "/reviews/1/files/a.bin",
      },
    }),
  },
  play: async ({ canvasElement }) => {
    const facts = canvasElement.querySelectorAll(".file > .filefacts");
    await expect(facts.length).toBeGreaterThan(0);
    const line = facts[0];
    await expect(line?.querySelector(".kind")?.textContent).toBe("Binary file");
    await expect(line?.querySelector(".size")?.textContent).toBe("8 B");
    await expect(line?.querySelector(".hint")?.textContent).toBe(
      "Not shown as text.",
    );
    const load = line?.querySelector("a.load");
    await expect(load?.textContent).toBe("Load anyway");
    await expect(load?.getAttribute("href")).toBe("/reviews/1/files/a.bin");
  },
};

/** A page an agent wrote is framed under the sandbox, with Open beside it. */
export const SandboxedFrame: Story = {
  args: {
    review: withRendered({
      ...noFrame,
      frame: "/reviews/1/raw/1/page.html",
      sandboxed: true,
      title: "page.html",
    }),
  },
  play: async ({ canvasElement }) => {
    const frame = canvasElement.querySelector(".file > .rendered iframe");
    await expect(frame?.getAttribute("sandbox")).toBe("allow-scripts");
    await expect(frame?.getAttribute("src")).toBe("/reviews/1/raw/1/page.html");
    const cap = canvasElement.querySelector(".file > .rendered .cap");
    await expect(cap?.textContent).toContain("Sandboxed");
    await expect(cap?.querySelector("a.open")?.textContent).toContain("Open");
  },
};
