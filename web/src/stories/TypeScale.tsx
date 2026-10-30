import type { CSSProperties } from "react";

interface Row {
  readonly label: string;
  readonly sample: string;
  readonly style: CSSProperties;
}

const rows: readonly Row[] = [
  {
    label: "xl / 26",
    sample: "Product doc, second pass",
    style: {
      fontSize: "var(--t-xl)",
      fontWeight: 700,
      letterSpacing: "-.02em",
    },
  },
  {
    label: "lg / 20",
    sample: "Waiting on you",
    style: { fontSize: "var(--t-lg)", fontWeight: 650 },
  },
  {
    label: "md / 16",
    sample: "A round is frozen at request time.",
    style: { fontSize: "var(--t-md)" },
  },
  {
    label: "sm / 14",
    sample: "Rewrote the widening section, and the compare control is new.",
    style: { fontSize: "var(--t-sm)" },
  },
  {
    label: "xs / 12",
    sample: "Round 2, 1 file",
    style: { fontSize: "var(--t-xs)", color: "var(--ink-faint)" },
  },
  {
    label: "mono / 13",
    sample: "docs/product.md:214 0O 1lI",
    style: { fontFamily: "var(--mono)", fontSize: "var(--t-code)" },
  },
  {
    label: "letter / 17",
    sample: "Both comments from round 1 are done.",
    style: { font: "400 17px/1.6 var(--letter)" },
  },
];

/** The type scale: each size and family, with a line set in it. */
export function TypeScale() {
  return (
    <>
      {rows.map((row) => (
        <div className="typerow" key={row.label}>
          <span className="meta">{row.label}</span>
          <span style={row.style}>{row.sample}</span>
        </div>
      ))}
    </>
  );
}
