// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePreferences } from '../preferences';
import { ErrorNote, Field } from '../components/ui';

const THEMES = ['system', 'light', 'dark'];

/** Keeps its own state, so a click shows at once while the server answers. */
function PreferenceSwitch({
  label,
  value,
  onChange,
}: {
  label: string;
  value: boolean;
  onChange: (next: boolean) => void;
}) {
  const [checked, setChecked] = useState(value);
  const [stored, setStored] = useState(value);
  if (value !== stored) {
    setStored(value);
    setChecked(value);
  }
  return (
    <label className="check">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => {
          setChecked(e.target.checked);
          onChange(e.target.checked);
        }}
      />
      {label}
    </label>
  );
}

export function PreferencesView() {
  const { t } = useTranslation();
  const { preferences, update, error } = usePreferences();
  return (
    <>
      <h1>{t('preferences.title')}</h1>
      <ErrorNote error={error} />
      <Field label={t('preferences.theme')}>
        <select value={preferences.theme} onChange={(e) => update({ theme: e.target.value })}>
          {THEMES.map((theme) => (
            <option key={theme} value={theme}>
              {t(`preferences.theme_${theme}`)}
            </option>
          ))}
        </select>
      </Field>
      <PreferenceSwitch
        label={t('preferences.shortcuts')}
        value={preferences.shortcuts}
        onChange={(shortcuts) => update({ shortcuts })}
      />
      <p className="muted">{t('preferences.shortcutsHelp')}</p>
      <PreferenceSwitch
        label={t('preferences.contents')}
        value={preferences.contents}
        onChange={(contents) => update({ contents })}
      />
      <PreferenceSwitch
        label={t('preferences.hoverCards')}
        value={preferences.hover_cards}
        onChange={(hover_cards) => update({ hover_cards })}
      />
    </>
  );
}
