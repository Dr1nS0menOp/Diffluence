// SPDX-License-Identifier: Apache-2.0
// Adding a field, instruction text or a value of the template to a template's text.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { Editor } from '@tiptap/core';
import { Dialog, Field } from '../ui';
import type { TemplateItemKind } from './keys';

/** A value / can insert in a guided template: a built-in value, or a page of the set. */
export interface TemplateValueChoice {
  source: string;
  label: string;
}

/** Puts a value of the template at the cursor (spec 074, TPL-211, TPL-1013). */
function ValueForm({
  editor,
  values,
  onClose,
}: {
  editor: Editor;
  values: TemplateValueChoice[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [source, setSource] = useState(values[0]?.source ?? 'date');
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        editor.chain().focus().insertContent({ type: 'templateValue', attrs: { source } }).run();
        onClose();
      }}
    >
      <Field label={t('templates.valueSource')} help={t('templates.valueSourceHelp')}>
        <select value={source} autoFocus onChange={(e) => setSource(e.target.value)}>
          {values.map((v) => (
            <option key={v.source} value={v.source}>
              {v.label}
            </option>
          ))}
        </select>
      </Field>
      <div className="dialog-actions">
        <button type="submit" className="primary">
          {t('common.add')}
        </button>
      </div>
    </form>
  );
}

const FIELD_KINDS = ['line', 'text', 'choice'];

function InstructionForm({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        editor.chain().focus().insertContent({ type: 'instruction', attrs: { text } }).run();
        onClose();
      }}
    >
      <Field label={t('templates.instructionText')}>
        <input value={text} required autoFocus onChange={(e) => setText(e.target.value)} />
      </Field>
      <div className="dialog-actions">
        <button type="submit" className="primary">
          {t('common.add')}
        </button>
      </div>
    </form>
  );
}

function FieldForm({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [kind, setKind] = useState('line');
  const [options, setOptions] = useState('');
  const [required, setRequired] = useState(false);
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        const attrs = {
          name,
          kind,
          options: kind === 'choice' ? options : null,
          required: required || null,
        };
        editor.chain().focus().insertContent({ type: 'templateField', attrs }).run();
        onClose();
      }}
    >
      <Field label={t('common.name')}>
        <input value={name} required autoFocus onChange={(e) => setName(e.target.value)} />
      </Field>
      <Field label={t('templates.fieldKind')}>
        <select value={kind} onChange={(e) => setKind(e.target.value)}>
          {FIELD_KINDS.map((k) => (
            <option key={k} value={k}>
              {t(`templates.kind_${k}`)}
            </option>
          ))}
        </select>
      </Field>
      {kind === 'choice' && (
        <Field label={t('templates.fieldOptions')} help={t('templates.fieldOptionsHelp')}>
          <textarea rows={3} value={options} required onChange={(e) => setOptions(e.target.value)} />
        </Field>
      )}
      <label className="check">
        <input type="checkbox" checked={required} onChange={(e) => setRequired(e.target.checked)} />
        {t('organise.required')}
      </label>
      <div className="dialog-actions">
        <button type="submit" className="primary">
          {t('common.add')}
        </button>
      </div>
    </form>
  );
}

export function TemplateItemDialog({
  editor,
  what,
  values,
  onClose,
}: {
  editor: Editor;
  what: TemplateItemKind;
  values: TemplateValueChoice[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog title={t(`insert.${what === 'value' ? 'templateValue' : what}.name`)} onClose={onClose}>
      {what === 'field' && <FieldForm editor={editor} onClose={onClose} />}
      {what === 'instruction' && <InstructionForm editor={editor} onClose={onClose} />}
      {what === 'value' && <ValueForm editor={editor} values={values} onClose={onClose} />}
    </Dialog>
  );
}
