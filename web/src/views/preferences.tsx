// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DATE_FORMATS } from '../dates';
import { usePreferences } from '../preferences';
import { ErrorNote, Field } from '../components/ui';
import { AccountNav } from './account';

const THEMES = ['system', 'light', 'dark'];
const LANGUAGES = ['en'];
const FIRST_DAYS = [1, 0, 6];

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
      <h1>{t('profile.title')}</h1>
      <AccountNav />
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
      <Field label={t('preferences.language')}>
        <select value={preferences.language} onChange={(e) => update({ language: e.target.value })}>
          {LANGUAGES.map((language) => (
            <option key={language} value={language}>
              {t(`preferences.language_${language}`)}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('preferences.timeZone')}>
        <select value={preferences.time_zone} onChange={(e) => update({ time_zone: e.target.value })}>
          <option value="">{t('preferences.timeZoneDevice')}</option>
          {Intl.supportedValuesOf('timeZone').map((zone) => (
            <option key={zone} value={zone}>
              {zone}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('preferences.dateFormat')}>
        <select value={preferences.date_format} onChange={(e) => update({ date_format: e.target.value })}>
          {DATE_FORMATS.map((format) => (
            <option key={format} value={format}>
              {t(`preferences.dateFormat_${format}`)}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('preferences.firstDay')} help={t('preferences.firstDayHelp')}>
        <select value={preferences.first_day} onChange={(e) => update({ first_day: Number(e.target.value) })}>
          {FIRST_DAYS.map((day) => (
            <option key={day} value={day}>
              {t(`preferences.firstDay_${day}`)}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('preferences.startScreen')}>
        <select
          value={preferences.start_screen ?? ''}
          onChange={(e) => update({ start_screen: e.target.value })}
        >
          <option value="">{t('preferences.start_company')}</option>
          <option value="personal">{t('preferences.start_personal')}</option>
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
