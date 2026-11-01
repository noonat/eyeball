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
    "!./Project.stories.tsx",
    "!./Tree.stories.tsx",
    "!./File.stories.tsx",
    "!./Arrival.stories.tsx",
    "!./Routes.stories.tsx",
    "!./ScreenCoverage.stories.tsx",
    "!./ProjectSheet.stories.tsx",
    "!./TreeSheet.stories.tsx",
    "!./FileSheet.stories.tsx",
    "!./MediaSheet.stories.tsx",
    "!./Review.stories.tsx",
    "!./ReviewStates.stories.tsx",
    "!./ReviewParts.stories.tsx",
    "!./ReviewSheet.stories.tsx",
    "!./ReviewLargeSheet.stories.tsx",
    "!./ReviewCommentsSheet.stories.tsx",
    "!./ReviewBarSheet.stories.tsx",
    "!./Stream.stories.tsx",
    "../components/*.stories.tsx",
  ],
  { eager: true },
);

/** Every state of every foundation primitive. */
export const all = entries(modules);
