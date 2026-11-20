// SPDX-License-Identifier: Apache-2.0
// The parts of Space settings that shape how a space is organised and how it looks.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type LabelCount, type Space } from '../api';
import { useCapabilities } from '../capabilities';
import { AnimalChoice } from '../components/animal-choice';
import { ChoiceTiles } from '../components/choice-tiles';
import {
  IconAutomation,
  IconBlog,
  IconChart,
  IconComment,
  IconDate,
  IconEmoji,
  IconLock,
  IconStatus,
} from '../components/icons';
import { SaveReset } from '../components/save-reset';
import { Switch } from '../components/switch';
import { useSaved } from '../components/toast';
import { Dialog, ErrorNote, Field, FilePicker } from '../components/ui';

/** The switches of a space, each with its icon (SPC-115). Requests for access stay (D14). */
const FEATURES = [
  { id: 'blog', icon: IconBlog },
  { id: 'comments', icon: IconComment },
  { id: 'reactions', icon: IconEmoji },
  { id: 'access_requests', icon: IconLock },
  { id: 'stages', icon: IconStatus },
  { id: 'analytics', icon: IconChart },
  { id: 'automation', icon: IconAutomation },
  { id: 'calendar', icon: IconDate },
] as const;
const FEATURE_ICON_SIZE = 18;

function useSpaceRefresh(space: Space) {
  const client = useQueryClient();
  return async () => {
    await client.invalidateQueries({ queryKey: ['space', space.key] });
    await client.invalidateQueries({ queryKey: ['space-id', space.id] });
    await client.invalidateQueries({ queryKey: ['overview', space.id] });
  };
}

/** The switches of a space in one card; each applies at once and says so (SPC-116, NAV-167). */
export function FeaturesSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const refresh = useSpaceRefresh(space);
  const saved = useSaved();
  const capabilities = useCapabilities();
  const save = useMutation({
    mutationFn: (values: Record<string, boolean>) => api.put(`/spaces/${space.id}/features`, values),
    onSuccess: saved,
    onSettled: refresh,
  });
  // A switch of an experimental part is absent while the part is off for the instance.
  const shown = FEATURES.filter((f) => f.id !== 'automation' || capabilities.automation);
  return (
    <>
      <ErrorNote error={save.error} />
      <section className="settings-card" aria-labelledby="features-card">
        <h2 id="features-card" className="settings-card-head">
          {t('spaceFeatures.card')}
        </h2>
        <ul className="settings-card-rows">
          {shown.map(({ id, icon: Icon }) => (
            <li key={id} className="settings-card-row">
              <Icon size={FEATURE_ICON_SIZE} />
              <Switch checked={space.features?.[id] ?? true} onChange={(on) => save.mutate({ [id]: on })}>
                {t(`spaceSettings.feature_${id}`)}
              </Switch>
            </li>
          ))}
        </ul>
      </section>
    </>
  );
}
/** A fixed list, and renaming, which merges when the new name already exists. */
export function LabelsSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['space-labels', space.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () =>
      api.get<{ labels: LabelCount[]; fixed: boolean; fixed_labels: string[] }>(`/spaces/${space.id}/labels`),
  });
  const refresh = () => client.invalidateQueries({ queryKey: key });
  const fixed = useMutation({
    mutationFn: (v: { fixed: boolean; labels: string[] }) => api.put(`/spaces/${space.id}/labels`, v),
    onSettled: refresh,
  });
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const rename = useMutation({
    mutationFn: () => api.post(`/spaces/${space.id}/labels/rename`, { from, to }),
    onSuccess: () => {
      setFrom('');
      setTo('');
    },
    onSettled: refresh,
  });
  const used = q.data?.labels ?? [];
  const list = q.data?.fixed_labels ?? [];
  return (
    <section>
      <ErrorNote error={q.error ?? fixed.error ?? rename.error} />
      <label className="check">
        <input
          type="checkbox"
          checked={q.data?.fixed ?? false}
          onChange={(e) =>
            fixed.mutate({
              fixed: e.target.checked,
              labels: list.length > 0 ? list : used.map((l) => l.label),
            })
          }
        />
        {t('spaceSettings.fixedLabels')}
      </label>
      {q.data?.fixed && <p className="muted">{list.join(', ')}</p>}
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          rename.mutate();
        }}
      >
        <select
          required
          aria-label={t('spaceSettings.renameFrom')}
          value={from}
          onChange={(e) => setFrom(e.target.value)}
        >
          <option value="">{t('spaceSettings.renameFrom')}</option>
          {used.map((l) => (
            <option key={l.label} value={l.label}>
              {l.label}
            </option>
          ))}
        </select>
        <input
          required
          value={to}
          aria-label={t('spaceSettings.renameTo')}
          placeholder={t('spaceSettings.renameTo')}
          onChange={(e) => setTo(e.target.value)}
        />
        <button type="submit">{t('spaceSettings.rename')}</button>
      </form>
    </section>
  );
}

