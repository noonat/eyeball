import { Icon, type IconName } from "./Icon";

interface CrumbProps {
  /** A folder for a directory, a file's glyph for a file. */
  readonly icon: IconName;
  /** The path above the name, which a long one truncates from its start. */
  readonly dir: string;
  readonly name: string;
}

/** Where a tree or a file page is: its directory, dimmed, then its own name. */
export function Crumb({ icon, dir, name }: CrumbProps) {
  return (
    <div className="crumb">
      <Icon name={icon} />
      {dir === "" ? null : (
        <span className="dir">
          <bdi>{dir}</bdi>
        </span>
      )}
      <span className="name">{name}</span>
    </div>
  );
}
