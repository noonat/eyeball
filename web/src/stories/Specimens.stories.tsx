import type { Meta, StoryObj } from "@storybook/react-vite";
import { all } from "./catalog";
import { sections } from "./sections";
import { Runs, statesOf, type Section } from "./Sheet";

const meta = {
  title: "Foundations/Specimens",
  parameters: { frame: "screen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function section(id: string): Section {
  const found = sections.find((candidate) => candidate.id === id);
  if (found === undefined) {
    throw new Error(`no section ${id}`);
  }
  return found;
}

function Specimen({ id }: { readonly id: string }) {
  return <Runs states={statesOf(section(id), all)} />;
}

// One story per section, written out because Storybook's indexer reads object
// literals. The sheet links each by specimenName, and its own story fails when
// a section has none.
export const Palette: Story = {
  render: () => <Specimen id="palette" />,
};
export const Type: Story = {
  render: () => <Specimen id="type" />,
};
export const Icons: Story = {
  render: () => <Specimen id="icons" />,
};
export const State: Story = {
  render: () => <Specimen id="state" />,
};
export const Attention: Story = {
  render: () => <Specimen id="attention" />,
};
export const Comments: Story = {
  render: () => <Specimen id="comments" />,
};
export const Actions: Story = {
  render: () => <Specimen id="actions" />,
};
export const Firstrun: Story = {
  render: () => <Specimen id="firstrun" />,
};
