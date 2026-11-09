// SPDX-License-Identifier: Apache-2.0
// Space settings, Public site (spec 014, G15): a space's window to the world, with a
// preview before anything goes out.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PublicSite, type SiteBuild, type SiteChoice, type SiteReport, type Space } from '../api';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { ErrorNote, Field, Loading } from '../components/ui';
import { TableBox } from '../components/table-box';

// A build runs in the background, and this is how it shows up.
const POLL_MS = 2000;
const ACCENTS = ['blue', 'green', 'teal', 'purple', 'red', 'orange'];
const UPDATE_MODES = ['manual', 'publish', 'daily'];

function NewSite({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState('');
  const create = useMutation({
    mutationFn: () => api.post<PublicSite>(`/spaces/${space.id}/sites`, { name }),
    onSuccess: async () => {
      setName('');
      await client.invalidateQueries({ queryKey: ['sites', space.id] });
    },
  });
  return (
    <section>
      <h2>{t('sites.new')}</h2>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <ErrorNote error={create.error} />
        <Field label={t('common.name')}>
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <button type="submit" className="primary">
          {t('sites.create')}
        </button>
      </form>
    </section>
  );
}

function SiteDetails({ site, onSaved }: { site: PublicSite; onSaved: () => void }) {
  const { t } = useTranslation();
  const [form, setForm] = useState({
    name: site.name,
    address: site.address,
    language: site.language,
    accent: site.accent,
    show_names: site.show_names,
    search_engines: site.search_engines,
    update_mode: site.update_mode,
  });
  const save = useMutation({ mutationFn: () => api.patch(`/sites/${site.id}`, form), onSuccess: onSaved });
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ErrorNote error={save.error} />
      <Field label={t('common.name')}>
        <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
      </Field>
      <Field label={t('sites.address')} help={t('sites.addressHelp')}>
        <input value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} />
      </Field>
      <Field label={t('sites.language')}>
        <input value={form.language} onChange={(e) => setForm({ ...form, language: e.target.value })} />
      </Field>
      <Field label={t('sites.accent')}>
        <select value={form.accent} onChange={(e) => setForm({ ...form, accent: e.target.value })}>
          {ACCENTS.map((a) => (
            <option key={a} value={a}>
              {t(`sites.accent_${a}`)}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('sites.updates')}>
        <select value={form.update_mode} onChange={(e) => setForm({ ...form, update_mode: e.target.value })}>
          {UPDATE_MODES.map((m) => (
            <option key={m} value={m}>
              {t(`sites.update_${m}`)}
            </option>
          ))}
        </select>
      </Field>
      <label className="check">
        <input
          type="checkbox"
          checked={form.show_names}
          onChange={(e) => setForm({ ...form, show_names: e.target.checked })}
        />{' '}
        {t('sites.showNames')}
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={form.search_engines}
          onChange={(e) => setForm({ ...form, search_engines: e.target.checked })}
        />{' '}
        {t('sites.searchEngines')}
      </label>
      <p className="muted">{t('sites.blockingIsARequest')}</p>
      <button type="submit">{t('common.save')}</button>
    </form>
  );
}

function ChoosePages({ site, onSaved }: { site: PublicSite; onSaved: () => void }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['site-pages', site.id],
    queryFn: () => api.get<{ pages: SiteChoice[] }>(`/sites/${site.id}/pages`),
  });
  const [picked, setPicked] = useState<Set<string> | null>(null);
  const pages = q.data?.pages ?? [];
  const chosen = picked ?? new Set(pages.filter((p) => p.chosen).map((p) => p.id));
  const save = useMutation({
    mutationFn: () => api.put(`/sites/${site.id}/pages`, { page_ids: [...chosen] }),
    onSuccess: onSaved,
  });
  const toggle = (id: string, on: boolean) => {
    const next = new Set(chosen);
    if (on) next.add(id);
    else next.delete(id);
    setPicked(next);
  };
  if (q.error) return <ErrorNote error={q.error} />;
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ErrorNote error={save.error} />
      <fieldset className="choices">
        <legend>{t('sites.pages')}</legend>
        <p className="muted">{t('sites.pagesHelp')}</p>
        <ul className="choose-list">
          {pages.map((p) => (
            <li key={p.id}>
              <label className="check">
                <input
                  type="checkbox"
                  disabled={p.blocked}
                  checked={chosen.has(p.id)}
                  onChange={(e) => toggle(p.id, e.target.checked)}
                />{' '}
                {p.title}
              </label>
              {p.blocked && <span className="muted"> {t('sites.blocked')}</span>}
            </li>
          ))}
        </ul>
        <p role="status" className="muted">
          {t('exchange.chosenCount', { count: chosen.size })}
        </p>
      </fieldset>
      <button type="submit">{t('common.save')}</button>
    </form>
  );
}

