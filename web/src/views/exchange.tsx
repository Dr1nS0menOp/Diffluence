// SPDX-License-Identifier: Apache-2.0
// Everything can leave and come in (spec 014): the export dialog, the steps of an
// import, and the rules for administrators.
import { useEffect, useMemo, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type ExportOptions,
  type ExportPolicy,
  type ExportRun,
  type ImportRun,
  type ImportTarget,
  type PublicLink,
  type Space,
  type TreeNode,
} from '../api';
import { Dialog, ErrorNote, Field, FilePicker, Loading, formatSize } from '../components/ui';
import { offeredNames, useCapabilities } from '../capabilities';
import { Link } from '../router';
import { TableBox } from '../components/table-box';

const POLL_MS = 1500;
const ENDED = ['done', 'failed', 'expired'];
const IMPORT_ENDED = ['ready', 'done', 'failed', 'undone'];

type Scope = 'page' | 'branch' | 'chosen' | 'space';

/** What each format can hold, so only that is offered (G1). */
const FORMAT_OPTIONS: Record<string, (keyof ExportOptions)[]> = {
  pdf: ['comments', 'restricted'],
  docx: ['comments', 'restricted'],
  odt: ['comments', 'restricted'],
  html: ['comments', 'versions', 'attachments', 'restricted'],
  markdown: ['restricted'],
  text: ['attachments', 'restricted'],
};

function descendantsOf(nodes: TreeNode[], id: string): string[] {
  const below = nodes.filter((n) => n.parent_id === id);
  return below.flatMap((n) => [n.id, ...descendantsOf(nodes, n.id)]);
}

function ParentBox({
  node,
  nodes,
  chosen,
  onChange,
}: {
  node: TreeNode;
  nodes: TreeNode[];
  chosen: Set<string>;
  onChange: (ids: string[], on: boolean) => void;
}) {
  const ref = useRef<HTMLInputElement>(null);
  const below = useMemo(() => descendantsOf(nodes, node.id), [nodes, node.id]);
  const some = below.some((id) => chosen.has(id));
  const all = below.every((id) => chosen.has(id));
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = below.length > 0 && some && !(all && chosen.has(node.id));
  }, [below.length, some, all, chosen, node.id]);
  return (
    <label className="check">
      <input
        ref={ref}
        type="checkbox"
        checked={chosen.has(node.id)}
        onChange={(e) => onChange([node.id, ...below], e.target.checked)}
      />{' '}
      {node.title}
    </label>
  );
}

/** A nested list of native checkboxes: a tree item may not hold other controls. */
function ChooseList({
  nodes,
  shown,
  parent,
  chosen,
  onChange,
}: {
  nodes: TreeNode[];
  shown: TreeNode[];
  parent: string | null;
  chosen: Set<string>;
  onChange: (ids: string[], on: boolean) => void;
}) {
  const level = shown.filter(
    (n) => n.parent_id === parent || (parent === null && !shown.some((p) => p.id === n.parent_id)),
  );
  if (level.length === 0) return null;
  return (
    <ul className="choose-list">
      {level.map((node) => (
        <li key={node.id}>
          <ParentBox node={node} nodes={nodes} chosen={chosen} onChange={onChange} />
          <ChooseList nodes={nodes} shown={shown} parent={node.id} chosen={chosen} onChange={onChange} />
        </li>
      ))}
    </ul>
  );
}

function ChoosePages({
  spaceId,
  chosen,
  onChange,
}: {
  spaceId: string;
  chosen: Set<string>;
  onChange: (next: Set<string>) => void;
}) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  const tree = useQuery({
    queryKey: ['tree', spaceId],
    queryFn: () => api.get<{ pages: TreeNode[] }>(`/spaces/${spaceId}/tree`),
  });
  const nodes = useMemo(() => tree.data?.pages ?? [], [tree.data]);
  const shown = filter ? nodes.filter((n) => n.title.toLowerCase().includes(filter.toLowerCase())) : nodes;
  const toggle = (ids: string[], on: boolean) => {
    const next = new Set(chosen);
    ids.forEach((id) => (on ? next.add(id) : next.delete(id)));
    onChange(next);
  };
  return (
    <fieldset className="choices">
      <legend>{t('exchange.choosePages')}</legend>
      <Field label={t('exchange.filter')}>
        <input type="search" value={filter} onChange={(e) => setFilter(e.target.value)} />
      </Field>
      <div className="row">
        <button type="button" className="quiet" onClick={() => onChange(new Set(nodes.map((n) => n.id)))}>
          {t('exchange.selectAll')}
        </button>
        <button type="button" className="quiet" onClick={() => onChange(new Set())}>
          {t('exchange.selectNone')}
        </button>
      </div>
      <ChooseList nodes={nodes} shown={shown} parent={null} chosen={chosen} onChange={toggle} />
      <p role="status" className="muted">
        {t('exchange.chosenCount', { count: chosen.size })}
      </p>
    </fieldset>
  );
}

