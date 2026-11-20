// SPDX-License-Identifier: Apache-2.0
// Statuses and approvals of a space in one section (spec 083, SPC-140): the statuses, which apply
// at once; the review period and the approval rule, each with its own Save and Reset changes.
// Each part shows only to whoever may change it.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Space, type Stage } from '../api';
import { SubjectPicker } from '../components/picker';
import { SaveReset } from '../components/save-reset';
import { Switch } from '../components/switch';
import { useSaved } from '../components/toast';
import { ErrorNote } from '../components/ui';

const MARK_COLOURS = ['grey', 'yellow', 'green', 'blue', 'purple', 'red'];
const MAX_STATUS_LENGTH = 30;
const MAX_REVIEW_MONTHS = 120;

function useSpaceRefresh(space: Space) {
  const client = useQueryClient();
  return async () => {
    await client.invalidateQueries({ queryKey: ['space', space.key] });
    await client.invalidateQueries({ queryKey: ['space-id', space.id] });
  };
}

export function StatusesAndApprovals({ space }: { space: Space }) {
  return (
    <>
      {space.can?.['space.statuses'] && <StatusList space={space} />}
      {space.can?.['space.statuses'] && <ReviewPeriod space={space} />}
      {space.can?.['space.approvals'] && <ApprovalRule space={space} />}
    </>
  );
}

function StatusList({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const saved = useSaved();
  const key = ['statuses', space.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ statuses: Stage[]; free: boolean }>(`/spaces/${space.id}/statuses`),
  });
  const save = useMutation({
    mutationFn: (v: { statuses: Stage[]; free: boolean }) => api.put(`/spaces/${space.id}/statuses`, v),
    onSuccess: saved,
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  const [word, setWord] = useState('');
  const [color, setColor] = useState('blue');
  const list = q.data?.statuses ?? [];
  const free = q.data?.free ?? false;
  return (
    <section className="settings-unit" aria-labelledby="statuses-part">
      <h2 id="statuses-part">{t('spaceStatuses.statuses')}</h2>
      <p className="muted">{t('spaceStatuses.statusesHelp')}</p>
      <ErrorNote error={q.error ?? save.error} />
      <ul className="plain-list">
        {list.map((s) => (
          <li key={s.text} className="row">
            <span className={`status-mark mark-${s.color}`}>{s.text}</span>
            <button
              type="button"
              className="ghost"
              onClick={() => save.mutate({ statuses: list.filter((x) => x.text !== s.text), free })}
            >
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate({ statuses: [...list, { text: word, color }], free });
          setWord('');
        }}
      >
        <input
          required
          maxLength={MAX_STATUS_LENGTH}
          value={word}
          aria-label={t('organise.statusWord')}
          placeholder={t('organise.statusWord')}
          onChange={(e) => setWord(e.target.value)}
        />
        <select
          aria-label={t('spaceSettings.colour')}
          value={color}
          onChange={(e) => setColor(e.target.value)}
        >
          {MARK_COLOURS.map((c) => (
            <option key={c} value={c}>
              {t(`editor.colors.${c}`)}
            </option>
          ))}
        </select>
        <button type="submit">{t('common.add')}</button>
      </form>
      <Switch checked={free} onChange={(on) => save.mutate({ statuses: list, free: on })}>
        {t('spaceSettings.freeStatus')}
      </Switch>
    </section>
  );
}

function ReviewPeriod({ space }: { space: Space }) {
  const { t } = useTranslation();
  const refresh = useSpaceRefresh(space);
  const stored = space.review_months ?? 0;
  const [months, setMonths] = useState(stored);
  const review = useMutation({
    mutationFn: () => api.put(`/spaces/${space.id}/review`, { months }),
    onSuccess: refresh,
  });
  return (
    <section className="settings-unit" aria-labelledby="review-part">
      <h2 id="review-part">{t('spaceStatuses.review')}</h2>
      <p className="muted">{t('spaceSettings.reviewHelp')}</p>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          review.mutate();
        }}
      >
        <ErrorNote error={review.error} />
        <label className="field">
          <span>{t('spaceSettings.reviewMonths')}</span>
          <span className="number-unit">
            <input
              type="number"
              className="narrow-number"
              min={0}
              max={MAX_REVIEW_MONTHS}
              value={months}
              onChange={(e) => setMonths(Number(e.target.value))}
            />
            <span>{t('spaceStatuses.months')}</span>
          </span>
        </label>
        <SaveReset changed={months !== stored} busy={review.isPending} onReset={() => setMonths(stored)} />
      </form>
    </section>
  );
}

interface ApprovalForm {
  required: boolean;
  approvers: { id: string; name: string }[];
}

function ApprovalRule({ space }: { space: Space }) {
  const rule = useQuery({
    queryKey: ['approval-rule', space.id],
    queryFn: () => api.get<ApprovalForm>(`/spaces/${space.id}/approvals`),
  });
  if (!rule.data) return <ErrorNote error={rule.error} />;
  // The form starts again from what is stored whenever that changes.
  return <ApprovalRuleForm key={JSON.stringify(rule.data)} space={space} stored={rule.data} />;
}

function ApprovalRuleForm({ space, stored }: { space: Space; stored: ApprovalForm }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const refresh = useSpaceRefresh(space);
  const [form, setForm] = useState(stored);
  const approval = useMutation({
    mutationFn: () =>
      api.put(`/spaces/${space.id}/approvals`, {
        required: form.required,
        approvers: form.approvers.map((a) => a.id),
      }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['approval-rule', space.id] });
      await refresh();
    },
  });
  const changed = JSON.stringify(form) !== JSON.stringify(stored);
  return (
    <section className="settings-unit" aria-labelledby="approval-part">
      <h2 id="approval-part">{t('spaceStatuses.approval')}</h2>
      <p className="muted">{t('spaceStatuses.approvalHelp')}</p>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          approval.mutate();
        }}
      >
        <ErrorNote error={approval.error} />
        <label className="check">
          <input
            type="checkbox"
            checked={form.required}
            onChange={(e) => setForm({ ...form, required: e.target.checked })}
          />
          {t('spaceSettings.requireApproval')}
        </label>
        <ul className="plain-list">
          {form.approvers.map((a) => (
            <li key={a.id} className="row">
              {a.name}
              <button
                type="button"
                className="ghost"
                onClick={() => setForm({ ...form, approvers: form.approvers.filter((x) => x.id !== a.id) })}
              >
                {t('common.remove')}
              </button>
            </li>
          ))}
        </ul>
        <SubjectPicker
          label={t('spaceSettings.fixedApprover')}
          usersOnly
          onPick={(e) =>
            form.approvers.some((a) => a.id === e.id) ||
            setForm({ ...form, approvers: [...form.approvers, { id: e.id, name: e.name }] })
          }
        />
        <SaveReset changed={changed} busy={approval.isPending} onReset={() => setForm(stored)} />
      </form>
    </section>
  );
}
