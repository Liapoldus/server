import fs from 'node:fs';
import ts from 'typescript';
const base='/Users/docup/Projects/Liapoldus Engine/plugins/server/';
let patch='*** Begin Patch\n';
function put(file,old,next){if(old===next)return;patch+=`*** ${old===null?'Add':'Update'} File: ${base+file}\n`+(old===null?'':'@@\n'+old.trimEnd().split('\n').map(l=>'-'+l).join('\n')+'\n')+next.trimEnd().split('\n').map(l=>'+'+l).join('\n')+'\n';}
const files=fs.readdirSync('contracts/v1').filter(f=>f.endsWith('.json'));
if(!fs.existsSync('tests/support/contracts.ts'))put('tests/support/contracts.ts',null,`import { readFileSync } from 'node:fs';\nimport { parseJSON } from './json.js';\n\ninterface Documents {\n${files.map(f=>` '${f}': typeof import('../../contracts/v1/${f}').default;`).join('\n')}\n}\n\nexport function readContract<K extends keyof Documents>(name: K): Documents[K] {\n return parseJSON<Documents[K]>(readFileSync(new URL('../../contracts/v1/'+name,import.meta.url),'utf8'));\n}\n`);
const shapes={
 'acme-lifecycle': 'interface Result { revision: string; certificateStatus: { serial: string }; tlsProbe: unknown; renewal: unknown }',
 'config-apply': 'interface Result { calls: unknown[]; activeRevision: string; activeConfig: string; revision: string; certificateStatus: unknown }',
 'config-apply-schema-runtime': 'interface Result { calls: unknown[]; runtime: { lastValidated: unknown }; activeRevision: string; activeConfig: unknown }',
 'http-stream-limits-runtime': 'interface Result { [key: string]: unknown; concurrent: { status: number }; maximum: { status: number; body: string }; backpressure: { pausedSentChunks: number; totalChunks: number; bodyBytes: number } }',
 'site-publish-cross-process': 'interface Result { outcomes: { accepted: boolean; code: string }[] }',
 'site-publish-cross-process-recovery': 'interface Result { acceptingProcessKilled: boolean; accepted: unknown; recoveredBy: unknown[]; expectedDigest: string }',
 'site-publish-runtime': 'interface Result { [key: string]: unknown; accepted: { operationId: string }; replay: { operationId: string }; conflict: { code: string }; digestMismatch: { code: string }; duplicateMetadata: { code: string }; rollback: { operationId: string }; rollbackRepeat: { operationId: string } }',
 'route-matching': 'interface Result { responses: { status: number; body: string }[]; candidateCodes: string[]; revisionAfterCandidates: string }',
 'upstream-pools': 'interface Result { responses: { status: number; body: string }[] }',
 'settings-vectors-runtime': 'interface Result { accepted: boolean; status: number; location: string }',
};
for(const file of fs.readdirSync('tests').filter(f=>f.endsWith('.ts') && new RegExp(process.argv[2]??'.').test(f))){
 const old=fs.readFileSync('tests/'+file,'utf8'); let next=old;
 const ast=ts.createSourceFile(file,next,ts.ScriptTarget.Latest,true);
 const replacements=[];
 function walk(node){
  if(ts.isCallExpression(node)&&node.expression.getText(ast)==='JSON.parse'){
   const text=node.getText(ast);const contract=text.match(/contracts\/v1\/([^"`]+\.json)/)?.[1] ?? (text.includes('dispatch.responseCookies.schema')?'http-response-action.schema.json':null);
   if(contract&&files.includes(contract))replacements.push({start:node.getStart(ast),end:node.end,text:`readContract('${contract}')`});
   else { const asserted=ts.isAsExpression(node.parent);const key=file.replace('.test.ts','');replacements.push({start:node.expression.getStart(ast),end:node.expression.end,text: asserted?'parseJSON':`parseJSON<${shapes[key]?'Result':'Record<string, unknown>'}>`}); }
  }
  ts.forEachChild(node,walk);
 }walk(ast);
 for(const r of replacements.sort((a,b)=>b.start-a.start))next=next.slice(0,r.start)+r.text+next.slice(r.end);
 // Async artifact helpers now use the same precise filename-indexed reader.
 next=next.replace(/await json\(`\$\{root\}\/contracts\/v1\/([^`]+)`\)/g,(_,name)=>`readContract('${name}')`);
 next=next.replace(/async function json\(path: string\): Promise<any> \{[^]*?\n}\n/,'');
 next=next.replace(/\((\w+): any\)/g,'($1)').replace(/ as any\[\]/g,'');
 next=next.replace(/Promise<Record<string, any>>|Promise<any>/g,'Promise<Result>');
 if(file==='settings-vectors-runtime.test.ts') next=next.replace(/function requiredVector\(id: string\): any/,'function requiredVector(id: string)').replace(/function (runRuntime|runFixture)\(([^]*?)\): any/g,'function $1($2): Result');
 if(file==='route-matching.test.ts')next=next.replace(/function vector\(id: string\): any/,'function vector(id: string)');
 if(next.includes('readContract('))next=`import { readContract } from './support/contracts.js';\n`+next;
 if(next.includes('parseJSON'))next=`import { parseJSON } from './support/json.js';\n`+next;
 if(shapes[file.replace('.test.ts','')]&&next.includes('Result'))next+='\n'+shapes[file.replace('.test.ts','')]+'\n';
 put('tests/'+file,old,next);
}
console.log(patch+'*** End Patch');
