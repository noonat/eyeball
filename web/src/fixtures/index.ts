import type { Queue } from "../api/types";
import awkwardJson from "./queue-awkward.json";
import nothingWaitingJson from "./queue-nothing-waiting.json";
import populatedJson from "./queue-populated.json";

// Each assignment is a type check of what the server sent against the generated
// types, so a field that drifts from the types fails the build here.
export const populated: Queue = populatedJson;
export const nothingWaiting: Queue = nothingWaitingJson;
export const awkward: Queue = awkwardJson;
