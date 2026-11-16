// SPDX-License-Identifier: Apache-2.0
// A person chooses which entries of their own sidebar show, and in which order (spec 078,
// NAV-137 to NAV-142). Nothing changes until they save, and it changes only their sidebar.
import { useState, type DragEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { usePreferences } from '../preferences';
import { moved } from '../views/lists';
import { IconArrowDown, IconArrowUp, IconGrip, IconMore } from './icons';
import { Menu, MenuItem } from './menu';
import { arrangeEntries, chosenEntries, orderedEntries, type SidebarEntry } from './sidebar-entries';
import { ENTRY_VIEWS, START_ENTRY } from './sidebar-entry-views';
import { Dialog } from './ui';

const ROW_ICON_SIZE = 18;

interface Row {
  id: SidebarEntry;
  shown: boolean;
}

export function CustomiseSidebarDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const { preferences, update } = usePreferences();
  const [rows, setRows] = useState<Row[]>(() => {
    const { hidden } = arrangeEntries(preferences.sidebar_order, preferences.sidebar_hidden);
    return orderedEntries(preferences.sidebar_order).map((id) => ({ id, shown: !hidden.includes(id) }));
  });
  const [dragged, setDragged] = useState<number | null>(null);
  const move = (at: number, step: number) => setRows(moved(rows, at, step));
  const dropOn = (at: number) => (e: DragEvent<HTMLLIElement>) => {
    if (dragged === null) return;
    e.preventDefault();
    move(dragged, at - dragged);
    setDragged(null);
  };
  const save = () => {
    const { order, hidden } = chosenEntries(rows);
    update({ sidebar_order: order, sidebar_hidden: hidden });
    onClose();
  };
  return (
    <Dialog title={t('customise.title')} onClose={onClose}>
      <p>{t('customise.hiddenUnderMore')}</p>
      <p className="muted">{t('customise.onlyYou')}</p>
      <h3 className="customise-heading">{t('customise.entries')}</h3>
      <ul className="customise-list">
        <li className="customise-row locked">
          <span className="customise-grip" />
          <input type="checkbox" checked disabled aria-label={t(START_ENTRY.label)} />
          <START_ENTRY.Icon size={ROW_ICON_SIZE} />
          <span className="grow">{t(START_ENTRY.label)}</span>
          <span className="muted">{t('customise.alwaysShown')}</span>
        </li>
        {rows.map((row, at) => {
          const view = ENTRY_VIEWS[row.id];
          const name = t(view.label);
          return (
            <li
              key={row.id}
              className={dragged === at ? 'customise-row dragging' : 'customise-row'}
              onDragOver={(e) => {
                if (dragged !== null) e.preventDefault();
              }}
              onDrop={dropOn(at)}
            >
              {/* Dragging is for the mouse; More offers the same moves by keyboard (WCAG 2.5.7). */}
              <span
                className="customise-grip"
                draggable
                aria-hidden="true"
                onDragStart={(e) => {
                  e.dataTransfer.effectAllowed = 'move';
                  setDragged(at);
                }}
                onDragEnd={() => setDragged(null)}
              >
                <IconGrip size={ROW_ICON_SIZE} />
              </span>
              <input
                type="checkbox"
                checked={row.shown}
                aria-label={name}
                onChange={(e) =>
                  setRows(rows.map((r, i) => (i === at ? { ...r, shown: e.target.checked } : r)))
                }
              />
              <view.Icon size={ROW_ICON_SIZE} />
              <span className="grow">{name}</span>
              <Menu
                label={t('customise.moreFor', { name })}
                trigger={<IconMore size={ROW_ICON_SIZE} />}
                floating
              >
                <MenuItem disabled={at === 0} onClick={() => move(at, -1)}>
                  <IconArrowUp size={ROW_ICON_SIZE} /> {t('customise.moveUp')}
                </MenuItem>
                <MenuItem disabled={at === rows.length - 1} onClick={() => move(at, 1)}>
                  <IconArrowDown size={ROW_ICON_SIZE} /> {t('customise.moveDown')}
                </MenuItem>
              </Menu>
            </li>
          );
        })}
      </ul>
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button type="button" className="primary" onClick={save}>
          {t('customise.save')}
        </button>
      </div>
    </Dialog>
  );
}
