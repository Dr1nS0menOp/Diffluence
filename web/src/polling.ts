// SPDX-License-Identifier: Apache-2.0

const SPREAD_SHARE = 0.1;

/**
 * A timer's next wait, with a small random part, so a thousand tabs opened at nine
 * o'clock do not stay in step (ARC-161). Pass it as a function to `refetchInterval`.
 */
export function spreadPoll(waitMs: number): () => number {
  return () => waitMs + Math.random() * waitMs * SPREAD_SHARE;
}
