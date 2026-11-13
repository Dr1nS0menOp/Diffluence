// SPDX-License-Identifier: Apache-2.0
// The workflows of a space, in its settings (spec 064, wave DC1). Wave DC1 makes and changes
// workflows through the API; the editor for them comes with wave DC5.
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type Space, type Workflow } from '../api';
import { TableBox } from '../components/table-box';
import { ErrorNote, Loading } from '../components/ui';

export function WorkflowsSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['workflows', space.id],
    queryFn: () => api.get<{ workflows: Workflow[] }>(`/spaces/${space.id}/workflows`),
  });
  if (q.isPending) return <Loading />;
  const list = q.data?.workflows ?? [];
  return (
    <section>
      <p className="muted">{t('control.settingsIntro')}</p>
      <ErrorNote error={q.error} />
      {list.length === 0 ? (
        <p className="muted">{t('control.none')}</p>
      ) : (
        <TableBox>
          <table>
            <thead>
              <tr>
                <th>{t('control.columnName')}</th>
                <th>{t('control.columnStates')}</th>
                <th>{t('control.columnVersion')}</th>
                <th>{t('control.columnPages')}</th>
              </tr>
            </thead>
            <tbody>
              {list.map((w) => (
                <tr key={w.id}>
                  <td>
                    {w.name}{' '}
                    {w.space_workflow && <span className="lozenge">{t('control.spaceWorkflow')}</span>}
                  </td>
                  <td>{w.definition.states.map((s) => s.name).join(', ')}</td>
                  <td>{w.version}</td>
                  <td>{w.pages}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
    </section>
  );
}
