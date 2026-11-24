import type { ReactNode } from "react";
import { Link, useInRouterContext } from "react-router";
import { draws } from "../app/routes";

interface AppLinkProps {
  readonly href: string;
  readonly className?: string | undefined;
  /** Marks the link as the view shown, for a control choosing between views. */
  readonly current?: boolean | undefined;
  /** Names a link that has only an icon. */
  readonly "aria-label"?: string | undefined;
  readonly title?: string | undefined;
  readonly children: ReactNode;
}

/**
 * A link the server wrote. An address the app draws moves the router, so the
 * move loads no page and the cache and the scroll survive it. Any other is a
 * plain anchor, which loads the page. So is every link outside a router, such
 * as the error screen's way back, where a router link would have nothing to
 * move.
 */
export function AppLink({
  href,
  className,
  current,
  title,
  children,
  ...rest
}: AppLinkProps) {
  const inRouter = useInRouterContext();
  const aria = {
    ...(current === true ? { "aria-current": true as const } : {}),
    ...(rest["aria-label"] === undefined
      ? {}
      : { "aria-label": rest["aria-label"] }),
    ...(title === undefined ? {} : { title }),
  };
  if (inRouter && draws(href)) {
    return (
      <Link
        to={href}
        // No route is discovered lazily in a data router, and the default would
        // mark each link with data-discover.
        discover="none"
        className={className}
        {...aria}
      >
        {children}
      </Link>
    );
  }
  return (
    <a href={href} className={className} {...aria}>
      {children}
    </a>
  );
}
