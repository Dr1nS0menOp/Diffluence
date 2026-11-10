// SPDX-License-Identifier: Apache-2.0
// Who has access to a space: people and groups, each holding one or more roles (specs 043
// and 049). It replaces the matrix of ticks. A role is added or taken away one at a time,
// every change is previewed by the server and asks first when someone would lose the space,
// and the server audits it with before and after.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, NIL_ID, type GrantPreview, type Me, type Space, type Subject } from '../api';
import { IconClose } from '../components/icons';
import { PersonPicture, personQuery } from '../components/person';
import { SubjectPicker } from '../components/picker';
import { Avatar, Dialog, ErrorNote, Field, Loading } from '../components/ui';
import { permissionLabel } from '../permission-words';
import { useRoles, type Role } from './roles';
import { PreviewText } from './space-grants';

// The empty first choice of a list that adds a role.
const NO_CHOICE = '';

interface Assignment {
  subject: Subject;
  name: string;
  roles: { id: string; name: string }[];
}

interface RoleChange {
  subject: Subject;
  roles: string[];
}

/** The roles a subject holds with one more, in the order they were given. */
export function withRole(held: string[], added: string): string[] {
  return held.includes(added) ? held : [...held, added];
}

/** The roles a subject holds without one of them. */
export function withoutRole(held: string[], removed: string): string[] {
  return held.filter((id) => id !== removed);
}

function subjectLabel(t: (key: string) => string, a: { subject: Subject; name: string }): string {
  if (a.subject.type === 'all_members') return t('permissions.allMembers');
  if (a.subject.type === 'all_guests') return t('permissions.allGuests');
  return a.name;
}

function SubjectPicture({ subject, name }: { subject: Subject; name: string }) {
  const isPerson = subject.type === 'user';
  const person = useQuery({ ...personQuery(subject.id), enabled: isPerson }).data;
  if (isPerson && person) return <PersonPicture person={person} />;
  return <Avatar name={name} />;
}

function SubjectKind({ subject }: { subject: Subject }) {
  const { t } = useTranslation();
  if (subject.type === 'user') return <>{t('roles.subject_user')}</>;
  if (subject.type === 'group') return <>{t('roles.subject_group')}</>;
  return <>{t('people.everyoneOfAKind')}</>;
}

/** Saves a choice of roles after the server said what it does; asks first when someone loses the space. */
function useRoleChange(space: Space) {
  const client = useQueryClient();
  const [asking, setAsking] = useState<{ change: RoleChange; preview: GrantPreview } | null>(null);
  const [lastPreview, setLastPreview] = useState<GrantPreview | null>(null);
  const save = useMutation({
    mutationFn: (change: RoleChange) => api.put(`/spaces/${space.id}/role-assignments`, change),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['role-assignments'] });
      await client.invalidateQueries({ queryKey: ['roles'] });
      await client.invalidateQueries({ queryKey: ['grants', space.id] });
      await client.invalidateQueries({ queryKey: ['space-access', space.id] });
      await client.invalidateQueries({ queryKey: ['space-people', space.id] });
    },
  });
  const check = useMutation({
    mutationFn: (change: RoleChange) =>
      api.post<GrantPreview>(`/spaces/${space.id}/role-assignments/preview`, change),
    onSuccess: (preview, change) => {
      setLastPreview(null);
      if (preview.loses_view) {
        setAsking({ change, preview });
        return;
      }
      save.mutate(change, { onSuccess: () => setLastPreview(preview) });
    },
  });
  return {
    change: (next: RoleChange) => check.mutate(next),
    confirm: () => {
      if (asking) save.mutate(asking.change, { onSuccess: () => setLastPreview(asking.preview) });
      setAsking(null);
    },
    cancel: () => setAsking(null),
    asking,
    lastPreview,
    error: check.error ?? save.error,
  };
}

/** A list of roles in three groups: built in, made for the instance, and combinations without a name. */
function RoleSelect({
  roles,
  value,
  label,
  placeholder,
  onChange,
}: {
  roles: Role[];
  value: string;
  label: string;
  placeholder?: string;
  onChange: (roleId: string) => void;
}) {
  const { t } = useTranslation();
  const groups = [
    { label: t('access.builtInRoles'), roles: roles.filter((r) => r.built_in) },
    { label: t('access.ownRoles'), roles: roles.filter((r) => !r.built_in && !r.unnamed) },
    { label: t('access.unnamedRoles'), roles: roles.filter((r) => r.unnamed) },
  ].filter((g) => g.roles.length > 0);
  return (
    <select
      className="role-choice"
      aria-label={label}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      {placeholder !== undefined && (
        <option value={NO_CHOICE} disabled>
          {placeholder}
        </option>
      )}
      {groups.map((g) => (
        <optgroup key={g.label} label={g.label}>
          {g.roles.map((r) => (
            <option key={r.id} value={r.id}>
              {r.name}
            </option>
          ))}
        </optgroup>
      ))}
    </select>
  );
}

