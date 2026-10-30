/** The tokens a swatch shows, in the order the tearout lists them. */
export const swatchTokens = [
  "--bg",
  "--panel",
  "--bg-sunk",
  "--line-strong",
  "--ink",
  "--ink-dim",
  "--ink-faint",
  "--accent-vivid",
  "--hl",
  "--hl-line",
  "--syntax-keyword",
  "--ok-vivid",
  "--warn-vivid",
  "--danger-vivid",
  "--add-vivid",
  "--add-wash",
  "--add-word",
  "--del-vivid",
  "--del-wash",
  "--del-word",
] as const;

/** The palette: each token as a filled box and its name. */
export function Palette() {
  return (
    <div className="swatches">
      {swatchTokens.map((token) => (
        <div className="sw" key={token}>
          <div className="chipbox" style={{ background: `var(${token})` }} />
          <div className="lbl">{token}</div>
        </div>
      ))}
    </div>
  );
}
