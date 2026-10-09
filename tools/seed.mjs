// One-time mechanical translation of audited versioned documents to Go declarations.
import fs from 'node:fs';
import crypto from 'node:crypto';
const files = fs.readdirSync('contracts/v1').filter(f => f.endsWith('.json')).sort();
const field = k => ({'*':'Star', '**':'DoubleStar', '?':'Question', '$schema':'Schema', '$id':'ID', '$defs':'Defs', '$ref':'Ref', actionId:'ActionID', page:'PageID', section:'SectionID', action:'ActionID', certificateStatusCapability:'CertificateStatusCapability'}[k] ?? k.replace(/(^|[-_])([a-z])/g, (_,a,b)=>b.toUpperCase()).replace(/Id$/,'ID').replace(/Http/g,'HTTP').replace(/Sha256/g,'SHA256'));
const name = f => field(f.replace(/\.json$/,'').replace(/\./g,'-'));
const mapKeys = new Set(['operations','errors','properties','$defs','resources','pageResources','stateMapping','rowInput','responses','resolved']);
const schemaStrings = ['$schema','$id','$ref','title','description','format','pattern'];
const schemaNumbers = ['minLength','maxLength','minimum','maximum','minItems','maxItems','minProperties'];
function scalar(v) {
  if(v===null)return 'nil';
  if(Array.isArray(v))return `[]any{${v.map(scalar).join(', ')}}`;
  if(typeof v==='object')return `map[string]any{${Object.entries(v).map(([k,x])=>`${JSON.stringify(k)}: ${scalar(x)}`).join(', ')}}`;
  return JSON.stringify(v);
}
function schemaLiteral(v) {
  return 'Schema{\n'+Object.entries(v).map(([k,x])=>{
    let l;
    if(['properties','$defs'].includes(k))l=`map[string]Schema{${Object.entries(x).map(([p,s])=>`${JSON.stringify(p)}: ${schemaLiteral(s)}`).join(', ')}}`;
    else if(['items','propertyNames','if','then','else','not'].includes(k))l=`valuePtr(${schemaLiteral(x)})`;
    else if(['oneOf','allOf'].includes(k))l=`[]Schema{${x.map(schemaLiteral).join(', ')}}`;
    else if(schemaNumbers.includes(k))l=`valuePtr(int(${x}))`;
    else if(k==='uniqueItems')l=`valuePtr(${x})`;
    else if(k==='required')l=`valuePtr([]string{${x.map(JSON.stringify).join(', ')}})`;
    else if(k==='additionalProperties' && typeof x==='object')l=schemaLiteral(x);
    else l=scalar(x);
    return `${field(k)}: ${l},`;
  }).join('\n')+'\n}';
}
function infer(values, key='') {
  if((key==='schema' || /^(request|response|input)Schema$/.test(key)) && values.filter(v=>v!==undefined).every(v=>v!==null && typeof v==='object' && !Array.isArray(v))) return {kind:'schema'};
  values = values.filter(v => v !== undefined);
  if (values.some(v=>v===null)) return {kind:'any'};
  const kinds = new Set(values.map(v=>Array.isArray(v)?'array':typeof v));
  if (kinds.size !== 1) return {kind:'any'};
  const kind = [...kinds][0];
  if (kind==='object') {
    if(mapKeys.has(key)) return {kind:'map', item:infer(values.flatMap(v=>Object.values(v)))};
    const keys = [...new Set(values.flatMap(v=>Object.keys(v)))];
    return {kind:'object', fields:keys.map(k=>({key:k, optional:values.some(v=>!Object.hasOwn(v,k)), type:infer(values.map(v=>v[k]),k)}))};
  }
  if(kind==='array') return {kind:'array',item:infer(values.flat())};
  return {kind:kind==='number'?'int':kind==='boolean'?'bool':kind ?? 'any'};
}
function build(doc, docName) {
  const types=[];
  function type(t,n) {
    if(t.kind==='schema') return 'Schema';
    if(t.kind==='object') {
      t.name=n;
      const lines=t.fields.map(f=>{f.goType=type(f.type,n+field(f.key)); return `${field(f.key)} ${f.optional?'*':''}${f.goType} \`json:"${f.key}${f.optional?',omitempty':''}"\``;});
      types.push(`type ${n} struct {\n${lines.join('\n')}\n}`);
      return n;
    }
    if(t.kind==='array') return '[]'+type(t.item,n+'Item');
    if(t.kind==='map') return 'map[string]'+type(t.item,n+'Entry');
    return t.kind==='string'?'string':t.kind;
  }
  function literal(v,t) {
    if(t.kind==='schema') return schemaLiteral(v);
    if(t.kind==='object') return `${t.name}{\n${t.fields.filter(f=>Object.hasOwn(v,f.key)).map(f=>`${field(f.key)}: ${f.optional?(f.type.kind==='any'?'valuePtr[any](':'valuePtr(') : ''}${literal(v[f.key],f.type)}${f.optional?')':''},`).join('\n')}\n}`;
    if(t.kind==='array') return `[]${typeName(t.item)}{${v.map(x=>literal(x,t.item)).join(', ')}}`;
    if(t.kind==='map') return `map[string]${typeName(t.item)}{\n${Object.entries(v).map(([k,x])=>`${JSON.stringify(k)}: ${literal(x,t.item)},`).join('\n')}\n}`;
    if(t.kind==='any') {
      if(v===null)return 'nil';
      if(Array.isArray(v))return `[]any{${v.map(x=>literal(x,{kind:'any'})).join(', ')}}`;
      if(typeof v==='object')return `map[string]any{${Object.entries(v).map(([k,x])=>`${JSON.stringify(k)}: ${literal(x,{kind:'any'})}`).join(', ')}}`;
    }
    if(t.kind==='int')return `int(${v})`;
    return JSON.stringify(v);
  }
  function typeName(t){return t.kind==='schema'?'Schema':t.kind==='object'?t.name:t.kind==='array'?'[]'+typeName(t.item):t.kind==='map'?'map[string]'+typeName(t.item):t.kind;}
  const tree=infer([doc],docName.endsWith('Schema')?'schema':'');type(tree,docName+'Document');
  return types.join('\n\n')+`\n\nfunc define${docName}() ${typeName(tree)} {\nreturn ${literal(doc,tree)}\n}\n`;
}
let patch='*** Begin Patch\n';
function add(file,text){patch+=`*** Add File: /Users/docup/Projects/Liapoldus Engine/plugins/server/${file}\n`+text.split('\n').map(l=>'+'+l).join('\n')+'\n';}
const docs=files.map(f=>({f,doc:JSON.parse(fs.readFileSync('contracts/v1/'+f,'utf8'))}));
add('contracts/definitions/schema.go',`package definitions\n\n// Schema is the declarative JSON Schema vocabulary used by Server contracts.\n// JSON-valued keywords preserve union types, false, zero and empty arrays.\ntype Schema struct {\n${schemaStrings.map(k=>`${field(k)} string \`json:"${k},omitempty"\``).join('\n')}\n${schemaNumbers.map(k=>`${field(k)} *int \`json:"${k},omitempty"\``).join('\n')}\nType any \`json:"type,omitempty"\`\nConst any \`json:"const,omitempty"\`\nDefault any \`json:"default,omitempty"\`\nEnum []any \`json:"enum,omitempty"\`\nAdditionalProperties any \`json:"additionalProperties,omitempty"\`\nUniqueItems *bool \`json:"uniqueItems,omitempty"\`\nRequired *[]string \`json:"required,omitempty"\`\nProperties map[string]Schema \`json:"properties,omitempty"\`\nDefs map[string]Schema \`json:"$defs,omitempty"\`\nItems *Schema \`json:"items,omitempty"\`\nPropertyNames *Schema \`json:"propertyNames,omitempty"\`\nIf *Schema \`json:"if,omitempty"\`\nThen *Schema \`json:"then,omitempty"\`\nElse *Schema \`json:"else,omitempty"\`\nNot *Schema \`json:"not,omitempty"\`\nOneOf []Schema \`json:"oneOf,omitempty"\`\nAllOf []Schema \`json:"allOf,omitempty"\`\n}\n`);
for(const {f,doc} of docs) add('contracts/definitions/'+f.replace('.json','.go').replaceAll('-','_'), 'package definitions\n\n'+build(doc,name(f)));
const paths=docs.map(({f})=>`"${f}": define${name(f)}(),`).join('\n');
add('contracts/definitions/documents.go',`// Package definitions owns Server v1 product declarations. JSON files are exports.\npackage definitions\n\nimport "encoding/json"\n\nfunc valuePtr[T any](value T) *T { return &value }\n\nfunc Documents() map[string]any { return map[string]any{\n${paths}\n} }\n\nfunc Bytes(name string) ([]byte,error) {\n value, ok := Documents()[name]\n if !ok { return nil, &UnknownDocument{Name: name} }\n return json.Marshal(value)\n}\n\ntype UnknownDocument struct { Name string }\nfunc (err *UnknownDocument) Error() string { return "unknown Server contract: " + err.Name }\n\nfunc AdminActions() AdminActionsDocument { return defineAdminActions() }\nfunc AdminQuery() AdminQueryDocument { return defineAdminQuery() }\nfunc Dispatch() HTTPDispatchDocument { return defineHTTPDispatch() }\nfunc Stream() HTTPStreamDocument { return defineHTTPStream() }\nfunc SiteLink() SitePublishManifestDocument { return defineSitePublishManifest() }\n`);
console.log(patch+'*** End Patch');
