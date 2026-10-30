import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { queueKey } from "../api/queue";
import type { Queue } from "../api/types";
import { useQueueStream } from "../app/useQueueStream";

const meta = {
  title: "App/Stream",
  parameters: { sheet: false },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

function view(stamp: string): Queue {
  return {
    waiting: null,
    working: null,
    settled: null,
    waiting_count: 0,
    stamp,
  };
}

/** An EventSource the test drives: it hears only what the test sends. */
class FakeEventSource extends EventTarget {
  static opened: FakeEventSource[] = [];
  constructor(readonly url: string) {
    super();
    FakeEventSource.opened.push(this);
  }
  close(): void {
    // Nothing is held open.
  }
  send(data: string): void {
    this.dispatchEvent(new MessageEvent("message", { data }));
  }
}

/** What a story's render hands to its play function. */
interface Controls {
  readonly client: QueryClient;
  readonly held: ((queue: Queue) => void)[];
  calls: number;
}

let controls: Controls | null = null;

function Held({ load }: { readonly load: () => Promise<Queue> }) {
  const { data } = useQuery({ queryKey: queueKey, queryFn: load });
  useQueueStream();
  return <p aria-label="held stamp">{data?.stamp ?? "none"}</p>;
}

/**
 * A fetch under way when a stamp arrives may have read the store before the
 * change. It returns the old view, and the change must still reach the screen:
 * TanStack Query clears an invalidation when any fetch succeeds, so the stamp
 * the stream sent has to be compared again once that fetch is done.
 */
export const StampArrivesDuringAFetch: Story = {
  beforeEach: () => {
    const real: unknown = Reflect.get(window, "EventSource");
    FakeEventSource.opened = [];
    Reflect.set(window, "EventSource", FakeEventSource);
    return () => {
      Reflect.set(window, "EventSource", real);
    };
  },
  render: () => {
    const made: Controls = {
      client: new QueryClient({
        defaultOptions: {
          queries: { retry: false, refetchOnWindowFocus: false },
        },
      }),
      held: [],
      calls: 0,
    };
    controls = made;
    // The first fetch answers at once with the old view. The second is held
    // until the test lets it go, and every later one has the change.
    const load = (): Promise<Queue> => {
      made.calls++;
      if (made.calls === 1) {
        return Promise.resolve(view("s0"));
      }
      if (made.calls === 2) {
        return new Promise((resolve) => {
          made.held.push(resolve);
        });
      }
      return Promise.resolve(view("s2"));
    };
    return (
      <QueryClientProvider client={made.client}>
        <Held load={load} />
      </QueryClientProvider>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const run = controls;
    const stream = FakeEventSource.opened[0];
    if (run === null || stream === undefined) {
      throw new Error("the story drew no app");
    }
    const held = canvas.getByLabelText("held stamp");
    await waitFor(() => expect(held).toHaveTextContent("s0"));

    // A second fetch starts, as a return to the foreground starts one, and it
    // is still reading when the stream reports the change.
    void run.client.invalidateQueries({ queryKey: queueKey });
    await waitFor(() => expect(run.calls).toBe(2));
    stream.send("s2");
    await expect(run.calls).toBe(2);

    // It answers with the view as it was before the change.
    run.held[0]?.(view("s0"));
    await waitFor(() => expect(held).toHaveTextContent("s2"));

    // The stamps agree now, so nothing fetches again.
    await new Promise((resolve) => setTimeout(resolve, 300));
    await expect(run.calls).toBe(3);
  },
};
