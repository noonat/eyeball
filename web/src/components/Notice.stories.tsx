import type { Meta, StoryObj } from "@storybook/react-vite";
import { Notice } from "./Notice";

const meta = {
  title: "Foundations/Notice",
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** What could not have been prevented, in three tones. */
export const Tones: Story = {
  render: () => (
    <>
      <Notice tone="neutral">
        Round 1 was approved 4 days ago. This round changes 2 of the 3 files it
        covered.
      </Notice>
      <Notice tone="warn">
        <strong>Requesting changes needs something to act on.</strong> Add a
        comment, or write a note with the verdict.
      </Notice>
      <Notice tone="danger">
        <strong>1 comment did not save.</strong> Retry it, or delete it, before
        deciding.
      </Notice>
      <Notice tone="danger">
        <strong>A comment is still being written.</strong> Add it, or cancel it,
        before deciding.
      </Notice>
    </>
  ),
};

/** A lapsed subscription is reported, with the bell and a way to fix it. */
export const Lapsed: Story = {
  render: () => (
    <Notice tone="warn" icon="bell">
      <strong>Notifications stopped arriving.</strong> This device&rsquo;s
      subscription lapsed 2 days ago. <a href="#">Re-subscribe</a>
    </Notice>
  ),
};
