// SPDX-License-Identifier: Apache-2.0
// A change of the console that lets more through than before, such as fewer backups kept or a
// weaker rule for logging in, is a system administrator's, with a second factor (spec 076).
import { useTranslation } from 'react-i18next';
import { ApiError } from '../api';
import { Field } from './ui';

/** What a confirmed change adds to a form: a second factor, and a reason where one is asked. */
export interface Confirmation {
  code: string;
  reason: string;
}

/** True for the refusal of a change that needs a confirmed change. */
export const needsConfirmation = (error: unknown) =>
  error instanceof ApiError && error.code === 'confirmation_required';

/** The fields of a confirmed change: a code, or nothing after a passkey just now, and a reason. */
export function ConfirmFields({
  value,
  onChange,
  note,
  withReason,
}: {
  value: Confirmation;
  onChange: (next: Confirmation) => void;
  note: string;
  withReason?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <p className="note">{note}</p>
      {withReason && (
        <Field label={t('rules.confirmReason')} help={t('rules.confirmReasonHelp')}>
          <input value={value.reason} onChange={(e) => onChange({ ...value, reason: e.target.value })} />
        </Field>
      )}
      <Field label={t('rules.confirmCode')} help={t('rules.confirmCodeHelp')}>
        <input
          value={value.code}
          onChange={(e) => onChange({ ...value, code: e.target.value })}
          autoComplete="one-time-code"
          inputMode="text"
        />
      </Field>
    </>
  );
}
