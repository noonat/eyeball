import {
  createElement,
  isValidElement,
  type ComponentType,
  type ReactNode,
} from "react";
import { storyNameFromExport, toId } from "storybook/internal/csf";
import type { Frame } from "./Framed";
import type { Theme } from "./ThemeStamp";

/** One state of one foundation component, as a story describes it. */
export interface Entry {
  /** The group's title, such as "Foundations/Pill". */
  readonly group: string;
  /** The story's name, such as "Tones". */
  readonly name: string;
  /** Storybook's id for the story, which names its full-screen address. */
  readonly id: string;
  readonly frame: Frame;
  /** False for a story that is a test and adds nothing to look at. */
  readonly onSheet: boolean;
  readonly view: ReactNode;
}

type Module = Readonly<Record<string, unknown>>;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function onSheetOf(parameters: unknown): boolean {
  return !(isRecord(parameters) && parameters["sheet"] === false);
}

function frameOf(parameters: unknown): Frame {
  if (isRecord(parameters) && parameters["frame"] === "screen") {
    return "screen";
  }
  return "page";
}

// A function is the most a runtime check can say about a render function or a
// component, so these guards say the rest on the story format's behalf.
function isRender(value: unknown): value is (args: object) => unknown {
  return typeof value === "function";
}

function isComponent(value: unknown): value is ComponentType<object> {
  return typeof value === "function";
}

/** What a story draws: its render function, or its component with its args. */
function viewOf(
  meta: Record<string, unknown>,
  story: Record<string, unknown>,
): ReactNode {
  const args = isRecord(story["args"]) ? story["args"] : {};
  const render = story["render"];
  if (isRender(render)) {
    const drawn = render(args);
    return isValidElement(drawn) ? drawn : null;
  }
  const component = meta["component"];
  return isComponent(component) ? createElement(component, args) : null;
}

function rank(path: string): number {
  return path.startsWith("./") ? 0 : 1;
}

/** The entries of every story module, each in the order it declares them. */
export function entries(modules: Readonly<Record<string, Module>>): Entry[] {
  const out: Entry[] = [];
  // The specimens in this directory come first, then the components.
  const paths = Object.keys(modules).toSorted(
    (a, b) => rank(a) - rank(b) || a.localeCompare(b),
  );
  for (const path of paths) {
    const mod = modules[path];
    const meta = mod?.["default"];
    if (mod === undefined || !isRecord(meta)) {
      continue;
    }
    const title = String(meta["title"]);
    for (const [key, story] of Object.entries(mod)) {
      // The production build adds __namedExportsOrder, an array, to every story
      // module, which is not a story and has nothing to draw.
      if (key === "default" || !isRecord(story) || Array.isArray(story)) {
        continue;
      }
      const name = storyNameFromExport(key);
      const parameters = {
        ...(isRecord(meta["parameters"]) ? meta["parameters"] : {}),
        ...(isRecord(story["parameters"]) ? story["parameters"] : {}),
      };
      out.push({
        group: title,
        name,
        id: toId(title, name),
        frame: frameOf(parameters),
        onSheet: onSheetOf(parameters),
        view: viewOf(meta, story),
      });
    }
  }
  return out;
}

/** The full-screen address of a story, relative to the page that links it. */
export function storyHref(id: string, theme: Theme): string {
  const globals = theme === "dark" ? "&globals=theme:dark" : "";
  return `iframe.html?id=${id}&viewMode=story${globals}`;
}

/** One panel: the states it holds, in the order the section lists them. */
export interface Panel {
  /** Story ids, such as "foundations-eye--open". */
  readonly states: readonly string[];
}

/** One primitive's section, as foundations.html writes one. */
export interface Section {
  readonly id: string;
  readonly title: string;
  readonly tag: string;
  readonly intro: ReactNode;
  readonly panels: readonly Panel[];
  readonly notes?: readonly ReactNode[];
}

/**
 * Runs of states that sit in the padded column share one, as the static
 * sheet's panels do, and a whole-screen state stands on its own.
 */
function runs(states: readonly Entry[]): Entry[][] {
  const out: Entry[][] = [];
  for (const entry of states) {
    const last = out.at(-1);
    if (
      last !== undefined &&
      last[0]?.frame === "page" &&
      entry.frame === "page"
    ) {
      last.push(entry);
    } else {
      out.push([entry]);
    }
  }
  return out;
}

