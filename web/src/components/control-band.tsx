// SPDX-License-Identifier: Apache-2.0
// The band of a controlled page, under the byline (spec 064, DOC-302, DOC-303, DOC-422):
// readers see the state and the approved version; authors also see that what they look at
// is not approved yet, and the steps they can take.
import { useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type ControlInfo, type PageDetail, type WorkflowTransition } from '../api';
import { IconArchive, IconClock, IconPencil, IconVerified } from './icons';
import { Signatures } from './signatures';
import { pendingSigning, SigningDialog } from './signing-dialog';
import { Dialog, ErrorNote, Field, RelativeTime } from './ui';
import { pageTitle } from '../page-name';

const MARK_ICON_SIZE = 16;

/** Whether the person may open the review panel: a reviewer now, or an author of a page in review. */
export function mayOpenReview(page: PageDetail): boolean {
  const c = page.control;
  return !!c && (c.reviewing || (c.in_review && page.can['page.edit'] === true));
}

/** The band near the title of a controlled page. An ordinary page has none. What follows it,
 * such as the release status of a page in a draft space, joins the same band (PUB-1225). */
export function ControlBand({
  page,
  onReview,
  children,
}: {
  page: PageDetail;
  onReview: () => void;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const [step, setStep] = useState<WorkflowTransition | null>(null);
  // A signing step that comes back from the identity provider opens again (DOC-731).
  const [back] = useState(() => new URLSearchParams(window.location.search).get('signing'));
  const [resumed, setResumed] = useState(() => (back === 'confirmed' ? pendingSigning(page.id) : null));
  useEffect(() => {
    if (!back) return;
    const url = new URL(window.location.href);
    url.searchParams.delete('signing');
    window.history.replaceState(window.history.state, '', url);
  }, [back]);
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
        ? t('control.governedBranch', {
            workflow: c.workflow_name,
            page: pageTitle(t, c.governed_from_title),
          })
        : t('control.governedPage', { workflow: c.workflow_name });
  return (
    <div role="status" className="note control-band" data-kind={c.state_kind}>
      <p>
        <StateIcon kind={c.state_kind} /> <strong>{c.state_name}</strong> {approved}
      </p>
      {c.shows_working && <p>{t('control.working', { number: c.head_version })}</p>}
      {c.state_kind === 'obsolete' && <p>{t('control.withdrawn')}</p>}
      <p className="control-governed">{governed}</p>
      {children}
      {c.reviewing && <p>{t('review.youAreAsked')}</p>}
      {back === 'failed' && <p className="note-error">{t('signing.providerFailed')}</p>}
      <Signatures list={c.signatures ?? []} />
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
      {resumed && resumed.step.transition && (
        <SignedStep
          page={page}
          transition={resumed.step.transition}
          version={resumed.step.version}
          comment={resumed.comment}
          resumed={resumed.token}
          onClose={() => setResumed(null)}
        />
      )}
    </div>
  );
}

/** The signing dialog of an author's own step, which takes the step once signed. */
function SignedStep({
  page,
  transition,
  version,
  comment,
  resumed,
  onClose,
}: {
  page: PageDetail;
  transition: string;
  version: number;
  comment: string;
  resumed?: string;
  onClose: () => void;
}) {
  const client = useQueryClient();
  return (
    <SigningDialog
      pageId={page.id}
      step={{ transition, version }}
      comment={comment}
      resumed={resumed}
      onSign={(confirmation) =>
        api.post(`/pages/${page.id}/transitions`, { transition, version, comment, confirmation })
      }
      onSigned={() => {
        void client.invalidateQueries({ queryKey: ['page', page.id] });
        void client.invalidateQueries({ queryKey: ['tree', page.space_id] });
        onClose();
      }}
      onClose={onClose}
    />
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
  const [signing, setSigning] = useState(false);
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
  if (signing) {
    return (
      <SignedStep
        page={page}
        transition={step.id}
        version={control.head_version}
        comment={comment}
        onClose={onClose}
      />
    );
  }
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
          onClick={() => (step.signature_meaning ? setSigning(true) : take.mutate())}
        >
          {step.signature_meaning ? t('signing.continue') : step.name}
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
