import { execFileSync } from 'node:child_process';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '..');

describe('manually started Server plugin', () => {
  it('builds independently against the current Plugin SDK', () => {
    expect(() => execFileSync('go', ['build', './...'], {
      cwd: root,
      env: {
        ...process.env,
        GOWORK: 'off',
        GOFLAGS: [process.env.GOFLAGS, '-p=1'].filter(Boolean).join(' '),
      },
      stdio: 'pipe',
    })).not.toThrow();
  }, 120_000);
});
