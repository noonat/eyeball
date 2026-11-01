import type { Rendering, Side } from "../api/types";
import { cx } from "./cx";
import { Icon } from "./Icon";

interface RenderedFileProps {
  readonly rendered: Rendering;
}

/**
 * A file the browser shows itself: the line naming one the surface does not
 * show, one frame holding a page or a PDF, or its images and players, before
 * over after.
 */
export function RenderedFile({ rendered }: RenderedFileProps) {
  if (rendered.facts !== null) {
    const facts = rendered.facts;
    return (
      <div className="file">
        <div className="filefacts">
          <span className="kind">{facts.kind}</span>
          {facts.size === "" ? null : (
            <span className="size">{facts.size}</span>
          )}
          {facts.hint === "" ? null : (
            <span className="hint">{facts.hint}</span>
          )}
          {facts.load === "" ? null : (
            <a className="load" href={facts.load}>
              Load anyway
            </a>
          )}
        </div>
      </div>
    );
  }
  if (rendered.frame !== "") {
    return <Frame rendered={rendered} />;
  }
  return (
    <div className="beforeafter">
      {(rendered.sides ?? []).map((side) => (
        <Preview key={side.src} side={side} />
      ))}
    </div>
  );
}

/** A page or a PDF in a frame, with Open beside it. */
function Frame({ rendered }: RenderedFileProps) {
  return (
    <div className="preview">
      <div className="cap">
        {rendered.sandboxed ? (
          <>
            <Icon name="triangle-alert" />
            <span>Sandboxed</span>
          </>
        ) : (
          <>
            <span>{rendered.kind}</span>
            <span className="dim">{rendered.size}</span>
          </>
        )}
        <a
          className="open"
          href={rendered.frame}
          target="_blank"
          rel="noopener"
        >
          <Icon name="maximize-2" />
          Open
        </a>
      </div>
      <iframe
        className="pageframe"
        // A page an agent wrote runs under the sandbox. A PDF has no script,
        // and a browser's viewer does not open inside one.
        sandbox={rendered.sandboxed ? "allow-scripts" : undefined}
        loading="lazy"
        title={rendered.title}
        src={rendered.frame}
      />
    </div>
  );
}

/** One image or player, captioned. */
function Preview({ side }: { readonly side: Side }) {
  return (
    <div className={cx("preview", side.tint === "" ? undefined : side.tint)}>
      <div className="cap">
        <span>{side.label}</span>
        <span className="dim">{side.caption}</span>
      </div>
      {side.element === "video" ? (
        <video controls playsInline preload="metadata" src={side.src} />
      ) : side.element === "audio" ? (
        <audio controls preload="metadata" src={side.src} />
      ) : (
        <img
          className="shot natural"
          loading="lazy"
          src={side.src}
          alt={side.label}
        />
      )}
    </div>
  );
}
