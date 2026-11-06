// SPDX-License-Identifier: Apache-2.0
// The square a person cuts from a picture. Pure sums, so they can be tested without a
// canvas. Everything is in pixels of the original image.

export const MAX_ZOOM = 4;
const ZOOM_STEP = 1.25;
const MOVE_SHARE = 0.05;

export interface Crop {
  x: number;
  y: number;
  size: number;
}

/** The largest square, in the middle: where every crop starts. */
export function startCrop(width: number, height: number): Crop {
  const size = Math.min(width, height);
  return { x: Math.floor((width - size) / 2), y: Math.floor((height - size) / 2), size };
}

function keepInside(crop: Crop, width: number, height: number): Crop {
  const size = Math.max(1, Math.min(crop.size, width, height));
  return {
    size,
    x: Math.min(Math.max(0, crop.x), width - size),
    y: Math.min(Math.max(0, crop.y), height - size),
  };
}

/** Moves the square by a share of its own size, so a step feels the same at every zoom. */
export function moveCrop(crop: Crop, steps: { x: number; y: number }, width: number, height: number): Crop {
  const step = Math.max(1, Math.round(crop.size * MOVE_SHARE));
  return keepInside({ ...crop, x: crop.x + steps.x * step, y: crop.y + steps.y * step }, width, height);
}

/** Zooms around the centre of the square. A positive step goes closer. */
export function zoomCrop(crop: Crop, step: number, width: number, height: number): Crop {
  const smallest = Math.ceil(Math.min(width, height) / MAX_ZOOM);
  const wanted = Math.round(crop.size / ZOOM_STEP ** step);
  const size = Math.max(smallest, Math.min(wanted, width, height));
  const shift = (crop.size - size) / 2;
  return keepInside({ size, x: Math.round(crop.x + shift), y: Math.round(crop.y + shift) }, width, height);
}
