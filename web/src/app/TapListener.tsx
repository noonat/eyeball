import { useEffect } from "react";
import { useNavigate } from "react-router";
import { answerTaps, load } from "../../ts/tap";

/** The addresses the router draws. Any other is a template page. */
const drawn: readonly string[] = ["/"];

/**
 * Answers the service worker's notification taps. An address the router draws
 * is moved to with no page load. Any other is loaded, which is every review
 * until its screen moves here.
 */
export function TapListener() {
  const navigate = useNavigate();
  useEffect(
    () =>
      answerTaps((to) => {
        const { pathname } = new URL(to, location.origin);
        if (drawn.includes(pathname)) {
          void navigate(to);
        } else {
          load(to);
        }
      }),
    [navigate],
  );
  return null;
}
