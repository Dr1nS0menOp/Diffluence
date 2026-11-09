// SPDX-License-Identifier: Apache-2.0
// The words for permissions: a name and one line on what each allows, in groups, so no
// key such as create_personal_space reaches a screen (spec 036).

export type PermissionScope = 'space' | 'instance';
export type PermissionGroup = 'pages' | 'comments' | 'files' | 'people' | 'space' | 'administration';

type Translate = (key: string) => string;

const SPACE_GROUPS: [PermissionGroup, string[]][] = [
  [
    'pages',
    [
      'space.view',
      'page.create',
      'page.edit',
      'page.archive',
      'page.delete_own',
      'page.delete_any',
      'space.export',
    ],
  ],
  ['comments', ['comment.add', 'comment.delete_own', 'comment.delete_any']],
  ['files', ['attachment.add', 'attachment.delete_own', 'attachment.delete_any']],
  ['people', ['space.manage_access', 'restriction.manage']],
  [
    'space',
    [
      'space.administer',
      'site.publish',
      'space.automation',
      'space.templates',
      'space.audit',
      'analytics.view',
    ],
  ],
];

const INSTANCE_GROUPS: [PermissionGroup, string[]][] = [
  ['people', ['use_product', 'view_profiles', 'invite_people']],
  ['space', ['create_space', 'create_personal_space']],
  ['administration', ['administrator', 'system_administrator']],
];

/** The translation key of a permission, without the "permissions." in front. */
export function permissionKey(scope: PermissionScope, permission: string): string {
  return scope === 'instance' ? `global_${permission}` : permission.replace(/\./g, '_');
}

/** The name people see for a permission. */
export function permissionLabel(t: Translate, scope: PermissionScope, permission: string): string {
  return t(`permissions.${permissionKey(scope, permission)}`);
}

/** One line on what a permission allows. */
export function permissionHelp(t: Translate, scope: PermissionScope, permission: string): string {
  return t(`permissions.${permissionKey(scope, permission)}_help`);
}

/** The groups a permission of a scope belongs to, in the order they show. */
export function permissionGroups(scope: PermissionScope): [PermissionGroup, string[]][] {
  return scope === 'instance' ? INSTANCE_GROUPS : SPACE_GROUPS;
}

/**
 * Sorts the permissions the server offers into their groups. One the groups do not know
 * yet shows in the last group rather than not at all.
 */
export function groupPermissions(
  scope: PermissionScope,
  offered: string[],
): { group: PermissionGroup; permissions: string[] }[] {
  const groups = permissionGroups(scope);
  const known = new Set(groups.flatMap(([, list]) => list));
  const unknown = offered.filter((p) => !known.has(p));
  return groups
    .map(([group, list], i) => ({
      group,
      permissions: [...list.filter((p) => offered.includes(p)), ...(i === groups.length - 1 ? unknown : [])],
    }))
    .filter((g) => g.permissions.length > 0);
}