/** Gives a combination without a name a name. Only the name changes, so nobody gains or loses anything. */
function NameRoleDialog({ role, onClose }: { role: Role; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState('');
  const rename = useMutation({
    mutationFn: () =>
      api.put(`/admin/roles/${role.id}`, {
        name,
        description: role.description,
        permissions: role.permissions,
      }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['roles'] });
      await client.invalidateQueries({ queryKey: ['role-assignments'] });
      onClose();
    },
  });
  return (
    <Dialog title={t('access.nameRole')} onClose={onClose}>
      <form
        className="settings-form"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          rename.mutate();
        }}
      >
        <p className="muted">{t('access.nameRoleHelp')}</p>
        <p>{role.name}</p>
        <ErrorNote error={rename.error} />
        <Field label={t('common.name')}>
          <input value={name} maxLength={100} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="primary" disabled={rename.isPending}>
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** One role someone holds: its name opens the permissions it gives, and a button takes it away. */
function HeldRole({
  role,
  holder,
  canRemove,
  isAdministrator,
  onRemove,
  onName,
}: {
  role: Role;
  holder: string;
  canRemove: boolean;
  isAdministrator: boolean;
  onRemove: () => void;
  onName: (role: Role) => void;
}) {
  const { t } = useTranslation();
  return (
    <li className="held-role">
      <details>
        <summary>{role.name}</summary>
        <p className="muted">{role.permissions.map((p) => permissionLabel(t, 'space', p)).join(', ')}</p>
        {role.unnamed && (
          <p className="muted">
            {t('access.unnamedHelp')} {!isAdministrator && t('access.unnamedAsk')}
          </p>
        )}
        {role.unnamed && isAdministrator && (
          <button type="button" className="ghost" onClick={() => onName(role)}>
            {t('access.nameRole')}
          </button>
        )}
      </details>
      {canRemove && (
        <button
          type="button"
          className="icon ghost"
          aria-label={t('access.takeRole', { role: role.name, name: holder })}
          title={t('access.takeRole', { role: role.name, name: holder })}
          onClick={onRemove}
        >
          <IconClose size={14} />
        </button>
      )}
    </li>
  );
}

function HolderRow({
  assignment,
  roles,
  space,
  isAdministrator,
  onChange,
  onName,
}: {
  assignment: Assignment;
  roles: Role[];
  space: Space;
  isAdministrator: boolean;
  onChange: (next: RoleChange) => void;
  onName: (role: Role) => void;
}) {
  const { t } = useTranslation();
  const name = subjectLabel(t, assignment);
  const heldIds = assignment.roles.map((r) => r.id);
  const held = heldIds.flatMap((id) => roles.filter((r) => r.id === id));
  const isOwner = assignment.subject.type === 'user' && assignment.subject.id === space.owner_id;
  // The owner keeps at least one role here; the server also keeps every space an administrator.
  const canRemoveOne = !isOwner || held.length > 1;
  return (
    <li className="member-row">
      <SubjectPicture subject={assignment.subject} name={name} />
      <span className="member-text">
        <span className="member-name">{name}</span>
        <span className="muted">
          <SubjectKind subject={assignment.subject} />
        </span>
        <ul className="held-roles" aria-label={t('access.rolesOf', { name })}>
          {held.map((role) => (
            <HeldRole
              key={role.id}
              role={role}
              holder={name}
              canRemove={canRemoveOne}
              isAdministrator={isAdministrator}
              onName={onName}
              onRemove={() => onChange({ subject: assignment.subject, roles: withoutRole(heldIds, role.id) })}
            />
          ))}
        </ul>
      </span>
      <RoleSelect
        roles={roles.filter((r) => !heldIds.includes(r.id))}
        value={NO_CHOICE}
        label={t('access.addRoleFor', { name })}
        placeholder={t('access.addRole')}
        onChange={(roleId) => onChange({ subject: assignment.subject, roles: withRole(heldIds, roleId) })}
      />
      {!isOwner && (
        <button
          type="button"
          className="ghost"
          onClick={() => onChange({ subject: assignment.subject, roles: [] })}
        >
          {t('permissions.removeAccess')}
        </button>
      )}
    </li>
  );
}

/**
 * Picks a person, a group, all members or all guests, then a role they get. Someone who
 * already holds roles here keeps them and gains this one.
 */
