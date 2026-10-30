import { Button } from "./Button";
import { Notice } from "./Notice";
import { Step, Steps } from "./Steps";

/**
 * What the device still has to do for notifications to arrive. Only the device
 * knows, so the screen takes it as a prop rather than from the API.
 */
export type Setup = "ready" | "not-installed" | "not-allowed" | "lapsed";

interface QueueSetupProps {
  readonly setup: Setup;
}

/** The setup notice and steps above the queue, or nothing once all is well. */
export function QueueSetup({ setup }: QueueSetupProps) {
  if (setup === "ready") {
    return null;
  }
  if (setup === "lapsed") {
    return (
      <Notice tone="warn" icon="bell">
        <strong>Notifications stopped arriving.</strong> This device&rsquo;s
        subscription lapsed.{" "}
        <a role="button" tabIndex={0}>
          Re-subscribe
        </a>
      </Notice>
    );
  }
  const installed = setup === "not-allowed";
  return (
    <>
      <Notice tone="warn" icon="bell">
        <strong>Notifications are not set up.</strong> Nothing will reach this
        phone until both steps are done.
      </Notice>
      <Steps>
        {installed ? (
          <Step
            number={1}
            done
            what="Installed to the home screen"
            why="iOS delivers web push only to an installed page."
          />
        ) : (
          <Step
            number={1}
            what="Add to Home Screen"
            why="Share, then Add to Home Screen. iOS delivers web push only to an installed page."
          />
        )}
        <Step
          number={2}
          what="Allow notifications"
          action={
            <Button small tone="accent">
              Allow
            </Button>
          }
          why={
            installed
              ? "One notification per review. Comments and verdicts send none."
              : "Available once the page is opened from the home screen."
          }
        />
      </Steps>
    </>
  );
}
