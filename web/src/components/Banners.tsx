import { Icon } from "./Icon";

interface BannersProps {
  /** The stream has been down long enough to say so. */
  readonly offline?: boolean | undefined;
  /** The daemon runs a newer build than the one that served this page. */
  readonly updateReady?: boolean | undefined;
  readonly onRetry?: (() => void) | undefined;
}

/**
 * The two notices under the top bar. Each is in a holder that is hidden rather
 * than absent, since .offline and .update set display, which outranks hidden on
 * the element itself.
 */
export function Banners({ offline, updateReady, onRetry }: BannersProps) {
  return (
    <>
      <div id="update" hidden={updateReady !== true}>
        <div className="update">
          <Icon name="refresh-cw" />
          <span>Update ready.</span>
          <a className="refresh" role="button" tabIndex={0}>
            Refresh
          </a>
        </div>
      </div>
      <div id="offline" hidden={offline !== true}>
        <div className="offline">
          <Icon name="triangle-alert" />
          <span>Cannot reach Eyeball.</span>
          <a className="retry" role="button" tabIndex={0} onClick={onRetry}>
            Retry
          </a>
        </div>
      </div>
    </>
  );
}
