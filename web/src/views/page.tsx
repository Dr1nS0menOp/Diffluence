// SPDX-License-Identifier: Apache-2.0
import { useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type Attachment,
  type DiffBlock,
  type DirectoryEntry,
  type Me,
  type NamedSubject,
  type Page,
  type PageDetail,
  type Restrictions,
  type TreeNode,
  type Version,
} from '../api';
import { Link, navigate, useLocation } from '../router';
import { DocumentView } from '../components/editor';
import { SubjectPicker } from '../components/picker';
import { Dialog, ErrorNote, Field, formatDate, formatSize, Loading } from '../components/ui';

function usePage(id: string) {
  return useQuery({ queryKey: ['page', id], queryFn: () => api.get<PageDetail>(`/pages/${id}`) });
}

function Breadcrumbs({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  if (page.breadcrumbs.length === 0) return null;
  return (
    <nav aria-label={t('page.breadcrumbs')} className="crumbs">
      {page.breadcrumbs.map((c) => (
        <span key={c.id}>
          <Link href={`/pages/${c.id}`}>{c.title}</Link>
          <span aria-hidden="true"> / </span>
        </span>
      ))}
    </nav>
  );
}

function Attachments({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const file = useRef<HTMLInputElement>(null);
  const key = ['attachments', page.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ attachments: Attachment[] }>(`/pages/${page.id}/attachments`),
  });
  const upload = useMutation({
    mutationFn: (f: File) =>
      api.upload(`/pages/${page.id}/attachments?filename=${encodeURIComponent(f.name)}`, f),
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/attachments/${id}`),
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  const list = q.data?.attachments ?? [];
  return (
    <section className="attachments">
      <h2>{t('attachments.title')}</h2>
      <ErrorNote error={upload.error ?? remove.error} />
      {list.length === 0 && <p className="muted">{t('attachments.none')}</p>}
      <ul className="plain-list">
        {list.map((a) => (
          <li key={a.id} className="row">
            {a.inline && <img className="thumb" src={`/api/v1/attachments/${a.id}/content`} alt="" />}
            <a href={`/api/v1/attachments/${a.id}/content?download=1`}>{a.filename}</a>
            <span className="muted">
              {t('attachments.version', { number: a.version })} · {formatSize(a.size)}
            </span>
            {page.can['attachment.add'] && (
              <button type="button" className="ghost" onClick={() => remove.mutate(a.id)}>
                {t('common.delete')}
              </button>
            )}
          </li>
        ))}
      </ul>
      {page.can['attachment.add'] && (
        <>
          <input
            ref={file}
            type="file"
            aria-label={t('attachments.upload')}
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) upload.mutate(f);
              e.target.value = '';
            }}
          />
        </>
      )}
    </section>
  );
}

function SubjectList({
  title,
  subjects,
  onChange,
  editable,
}: {
  title: string;
  subjects: NamedSubject[];
  onChange: (next: NamedSubject[]) => void;
  editable: boolean;
}) {
  const { t } = useTranslation();
  return (
    <section>
      <h3>{title}</h3>
      {subjects.length === 0 && <p className="muted">{t('restrictions.everyone')}</p>}
      <ul className="plain-list">
        {subjects.map((s) => (
          <li key={s.type + s.id} className="row">
            {s.name || t('common.unknown')}
            {editable && (
              <button
                type="button"
                className="ghost"
                onClick={() => onChange(subjects.filter((x) => x.id !== s.id))}
              >
                {t('common.remove')}
              </button>
            )}
          </li>
        ))}
      </ul>
      {editable && (
        <SubjectPicker
          label={t('common.add')}
          onPick={(e: DirectoryEntry) => onChange([...subjects, { type: e.type, id: e.id, name: e.name }])}
        />
      )}
    </section>
  );
}

function WhyPanel({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const [who, setWho] = useState<DirectoryEntry | null>(null);
  const [action, setAction] = useState('page.view');
  const q = useQuery({
    queryKey: ['why', page.id, who?.id, action],
    queryFn: () =>
      api.get<{ allowed: boolean; checks: { layer: string; rule: string; passed: boolean }[] }>(
        `/pages/${page.id}/explain?user_id=${who?.id}&action=${action}`,
      ),
    enabled: !!who,
  });
  return (
    <section>
      <h3>{t('page.why')}</h3>
      <div className="row">
        <SubjectPicker label={t('restrictions.whyUser')} usersOnly onPick={setWho} />
        <select
          aria-label={t('restrictions.action')}
          value={action}
          onChange={(e) => setAction(e.target.value)}
        >
          <option value="page.view">{t('restrictions.actions_view')}</option>
          <option value="page.edit">{t('restrictions.actions_edit')}</option>
        </select>
      </div>
      {who && <p>{who.name}</p>}
      <ErrorNote error={q.error} />
      {q.data && (
        <>
          <p className={q.data.allowed ? 'note' : 'note note-error'} role="status">
            {q.data.allowed ? t('restrictions.allowed') : t('restrictions.denied')}
          </p>
          <table>
            <tbody>
              {q.data.checks.map((c) => (
                <tr key={c.layer + c.rule}>
                  <td>{c.layer}</td>
                  <td>{c.rule}</td>
                  <td>{c.passed ? t('restrictions.passed') : t('restrictions.failed')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </section>
  );
}

function RestrictionsDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['restrictions', page.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<Restrictions>(`/pages/${page.id}/restrictions`),
  });
  const set = useMutation({
    mutationFn: (v: { kind: string; subjects: NamedSubject[] }) =>
      api.put(`/pages/${page.id}/restrictions`, {
        kind: v.kind,
        subjects: v.subjects.map(({ type, id }) => ({ type, id })),
      }),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: key });
      await client.invalidateQueries({ queryKey: ['page', page.id] });
    },
  });
  const editable = page.can['restriction.manage'] ?? false;
  return (
    <Dialog title={t('restrictions.title')} onClose={onClose} wide>
      <p className="muted">{t('restrictions.help')}</p>
      <ErrorNote error={set.error ?? q.error} />
      {q.data && (
        <>
          {q.data.inherited.length > 0 && (
            <>
              <p>{t('restrictions.inherited')}</p>
              <ul className="plain-list">
                {q.data.inherited.map((i) => (
                  <li key={i.page_id}>
                    <Link href={`/pages/${i.page_id}`}>{i.title}</Link>
                  </li>
                ))}
              </ul>
            </>
          )}
          <SubjectList
            title={t('restrictions.view')}
            subjects={q.data.view}
            editable={editable}
            onChange={(subjects) => set.mutate({ kind: 'view', subjects })}
          />
          <SubjectList
            title={t('restrictions.edit')}
            subjects={q.data.edit}
            editable={editable}
            onChange={(subjects) => set.mutate({ kind: 'edit', subjects })}
          />
        </>
      )}
      {page.can['space.administer'] && <WhyPanel page={page} />}
    </Dialog>
  );
}

function MoveDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const tree = useQuery({
    queryKey: ['tree', page.space_id],
    queryFn: () => api.get<{ pages: TreeNode[] }>(`/spaces/${page.space_id}/tree`),
  });
  const [parent, setParent] = useState(page.parent_id ?? '');
  const move = useMutation({
    mutationFn: () => api.post(`/pages/${page.id}/move`, { parent_id: parent || null }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      onClose();
    },
  });
  return (
    <Dialog title={t('page.move')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          move.mutate();
        }}
      >
        <ErrorNote error={move.error} />
        <Field label={t('page.moveTo')}>
          <select value={parent} onChange={(e) => setParent(e.target.value)}>
            <option value="">{t('page.topLevel')}</option>
            {tree.data?.pages
              .filter((n) => n.id !== page.id && n.status === 'published')
              .map((n) => (
                <option key={n.id} value={n.id}>
                  {n.title}
                </option>
              ))}
          </select>
        </Field>
        <button type="submit" className="primary">
          {t('page.move')}
        </button>
      </form>
    </Dialog>
  );
}

function RenameDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [title, setTitle] = useState(page.title);
  const rename = useMutation({
    mutationFn: () => api.patch(`/pages/${page.id}`, { title }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
      onClose();
    },
  });
  return (
    <Dialog title={t('page.rename')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          rename.mutate();
        }}
      >
        <ErrorNote error={rename.error} />
        <Field label={t('page.newTitle')}>
          <input value={title} onChange={(e) => setTitle(e.target.value)} required autoFocus />
        </Field>
        <button type="submit" className="primary">
          {t('common.save')}
        </button>
      </form>
    </Dialog>
  );
}

function CopyDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [children, setChildren] = useState(false);
  const copy = useMutation({
    mutationFn: () =>
      api.post<Page>(`/pages/${page.id}/copy`, { parent_id: page.parent_id, include_children: children }),
    onSuccess: async (p) => {
      await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
      navigate(`/pages/${p.id}`);
    },
  });
  return (
    <Dialog title={t('page.copy')} onClose={onClose}>
      <ErrorNote error={copy.error} />
      <label className="check">
        <input type="checkbox" checked={children} onChange={(e) => setChildren(e.target.checked)} />
        {t('page.copyChildren')}
      </label>
      <button type="button" className="primary" onClick={() => copy.mutate()}>
        {t('page.copy')}
      </button>
    </Dialog>
  );
}

export function PageView({ pageId }: { pageId: string; me: Me }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = usePage(pageId);
  const [dialog, setDialog] = useState('');
  const favourite = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/pages/${pageId}/favourite`) : api.del(`/pages/${pageId}/favourite`),
    onSettled: () => client.invalidateQueries({ queryKey: ['page', pageId] }),
  });
  const remove = useMutation({
    mutationFn: () => api.del(`/pages/${pageId}`),
    onSuccess: async () => {
      const space = q.data?.space_id;
      await client.invalidateQueries({ queryKey: ['tree', space] });
      client.removeQueries({ queryKey: ['page', pageId] });
      navigate('/');
    },
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const page = q.data;
  const close = () => setDialog('');
  const canEdit = page.can['page.edit'];
  return (
    <article>
      <Breadcrumbs page={page} />
      <header className="page-head">
        <h1>{page.title}</h1>
        {page.restricted && <span className="badge">{t('page.restricted')}</span>}
      </header>
      <div className="row actions">
        {canEdit && (
          <Link className="button primary" href={`/pages/${page.id}/edit`}>
            {t('page.edit')}
          </Link>
        )}
        <Link className="button" href={`/pages/${page.id}/history`}>
          {t('page.history')}
        </Link>
        <button type="button" onClick={() => favourite.mutate(!page.favourite)} aria-pressed={page.favourite}>
          {page.favourite ? t('page.unfavourite') : t('page.favourite')}
        </button>
        <button type="button" onClick={() => setDialog('restrictions')}>
          {t('page.restrictions')}
        </button>
        {canEdit && (
          <>
            <button type="button" onClick={() => setDialog('rename')}>
              {t('page.rename')}
            </button>
            <button type="button" onClick={() => setDialog('move')}>
              {t('page.move')}
            </button>
          </>
        )}
        {page.can['page.create'] && page.current_version > 0 && (
          <button type="button" onClick={() => setDialog('copy')}>
            {t('page.copy')}
          </button>
        )}
        {page.can['space.export'] && page.current_version > 0 && (
          <>
            <a className="button" href={`/api/v1/pages/${page.id}/export.md`}>
              {t('page.exportMd')}
            </a>
            <a className="button" href={`/api/v1/pages/${page.id}/export.zip`}>
              {t('page.exportZip')}
            </a>
          </>
        )}
        {page.can['page.delete'] && (
          <button
            type="button"
            className="danger"
            onClick={() => {
              if (window.confirm(t('page.confirmDelete'))) remove.mutate();
            }}
          >
            {t('page.delete')}
          </button>
        )}
      </div>
      <ErrorNote error={remove.error ?? favourite.error} />
      {page.status === 'draft' && (
        <p role="status" className="note">
          {t('page.unpublished')}
        </p>
      )}
      {page.has_draft && page.status !== 'draft' && (
        <p role="status" className="note">
          <Link href={`/pages/${page.id}/edit`}>{t('page.draftExists')}</Link>
        </p>
      )}
      <DocumentView document={page.document} />
      <Attachments page={page} />
      {dialog === 'restrictions' && <RestrictionsDialog page={page} onClose={close} />}
      {dialog === 'move' && <MoveDialog page={page} onClose={close} />}
      {dialog === 'rename' && <RenameDialog page={page} onClose={close} />}
      {dialog === 'copy' && <CopyDialog page={page} onClose={close} />}
    </article>
  );
}

export function HistoryView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const page = usePage(pageId);
  const q = useQuery({
    queryKey: ['versions', pageId],
    queryFn: () => api.get<{ versions: Version[] }>(`/pages/${pageId}/versions`),
  });
  const [picked, setPicked] = useState<number[]>([]);
  const [shown, setShown] = useState<number | null>(null);
  const version = useQuery({
    queryKey: ['version', pageId, shown],
    queryFn: () => api.get<Version>(`/pages/${pageId}/versions/${shown}`),
    enabled: shown !== null,
  });
  const restore = useMutation({
    mutationFn: (n: number) => api.post(`/pages/${pageId}/versions/${n}/restore`),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['page', pageId] });
      await client.invalidateQueries({ queryKey: ['versions', pageId] });
      navigate(`/pages/${pageId}`);
    },
  });
  const toggle = (n: number) =>
    setPicked((p) => (p.includes(n) ? p.filter((x) => x !== n) : [...p.slice(-1), n]));
  const pair = [...picked].sort((a, b) => a - b);
  const canEdit = page.data?.can['page.edit'];
  return (
    <>
      <h1>{t('history.title')}</h1>
      <p>
        <Link href={`/pages/${pageId}`}>{page.data?.title}</Link>
      </p>
      <ErrorNote error={q.error ?? restore.error} />
      <table>
        <thead>
          <tr>
            <th>{t('history.pick')}</th>
            <th>{t('common.name')}</th>
            <th>{t('common.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {q.data?.versions.map((v, i) => (
            <tr key={v.number}>
              <td>
                <input
                  type="checkbox"
                  aria-label={t('history.version', { number: v.number })}
                  checked={picked.includes(v.number)}
                  onChange={() => toggle(v.number)}
                />
              </td>
              <td>
                <strong>{t('history.version', { number: v.number })}</strong>{' '}
                {t('history.by', { name: v.author_name })}{' '}
                <span className="muted">{formatDate(v.created_at)}</span>
                {i === 0 && <span className="badge">{t('history.current')}</span>}
                {v.restored_from !== null && (
                  <span className="muted"> · {t('history.restoredFrom', { number: v.restored_from })}</span>
                )}
                {v.summary && <div>{v.summary}</div>}
              </td>
              <td className="row">
                <button type="button" className="ghost" onClick={() => setShown(v.number)}>
                  {t('history.view')}
                </button>
                {canEdit && i > 0 && (
                  <button
                    type="button"
                    className="ghost"
                    onClick={() => {
                      if (window.confirm(t('history.confirmRestore'))) restore.mutate(v.number);
                    }}
                  >
                    {t('history.restore')}
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {pair.length === 2 && (
        <Link className="button primary" href={`/pages/${pageId}/compare?from=${pair[0]}&to=${pair[1]}`}>
          {t('history.compareSelected')}
        </Link>
      )}
      {shown !== null && (
        <Dialog title={t('history.version', { number: shown })} onClose={() => setShown(null)} wide>
          {version.data?.document ? <DocumentView document={version.data.document} /> : <Loading />}
        </Dialog>
      )}
    </>
  );
}

function Block({ block }: { block: DiffBlock }) {
  if (block.op === 'changed' && block.words) {
    return (
      <p className="diff changed">
        {block.words.map((w, i) => (
          <span key={i} className={w.op}>
            {w.text}{' '}
          </span>
        ))}
      </p>
    );
  }
  return (
    <p className={'diff ' + block.op}>{block.op === 'added' ? block.after : block.before || block.after}</p>
  );
}

export function CompareView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const { searchParams } = useLocation();
  const from = searchParams.get('from');
  const to = searchParams.get('to');
  const q = useQuery({
    queryKey: ['compare', pageId, from, to],
    queryFn: () =>
      api.get<{ from: Version; to: Version; blocks: DiffBlock[] }>(
        `/pages/${pageId}/compare?from=${from}&to=${to}`,
      ),
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const changed = q.data.blocks.some((b) => b.op !== 'equal') || q.data.from.title !== q.data.to.title;
  return (
    <>
      <h1>{t('editor.compare')}</h1>
      <p>
        <Link href={`/pages/${pageId}/history`}>{t('history.title')}</Link>
      </p>
      <p>
        {t('history.version', { number: q.data.from.number })} →{' '}
        {t('history.version', { number: q.data.to.number })}
      </p>
      {!changed && <p className="muted">{t('history.unchanged')}</p>}
      <div className="compare">
        {q.data.blocks.map((b, i) => (
          <Block key={i} block={b} />
        ))}
      </div>
    </>
  );
}
