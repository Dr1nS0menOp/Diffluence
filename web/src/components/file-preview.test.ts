// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { isTextFile } from './file-preview';

describe('which files show as text', () => {
  it('are text, JSON, XML and YAML', () => {
    for (const type of [
      'text/plain',
      'text/markdown; charset=utf-8',
      'application/json',
      'application/ld+json',
      'application/xml',
      'image/svg+xml',
      'application/yaml',
      'application/x-yaml',
    ]) {
      expect(isTextFile(type), type).toBe(true);
    }
  });

  it('are not office files whose type only has xml or json somewhere in its name', () => {
    for (const type of [
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      'application/pdf',
      'image/png',
      'application/octet-stream',
    ]) {
      expect(isTextFile(type), type).toBe(false);
    }
  });
});
