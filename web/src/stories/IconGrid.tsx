import { Icon, type IconName } from "../components/Icon";

/** Every icon in the subset, with the name that summons it. */
export const iconNames: readonly IconName[] = [
  "chevron-left",
  "chevron-right",
  "folder",
  "bot",
  "message-square",
  "check",
  "x",
  "pencil",
  "trash-2",
  "refresh-cw",
  "info",
  "triangle-alert",
  "circle-alert",
  "bell",
  "file-text",
  "clock",
  "chevron-down",
  "maximize-2",
];

/** The icon inventory. */
export function IconGrid() {
  return (
    <div className="icongrid">
      {iconNames.map((name) => (
        <div className="ic" key={name}>
          <Icon name={name} />
          <span className="nm">{name}</span>
        </div>
      ))}
    </div>
  );
}
