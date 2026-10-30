import type { ReactNode } from "react";
import { storyHref, type Entry } from "./Sheet";
import type { Theme } from "./ThemeStamp";

/** One state of a screen, as the sheet lays it out. */
export interface ScreenState {
  /** Storybook's id for the state's story. */
  readonly id: string;
  readonly caption: string;
}

interface ScreenSheetProps {
  readonly title: string;
  readonly intro: ReactNode;
  readonly entries: readonly Entry[];
  readonly states: readonly ScreenState[];
  /** The wide layout's story, which a phone frame cannot show. */
  readonly wide: ScreenState;
  readonly notes?: readonly ReactNode[];
}

interface FrameProps {
  readonly entry: Entry;
  readonly state: ScreenState;
  readonly theme: Theme;
}

function Frame({ entry, state, theme }: FrameProps) {
  return (
    <figure className="frame">
      <figcaption>
        {theme} &mdash; {state.caption}
        <a
          href={storyHref(state.id, theme)}
          aria-label={`${state.caption}, ${theme}, full screen`}
        >
          Full screen
        </a>
      </figcaption>
      <div className="phone" data-theme={theme}>
        <div className="app">{entry.view}</div>
      </div>
    </figure>
  );
}

/**
 * A screen's states in 390 by 800 phone frames, each light beside dark with a
 * Full screen button, which the phone needs because typing a story's address
 * into Storybook is the step that gets skipped.
 */
export function ScreenSheet({
  title,
  intro,
  entries,
  states,
  wide,
  notes,
}: ScreenSheetProps) {
  const byId = new Map(entries.map((entry) => [entry.id, entry]));
  return (
    <div className="sheet-page">
      <div className="sheet-body">
        <h1>{title}</h1>
        <p>{intro}</p>
        {states.map((state) => {
          const entry = byId.get(state.id);
          return entry === undefined ? null : (
            <section className="spec" key={state.id} id={state.id}>
              <div className="pair">
                <Frame entry={entry} state={state} theme="light" />
                <Frame entry={entry} state={state} theme="dark" />
              </div>
            </section>
          );
        })}
        <section className="spec" id="wide">
          <h2>
            Wide layout <span className="tag">full screen</span>
          </h2>
          <p>
            A phone frame cannot show it beside a second copy of itself. Open it
            at a desktop width.
          </p>
          <div className="pair">
            <a
              className="wide-link"
              href={storyHref(wide.id, "light")}
              aria-label="Full screen, light, wide layout"
            >
              Full screen, light
            </a>
            <a
              className="wide-link"
              href={storyHref(wide.id, "dark")}
              aria-label="Full screen, dark, wide layout"
            >
              Full screen, dark
            </a>
          </div>
        </section>
        {notes === undefined ? null : (
          <ul className="note-list">
            {notes.map((note, i) => (
              <li key={i}>{note}</li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

/** What a sheet cannot draw: a state naming a story that does not exist. */
export function missingStates(
  entries: readonly Entry[],
  states: readonly ScreenState[],
): string[] {
  const known = new Set(entries.map((entry) => entry.id));
  return states.map((state) => state.id).filter((id) => !known.has(id));
}