function AddHolder({
  roles,
  holders,
  onChange,
}: {
  roles: Role[];
  holders: Assignment[];
  onChange: (next: RoleChange) => void;
}) {
  const { t } = useTranslation();
  const [picked, setPicked] = useState<{ subject: Subject; name: string } | null>(null);
  const viewer = roles.find((r) => r.built_in === 'viewer') ?? roles[0];
  const [roleId, setRoleId] = useState(viewer?.id ?? '');
  const everyone = (type: 'all_members' | 'all_guests') =>
    setPicked({
      subject: { type, id: NIL_ID },
      name: subjectLabel(t, { subject: { type, id: NIL_ID }, name: '' }),
    });
  return (
    <section aria-labelledby="give-access-heading">
      <h2 id="give-access-heading">{t('access.give')}</h2>
      {!picked && (
        <div className="row">
          <SubjectPicker
            label={t('roles.give')}
            onPick={(e) => setPicked({ subject: { type: e.type, id: e.id }, name: e.name })}
          />
          <button type="button" className="quiet" onClick={() => everyone('all_members')}>
            {t('permissions.allMembers')}
          </button>
          <button type="button" className="quiet" onClick={() => everyone('all_guests')}>
            {t('permissions.allGuests')}
          </button>
        </div>
      )}
      {picked && (
        <form
          className="row"
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            const already = holders.find(
              (h) => h.subject.type === picked.subject.type && h.subject.id === picked.subject.id,
            );
            const held = already?.roles.map((r) => r.id) ?? [];
            onChange({ subject: picked.subject, roles: withRole(held, roleId) });
            setPicked(null);
          }}
        >
          <span className="grow">{t('access.giveTo', { name: picked.name })}</span>
          <RoleSelect
            roles={roles}
            value={roleId}
            label={t('access.roleToGive', { name: picked.name })}
            onChange={setRoleId}
          />
          <button type="submit" className="primary" disabled={!roleId}>
            {t('access.give')}
          </button>
          <button type="button" className="quiet" onClick={() => setPicked(null)}>
            {t('common.cancel')}
          </button>
        </form>
      )}
    </section>
  );
}

/** Who holds which role in a space, for whoever manages access there: on the Access screen and in the settings. */
export function SpaceRolesPanel({ space }: { space: Space }) {
  const { t } = useTranslation();
  const path = `/spaces/${space.id}/role-assignments`;
  const assignments = useQuery({
    queryKey: ['role-assignments', path],
    queryFn: () => api.get<{ assignments: Assignment[] }>(path),
  });
  const roles = useRoles('space');
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') });
  const isAdministrator = me.data?.global_permissions.includes('administrator') ?? false;
  const change = useRoleChange(space);
  const [text, setText] = useState('');
  const [naming, setNaming] = useState<Role | null>(null);
  const error = assignments.error ?? roles.error;
  if (error) return <ErrorNote error={error} />;
  if (!assignments.data || !roles.data) return <Loading />;
  const list = roles.data.roles;
  const wanted = text.trim().toLowerCase();
  const shown = assignments.data.assignments.filter((a) => subjectLabel(t, a).toLowerCase().includes(wanted));
  return (
    <section>
      <p className="muted">{t('access.rolesHelp')}</p>
      <ErrorNote error={change.error} />
      {change.lastPreview && (
        <div role="status" className="note">
          <PreviewText preview={change.lastPreview} />
        </div>
      )}
      <input
        type="search"
        className="list-filter"
        value={text}
        aria-label={t('access.search')}
        placeholder={t('access.search')}
        onChange={(e) => setText(e.target.value)}
      />
      {assignments.data.assignments.length === 0 && <p className="muted">{t('roles.nobody')}</p>}
      <ul className="member-list">
        {shown.map((a) => (
          <HolderRow
            key={a.subject.type + a.subject.id}
            assignment={a}
            roles={list}
            space={space}
            isAdministrator={isAdministrator}
            onChange={change.change}
            onName={setNaming}
          />
        ))}
      </ul>
      <AddHolder roles={list} holders={assignments.data.assignments} onChange={change.change} />
      {naming && <NameRoleDialog role={naming} onClose={() => setNaming(null)} />}
      {change.asking && (
        <Dialog title={t('access.confirmTitle')} onClose={change.cancel}>
          <PreviewText preview={change.asking.preview} />
          <div className="dialog-actions">
            <button type="button" className="quiet" onClick={change.cancel}>
              {t('common.cancel')}
            </button>
            <button type="button" className="danger" onClick={change.confirm}>
              {t('common.save')}
            </button>
          </div>
        </Dialog>
      )}
    </section>
  );
}