function RunResult({ run }: { run: ExportRun }) {
  const { t } = useTranslation();
  const { report } = run;
  return (
    <div>
      {run.state === 'done' && (
        <>
          <p role="status" className="note">
            {t('exchange.ready', { name: run.filename, size: formatSize(run.size) })}
          </p>
          <p>
            <a className="button primary" href={`/api/v1/exports/${run.id}/file`}>
              {t('exchange.download')}
            </a>
          </p>
          <p className="muted">{t('exchange.dayLimit')}</p>
        </>
      )}
      {run.state === 'failed' && (
        <p role="alert" className="note note-error">
          {report.explanation || t('exchange.failed')}
        </p>
      )}
      {run.state === 'expired' && (
        <p role="status" className="note">
          {t('exchange.expired')}
        </p>
      )}
      {report.skipped > 0 && <p className="muted">{t('exchange.skipped', { count: report.skipped })}</p>}
      {report.restricted > 0 && (
        <p className="muted">{t('exchange.restrictedLeft', { count: report.restricted })}</p>
      )}
      {(report.drafts?.length ?? 0) > 0 && (
        <p className="muted">{t('exchange.draftsLeft', { names: report.drafts?.join(', ') })}</p>
      )}
      {(report.failed?.length ?? 0) > 0 && (
        <p className="muted">{t('exchange.pagesFailed', { names: report.failed?.join(', ') })}</p>
      )}
    </div>
  );
}

function useRun(id: string | null) {
  return useQuery({
    queryKey: ['export', id],
    queryFn: () => api.poll<ExportRun>(`/exports/${id}`),
    enabled: !!id,
    refetchInterval: (q) => (ENDED.includes(q.state.data?.state ?? '') ? false : POLL_MS),
  });
}

function RunProgress({ id }: { id: string }) {
  const { t } = useTranslation();
  const run = useRun(id);
  if (run.error) return <ErrorNote error={run.error} />;
  if (!run.data || !ENDED.includes(run.data.state)) {
    return (
      <div>
        <progress aria-label={t('exchange.working')} />
        <p role="status">{t('exchange.working')}</p>
        <p className="muted">{t('exchange.mayClose')}</p>
      </div>
    );
  }
  return <RunResult run={run.data} />;
}

const NO_OPTIONS: ExportOptions = {
  comments: false,
  versions: false,
  attachments: false,
  restricted: false,
  paper: 'A4',
  orientation: 'portrait',
  contents: true,
};