/** The animal or character of a space, and a picture of its own (spec 083, step 4a). */
export function PictureSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const refresh = useSpaceRefresh(space);
  const [icon, setIcon] = useState(space.icon);
  const choose = useMutation({
    mutationFn: () =>
      api.patch(`/spaces/${space.id}`, { name: space.name, description: space.description, icon }),
    onSuccess: refresh,
  });
  const upload = useMutation({
    mutationFn: (file: File) => api.upload(`/spaces/${space.id}/icon`, file),
    onSuccess: refresh,
  });
  const removeIcon = useMutation({
    mutationFn: () => api.del(`/spaces/${space.id}/icon`),
    onSuccess: refresh,
  });
  return (
    <>
      {space.can?.['space.administer'] && (
        <form
          className="settings-unit"
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            choose.mutate();
          }}
        >
          <ErrorNote error={choose.error} />
          <AnimalChoice spaceKey={space.key} name={space.name} value={icon} onChange={setIcon} />
          <SaveReset
            changed={icon !== space.icon}
            busy={choose.isPending}
            onReset={() => setIcon(space.icon)}
          />
        </form>
      )}
      {space.can?.['space.look'] && (
        <section className="settings-unit" aria-labelledby="uploaded-picture">
          <h2 id="uploaded-picture">{t('spaceDetails.uploaded')}</h2>
          <p className="muted">{t('spaceDetails.uploadedHelp')}</p>
          <ErrorNote error={upload.error ?? removeIcon.error} />
          <div className="row">
            <FilePicker
              label={t('spaceSettings.picture')}
              accept="image/png,image/jpeg,image/gif,image/webp"
              onPick={(f) => upload.mutate(f)}
            />
            {space.has_icon && (
              <button type="button" className="quiet" onClick={() => removeIcon.mutate()}>
                {t('spaceSettings.removePicture')}
              </button>
            )}
          </div>
        </section>
      )}
    </>
  );
}

type TitleAlign = 'start' | 'center';

/** The title alignment of new pages as two tiles (PAG-126, SPC-143, SPC-151). */
export function LayoutSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const refresh = useSpaceRefresh(space);
  const stored: TitleAlign = space.title_align === 'center' ? 'center' : 'start';
  const [align, setAlign] = useState<TitleAlign>(stored);
  const save = useMutation({
    mutationFn: () => api.put(`/spaces/${space.id}/title-align`, { align }),
    onSuccess: refresh,
  });
  return (
    <form
      className="space-form"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ErrorNote error={save.error} />
      <ChoiceTiles
        legend={t('spaceLayout.title')}
        value={align}
        onChange={setAlign}
        tiles={[
          { value: 'start', title: t('spaceLayout.start'), help: t('spaceLayout.startHelp') },
          { value: 'center', title: t('spaceLayout.centre'), help: t('spaceLayout.centreHelp') },
        ]}
      />
      <SaveReset changed={align !== stored} busy={save.isPending} onReset={() => setAlign(stored)} />
    </form>
  );
}
/** Deleting a space asks for its key, and offers something milder first (SPC-073). */
export function DeleteSpaceDialog({
  space,
  onDelete,
  onClose,
}: {
  space: Space;
  onDelete: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [typed, setTyped] = useState('');
  return (
    <Dialog title={t('space.deleteSpace')} onClose={onClose}>
      <p>{t('space.confirmDelete')}</p>
      <p className="muted">{t('spaceSettings.deleteInstead')}</p>
      <Field label={t('spaceSettings.typeKey', { key: space.key })}>
        <input value={typed} autoFocus onChange={(e) => setTyped(e.target.value)} />
      </Field>
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button
          type="button"
          className="danger"
          disabled={typed !== space.key}
          onClick={() => {
            onClose();
            onDelete();
          }}
        >
          {t('space.deleteSpace')}
        </button>
      </div>
    </Dialog>
  );
}

export function DeleteSpaceButton({ space, onDelete }: { space: Space; onDelete: () => void }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" className="danger" onClick={() => setOpen(true)}>
        {t('space.deleteSpace')}
      </button>
      {open && <DeleteSpaceDialog space={space} onDelete={onDelete} onClose={() => setOpen(false)} />}
    </>
  );
}