function ReportList({ title, items }: { title: string; items: string[] }) {
  if (items.length === 0) return null;
  return (
    <>
      <h4>{title}</h4>
      <ul>
        {[...new Set(items)].map((item) => (
          <li key={item}>{item}</li>
        ))}
      </ul>
    </>
  );
}

function PreviewAndPublish({ site, onDone }: { site: PublicSite; onDone: () => void }) {
  const { t } = useTranslation();
  const [confirm, setConfirm] = useState('');
  const preview = useMutation({ mutationFn: () => api.post<SiteReport>(`/sites/${site.id}/preview`) });
  const publish = useMutation({
    mutationFn: () => api.post(`/sites/${site.id}/publish`, { confirm }),
    onSuccess: onDone,
  });
  const report = preview.data;
  const first = site.live_build === 0;
  return (
    <div>
      <ErrorNote error={preview.error ?? publish.error} />
      <p>
        <button type="button" onClick={() => preview.mutate()}>
          {t('sites.preview')}
        </button>
      </p>
      {report && (
        <div role="status">
          <ReportList title={t('sites.added')} items={report.added} />
          <ReportList title={t('sites.changed')} items={report.changed} />
          <ReportList title={t('sites.removed')} items={report.removed} />
          <ReportList title={t('sites.leftOut')} items={report.left_out} />
          <ReportList title={t('sites.deadLinks')} items={report.dead_links} />
          <ReportList title={t('sites.noAlt')} items={report.no_alt} />
          <ReportList title={t('sites.metadata')} items={report.metadata} />
          {first && (
            <Field label={t('sites.typeName', { name: site.name })}>
              <input value={confirm} onChange={(e) => setConfirm(e.target.value)} />
            </Field>
          )}
          <button
            type="button"
            className="primary"
            disabled={first && confirm !== site.name}
            onClick={() => publish.mutate()}
          >
            {t('sites.publish')}
          </button>
        </div>
      )}
    </div>
  );
}

function Builds({ site, onDone }: { site: PublicSite; onDone: () => void }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['site-builds', site.id],
    queryFn: () => api.poll<{ builds: SiteBuild[] }>(`/sites/${site.id}/builds`),
    refetchInterval: POLL_MS,
  });
  const rollback = useMutation({
    mutationFn: (number: number) => api.post(`/sites/${site.id}/rollback`, { number }),
    onSuccess: onDone,
  });
  const withdraw = useMutation({
    mutationFn: () => api.post(`/sites/${site.id}/withdraw`),
    onSuccess: onDone,
  });
  const builds = q.data?.builds ?? [];
  return (
    <div>
      <ErrorNote error={rollback.error ?? withdraw.error} />
      {builds.length > 0 && (
        <TableBox>
          <table>
            <caption>{t('sites.builds')}</caption>
            <thead>
              <tr>
                <th scope="col">{t('sites.build')}</th>
                <th scope="col">{t('sites.state')}</th>
                <th scope="col">{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {builds.map((b) => (
                <tr key={b.number}>
                  <th scope="row">{b.number}</th>
                  <td>
                    {t(`sites.state_${b.state}`)}
                    {b.report.stopped ? ` · ${b.report.stopped}` : ''}
                  </td>
                  <td>
                    {b.state === 'old' && (
                      <button type="button" className="quiet" onClick={() => rollback.mutate(b.number)}>
                        {t('sites.rollback')}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
      {site.live_build > 0 && (
        <div className="row">
          <a className="button" href={`/api/v1/sites/${site.id}/download`}>
            {t('sites.download')}
          </a>
          <button type="button" className="danger" onClick={() => withdraw.mutate()}>
            {t('exchange.withdraw')}
          </button>
        </div>
      )}
    </div>
  );
}

function SiteCard({ site, space }: { site: PublicSite; space: Space }) {
  const client = useQueryClient();
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['sites', space.id] });
    await client.invalidateQueries({ queryKey: ['site-builds', site.id] });
    await client.invalidateQueries({ queryKey: ['site-pages', site.id] });
  };
  return (
    <section>
      <h2>{site.name}</h2>
      <SiteDetails site={site} onSaved={refresh} />
      <ChoosePages site={site} onSaved={refresh} />
      <PreviewAndPublish site={site} onDone={refresh} />
      <Builds site={site} onDone={refresh} />
    </section>
  );
}

export function SitesView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const q = useQuery(spaceByKeyQuery(spaceKey));
  const space = q.data;
  const sites = useQuery({
    queryKey: ['sites', space?.id],
    queryFn: () => api.poll<{ sites: PublicSite[] }>(`/spaces/${space?.id}/sites`),
    enabled: !!space,
    refetchInterval: POLL_MS,
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!space) return <Loading />;
  return (
    <Frame space={space}>
      <h1>{t('sites.title')}</h1>
      <p className="muted">{t('sites.intro')}</p>
      <ErrorNote error={sites.error} />
      {sites.data?.sites.map((site) => (
        <SiteCard key={site.id} site={site} space={space} />
      ))}
      <NewSite space={space} />
    </Frame>
  );
}
