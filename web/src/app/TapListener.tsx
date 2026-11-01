import { useEffect } from "react";
import { useNavigate } from "react-router";
import { answerTaps, load } from "../../ts/tap";
import { draws } from "./routes";

/**
 * Answers the service worker's notification taps. An address the router draws
 * is moved to with no page load. Any other is loaded, which is every review
 * until its screen moves here. Which is which is decided by `draws`, the same
 * function the links use.
 */
export function TapListener() {
  const navigate = useNavigate();
  useEffect(
    () =>
      answerTaps((to) => {
        if (draws(to)) {
          void navigate(to);
        } else {
          load(to);
        }
      }),
    [navigate],
  );
  return null;
}
