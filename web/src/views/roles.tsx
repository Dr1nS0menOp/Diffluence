// SPDX-License-Identifier: Apache-2.0
// Roles an administrator makes, and who holds them, per space and for the instance (spec 036).
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type DirectoryEntry, type Space, type Subject } from '../api';
import { SubjectPicker } from '../components/picker';
import { ConfirmButton, Dialog, ErrorNote, Field, Loading } from '../components/ui';
import { groupPermissions, permissionHelp, permissionLabel } from '../permission-words';
import { TableBox } from '../components/table-box';

type RoleScope = 'space' | 'instance';

export interface Role {
  id: string;
  scope: RoleScope;
  built_in?: string;
  name: string;
  description: string;
  permissions: string[];
  in_use: number;
}

interface RoleList {
  roles: Role[];
  permissions: string[];
}

interface RoleImpact {
  assignments: number;
  spaces: number;
  people: number;
  gains: string[];
  loses: string[];
}

interface Assignment {
  subject: Subject;
  name: string;
  roles: { id: string; name: string }[];
}

interface RoleHolder {
  subject: Subject;
  name: string;
  space_id?: string;
  space_name?: string;
}

function useRoles(scope: RoleScope) {
  return useQuery({
    queryKey: ['roles', scope],
    queryFn: () => api.get<RoleList>(`/roles?scope=${scope}`),
  });
}

/** Whether an error asks for a confirmation after a preview. */
function needsConfirm(error: unknown): boolean {
  return error instanceof ApiError && error.code === 'confirm_needed';
}

function subjectName(t: (key: string) => string, a: { subject: Subject; name: string }): string {
  if (a.subject.type === 'all_members') return t('permissions.allMembers');
  if (a.subject.type === 'all_guests') return t('permissions.allGuests');
  return a.name;
}

/** Who holds a role, and in which space. */
function Holders({ role }: { role: Role }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['role-holders', role.id],
    queryFn: () => api.get<{ holders: RoleHolder[]; total: number }>(`/admin/roles/${role.id}/holders`),
  });
  return (
    <section className="role-holders" aria-labelledby="role-holders-heading">
      <h3 id="role-holders-heading">{t('roles.whoHasRole')}</h3>
      <ErrorNote error={q.error} />
      {!q.data && !q.error && <Loading />}
      {q.data && q.data.total === 0 && <p className="muted">{t('roles.noHolders')}</p>}
      <ul className="plain-list">
        {q.data?.holders.map((h) => (
          <li key={h.subject.type + h.subject.id + (h.space_id ?? '')}>
            {h.space_id
              ? t('roles.holderIn', { name: subjectName(t, h), space: h.space_name })
              : subjectName(t, h)}
          </li>
        ))}
      </ul>
      {q.data && q.data.total > q.data.holders.length && (
        <p className="muted">{t('roles.andMore', { count: q.data.total - q.data.holders.length })}</p>
      )}
    </section>
  );
}

