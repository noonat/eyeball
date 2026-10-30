import type { ReactNode } from "react";
import { Icon } from "./Icon";

interface StepProps {
  /** The step's number, shown until it is done. */
  readonly number: number;
  readonly what: string;
  readonly why: string;
  readonly done?: boolean;
  /** The control that does the step, when there is one. */
  readonly action?: ReactNode;
}

/** One first-run step: what to do, why, and the control that does it. */
export function Step({ number, what, why, done, action }: StepProps) {
  return (
    <div className={done === true ? "step done" : "step"}>
      <span className="mark">
        {done === true ? <Icon name="check" /> : number}
      </span>
      <span className="what">{what}</span>
      {action}
      <span className="why">{why}</span>
    </div>
  );
}

interface StepsProps {
  readonly children: ReactNode;
}

/** The first-run steps, in order. */
export function Steps({ children }: StepsProps) {
  return <div className="steps">{children}</div>;
}
