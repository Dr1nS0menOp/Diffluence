// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type Attachment,
  type DeleteImpact,
  type DirectoryEntry,
  type NamedSubject,
  type PageRef,
  type PageSummary,
  type Me,
  type Restrictions,
  type TreeNode,
} from '../api';
import { Link, navigate } from '../router';
import { SubjectPicker } from '../components/picker';
import { useToast } from '../components/toast';
import { countBelow } from '../components/tree-model';
import { Dialog, ErrorNote, Field, formatSize, Loading } from '../components/ui';
import { FileDetailsDialog } from './file-details';

/** Every file of a page. Files are added in the editor, by dropping them into the document. */
export function AttachmentsDialog({ page, onClose }: { page: PageSummary; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['attachments', page.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ attachments: Attachment[] }>(`/pages/${page.id}/attachments`),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/attachments/${id}`),
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  const [openId, setOpenId] = useState<string | null>(null);
  const canAdd = page.can['attachment.add'];
  const open = q.data?.attachments.find((a) => a.id === openId);
  if (open) {
    return (
      <FileDetailsDialog
        file={open}
        canChange={canAdd ?? false}
        canRemove={page.can['attachment.delete_any'] ?? false}
        onChanged={() => client.invalidateQueries({ queryKey: key })}
        onClose={() => setOpenId(null)}
      />
    );
  }
  return (
    <Dialog title={t('attachments.title')} onClose={onClose}>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? remove.error} />
      {q.data?.attachments.length === 0 && (
        <>
          <p className="muted">{t('attachments.none')}</p>
          {canAdd && page.can['page.edit'] && <p className="muted">{t('attachments.howToAdd')}</p>}
        </>
      )}
      <ul className="plain-list">
        {q.data?.attachments.map((a) => (
          <li key={a.id} className="row">
            {a.inline && <img className="thumb" src={`/api/v1/attachments/${a.id}/content`} alt="" />}
            <a href={`/api/v1/attachments/${a.id}/content?download=1`}>{a.filename}</a>
            <span className="muted">
              {t('attachments.version', { number: a.version })} · {formatSize(a.size)}
            </span>
            <button type="button" className="ghost" onClick={() => setOpenId(a.id)}>
              {t('files.details')}
            </button>
            {canAdd && (
              <button type="button" className="ghost" onClick={() => remove.mutate(a.id)}>
                {t('common.delete')}
              </button>
            )}
          </li>
        ))}
      </ul>
      {(q.data?.attachments.length ?? 0) > 0 && (
        <a href={`/api/v1/pages/${page.id}/attachments.zip`}>{t('files.downloadAll')}</a>
      )}
    </Dialog>
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

function WhyPanel({ page }: { page: PageSummary }) {
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

const RESTRICTION_STATES = ['open', 'view_only', 'private'] as const;
type RestrictionState = (typeof RESTRICTION_STATES)[number];

/** The three ways a page can stand, as a person thinks of them (ACC-064). */
export function restrictionState(r: { view: unknown[]; edit: unknown[] }): RestrictionState {
  if (r.view.length > 0) return 'private';
  return r.edit.length > 0 ? 'view_only' : 'open';
}

function RestrictionChoice({
  page,
  current,
  onChoose,
}: {
  page: PageSummary;
  current: RestrictionState;
  onChoose: (state: RestrictionState) => void;
}) {
  const { t } = useTranslation();
  const tree = useQuery({
    queryKey: ['tree', page.space_id],
    queryFn: () => api.get<{ pages: TreeNode[] }>(`/spaces/${page.space_id}/tree`),
  });
  const below = countBelow(tree.data?.pages ?? [], page.id);
  return (
    <fieldset className="restriction-states">
      <legend>{t('restrictions.state')}</legend>
      {RESTRICTION_STATES.map((state) => (
        <label key={state} className="check">
          <input
            type="radio"
            name="restriction-state"
            checked={current === state}
            onChange={() => onChoose(state)}
          />
          <span>
            {t(`restrictions.state_${state}`)}
            <small className="muted"> {t(`restrictions.stateHelp_${state}`)}</small>
          </span>
        </label>
      ))}
      {below > 0 && <p className="muted">{t('restrictions.below', { count: below })}</p>}
    </fieldset>
  );
}

export function RestrictionsDialog({ page, onClose }: { page: PageSummary; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['restrictions', page.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<Restrictions>(`/pages/${page.id}/restrictions`),
  });
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') }).data;
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: key });
    await client.invalidateQueries({ queryKey: ['page', page.id] });
  };
  const put = (kind: string, subjects: NamedSubject[]) =>
    api.put(`/pages/${page.id}/restrictions`, {
      kind,
      subjects: subjects.map(({ type, id }) => ({ type, id })),
    });
  const set = useMutation({
    mutationFn: (v: { kind: string; subjects: NamedSubject[] }) => put(v.kind, v.subjects),
    onSettled: refresh,
  });
  // The person choosing starts on the list: the server would add them anyway (ACC-065).
  const startWith = (list: NamedSubject[]) =>
    list.length > 0 || !me ? list : [{ type: 'user', id: me.user.id, name: me.user.display_name }];
  const [chosen, setChosen] = useState<RestrictionState | null>(null);
  const choose = useMutation({
    mutationFn: async (state: RestrictionState) => {
      await put('view', state === 'private' ? startWith(q.data?.view ?? []) : []);
      await put('edit', state === 'view_only' ? startWith(q.data?.edit ?? []) : []);
    },
    onSettled: async () => {
      await refresh();
      setChosen(null);
    },
  });
  const editable = page.can['restriction.manage'] ?? false;
  // A choice shows at once, while the server still has to answer.
  const state = chosen ?? (q.data ? restrictionState(q.data) : 'open');
  return (
    <Dialog title={t('restrictions.title')} onClose={onClose} wide>
      <p className="muted">{t('restrictions.help')}</p>
      <ErrorNote error={set.error ?? choose.error ?? q.error} />
      {q.data && (
        <>
          {editable && (
            <RestrictionChoice
              page={page}
              current={state}
              onChoose={(next) => {
                setChosen(next);
                choose.mutate(next);
              }}
            />
          )}
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
          {(state === 'private' || !editable) && (
            <SubjectList
              title={t('restrictions.view')}
              subjects={q.data.view}
              editable={editable}
              onChange={(subjects) => set.mutate({ kind: 'view', subjects })}
            />
          )}
          {(state === 'view_only' || !editable) && (
            <SubjectList
              title={t('restrictions.edit')}
              subjects={q.data.edit}
              editable={editable}
              onChange={(subjects) => set.mutate({ kind: 'edit', subjects })}
            />
          )}
        </>
      )}
      {page.can['space.administer'] && <WhyPanel page={page} />}
    </Dialog>
  );
}

export function RenameDialog({ page, onClose }: { page: PageSummary; onClose: () => void }) {
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
        <div className="dialog-actions">
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** Says what a delete touches first, and offers Undo after it (spec 009, B11). */
export function DeleteDialog({ page, onClose }: { page: PageSummary; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const impact = useQuery({
    queryKey: ['impact', page.id],
    queryFn: () => api.get<DeleteImpact>(`/pages/${page.id}/impact`),
    staleTime: 0,
  });
  const usedBy = useQuery({
    queryKey: ['used-by', page.id],
    queryFn: () => api.get<{ pages: PageRef[] }>(`/pages/${page.id}/used-by`),
    staleTime: 0,
  });
  const refresh = () => client.invalidateQueries({ queryKey: ['tree', page.space_id] });
  const undo = async () => {
    try {
      await api.post(`/pages/${page.id}/undo-delete`);
      await refresh();
      navigate(`/pages/${page.id}`);
    } catch (e) {
      toast({ message: e instanceof Error ? e.message : t('app.error') });
    }
  };
  const remove = useMutation({
    mutationFn: () => api.del(`/pages/${page.id}`),
    onSuccess: async () => {
      await refresh();
      client.removeQueries({ queryKey: ['page', page.id] });
      onClose();
      navigate(page.parent_id ? `/pages/${page.parent_id}` : '/');
      toast({
        message: t('page.deleted', { title: page.title }),
        action: { label: t('page.undo'), run: () => void undo() },
      });
    },
  });
  const pages = impact.data?.pages ?? 1;
  const links = impact.data?.incoming ?? [];
  const moreLinks = (impact.data?.incoming_total ?? 0) - links.length;
  return (
    <Dialog title={t('page.delete')} onClose={onClose}>
      <ErrorNote error={impact.error ?? usedBy.error ?? remove.error} />
      <p>{pages > 1 ? t('page.confirmDeleteMany', { count: pages }) : t('page.confirmDelete')}</p>
      {links.length > 0 && (
        <>
          <p>{t('page.linksWillBreak')}</p>
          <ul className="plain-list">
            {links.map((l) => (
              <li key={l.id}>
                <Link href={`/pages/${l.id}`}>{l.title}</Link>
              </li>
            ))}
          </ul>
          {moreLinks > 0 && <p className="muted">{t('page.moreLinks', { count: moreLinks })}</p>}
        </>
      )}
      {(usedBy.data?.pages.length ?? 0) > 0 && (
        <>
          <p>{t('page.textIsShownOn')}</p>
          <ul className="plain-list">
            {usedBy.data?.pages.map((l) => (
              <li key={l.id}>
                <Link href={`/pages/${l.id}`}>{l.title}</Link>
              </li>
            ))}
          </ul>
        </>
      )}
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button type="button" className="danger" disabled={remove.isPending} onClick={() => remove.mutate()}>
          {t('page.delete')}
        </button>
      </div>
    </Dialog>
  );
}
