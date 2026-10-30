import { Eye } from "./Eye";
import { Icon } from "./Icon";

interface Back {
  readonly label: string;
  readonly href: string;
}

interface TopBarProps {
  /** How many reviews wait, shown by the eye. */
  readonly waiting: number;
  /** A screen below the queue names its title and the way back. */
  readonly back?: Back;
  readonly title?: string;
}

/** The bar every screen starts with: the wordmark or a way back, and the eye. */
export function TopBar({ waiting, back, title }: TopBarProps) {
  return (
    <div className="topbar">
      {back === undefined ? (
        <span className="wordmark">eyeball</span>
      ) : (
        <>
          <a className="back" href={back.href}>
            <Icon name="chevron-left" />
            {back.label}
          </a>
          <h1>{title}</h1>
        </>
      )}
      <Eye waiting={waiting} />
    </div>
  );
}
