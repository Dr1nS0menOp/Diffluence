// SPDX-License-Identifier: Apache-2.0
// A part of a screen that is still loading: a framed box in the part's own place, with a
// bold sentence that says what it waits for and pale lines in the shape of the result,
// without a spinner, while the rest of the screen stays usable (NAV-177).

/** How many rows of the result the placeholder shows, unless a part asks for another number. */
const DEFAULT_ROWS = 3;

export function LoadingBox({ sentence, rows = DEFAULT_ROWS }: { sentence: string; rows?: number }) {
  return (
    <div className="loading-box" role="status">
      <strong>{sentence}</strong>
      <div className="loading-shape" aria-hidden="true">
        {Array.from({ length: rows }, (_, i) => (
          <div key={i} className="loading-row">
            <span className="loading-line short" />
            <span className="loading-line" />
          </div>
        ))}
      </div>
    </div>
  );
}