/** One dialog for every export: format, then what, then the options that fit (G1). */
export function ExportDialog({
  spaceId,
  page,
  onClose,
}: {
  spaceId: string;
  page?: { id: string };
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const formats = useQuery({
    queryKey: ['export-formats'],
    queryFn: () => api.get<{ formats: string[] }>('/export-formats'),
  });
  // A format whose helper is off or does not answer is not offered (ARC-071).
  const offered = offeredNames(
    (formats.data?.formats ?? []).filter((f) => f !== 'bundle'),
    useCapabilities(),
  );
  const [format, setFormat] = useState('');
  const [scope, setScope] = useState<Scope>(page ? 'page' : 'space');
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  const [options, setOptions] = useState(NO_OPTIONS);
  const [runId, setRunId] = useState<string | null>(null);
  const picked = format || offered[0] || '';
  const start = useMutation({
    mutationFn: () => {
      const body: Record<string, unknown> = { space_id: spaceId, format: picked, options };
      if (scope === 'page' && page) body.page_ids = [page.id];
      if (scope === 'branch' && page) body.root_id = page.id;
      if (scope === 'chosen') body.page_ids = [...chosen];
      return api.post<ExportRun>('/exports', body);
    },
    onSuccess: (run) => setRunId(run.id),
  });
  const nothing = scope === 'chosen' && chosen.size === 0;
  const flags = FORMAT_OPTIONS[picked] ?? [];
  const setFlag = (key: keyof ExportOptions) => (e: ChangeEvent<HTMLInputElement>) =>
    setOptions({ ...options, [key]: e.target.checked });
  if (runId) {
    return (
      <Dialog title={t('exchange.export')} onClose={onClose}>
        <RunProgress id={runId} />
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>
      </Dialog>
    );
  }
  return (
    <Dialog title={t('exchange.export')} onClose={onClose} wide>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          start.mutate();
        }}
      >
        <ErrorNote error={formats.error ?? start.error} />
        <Field label={t('exchange.format')}>
          <select value={picked} onChange={(e) => setFormat(e.target.value)}>
            {offered.map((f) => (
              <option key={f} value={f}>
                {t(`exchange.format_${f}`)}
              </option>
            ))}
          </select>
        </Field>
        <fieldset className="choices">
          <legend>{t('exchange.what')}</legend>
          {page && (
            <>
              <label className="check">
                <input
                  type="radio"
                  name="scope"
                  checked={scope === 'page'}
                  onChange={() => setScope('page')}
                />{' '}
                {t('exchange.scopePage')}
              </label>
              <label className="check">
                <input
                  type="radio"
                  name="scope"
                  checked={scope === 'branch'}
                  onChange={() => setScope('branch')}
                />{' '}
                {t('exchange.scopeBranch')}
              </label>
            </>
          )}
          {!page && (
            <label className="check">
              <input
                type="radio"
                name="scope"
                checked={scope === 'space'}
                onChange={() => setScope('space')}
              />{' '}
              {t('exchange.scopeSpace')}
            </label>
          )}
          <label className="check">
            <input
              type="radio"
              name="scope"
              checked={scope === 'chosen'}
              onChange={() => setScope('chosen')}
            />{' '}
            {t('exchange.scopeChosen')}
          </label>
        </fieldset>
        {scope === 'chosen' && <ChoosePages spaceId={spaceId} chosen={chosen} onChange={setChosen} />}
        {flags.length > 0 && (
          <fieldset className="choices">
            <legend>{t('exchange.options')}</legend>
            {flags.map((flag) => (
              <label key={flag} className="check">
                <input type="checkbox" checked={!!options[flag]} onChange={setFlag(flag)} />{' '}
                {t(`exchange.option_${flag}`)}
              </label>
            ))}
          </fieldset>
        )}
        {picked === 'pdf' && (
          <fieldset className="choices">
            <legend>{t('exchange.paper')}</legend>
            <Field label={t('exchange.paperSize')}>
              <select
                value={options.paper}
                onChange={(e) => setOptions({ ...options, paper: e.target.value })}
              >
                <option value="A4">{t('exchange.paperA4')}</option>
                <option value="Letter">{t('exchange.paperLetter')}</option>
              </select>
            </Field>
            <Field label={t('exchange.orientation')}>
              <select
                value={options.orientation}
                onChange={(e) => setOptions({ ...options, orientation: e.target.value })}
              >
                <option value="portrait">{t('exchange.portrait')}</option>
                <option value="landscape">{t('exchange.landscape')}</option>
              </select>
            </Field>
            <label className="check">
              <input type="checkbox" checked={options.contents} onChange={setFlag('contents')} />{' '}
              {t('exchange.contents')}
            </label>
          </fieldset>
        )}
        {nothing && <p className="muted">{t('exchange.chooseFirst')}</p>}
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="primary" disabled={nothing || !picked || start.isPending}>
            {t('exchange.exportButton')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** Where the notification of a finished export leads. */
export function ExportView({ id }: { id: string }) {
  const { t } = useTranslation();
  return (
    <main id="main" className="plain reading">
      <h1>{t('exchange.export')}</h1>
      <RunProgress id={id} />
    </main>
  );
}

function ReportTree({ run }: { run: ImportRun }) {
  const { t } = useTranslation();
  const { report } = run;
  return (
    <>
      <p>
        {t('exchange.reportCounts', {
          pages: t('exchange.countPages', { count: report.pages }),
          files: t('exchange.countFiles', { count: report.files }),
          comments: t('exchange.countComments', { count: report.comments }),
          versions: t('exchange.countVersions', { count: report.versions }),
        })}
      </p>
      {report.tree.length > 0 && (
        <ul className="choose-list">
          {report.tree.map((line, i) => (
            <li key={`${i}-${line.title}`} className={`report-depth-${Math.min(line.depth, 4)}`}>
              {line.title}
            </li>
          ))}
        </ul>
      )}
      {report.warnings.length > 0 && (
        <>
          <h3>{t('exchange.warnings')}</h3>
          <ul>
            {report.warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
        </>
      )}
    </>
  );
}

function PeopleTable({ run, onDone }: { run: ImportRun; onDone: () => void }) {
  const { t } = useTranslation();
  const [choice, setChoice] = useState<ImportTarget>({ people: {}, fingerprint: '', space_key: '' });
  const save = useMutation({
    mutationFn: () => api.put<ImportRun>(`/imports/${run.id}/mapping`, choice),
    onSuccess: onDone,
  });
  const people = run.report.people ?? [];
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ErrorNote error={save.error} />
      <p className="muted">{t('exchange.signer', { fingerprint: run.report.signer })}</p>
      <Field label={t('exchange.fingerprint')} help={t('exchange.fingerprintHelp')}>
        <input
          value={choice.fingerprint}
          onChange={(e) => setChoice({ ...choice, fingerprint: e.target.value })}
        />
      </Field>
      <Field label={t('exchange.spaceKey')} help={t('exchange.spaceKeyHelp')}>
        <input
          value={choice.space_key}
          onChange={(e) => setChoice({ ...choice, space_key: e.target.value })}
        />
      </Field>
      {people.length > 0 && (
        <TableBox>
          <table>
            <caption>{t('exchange.people')}</caption>
            <thead>
              <tr>
                <th scope="col">{t('common.name')}</th>
                <th scope="col">{t('exchange.matchedTo')}</th>
              </tr>
            </thead>
            <tbody>
              {people.map((p) => (
                <tr key={p.id}>
                  <th scope="row">
                    {p.name} <span className="muted">{p.email}</span>
                  </th>
                  <td>{p.matched ? t('exchange.matched') : t('exchange.becomesYou')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
      <button type="submit" className="quiet">
        {t('common.save')}
      </button>
    </form>
  );
}

function useImport(id: string | null) {
  return useQuery({
    queryKey: ['import', id],
    queryFn: () => api.poll<ImportRun>(`/imports/${id}`),
    enabled: !!id,
    refetchInterval: (q) => (IMPORT_ENDED.includes(q.state.data?.state ?? '') ? false : POLL_MS),
  });
}

/** One run of an import: the report, then the run, then Undo. */
function ImportSteps({ id, spaceId }: { id: string; spaceId?: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useImport(id);
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['import', id] });
    if (spaceId) await client.invalidateQueries({ queryKey: ['tree', spaceId] });
  };
  const run = useMutation({ mutationFn: () => api.post(`/imports/${id}/run`), onSuccess: refresh });
  const undo = useMutation({
    mutationFn: (rest: boolean) => api.post(`/imports/${id}/undo`, { rest }),
    onSuccess: refresh,
  });
  const data = q.data;
  const state = data?.state;
  useEffect(() => {
    if (spaceId && (state === 'done' || state === 'failed' || state === 'undone')) {
      void client.invalidateQueries({ queryKey: ['tree', spaceId] });
    }
  }, [state, spaceId, client]);
  if (q.error) return <ErrorNote error={q.error} />;
  if (!data || !IMPORT_ENDED.includes(data.state)) {
    return (
      <div>
        <progress aria-label={t('exchange.reading')} />
        <p role="status">{data?.state === 'running' ? t('exchange.importing') : t('exchange.reading')}</p>
      </div>
    );
  }
  const touched = data.touched ?? [];
  return (
    <div>
      <ErrorNote error={run.error ?? undo.error} />
      {data.state === 'failed' && (
        <p role="alert" className="note note-error">
          {data.report.reason || t('exchange.importFailed')}
        </p>
      )}
      {data.state === 'done' && (
        <p role="status" className="note">
          {t('exchange.importDone')}
        </p>
      )}
      {data.state === 'undone' && (
        <p role="status" className="note">
          {t('exchange.undone')}
        </p>
      )}
      <ReportTree run={data} />
      {data.kind === 'bundle' && data.state === 'ready' && <PeopleTable run={data} onDone={refresh} />}
      {data.state === 'ready' && (
        <p>
          <button type="button" className="primary" onClick={() => run.mutate()}>
            {t('exchange.runImport')}
          </button>
        </p>
      )}
      {(data.state === 'done' || data.state === 'failed') &&
        data.kind !== 'bundle' &&
        touched.length === 0 && (
          <p>
            <button type="button" onClick={() => undo.mutate(false)}>
              {t('exchange.undo')}
            </button>
          </p>
        )}
      {touched.length > 0 && data.state === 'done' && (
        <>
          <p>{t('exchange.touched', { names: touched.join(', ') })}</p>
          <button type="button" onClick={() => undo.mutate(true)}>
            {t('exchange.undoRest')}
          </button>
        </>
      )}
    </div>
  );
}

/** Space settings, Import: every kind of file, and every import starts with a report. */
export function ImportSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [id, setId] = useState<string | null>(null);
  const start = useMutation({
    mutationFn: (f: File) =>
      api.upload<{ import_id: string }>(
        `/spaces/${space.id}/imports?report=1&filename=${encodeURIComponent(f.name)}`,
        f,
      ),
    onSuccess: (r) => setId(r.import_id),
  });
  return (
    <section>
      <h2>{t('space.import')}</h2>
      <p className="muted">{t('exchange.importHelp')}</p>
      <FilePicker
        label={t('space.import')}
        accept=".md,.markdown,.docx,.html,.htm,.zip"
        onPick={(f) => start.mutate(f)}
      />
      <ErrorNote error={start.error} />
      {id && <ImportSteps id={id} spaceId={space.id} />}
    </section>
  );
}

function PolicyRow({
  rule,
  onChange,
}: {
  rule: ExportPolicy['rules'][number];
  onChange: (next: ExportPolicy['rules'][number]) => void;
}) {
  const { t } = useTranslation();
  return (
    <tr>
      <th scope="row">{t(`exchange.format_${rule.format}`)}</th>
      <td>
        <input
          type="checkbox"
          aria-label={t('exchange.formatOn', { format: t(`exchange.format_${rule.format}`) })}
          checked={rule.enabled}
          onChange={(e) => onChange({ ...rule, enabled: e.target.checked })}
        />
      </td>
    </tr>
  );
}

/** Administration, Export: which formats, what sizes, and the two public switches. */
export function ExportTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['export-policy'],
    queryFn: () => api.get<ExportPolicy>('/admin/export-policy'),
  });
  const key = useQuery({
    queryKey: ['bundle-key'],
    queryFn: () => api.get<{ fingerprint: string }>('/admin/bundles/key'),
  });
  const links = useQuery({
    queryKey: ['public-links'],
    queryFn: () => api.get<{ links: PublicLink[] }>('/admin/public-links'),
  });
  const [form, setForm] = useState<ExportPolicy | null>(null);
  const [bundle, setBundle] = useState<string | null>(null);
  useEffect(() => {
    if (q.data) setForm(q.data);
  }, [q.data]);
  const save = useMutation({
    mutationFn: () => api.put('/admin/export-policy', { rules: form?.rules, limits: form?.limits }),
    onSuccess: () => client.invalidateQueries({ queryKey: ['export-policy'] }),
  });
  const upload = useMutation({
    mutationFn: (f: File) =>
      api.upload<{ id: string }>(`/admin/bundles?filename=${encodeURIComponent(f.name)}`, f),
    onSuccess: (r) => setBundle(r.id),
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!form) return <Loading />;
  const limits = form.limits;
  const setLimits = (next: Partial<ExportPolicy['limits']>) =>
    setForm({ ...form, limits: { ...limits, ...next } });
  return (
    <>
      <section>
        <h2>{t('exchange.formats')}</h2>
        {!form.pdf_available && <p className="note">{t('exchange.noHelper')}</p>}
        <form
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <ErrorNote error={save.error} />
          <TableBox>
            <table>
              <thead>
                <tr>
                  <th scope="col">{t('exchange.format')}</th>
                  <th scope="col">{t('exchange.formatEnabled')}</th>
                </tr>
              </thead>
              <tbody>
                {form.rules.map((rule) => (
                  <PolicyRow
                    key={rule.format}
                    rule={rule}
                    onChange={(next) =>
                      setForm({
                        ...form,
                        rules: form.rules.map((r) => (r.format === next.format ? next : r)),
                      })
                    }
                  />
                ))}
              </tbody>
            </table>
          </TableBox>
          <Field label={t('exchange.maxExport')}>
            <input
              type="number"
              min={1}
              value={limits.max_export_mib}
              onChange={(e) => setLimits({ max_export_mib: Number(e.target.value) })}
            />
          </Field>
          <Field label={t('exchange.maxImport')}>
            <input
              type="number"
              min={1}
              value={limits.max_import_mib}
              onChange={(e) => setLimits({ max_import_mib: Number(e.target.value) })}
            />
          </Field>
          <label className="check">
            <input
              type="checkbox"
              checked={limits.public_sites_enabled}
              onChange={(e) => setLimits({ public_sites_enabled: e.target.checked })}
            />{' '}
            {t('exchange.sitesOn')}
          </label>
          <label className="check">
            <input
              type="checkbox"
              checked={limits.public_links_enabled}
              onChange={(e) => setLimits({ public_links_enabled: e.target.checked })}
            />{' '}
            {t('exchange.linksOn')}
          </label>
          <p>
            <button type="submit" className="primary">
              {t('common.save')}
            </button>
          </p>
        </form>
      </section>
      <section>
        <h2>{t('exchange.bundles')}</h2>
        <p className="muted">{t('exchange.ownFingerprint', { fingerprint: key.data?.fingerprint ?? '' })}</p>
        <FilePicker label={t('exchange.importBundle')} accept=".zip" onPick={(f) => upload.mutate(f)} />
        <ErrorNote error={upload.error} />
        {bundle && <ImportSteps id={bundle} />}
      </section>
      <section>
        <h2>{t('exchange.publicLinks')}</h2>
        {(links.data?.links.length ?? 0) === 0 && <p className="muted">{t('exchange.noPublicLinks')}</p>}
        <ul>
          {links.data?.links.map((l) => (
            <li key={l.id}>
              <Link href={`/pages/${l.page_id}`}>{l.page_title}</Link>{' '}
              <span className="muted">{t('exchange.madeBy', { name: l.created_by })}</span>
            </li>
          ))}
        </ul>
      </section>
    </>
  );
}

function CopyText({ text }: { text: string }) {
  const { t } = useTranslation();
  const [message, setMessage] = useState('');
  const copy = () => {
    navigator.clipboard.writeText(text).then(
      () => setMessage(t('editor.copied')),
      () => setMessage(t('editor.copyFailed')),
    );
  };
  return (
    <>
      <button type="button" className="ghost" onClick={copy}>
        {t('editor.copy')}
      </button>
      <span role="status" className="muted">
        {message}
      </span>
    </>
  );
}

/** More, Public link: one page for the world, under an address nobody can guess. */
export function PublicLinkDialog({ page, onClose }: { page: { id: string }; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['public-link', page.id];
  const link = useQuery({
    queryKey: key,
    queryFn: () => api.get<PublicLink>(`/pages/${page.id}/public-link`),
    retry: false,
  });
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: key });
    await client.invalidateQueries({ queryKey: ['public-state', page.id] });
  };
  const make = useMutation({
    mutationFn: () => api.put(`/pages/${page.id}/public-link`),
    onSuccess: refresh,
  });
  const withdraw = useMutation({
    mutationFn: () => api.del(`/pages/${page.id}/public-link`),
    onSuccess: refresh,
  });
  return (
    <Dialog title={t('exchange.publicLink')} onClose={onClose}>
      <ErrorNote error={make.error ?? withdraw.error} />
      {link.data ? (
        <>
          <p>{t('exchange.linkVersion', { version: link.data.version })}</p>
          <div className="row">
            <code>{link.data.path}</code>
            <CopyText text={link.data.path} />
          </div>
          <p className="muted">{t('exchange.linkHostHelp')}</p>
          <p className="muted">{t('exchange.linkHelp')}</p>
          <div className="dialog-actions">
            <button type="button" className="danger" onClick={() => withdraw.mutate()}>
              {t('exchange.withdraw')}
            </button>
          </div>
        </>
      ) : (
        <>
          <p>{t('exchange.linkIntro')}</p>
          <div className="dialog-actions">
            <button type="button" className="primary" onClick={() => make.mutate()}>
              {t('exchange.makeLink')}
            </button>
          </div>
        </>
      )}
    </Dialog>
  );
}

/** Space settings, Export: the dialog, and a bundle of everything. */
export function ExportSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [bundle, setBundle] = useState<string | null>(null);
  const start = useMutation({
    mutationFn: () => api.post<ExportRun>(`/spaces/${space.id}/bundles`),
    onSuccess: (run) => setBundle(run.id),
  });
  return (
    <section>
      <h2>{t('exchange.export')}</h2>
      <p className="muted">{t('exchange.exportHelp')}</p>
      <ErrorNote error={start.error} />
      <div className="row">
        {space.can?.['space.export'] && (
          <button type="button" onClick={() => setOpen(true)}>
            {t('exchange.exportButton')}
          </button>
        )}
        {space.can?.['space.bundle'] && space.can?.['space.export'] && (
          <button type="button" onClick={() => start.mutate()}>
            {t('exchange.exportBundle')}
          </button>
        )}
      </div>
      {bundle && <RunProgress id={bundle} />}
      {open && <ExportDialog spaceId={space.id} onClose={() => setOpen(false)} />}
    </section>
  );
}

