import { describe, expect, it } from 'vitest';
import { HTTPError } from 'ky';
import { readApiError } from './api-error';

function httpError(status: number, body: unknown): HTTPError {
  return new HTTPError(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'content-type': 'application/json' },
    }),
    new Request('http://localhost/api/subscriptions/sub-1'),
    {} as never
  );
}

describe('readApiError', () => {
  it('reads the referenced groups from a 409 envelope', async () => {
    const info = await readApiError(
      httpError(409, {
        error: {
          code: 'referenced',
          message: 'provider is referenced',
          references: ['Group A', 'Group B'],
        },
      })
    );
    expect(info.status).toBe(409);
    expect(info.code).toBe('referenced');
    expect(info.references).toEqual(['Group A', 'Group B']);
  });

  it('returns nulls for non-HTTP errors and unparsable bodies', async () => {
    expect((await readApiError(new Error('boom'))).status).toBeNull();
    const info = await readApiError(
      new HTTPError(new Response('not json', { status: 500 }), new Request('http://localhost/'), {} as never)
    );
    expect(info.status).toBe(500);
    expect(info.message).toBe('');
    expect(info.references).toBeNull();
  });
});
