// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { ConfirmButton, Dialog, ErrorNote, Field, Loading, RelativeTime } from '../components/ui';

interface Endpoint {
  id: string;
  address: string;
  events: string[];
  with_titles: boolean;
  enabled: boolean;
  failures: number;
}
interface Delivery {
  id: string;
  event_type: string;
  state: string;
  attempts: number;
  last_status: number;
  created_at: string;
}
interface Listing {
  webhooks: Endpoint[];
  events: string[];
}

function ShownSecret({ secret, onClose }: { secret: string; onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <Dialog title={t('webhooks.secretTitle')} onClose={onClose}>
      <p>{t('webhooks.secretHelp')}</p>
      <p>
        <code>{secret}</code>
      </p>
      <div className="dialog-actions">
        <button type="button" className="primary" onClick={onClose}>
          {t('common.close')}
        </button>
      </div>
    </Dialog>
  );
}

function NewEndpoint({
  path,
  events,
  onSecret,
}: {
  path: string;
  events: string[];
  onSecret: (s: string) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [address, setAddress] = useState('');
  const [chosen, setChosen] = useState<string[]>([]);
  const [withTitles, setWithTitles] = useState(false);
  const create = useMutation({
    mutationFn: () =>
      api.post<{ secret: string }>(path, { address, events: chosen, with_titles: withTitles }),
    onSuccess: async (r) => {
      setAddress('');
      setChosen([]);
      onSecret(r.secret);
      await client.invalidateQueries({ queryKey: ['webhooks', path] });
    },
  });
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      <h2>{t('webhooks.new')}</h2>
      <ErrorNote error={create.error} />
      <Field label={t('webhooks.address')} help={t('webhooks.addressHelp')}>
        <input type="url" value={address} onChange={(e) => setAddress(e.target.value)} required />
      </Field>
      <fieldset className="matrix">
        <legend>{t('webhooks.events')}</legend>
        {events.map((name) => (
          <label key={name}>
            <input
              type="checkbox"
              checked={chosen.includes(name)}
              onChange={(e) =>
                setChosen(e.target.checked ? [...chosen, name] : chosen.filter((x) => x !== name))
              }
            />
            {name}
          </label>
        ))}
      </fieldset>
      <label className="check">
        <input type="checkbox" checked={withTitles} onChange={(e) => setWithTitles(e.target.checked)} />
        {t('webhooks.withTitles')}
      </label>
      <button type="submit" className="primary" disabled={create.isPending}>
        {t('webhooks.add')}
      </button>
    </form>
  );
}

function DeliveryLog({ endpoint, onClose }: { endpoint: Endpoint; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['webhook-log', endpoint.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ deliveries: Delivery[] }>(`/webhooks/${endpoint.id}/deliveries`),
  });
  const resend = useMutation({
    mutationFn: (id: string) => api.post(`/webhooks/${endpoint.id}/deliveries/${id}/resend`),
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  return (
    <Dialog title={t('webhooks.log')} onClose={onClose}>
      <ErrorNote error={q.error ?? resend.error} />
      {q.data?.deliveries.length === 0 && <p className="muted">{t('webhooks.noLog')}</p>}
      <ul className="plain-list">
        {q.data?.deliveries.map((d) => (
          <li key={d.id} className="row">
            <span className="grow">
              {d.event_type}: {t(`webhooks.state_${d.state}`)} ({d.last_status || '-'})
            </span>
            <span className="muted">
              <RelativeTime value={d.created_at} />
            </span>
            <button type="button" className="ghost" onClick={() => resend.mutate(d.id)}>
              {t('webhooks.resend')}
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}

function EndpointRow({
  endpoint,
  path,
  onSecret,
}: {
  endpoint: Endpoint;
  path: string;
  onSecret: (s: string) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [showLog, setShowLog] = useState(false);
  const refresh = () => client.invalidateQueries({ queryKey: ['webhooks', path] });
  const rotate = useMutation({
    mutationFn: () => api.post<{ secret: string }>(`/webhooks/${endpoint.id}/rotate`),
    onSuccess: (r) => onSecret(r.secret),
  });
  const remove = useMutation({ mutationFn: () => api.del(`/webhooks/${endpoint.id}`), onSettled: refresh });
  const toggle = useMutation({
    mutationFn: () =>
      api.patch(`/webhooks/${endpoint.id}`, {
        address: endpoint.address,
        events: endpoint.events,
        with_titles: endpoint.with_titles,
        enabled: !endpoint.enabled,
      }),
    onSettled: refresh,
  });
  return (
    <li className="row">
      <span className="grow">{endpoint.address}</span>
      {!endpoint.enabled && <span className="lozenge">{t('webhooks.off')}</span>}
      <button type="button" className="ghost" onClick={() => setShowLog(true)}>
        {t('webhooks.log')}
      </button>
      <button type="button" className="ghost" onClick={() => toggle.mutate()}>
        {endpoint.enabled ? t('webhooks.turnOff') : t('webhooks.turnOn')}
      </button>
      <button type="button" className="ghost" onClick={() => rotate.mutate()}>
        {t('webhooks.rotate')}
      </button>
      <ConfirmButton
        label={t('common.delete')}
        message={t('webhooks.deleteConfirm', { address: endpoint.address })}
        className="ghost"
        onConfirm={() => remove.mutate()}
      />
      <ErrorNote error={rotate.error ?? remove.error ?? toggle.error} />
      {showLog && <DeliveryLog endpoint={endpoint} onClose={() => setShowLog(false)} />}
    </li>
  );
}

/** The webhooks of a space, or of the instance, by the path of their list. */
export function WebhookList({ path }: { path: string }) {
  const { t } = useTranslation();
  const [secret, setSecret] = useState<string | null>(null);
  const q = useQuery({ queryKey: ['webhooks', path], queryFn: () => api.get<Listing>(path) });
  return (
    <>
      <p className="muted">{t('webhooks.help')}</p>
      <ErrorNote error={q.error} />
      {q.isPending && <Loading />}
      {q.data?.webhooks.length === 0 && <p className="muted">{t('webhooks.none')}</p>}
      <ul className="plain-list">
        {q.data?.webhooks.map((w) => (
          <EndpointRow key={w.id} endpoint={w} path={path} onSecret={setSecret} />
        ))}
      </ul>
      {q.data && <NewEndpoint path={path} events={q.data.events} onSecret={setSecret} />}
      {secret && <ShownSecret secret={secret} onClose={() => setSecret(null)} />}
    </>
  );
}

/** Space settings, Webhooks. */
export function WebhooksView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const q = useQuery(spaceByKeyQuery(spaceKey));
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!q.data) return <Loading />;
  return (
    <Frame space={q.data}>
      <h1>{t('webhooks.title')}</h1>
      <WebhookList path={`/spaces/${q.data.id}/webhooks`} />
    </Frame>
  );
}
