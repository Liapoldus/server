import { readFileSync } from 'node:fs';
import { parseJSON } from './json.js';
import type { Operation, Surface } from './models.js';

interface Documents {
 'admin-actions.json': Omit<typeof import('../../contracts/v1/admin-actions.json'), 'operations'> & { operations: Record<string, Operation> };
 'admin-actions.schema.json': typeof import('../../contracts/v1/admin-actions.schema.json');
 'admin-query.json': typeof import('../../contracts/v1/admin-query.json');
 'admin-query.schema.json': typeof import('../../contracts/v1/admin-query.schema.json');
 'admin-surface-vectors.json': typeof import('../../contracts/v1/admin-surface-vectors.json');
 'admin-surface.json': Surface;
 'admin-surface.schema.json': typeof import('../../contracts/v1/admin-surface.schema.json');
 'artifact-metadata.schema.json': typeof import('../../contracts/v1/artifact-metadata.schema.json');
 'artifact-operation-result.schema.json': typeof import('../../contracts/v1/artifact-operation-result.schema.json');
 'http-dispatch.json': typeof import('../../contracts/v1/http-dispatch.json');
 'http-response-action.schema.json': typeof import('../../contracts/v1/http-response-action.schema.json');
 'http-stream-vectors.json': typeof import('../../contracts/v1/http-stream-vectors.json');
 'http-stream.json': typeof import('../../contracts/v1/http-stream.json');
 'http-stream.schema.json': typeof import('../../contracts/v1/http-stream.schema.json');
 'plugin.json': typeof import('../../contracts/v1/plugin.json');
 'settings-semantics.json': typeof import('../../contracts/v1/settings-semantics.json');
 'settings-vectors.json': typeof import('../../contracts/v1/settings-vectors.json');
 'settings.schema.json': typeof import('../../contracts/v1/settings.schema.json');
 'site-manifest-semantics.json': typeof import('../../contracts/v1/site-manifest-semantics.json');
 'site-manifest-vectors.json': typeof import('../../contracts/v1/site-manifest-vectors.json');
 'site-manifest.schema.json': typeof import('../../contracts/v1/site-manifest.schema.json');
 'site-publish-manifest.json': typeof import('../../contracts/v1/site-publish-manifest.json');
}

export function readContract<K extends keyof Documents>(name: K): Documents[K] {
 return parseJSON<Documents[K]>(readFileSync(new URL('../../contracts/v1/'+name,import.meta.url),'utf8'));
}
