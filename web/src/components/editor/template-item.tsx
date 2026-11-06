// SPDX-License-Identifier: Apache-2.0
// Adding a field or instruction text to a template.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { Editor } from '@tiptap/core';
import { Dialog, Field } from '../ui';

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
  onClose,
}: {
  editor: Editor;
  what: 'field' | 'instruction';
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog title={t(`insert.${what}.name`)} onClose={onClose}>
      {what === 'field' ? (
        <FieldForm editor={editor} onClose={onClose} />
      ) : (
        <InstructionForm editor={editor} onClose={onClose} />
      )}
    </Dialog>
  );
}
