// SPDX-License-Identifier: Apache-2.0
// A choice among a few as tiles (SPC-151, NAV-160): each a bold title and a muted line, the chosen
// one with a filled radio. Native radio buttons, so the arrow keys move between them.
import { useId } from 'react';

export interface ChoiceTile<T extends string> {
  value: T;
  title: string;
  help: string;
}

export function ChoiceTiles<T extends string>({
  legend,
  tiles,
  value,
  onChange,
}: {
  legend: string;
  tiles: ChoiceTile<T>[];
  value: T;
  onChange: (value: T) => void;
}) {
  const name = useId();
  return (
    <fieldset className="choice-tiles">
      <legend>{legend}</legend>
      {tiles.map((tile) => (
        <label key={tile.value} className="choice-tile">
          <input
            type="radio"
            name={name}
            value={tile.value}
            checked={value === tile.value}
            onChange={() => onChange(tile.value)}
          />
          <span className="choice-tile-text">
            <span className="choice-tile-title">{tile.title}</span>
            <span className="choice-tile-help">{tile.help}</span>
          </span>
        </label>
      ))}
    </fieldset>
  );
}
