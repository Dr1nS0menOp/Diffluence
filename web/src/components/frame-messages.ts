// SPDX-License-Identifier: Apache-2.0
// The one place that talks to a frame (ARC-111). Every message is checked on two things
// before anyone reads it: it came from the window of our frame, and from the origin we
// expect. Copies of this code in each frame would drift apart, and the checks with them.

/** The origin of a frame sandboxed without allow-same-origin, such as the file viewers. */
export const OPAQUE_ORIGIN = 'null';

/** The parts of a MessageEvent that decide whether we listen to it. */
export interface IncomingMessage {
  source: unknown;
  origin: string;
  data: unknown;
}

/** Whether a message came from this frame, on this origin. */
export function fromFrame(event: IncomingMessage, frame: Window | null | undefined, origin: string): boolean {
  return !!frame && event.source === frame && event.origin === origin;
}

/**
 * Listens to one frame. `frame` is read at each message, because the frame may load after
 * the listener starts. Returns the function that stops listening.
 */
export function listenToFrame(
  frame: () => Window | null | undefined,
  origin: string,
  onMessage: (data: unknown) => void,
): () => void {
  const listen = (event: MessageEvent) => {
    if (fromFrame(event, frame(), origin)) onMessage(event.data);
  };
  window.addEventListener('message', listen);
  return () => window.removeEventListener('message', listen);
}

/**
 * Sends to one frame. A frame with an opaque origin cannot be named as a target, so it
 * gets '*': the message still only reaches that frame's window.
 */
export function postToFrame(
  frame: Window | null | undefined,
  origin: string,
  message: unknown,
  transfer: Transferable[] = [],
): void {
  frame?.postMessage(message, origin === OPAQUE_ORIGIN ? '*' : origin, transfer);
}
