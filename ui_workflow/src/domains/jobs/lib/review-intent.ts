import type { StepRun } from "@/domains/jobs/types";

export interface ReviewIntent {
  key: string;
  fingerprint: string;
  decision: "accept" | "regenerate";
  position?: number;
  feedback?: string;
  accepted?: boolean;
  receiptID?: string;
}
/** UI candidate identity only, not an engine generation or authorization token. */
export function candidateFingerprint(step: StepRun): string {
  return JSON.stringify([
    step.id,
    step.rendered_prompt,
    step.candidates.map((candidate) => [
      candidate.url.split(/[?#]/)[0],
      candidate.content,
    ]),
  ]);
}
export function feedbackBytes(feedback: string): number {
  return new TextEncoder().encode(feedback).length;
}
export function validCandidatePosition(
  step: StepRun,
  position: number | null,
): position is number {
  return (
    position !== null &&
    Number.isInteger(position) &&
    position >= 0 &&
    position < step.candidates.length
  );
}
