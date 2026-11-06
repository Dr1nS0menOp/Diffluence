// SPDX-License-Identifier: Apache-2.0
import { useTranslation } from 'react-i18next';
import { animalPicture, ANIMALS, isAnimal } from './animals';
import { Field, SpaceBadge } from './ui';

const OWN_ICON_LENGTH = 8;

/** Picks the icon of a space: an animal, one from its key, or a character of its own. */
export function AnimalChoice({
  spaceKey,
  name,
  value,
  onChange,
}: {
  spaceKey: string;
  name: string;
  value: string;
  onChange: (icon: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <fieldset className="animal-choice">
      <legend>{t('space.icon')}</legend>
      <div className="animal-now">
        <SpaceBadge space={{ key: spaceKey, name, icon: value }} size="large" />
        {value === '' ? (
          <span className="muted">{t('space.iconFromKey')}</span>
        ) : (
          <button type="button" className="ghost" onClick={() => onChange('')}>
            {t('space.iconUseKey')}
          </button>
        )}
      </div>
      <div className="animal-grid">
        {ANIMALS.map((animal) => (
          <button
            key={animal}
            type="button"
            className="ghost"
            aria-label={t(`animals.${animal}`)}
            title={t(`animals.${animal}`)}
            aria-pressed={animal === value}
            onClick={() => onChange(animal)}
          >
            <img src={animalPicture(animal)} alt="" />
          </button>
        ))}
      </div>
      <Field label={t('space.iconOwn')}>
        <input
          value={isAnimal(value) ? '' : value}
          maxLength={OWN_ICON_LENGTH}
          onChange={(e) => onChange(e.target.value)}
        />
      </Field>
    </fieldset>
  );
}
