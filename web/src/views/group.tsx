// SPDX-License-Identifier: Apache-2.0
// The screen of a group owner, outside the console (spec 013, F8).
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Group } from '../api';
import { PersonName } from '../components/person';
import { SubjectPicker } from '../components/picker';
import { ErrorNote, Loading } from '../components/ui';

interface Member {
  id: string;
  display_name: string;
}

export function GroupView({ id }: { id: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['owned-group', id],
    queryFn: () => api.get<{ group: Group; members: Member[] }>(`/groups/${id}`),
  });
  const refresh = () => client.invalidateQueries({ queryKey: ['owned-group', id] });
  const add = useMutation({
    mutationFn: (user: string) => api.put(`/groups/${id}/members/${user}`),
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: (user: string) => api.del(`/groups/${id}/members/${user}`),
    onSettled: refresh,
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const managed = q.data.group.source !== 'local';
  return (
    <>
      <h1>{q.data.group.name}</h1>
      {managed && <p className="note">{t('admin.managed')}</p>}
      <ErrorNote error={add.error ?? remove.error} />
      <h2>{t('admin.members')}</h2>
      <ul className="plain-list">
        {q.data.members.map((m) => (
          <li key={m.id} className="row">
            <span className="grow">
              <PersonName id={m.id} name={m.display_name} />
            </span>
            {!managed && (
              <button type="button" className="ghost" onClick={() => remove.mutate(m.id)}>
                {t('common.remove')}
              </button>
            )}
          </li>
        ))}
      </ul>
      {!managed && <SubjectPicker label={t('admin.addMember')} usersOnly onPick={(e) => add.mutate(e.id)} />}
    </>
  );
}