function RoleDialog({
  scope,
  role,
  all,
  onClose,
}: {
  scope: RoleScope;
  role: Role | null;
  all: string[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState(role?.name ?? '');
  const [description, setDescription] = useState(role?.description ?? '');
  const [chosen, setChosen] = useState<string[]>(role?.permissions ?? []);
  const [impact, setImpact] = useState<RoleImpact | null>(null);
  const fields = { name, description, permissions: chosen };
  const done = async () => {
    await client.invalidateQueries({ queryKey: ['roles'] });
    await client.invalidateQueries({ queryKey: ['role-assignments'] });
    onClose();
  };
  const save = useMutation({
    mutationFn: (confirm: boolean) =>
      role
        ? api.put(`/admin/roles/${role.id}`, { ...fields, confirm })
        : api.post('/admin/roles', { ...fields, scope }),
    onSuccess: done,
    onError: async (error) => {
      // A role in use changes only after the people it reaches have been shown.
      if (role && needsConfirm(error)) {
        setImpact(await api.post<RoleImpact>(`/admin/roles/${role.id}/preview`, { permissions: chosen }));
      }
    },
  });
  const toggle = (p: string) =>
    setChosen(chosen.includes(p) ? chosen.filter((x) => x !== p) : [...chosen, p]);
  const list = (names: string[]) => names.map((p) => permissionLabel(t, scope, p)).join(', ');
  return (
    <Dialog title={role ? t('roles.edit', { name: role.name }) : t('roles.new')} onClose={onClose}>
      <form
        className="settings-form"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate(false);
        }}
      >
        {!impact && <ErrorNote error={needsConfirm(save.error) ? null : save.error} />}
        <Field label={t('common.name')}>
          <input value={name} maxLength={100} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Field label={t('roles.description')}>
          <textarea
            value={description}
            maxLength={500}
            rows={2}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
        {groupPermissions(scope, all).map(({ group, permissions }) => (
          <fieldset key={group} className="role-permissions">
            <legend>{t(`roles.group_${group}`)}</legend>
            {permissions.map((p) => (
              <label key={p} className="role-permission">
                <input type="checkbox" checked={chosen.includes(p)} onChange={() => toggle(p)} />
                <span>
                  {permissionLabel(t, scope, p)}
                  <span className="muted">{permissionHelp(t, scope, p)}</span>
                </span>
              </label>
            ))}
          </fieldset>
        ))}
        {role && <Holders role={role} />}
        {impact && (
          <div role="status" className="note">
            <p>
              {t('roles.reaches', {
                assignments: impact.assignments,
                spaces: impact.spaces,
                people: impact.people,
              })}
            </p>
            {impact.gains.length > 0 && <p>{t('access.gains', { list: list(impact.gains) })}</p>}
            {impact.loses.length > 0 && <p>{t('access.loses', { list: list(impact.loses) })}</p>}
          </div>
        )}
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={onClose}>
            {t('common.cancel')}
          </button>
          {impact ? (
            <button type="button" className="primary" onClick={() => save.mutate(true)}>
              {t('roles.confirmChange')}
            </button>
          ) : (
            <button type="submit" className="primary" disabled={chosen.length === 0}>
              {t('common.save')}
            </button>
          )}
        </div>
      </form>
    </Dialog>
  );
}

