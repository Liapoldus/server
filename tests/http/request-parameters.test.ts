import { readContract } from '../support/contracts.js';
import { Ajv2020 } from 'ajv/dist/2020.js';
import { describe, expect, it } from 'vitest';

const dispatch = readContract('http-dispatch.json');
const schema = readContract('http-request-parameters.schema.json');
const validate = new Ajv2020({ allErrors: true }).compile(schema);

describe('Server typed HTTP request parameters contract', () => {
  it('publishes the versioned normalized envelope used by dispatch', () => {
    expect(dispatch.requestParameters).toMatchObject({
      field: 'parameters',
      version: 'server-http/v1',
      schema: 'contracts/v1/http-request-parameters.schema.json',
      values: ['path', 'query', 'headers', 'body'],
    });
    expect(validate({
      version: 'server-http/v1',
      path: { name: 'alice' },
      query: { text: 'hello', tags: ['one', 'two'], count: 3, enabled: true },
      headers: { 'X-Mode': 'safe' },
      body: { message: 'hello' },
    })).toBe(true);
  });

  it('rejects unsupported versions, untyped values, and unexpected envelope keys', () => {
    expect(validate({ version: 'server-http/v2' })).toBe(false);
    expect(validate({ version: 'server-http/v1', query: { object: { nested: true } } })).toBe(false);
    expect(validate({ version: 'server-http/v1', rawSecret: 'must-not-be-added' })).toBe(false);
  });
});