/** A page the world can read says so, to everyone who writes on it (EXC-043). */
export function PublicMark({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const state = useQuery({
    queryKey: ['public-state', pageId],
    queryFn: () => api.get<{ public: boolean }>(`/pages/${pageId}/public-state`),
  });
  if (!state.data?.public) return null;
  return <span className="lozenge">{t('exchange.publicMark')}</span>;
}

/** The look of a space on paper. */
export function PDFDesignSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['pdf-design', space.id],
    queryFn: () => api.get<PDFDesignForm>(`/spaces/${space.id}/pdf-design`),
  });
  const [form, setForm] = useState<PDFDesignForm | null>(null);
  useEffect(() => {
    if (q.data) setForm(q.data);
  }, [q.data]);
  const save = useMutation({ mutationFn: () => api.put(`/spaces/${space.id}/pdf-design`, form) });
  if (!form) return null;
  return (
    <section>
      <h2>{t('exchange.pdfDesign')}</h2>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <ErrorNote error={save.error} />
        <Field label={t('exchange.coverTitle')}>
          <input
            value={form.cover_title}
            onChange={(e) => setForm({ ...form, cover_title: e.target.value })}
          />
        </Field>
        <Field label={t('exchange.coverSubtitle')}>
          <input
            value={form.cover_subtitle}
            onChange={(e) => setForm({ ...form, cover_subtitle: e.target.value })}
          />
        </Field>
        <Field label={t('exchange.header')} help={t('exchange.marginHelp')}>
          <input value={form.header} onChange={(e) => setForm({ ...form, header: e.target.value })} />
        </Field>
        <Field label={t('exchange.footer')} help={t('exchange.marginHelp')}>
          <input value={form.footer} onChange={(e) => setForm({ ...form, footer: e.target.value })} />
        </Field>
        <button type="submit">{t('common.save')}</button>
      </form>
    </section>
  );
}

interface PDFDesignForm {
  cover_title: string;
  cover_subtitle: string;
  logo_id: string | null;
  header: string;
  footer: string;
}
