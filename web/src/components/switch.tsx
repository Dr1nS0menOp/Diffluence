// SPDX-License-Identifier: Apache-2.0
// A setting that is on or off and applies at once (NAV-161, NAV-166): a native checkbox in the
// role of a switch, its name before it, drawn by `.switch`. It has no Save of its own.
import type { ReactNode } from 'react';

export function Switch({
  checked,
  disabled = false,
  onChange,
  children,
}: {
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
  /** The name of the setting. */
  children: ReactNode;
}) {
  return (
    <label className="switch">
      <span>{children}</span>
      <input
        type="checkbox"
        role="switch"
        checked={checked}
        aria-checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
    </label>
  );
}
