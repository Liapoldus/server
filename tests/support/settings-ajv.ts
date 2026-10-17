import { Ajv2020 } from "ajv/dist/2020.js";

// Studio JSON editors are UI annotations only. Register them with strict Ajv so
// validation remains strict for every actual JSON Schema keyword.
export function settingsAjv(): Ajv2020 {
  const ajv = new Ajv2020({ allErrors: true });
  ajv.addKeyword({ keyword: "x-studio-json-document", schemaType: "boolean", valid: true });
  ajv.addKeyword({ keyword: "x-studio-json-value", schemaType: "boolean", valid: true });
  return ajv;
}
