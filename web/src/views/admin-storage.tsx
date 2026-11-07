// SPDX-License-Identifier: Apache-2.0
// Administration: how much a space may store (FIL-020).
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api';
import { Dialog, ErrorNote, Field } from '../components/ui';

export function StorageLimitDialog({
  space,
  limitMiB,
  onClose,
}: {
  space: { id: string; name: string };
  limitMiB: number;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [value, setValue] = useState(limitMiB);
  const save = useMutation({
    mutationFn: () => api.put(`/admin/spaces/${space.id}/storage-limit`, { limit_mib: value }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['admin-spaces'] });
      onClose();
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  return (
    <Dialog title={t('admin.storageLimitOf', { name: space.name })} onClose={onClose}>
      <form onSubmit={submit}>
        <ErrorNote error={save.error} />
        <Field label={t('admin.storageLimit')} help={t('admin.storageLimitHelp')}>
          <input
            type="number"
            min={0}
            value={value}
            autoFocus
            onChange={(e) => setValue(Number(e.target.value))}
          />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