interface RunsProps {
  readonly states: readonly Entry[];
}

/** A panel's states, as the static sheet's markup lays them out. */
export function Runs({ states }: RunsProps) {
  return (
    <>
      {runs(states).map((run) =>
        run[0]?.frame === "screen" ? (
          run.map((entry) => (
            <div className="entry" key={entry.id}>
              {entry.view}
            </div>
          ))
        ) : (
          <div className="page" key={run.map((entry) => entry.id).join()}>
            {run.map((entry) => (
              <div className="entry" key={entry.id}>
                {entry.view}
              </div>
            ))}
          </div>
        ),
      )}
    </>
  );
}

/** The states a section's panels hold, in order. */
export function statesOf(section: Section, all: readonly Entry[]): Entry[] {
  const byId = new Map(all.map((entry) => [entry.id, entry]));
  return section.panels.flatMap((panel) =>
    panel.states.flatMap((id) => byId.get(id) ?? []),
  );
}

/** The export name of the story that shows one section alone. */
export function specimenName(sectionId: string): string {
  return sectionId.slice(0, 1).toUpperCase() + sectionId.slice(1);
}

/** Where the full-screen stories of the sections live. */
export const specimenTitle = "Foundations/Specimens";

interface SpecimenProps {
  readonly section: Section;
  readonly states: readonly Entry[];
  readonly theme: Theme;
}

function Specimen({ section, states, theme }: SpecimenProps) {
  const id = toId(specimenTitle, storyNameFromExport(specimenName(section.id)));
  return (
    <figure className="frame">
      <figcaption>
        {theme}
        <a
          href={storyHref(id, theme)}
          aria-label={`${section.title}, ${theme}, full screen`}
        >
          Full screen
        </a>
      </figcaption>
      <div className="slab" data-theme={theme}>
        <div className="app">
          <Runs states={states} />
        </div>
      </div>
    </figure>
  );
}

interface SheetProps {
  readonly entries: readonly Entry[];
  readonly sections: readonly Section[];
}

/** What the sheet cannot draw: a section naming a story that does not exist. */
export function missing(
  all: readonly Entry[],
  sections: readonly Section[],
): string[] {
  const known = new Set(all.map((entry) => entry.id));
  return sections
    .flatMap((section) => section.panels.flatMap((panel) => panel.states))
    .filter((id) => !known.has(id));
}

/** Every story that is on the sheet and that no section places. */
export function unplaced(
  all: readonly Entry[],
  sections: readonly Section[],
): string[] {
  const placed = new Set(
    sections.flatMap((section) =>
      section.panels.flatMap((panel) => panel.states),
    ),
  );
  return all
    .filter((entry) => entry.onSheet && !placed.has(entry.id))
    .map((entry) => entry.id);
}

/** Each primitive's states together, light beside dark, as foundations.html. */
export function Sheet({ entries: all, sections }: SheetProps) {
  const byId = new Map(all.map((entry) => [entry.id, entry]));
  return (
    <div className="sheet-page">
      <div className="sheet-body">
        <h1>Foundations</h1>
        <p>
          The shell, the type scale, the palette, and every primitive whose
          props are local. Both themes side by side, every state of a primitive
          together. Each panel's Full screen button opens that panel alone, in
          its theme.
        </p>
        {sections.map((section, index) => (
          <div key={section.id}>
            {index > 0 ? <hr className="rule" /> : null}
            <section className="spec" id={section.id}>
              <h2>
                {section.title} <span className="tag">{section.tag}</span>
              </h2>
              <p>{section.intro}</p>
              {section.panels.map((panel) => {
                const states = panel.states.flatMap((id) => byId.get(id) ?? []);
                return (
                  <div className="pair" key={panel.states.join()}>
                    <Specimen section={section} states={states} theme="light" />
                    <Specimen section={section} states={states} theme="dark" />
                  </div>
                );
              })}
              {section.notes === undefined ? null : (
                <ul className="note-list">
                  {section.notes.map((note, i) => (
                    <li key={i}>{note}</li>
                  ))}
                </ul>
              )}
            </section>
          </div>
        ))}
      </div>
    </div>
  );
}
