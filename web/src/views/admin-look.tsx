// SPDX-License-Identifier: Apache-2.0
// The look of the instance (spec 013, F10). The server checks the contrast of an accent:
// these forms show its answer, and a preview in both themes.
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api';
import { appearanceQuery } from '../appearance';
import { ConfirmButton, ErrorNote, Field, FilePicker } from '../components/ui';
import { SectionForm } from './admin-rules';

const LOGO_TYPES = 'image/png,image/jpeg,image/webp';
const BANNER_LEVELS = ['info', 'warning'];

interface Look {
  instance_name: string;
  accent_light: string;
  accent_dark: string;
  login_welcome: string;
  terms_url: string;
  banner_text: string;
  banner_level: string;
  banner_ends: string | null;
  welcome_page: string;
}

function LogoSection() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const logo = useQuery(appearanceQuery).data?.logo;
  const refresh = () => client.invalidateQueries({ queryKey: appearanceQuery.queryKey });
  const set = useMutation({
    mutationFn: (file: File) => api.replace('/admin/appearance/logo', file),
    onSettled: refresh,
  });
  const remove = useMutation({ mutationFn: () => api.del('/admin/appearance/logo'), onSettled: refresh });
  return (
    <section>
      <h2>{t('instance.logo')}</h2>
      <p className="muted">{t('instance.logoHelp')}</p>
      <ErrorNote error={set.error ?? remove.error} />
      <div className="row">
        {logo && <img src={logo} alt={t('instance.logo')} className="brand-logo" />}
        <FilePicker label={t('instance.logoUpload')} accept={LOGO_TYPES} onPick={(f) => set.mutate(f)} />
        {logo && (
          <button type="button" onClick={() => remove.mutate()}>
            {t('common.remove')}
          </button>
        )}
      </div>
    </section>
  );
}

function ColourField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (next: string) => void;
}) {
  return (
    <Field label={label}>
      <input value={value} onChange={(e) => onChange(e.target.value)} />
    </Field>
  );
}

function LookForm() {
  const { t } = useTranslation();
  return (
    <SectionForm<Look> section="look" title={t('instance.title')}>
      {(form, set) => (
        <>
          <Field label={t('instance.instanceName')}>
            <input value={form.instance_name} onChange={(e) => set({ instance_name: e.target.value })} />
          </Field>
          <h3>{t('instance.accent')}</h3>
          <p className="muted">{t('instance.accentHelp')}</p>
          <ColourField
            label={t('instance.accentLight')}
            value={form.accent_light}
            onChange={(accent_light) => set({ accent_light })}
          />
          <ColourField
            label={t('instance.accentDark')}
            value={form.accent_dark}
            onChange={(accent_dark) => set({ accent_dark })}
          />
          <Field label={t('instance.loginWelcome')}>
            <textarea
              rows={2}
              value={form.login_welcome}
              onChange={(e) => set({ login_welcome: e.target.value })}
            />
          </Field>
          <Field label={t('instance.termsUrl')}>
            <input type="url" value={form.terms_url} onChange={(e) => set({ terms_url: e.target.value })} />
          </Field>
          <h3>{t('instance.banner')}</h3>
          <Field label={t('instance.bannerText')}>
            <input value={form.banner_text} onChange={(e) => set({ banner_text: e.target.value })} />
          </Field>
          <Field label={t('instance.bannerLevel')}>
            <select
              value={form.banner_level || 'info'}
              onChange={(e) => set({ banner_level: e.target.value })}
            >
              {BANNER_LEVELS.map((level) => (
                <option key={level} value={level}>
                  {t(`instance.bannerLevel_${level}`)}
                </option>
              ))}
            </select>
          </Field>
          <Field label={t('instance.bannerEnds')}>
            <input
              type="date"
              value={form.banner_ends?.slice(0, 10) ?? ''}
              onChange={(e) =>
                set({ banner_ends: e.target.value ? new Date(e.target.value).toISOString() : null })
              }
            />
          </Field>
          <Field label={t('instance.welcomePage')} help={t('instance.welcomePageHelp')}>
            <input value={form.welcome_page} onChange={(e) => set({ welcome_page: e.target.value })} />
          </Field>
        </>
      )}
    </SectionForm>
  );
}

export function LookTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const reset = useMutation({
    mutationFn: () => api.post('/admin/appearance/reset'),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: appearanceQuery.queryKey });
      await client.invalidateQueries({ queryKey: ['section', 'look'] });
    },
  });
  return (
    <>
      <LogoSection />
      <LookForm />
      <section>
        <h2>{t('instance.reset')}</h2>
        <p className="muted">{t('instance.resetHelp')}</p>
        <ErrorNote error={reset.error} />
        <ConfirmButton
          label={t('instance.reset')}
          message={t('instance.resetConfirm')}
          className="danger"
          onConfirm={() => reset.mutate()}
        />
      </section>
    </>
  );
}
