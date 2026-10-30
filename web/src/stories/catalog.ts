import { entries } from "./Sheet";

// Every story the components and the specimens declare, so a new state joins
// the sheet without anyone remembering to list it. The sheet and the full-screen
// section stories are built from these, so they are not in it.
const modules = import.meta.glob<Record<string, unknown>>(
  [
    "./*.stories.tsx",
    "!./Sheet.stories.tsx",
    "!./Specimens.stories.tsx",
    "!./Queue.stories.tsx",
    "!./QueueSheet.stories.tsx",
    "!./Stream.stories.tsx",
    "../components/*.stories.tsx",
  ],
  { eager: true },
);

/** Every state of every foundation primitive. */
export const all = entries(modules);
