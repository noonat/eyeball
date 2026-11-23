import { EmptyBox } from "./EmptyBox";
import { Screen } from "./Screen";

interface ErrorScreenProps {
  /** The bar's heading and the document's title, such as "Too large". */
  readonly title: string;
  /** The box's headline. */
  readonly heading: string;
  /** The sentence under the headline. */
  readonly text: string;
  /** How many reviews wait, or null where the count could not be read. */
  readonly waiting: number | null;
}

/**
 * A failure the server answered a page load with: the bar the template's error
 * page had, with its way back to the queue, over a box saying what failed. The
 * server decides the words, so the same sentence reaches a person here and a
 * script as plain text.
 */
export function ErrorScreen({
  title,
  heading,
  text,
  waiting,
}: ErrorScreenProps) {
  return (
    <Screen title={title} back="/" backLabel="Queue" waiting={waiting}>
      <EmptyBox as="p" headline={heading} text={text} />
    </Screen>
  );
}
