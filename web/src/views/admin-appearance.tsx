// SPDX-License-Identifier: Apache-2.0
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api';
import { appearanceQuery } from '../appearance';
import { ErrorNote, FilePicker } from '../components/ui';

/** The system font is the default; an organisation can bring its own. */
export function AppearanceSection() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery(appearanceQuery);
  const refresh = () => client.invalidateQueries({ queryKey: appearanceQuery.queryKey });
  const importFont = useMutation({
    mutationFn: (file: File) =>
      api.replace(`/admin/appearance/font?filename=${encodeURIComponent(file.name)}`, file),
    onSettled: refresh,
  });
  const removeFont = useMutation({
    mutationFn: () => api.del('/admin/appearance/font'),
    onSettled: refresh,
  });
  const font = q.data?.font;
  return (
    <section>
      <h2>{t('admin.appearance')}</h2>
      <p role="status">{font ? t('admin.fontImported', { name: font.name }) : t('admin.fontSystem')}</p>
      <p className="muted">{t('admin.fontHelp')}</p>
      <ErrorNote error={q.error ?? importFont.error ?? removeFont.error} />
      <div className="row">
        <FilePicker
          label={t('admin.fontImport')}
          accept=".woff2,.woff,font/woff2,font/woff"
          onPick={(file) => importFont.mutate(file)}
        />
        {font && (
          <button type="button" onClick={() => removeFont.mutate()}>
            {t('admin.fontRemove')}
          </button>
        )}
      </div>
    </section>
  );
}
