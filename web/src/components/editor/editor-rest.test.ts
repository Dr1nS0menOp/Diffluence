// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { getSchema } from '@tiptap/core';
import { chartData, smoothPath } from '../chart';
import { imageShare } from './attachments';
import { keepUnknownBlocks } from './blocks';
import { detectLanguage } from './code';
import { emojiGroups } from './emoji-picker';
import emoji from './emoji.json';
import { documentExtensions, knownBlockType } from './extensions';
import { turnIntoItems } from './insert';
import { KNOWN_NODE_TYPES } from './palette';
import { cleanWordHtml, pastedImages } from './paste';
import { foldedGroups } from './tool-fold';

describe('folding the row of tools', () => {
  it('folds nothing while everything fits', () => {
    expect(foldedGroups([100, 100, 100], 300, 40)).toBe(0);
  });

  it('folds from the end, and leaves room for the More button', () => {
    expect(foldedGroups([100, 100, 100], 299, 40)).toBe(1);
    expect(foldedGroups([100, 100, 100], 239, 40)).toBe(2);
  });

  it('can fold everything on a very small screen', () => {
    expect(foldedGroups([100, 100], 30, 40)).toBe(2);
  });
});

describe('turn into', () => {
  it('starts with what is likely for this kind of block', () => {
    expect(
      turnIntoItems('paragraph')
        .slice(0, 3)
        .map((item) => item.id),
    ).toEqual(['h2', 'bullet', 'task']);
    expect(turnIntoItems('codeBlock')[0]?.id).toBe('text');
  });

  it('keeps every entry', () => {
    expect(turnIntoItems('heading')).toHaveLength(turnIntoItems('paragraph').length);
  });
});

describe('image size', () => {
  it('is a share of the column that stays between its limits', () => {
    expect(imageShare(300, 600)).toBe(50);
    expect(imageShare(9000, 600)).toBe(100);
    expect(imageShare(1, 600)).toBe(5);
    expect(imageShare(300, 0)).toBe(100);
  });
});

describe('pasted images', () => {
  const pixel = 'R0lGODlhAQABAAAAACw=';

  it('takes along what is in the clipboard, and counts what is not', () => {
    const html = `<img src="data:image/gif;base64,${pixel}"><img src="https://example.org/a.png"><img src='https://example.org/b.png'>`;
    const { files, left } = pastedImages(html);
    expect(files).toHaveLength(1);
    expect(files[0]?.type).toBe('image/gif');
    expect(left).toBe(2);
  });

  it('never takes a script as an image', () => {
    const { files, left } = pastedImages(`<img src="data:image/svg+xml;base64,${pixel}">`);
    expect(files).toHaveLength(0);
    expect(left).toBe(1);
  });

  it('counts broken data as left behind, and does not fail', () => {
    const { files, left } = pastedImages('<img src="data:image/png;base64,@@@">');
    expect(files).toHaveLength(0);
    expect(left).toBe(1);
  });

  it('leaves our own images alone', () => {
    expect(pastedImages('<img data-attachment-id="x" src="/api/v1/attachments/x/content">')).toEqual({
      files: [],
      left: 0,
    });
  });
});

describe('text from Word and Google Docs', () => {
  const item = (marker: string, text: string) =>
    `<p class=MsoListParagraph style='mso-list:l0 level1 lfo1'><![if !supportLists]><span>${marker}</span><![endif]>${text}</p>`;

  it('turns a run of Word list paragraphs into one list', () => {
    expect(cleanWordHtml(`${item('·', 'One')}\n${item('·', 'Two')}`)).toBe(
      '<ul><li>One</li><li>Two</li></ul>',
    );
  });

  it('keeps numbers as a numbered list', () => {
    expect(cleanWordHtml(item('1.', 'One'))).toBe('<ol><li>One</li></ol>');
  });

  it('takes away the bold wrapper of Google Docs, which is not bold', () => {
    const html = '<b style="font-weight:normal" id="docs-internal-guid-1"><p>Text</p></b>';
    expect(cleanWordHtml(html)).toBe('<p>Text</p>');
  });

  it('leaves other HTML as it is', () => {
    expect(cleanWordHtml('<p><b>Bold</b></p>')).toBe('<p><b>Bold</b></p>');
  });
});

describe('blocks from a newer release', () => {
  const future = { type: 'hologram', attrs: { depth: 3 } };

  it('stay in the document as a note with the original inside', () => {
    const doc = { type: 'doc', content: [{ type: 'paragraph' }, future] };
    const kept = keepUnknownBlocks(doc, knownBlockType);
    expect(kept.content?.[1]).toEqual({
      type: 'unknownBlock',
      attrs: { name: 'hologram', original: JSON.stringify(future) },
    });
  });

  it('are found at any depth', () => {
    const doc = { type: 'doc', content: [{ type: 'blockquote', content: [future] }] };
    expect(keepUnknownBlocks(doc, knownBlockType).content?.[0]?.content?.[0]?.type).toBe('unknownBlock');
  });

  it('leave a document without them unchanged', () => {
    const doc = { type: 'doc', content: [{ type: 'paragraph' }] };
    expect(keepUnknownBlocks(doc, knownBlockType)).toEqual(doc);
  });
});

describe('the list of known blocks', () => {
  it('is the schema of the editor', () => {
    const schema = getSchema(documentExtensions());
    expect([...KNOWN_NODE_TYPES].sort()).toEqual(Object.keys(schema.nodes).sort());
  });
});

describe('charts', () => {
  const rows = [
    ['Month', 'Sales'],
    ['Jan', '10'],
    ['Feb', '4'],
    ['Jan', '6'],
  ];

  it('keep every row without a total', () => {
    expect(chartData(rows, 0, null).labels).toEqual(['Jan', 'Feb', 'Jan']);
  });

  it('total repeated labels', () => {
    expect(chartData(rows, 0, null, 'sum').series[0]?.values).toEqual([16, 4]);
    expect(chartData(rows, 0, null, 'average').series[0]?.values).toEqual([8, 4]);
    expect(chartData(rows, 0, null, 'count').series[0]?.values).toEqual([2, 1]);
  });

  it('do not count an empty cell as a zero', () => {
    const data = chartData(
      [
        ['A', 'B'],
        ['x', ''],
        ['x', '4'],
      ],
      0,
      null,
      'average',
    );
    expect(data.series[0]?.values).toEqual([4]);
  });

  it('draw a smooth line through every point', () => {
    const path = smoothPath([
      { x: 0, y: 0 },
      { x: 10, y: 5 },
    ]);
    expect(path.startsWith('M 0 0 C')).toBe(true);
    expect(path.endsWith('10 5')).toBe(true);
    expect(smoothPath([])).toBe('');
  });
});

describe('code blocks', () => {
  it('leave too little text alone', () => {
    expect(detectLanguage('x = 1')).toBeNull();
  });

  it('give a guess for a longer piece of code, which the writer can change', () => {
    const code =
      'import os\n\nclass Greeter(object):\n    def __init__(self, name):\n        self.name = name\n';
    expect(detectLanguage(code)).toEqual(expect.any(String));
  });
});

describe('emoji groups', () => {
  it('hold every emoji once', () => {
    const glyphs = emojiGroups().flatMap((group) => group.glyphs);
    expect(glyphs).toHaveLength(emoji.length);
  });
});
