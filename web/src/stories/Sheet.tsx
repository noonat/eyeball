import {
  createElement,
  isValidElement,
  type ComponentType,
  type ReactNode,
} from "react";
import { storyNameFromExport, toId } from "storybook/internal/csf";
import { Framed, type Frame } from "./Framed";
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
  readonly view: ReactNode;
}

type Module = Readonly<Record<string, unknown>>;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
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
      out.push({
        group: title,
        name,
        id: toId(title, name),
        frame: frameOf({
          ...(isRecord(meta["parameters"]) ? meta["parameters"] : {}),
          ...(isRecord(story["parameters"]) ? story["parameters"] : {}),
        }),
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

interface SheetFrameProps {
  readonly entry: Entry;
  readonly theme: Theme;
}

function SheetFrame({ entry, theme }: SheetFrameProps) {
  return (
    <figure className="frame">
      <figcaption>
        {theme}
        <a href={storyHref(entry.id, theme)}>Full screen</a>
      </figcaption>
      <div className="phone" data-theme={theme}>
        <Framed frame={entry.frame}>{entry.view}</Framed>
      </div>
    </figure>
  );
}

interface SheetProps {
  readonly entries: readonly Entry[];
}

/** Every foundation state in 390 by 800 frames, each light one beside its dark. */
export function Sheet({ entries: all }: SheetProps) {
  return (
    <div className="sheet-page">
      <div className="sheet-body">
        <h1>Foundations</h1>
        <p>
          Every foundation state, light beside dark, in a 390 by 800 frame. Each
          frame links to the state at full screen.
        </p>
        {all.map((entry) => (
          <section key={entry.id}>
            <h2>
              {entry.group.replace("Foundations/", "")}: {entry.name}
            </h2>
            <div className="pair">
              <SheetFrame entry={entry} theme="light" />
              <SheetFrame entry={entry} theme="dark" />
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}
