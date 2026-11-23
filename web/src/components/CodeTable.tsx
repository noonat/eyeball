import { Fragment, type ReactNode } from "react";
import type { CodeRow, Token, TokenRole } from "../api/types";
import { classes } from "../../ts/classes";
import { cx } from "./cx";

/** The class the stylesheet colors each role with. Plain text has none. */
const roleClass: Readonly<Record<TokenRole, string | undefined>> = {
  plain: undefined,
  keyword: "k",
  string: "s",
  comment: "c",
  number: "n",
  function: "f",
};

interface CodeTableProps {
  /** The rows the API sent, in reading order. */
  readonly rows: readonly CodeRow[];
  /**
   * What sits in the table under a row, as rows of its own: the comments on a
   * line, or a composer. A page with no comments leaves it out.
   */
  readonly under?: ((row: CodeRow) => ReactNode) | undefined;
  /** Whether a row is the one a composer is open on, which is drawn selected. */
  readonly selected?: ((row: CodeRow) => boolean) | undefined;
}

/**
 * A file's lines as a table: number, sign and text. A changed line is tinted,
 * and a word that changed within it is marked. The text is drawn from the
 * tokens the server cut, so no highlighting happens here.
 */
export function CodeTable({ rows, under, selected }: CodeTableProps) {
  return (
    // A line can run past the screen, so the table scrolls, and a scrolling
    // region has to be reachable by keyboard. The template's page was not.
    <div className="code narrow" tabIndex={0}>
      <table>
        <tbody>
          {rows.map((row, i) => (
            <Fragment key={i}>
              <CodeLine row={row} selected={selected?.(row) === true} />
              {under?.(row)}
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function CodeLine({
  row,
  selected,
}: {
  readonly row: CodeRow;
  readonly selected: boolean;
}) {
  if (row.kind === "hunk") {
    return (
      <tr className="hunkhead">
        <td colSpan={3}>{row.header}</td>
      </tr>
    );
  }
  // A removed line is numbered on the old side, which a comment cannot anchor
  // to, so it carries its own attribute.
  const number =
    row.kind === "del"
      ? { "data-old": row.number }
      : { "data-line": row.number };
  const tint =
    row.kind === "add" ? "add" : row.kind === "del" ? "del" : undefined;
  return (
    <tr className={cx(tint, selected && classes.sel) || undefined} {...number}>
      <td className="ln">{row.number}</td>
      <td className="sign">{row.sign}</td>
      <td className="src">
        {(row.tokens ?? []).map((token, i) => (
          <TokenRun key={i} token={token} />
        ))}
      </td>
    </tr>
  );
}

function TokenRun({ token }: { readonly token: Token }) {
  const role = roleClass[token.role];
  const text =
    role === undefined ? (
      token.text
    ) : (
      <span className={role}>{token.text}</span>
    );
  return token.mark ? <span className="w">{text}</span> : text;
}
