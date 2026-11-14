// SPDX-License-Identifier: Apache-2.0
// The band of a controlled page, under the byline (spec 064, DOC-302, DOC-303, DOC-422):
// readers see the state and the approved version; authors also see that what they look at
// is not approved yet, and the steps they can take.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type ControlInfo, type PageDetail, type WorkflowTransition } from '../api';
import { IconArchive, IconClock, IconPencil, IconVerified } from './icons';
import { Dialog, ErrorNote, Field, RelativeTime } from './ui';

const MARK_ICON_SIZE = 16;

/** Whether the person may open the review panel: a reviewer now, or an author of a page in review. */
export function mayOpenReview(page: PageDetail): boolean {
  const c = page.control;
  return !!c && (c.reviewing || (c.in_review && page.can['page.edit'] === true));
}

/** The band near the title of a controlled page. An ordinary page has none. */
export function ControlBand({ page, onReview }: { page: PageDetail; onReview: () => void }) {
  const { t } = useTranslation();
  const [step, setStep] = useState<WorkflowTransition | null>(null);
  const c = page.control;
  if (!c) return null;
  const approved =
    c.approved_version === null ? (
      t('control.neverApproved')
    ) : (
      <>
        <IconVerified size={MARK_ICON_SIZE} /> {t('control.approvedVersion', { number: c.approved_version })}
        {c.approved_at && (
          <>
            {' '}
            {t('control.approvedBy', { name: c.approved_by })} <RelativeTime value={c.approved_at} />
          </>
        )}
      </>
    );
  const governed =
    c.governed_by === 'space'
      ? t('control.governedSpace', { workflow: c.workflow_name })
      : c.governed_by === 'branch'
        ? t('control.governedBranch', { workflow: c.workflow_name, page: c.governed_from_title })
        : t('control.governedPage', { workflow: c.workflow_name });
  return (
    <div role="status" className="note control-band" data-kind={c.state_kind}>
      <p>
        <StateIcon kind={c.state_kind} /> <strong>{c.state_name}</strong> {approved}
      </p>
      {c.shows_working && <p>{t('control.working', { number: c.head_version })}</p>}
      {c.state_kind === 'obsolete' && <p>{t('control.withdrawn')}</p>}
      <p className="control-governed">{governed}</p>
      {c.reviewing && <p>{t('review.youAreAsked')}</p>}
      {(c.transitions.length > 0 || mayOpenReview(page)) && (
        <p className="control-steps">
          {mayOpenReview(page) && (
            <button type="button" className={c.reviewing ? 'primary' : undefined} onClick={onReview}>
              {t('review.open')}
            </button>
          )}
          {c.transitions.map((tr) => (
            <button key={tr.id} type="button" onClick={() => setStep(tr)}>
              {tr.name}
            </button>
          ))}
        </p>
      )}
      {step && <StepDialog page={page} control={c} step={step} onClose={() => setStep(null)} />}
    </div>
  );
}

function StepDialog({
  page,
  control,
  step,
  onClose,
}: {
  page: PageDetail;
  control: ControlInfo;
  step: WorkflowTransition;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [comment, setComment] = useState('');
  const take = useMutation({
    mutationFn: () =>
      api.post(`/pages/${page.id}/transitions`, {
        transition: step.id,
        version: control.head_version,
        comment,
      }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
      onClose();
    },
  });
  return (
    <Dialog title={step.name} onClose={onClose}>
      {step.description && <p>{step.description}</p>}
      <Field label={step.requires_comment ? t('control.commentNeeded') : t('control.comment')}>
        <textarea value={comment} onChange={(e) => setComment(e.target.value)} />
      </Field>
      <ErrorNote error={take.error} />
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button
          type="button"
          className="primary"
          disabled={take.isPending || (step.requires_comment && comment.trim() === '')}
          onClick={() => take.mutate()}
        >
          {step.name}
        </button>
      </div>
    </Dialog>
  );
}

/** The kind of a state as an icon beside its name, so the state never rests on colour (DOC-920). */
function StateIcon({ kind }: { kind: ControlInfo['state_kind'] }) {
  // An approved state shows the mark of its approved version beside it.
  if (kind === 'approved') return null;
  if (kind === 'in_review') return <IconClock size={MARK_ICON_SIZE} />;
  if (kind === 'obsolete') return <IconArchive size={MARK_ICON_SIZE} />;
  return <IconPencil size={MARK_ICON_SIZE} />;
}