function RoleTable({ scope }: { scope: RoleScope }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useRoles(scope);
  const [editing, setEditing] = useState<Role | null | 'new'>(null);
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/admin/roles/${id}`),
    onSettled: () => client.invalidateQueries({ queryKey: ['roles'] }),
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const data = q.data;
  return (
    <section>
      <div className="row">
        <h2 className="grow">{scope === 'space' ? t('roles.spaceRoles') : t('roles.instanceRoles')}</h2>
        <button type="button" onClick={() => setEditing('new')}>
          {t('roles.new')}
        </button>
      </div>
      <p className="muted">{scope === 'space' ? t('roles.spaceHelp') : t('roles.instanceHelp')}</p>
      <ErrorNote error={remove.error} />
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              <th>{t('roles.permissions')}</th>
              <th>{t('roles.heldBy')}</th>
              <th aria-label={t('common.actions')} />
            </tr>
          </thead>
          <tbody>
            {data.roles.map((role) => (
              <tr key={role.id}>
                <td>
                  {role.name} {role.built_in && <span className="lozenge">{t('roles.builtIn')}</span>}
                  {role.description && <div className="muted">{role.description}</div>}
                </td>
                <td className="muted">
                  {groupPermissions(scope, role.permissions)
                    .flatMap((g) => g.permissions)
                    .map((p) => permissionLabel(t, scope, p))
                    .join(', ')}
                </td>
                <td>{t('roles.holders', { count: role.in_use })}</td>
                <td className="row">
                  {!role.built_in && (
                    <>
                      <button type="button" className="ghost" onClick={() => setEditing(role)}>
                        {t('common.edit')}
                      </button>
                      {role.in_use === 0 && (
                        <ConfirmButton
                          label={t('common.delete')}
                          message={t('roles.deleteAsk', { name: role.name })}
                          className="ghost"
                          onConfirm={() => remove.mutate(role.id)}
                        />
                      )}
                    </>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
      {editing && (
        <RoleDialog
          scope={scope}
          role={editing === 'new' ? null : editing}
          all={data.permissions}
          onClose={() => setEditing(null)}
        />
      )}
    </section>
  );
}

/** Choosing the roles one subject holds, in a space or for the instance. */
function AssignDialog({
  scope,
  path,
  subject,
  name,
  holds,
  onClose,
}: {
  scope: RoleScope;
  path: string;
  subject: Subject;
  name: string;
  holds: string[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const roles = useRoles(scope);
  const [chosen, setChosen] = useState<string[]>(holds);
  const save = useMutation({
    mutationFn: () => api.put(path, { subject, roles: chosen }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['role-assignments'] });
      await client.invalidateQueries({ queryKey: ['roles'] });
      onClose();
    },
  });
  const toggle = (id: string) =>
    setChosen(chosen.includes(id) ? chosen.filter((x) => x !== id) : [...chosen, id]);
  return (
    <Dialog title={t('roles.rolesOf', { name })} onClose={onClose}>
      <form
        className="settings-form"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <ErrorNote error={roles.error ?? save.error} />
        <fieldset className="role-permissions">
          <legend>{t('roles.title')}</legend>
          {roles.data?.roles.map((role) => (
            <label key={role.id} className="check">
              <input type="checkbox" checked={chosen.includes(role.id)} onChange={() => toggle(role.id)} />
              {role.name}
            </label>
          ))}
        </fieldset>
        {chosen.length === 0 && <p className="muted">{t('roles.noneLeft')}</p>}
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function Assignments({ scope, path }: { scope: RoleScope; path: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState<{ subject: Subject; name: string; holds: string[] } | null>(null);
  const q = useQuery({
    queryKey: ['role-assignments', path],
    queryFn: () => api.get<{ assignments: Assignment[] }>(path),
  });
  const pick = (entry: DirectoryEntry) => {
    const held = q.data?.assignments.find((a) => a.subject.type === entry.type && a.subject.id === entry.id);
    setOpen({
      subject: { type: entry.type, id: entry.id },
      name: entry.name,
      holds: held?.roles.map((r) => r.id) ?? [],
    });
  };
  const everyone = (type: 'all_members' | 'all_guests') => {
    const held = q.data?.assignments.find((a) => a.subject.type === type);
    const name = type === 'all_members' ? t('permissions.allMembers') : t('permissions.allGuests');
    setOpen({
      subject: { type, id: '00000000-0000-0000-0000-000000000000' },
      name,
      holds: held?.roles.map((r) => r.id) ?? [],
    });
  };
  return (
    <section>
      <h2>{scope === 'space' ? t('roles.whoHolds') : t('roles.whoHoldsInstance')}</h2>
      <ErrorNote error={q.error} />
      {!q.data && !q.error && <Loading />}
      {q.data && q.data.assignments.length === 0 && <p className="muted">{t('roles.nobody')}</p>}
      <ul className="plain-list">
        {q.data?.assignments.map((a) => (
          <li key={a.subject.type + a.subject.id} className="row">
            <span className="grow">
              {subjectName(t, a)}
              <span className="muted"> {a.roles.map((r) => r.name).join(', ')}</span>
            </span>
            <button
              type="button"
              className="ghost"
              onClick={() =>
                setOpen({ subject: a.subject, name: subjectName(t, a), holds: a.roles.map((r) => r.id) })
              }
            >
              {t('roles.change')}
            </button>
          </li>
        ))}
      </ul>
      <div className="row">
        <SubjectPicker label={t('roles.give')} onPick={pick} />
        {scope === 'space' && (
          <>
            <button type="button" className="quiet" onClick={() => everyone('all_members')}>
              {t('permissions.allMembers')}
            </button>
            <button type="button" className="quiet" onClick={() => everyone('all_guests')}>
              {t('permissions.allGuests')}
            </button>
          </>
        )}
      </div>
      {open && (
        <AssignDialog
          scope={scope}
          path={path}
          subject={open.subject}
          name={open.name}
          holds={open.holds}
          onClose={() => setOpen(null)}
        />
      )}
    </section>
  );
}

/** The roles tab of the administration. */
export function RolesTab() {
  return (
    <>
      <RoleTable scope="space" />
      <RoleTable scope="instance" />
      <Assignments scope="instance" path="/admin/role-assignments" />
    </>
  );
}

/** Who holds which roles in a space, for those who manage access there. */
export function SpaceRolesSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  return (
    <>
      <p className="muted">{t('roles.spaceSectionHelp')}</p>
      <Assignments scope="space" path={`/spaces/${space.id}/role-assignments`} />
    </>
  );
}
