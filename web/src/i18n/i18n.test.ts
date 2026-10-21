import { describe, expect, it } from 'vitest';
import { en } from './en';
import { zh } from './zh';

function placeholders(value: string): string[] {
  // `{s}` is an English plural suffix, not an interpolation variable.
  return [...value.matchAll(/\{(\w+)\}/g)]
    .map((m) => m[1])
    .filter((name) => name !== 's')
    .sort();
}

describe('i18n dictionaries', () => {
  it('cover exactly the same keys', () => {
    expect(Object.keys(zh).sort()).toEqual(Object.keys(en).sort());
  });

  it('have no empty translations', () => {
    for (const [key, value] of Object.entries({ ...en, ...zh })) {
      expect(value.trim(), key).not.toBe('');
    }
  });

  it('preserve interpolation placeholders', () => {
    for (const key of Object.keys(en) as Array<keyof typeof en>) {
      expect(placeholders(zh[key]), key).toEqual(placeholders(en[key]));
    }
  });
});
