import { Icon } from "./Icon";

/** The line saying a file's mode changed, or that an added file is not regular. */
export function ModeLine({ mode }: { readonly mode: string }) {
  return (
    <div className="modeline">
      <Icon name="triangle-alert" />
      <span>{mode}</span>
    </div>
  );
}
