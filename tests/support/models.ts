// Structural views used while testing optional schema/descriptor fields.
export interface Schema {
  $defs: Record<string, Schema>;
  properties: Record<string, Schema>;
  required: string[];
  type: string | string[];
  const: string | number | boolean | null;
  enum: (string | number | boolean | null)[];
  additionalProperties: boolean | Schema;
  description: string;
}
export interface ArtifactInput {
  mediaTypes: string[];
  maxBytes: number;
  maxMetadataBytes: number;
  maxMultipartOverheadBytes: number;
}
export interface Action {
  id: string;
  capability: string;
  title: string;
  inputSchema: Schema;
  rowInput?: Record<string, string>;
  artifactInput?: ArtifactInput;
}
export interface Section {
  id: string;
  kind: string;
  columns: string[];
  dataCapability: string;
  actions: Action[];
}
export interface Page {
  id: string;
  title: string;
  capability: string;
  permissions: string[];
  sections: Section[];
}
export interface Surface {
  version: number;
  plugin: string;
  requiredCapabilities: string[];
  pages: Page[];
}
export interface Operation {
  kind: string;
  requestSchema?: Schema;
  responseSchema?: Schema;
  receiptSchema?: string;
  surfaceBinding: { page: string; section: string; action: string };
  idempotency: Record<string, unknown>;
  errors: Record<string, { http: number; retryable: boolean }>;
}
export function required<T>(value: T | undefined, context = 'required fixture value'): T {
  if (value === undefined) throw new Error(context);
  return value;
}
