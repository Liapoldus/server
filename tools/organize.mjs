// Mechanical move/import rewrite; content and WIP move together.
import fs from 'node:fs';import path from 'node:path';
const moves=new Map();
function move(from,to){if(!fs.existsSync(from)&&fs.existsSync(to)){moves.set(from,to);return;}if(fs.existsSync(to))throw Error('destination exists '+to);fs.mkdirSync(path.dirname(to),{recursive:true});fs.renameSync(from,to);moves.set(from,to);}
const modelGroups={settings:['settings.go','settings_definition.go','settings_secret_references.go','settings_test.go'],certificate:['certificate_inventory_error.go','certificate_page.go','certificate_status.go','certificate_summary.go'],site:['site_release.go']};
const symbols=new Map();
for(const [group,files] of Object.entries(modelGroups))for(const file of files){
 const old='internal/domain/models/'+file;
 const short=({settings:'model',settings_definition:'definition',settings_secret_references:'secrets',settings_test:'model_test',certificate_inventory_error:'error',certificate_page:'page',certificate_status:'status',certificate_summary:'summary',site_release:'release'})[file.replace('.go','')];
 const s=fs.readFileSync(fs.existsSync(old)?old:`internal/domain/models/${group}/${short}.go`,'utf8');
 for(const m of s.matchAll(/(?:type|func|const|var)\s+([A-Z]\w*)|^\s*([A-Z]\w*)\s*(?:=|\w+\s*=)/gm))symbols.set(m[1]??m[2],group);
 move(old,`internal/domain/models/${group}/${short}.go`);
}
for(const [from,to] of [['internal/application/configuration.go','internal/application/settings/service.go'],['internal/application/configuration_test.go','internal/application/settings/service_test.go'],['internal/application/site_publish.go','internal/application/site/publish.go'],['internal/presentation/restplugin/settings_secret_purposes.go','internal/presentation/restplugin/purposes.go'],['internal/presentation/restplugin/settings_secret_purposes_test.go','internal/presentation/restplugin/purposes_test.go'],['internal/infrastructure/site/release_store.go','internal/infrastructure/site/store.go']])move(from,to);
// Test categories keep runtime, contract and fixture responsibilities visible.
const categories={settings:['config-apply-schema-runtime','config-apply','settings-compiler','settings-schema','settings-vectors-runtime','http-request-cookie-settings','http-stream-limits-contract'],http:['http-dispatch','header-limits','http-response-cookie-contract','http-stream-contract','http-stream-limits-runtime','route-matching','upstream-pools','websocket-dispatch-runtime'],tls:['acme-lifecycle','custom-tls'],site:['site-archive-runtime','site-manifest-contract','site-manifest-runtime','site-publish-cross-process-recovery','site-publish-cross-process','site-publish-runtime'],lifecycle:['plugin-sdk-reload','manual-sdk-process','caddy-rest-process'],build:['caddy-fixture-storage-isolation','dns-provider-inventory','http2-http3','server-binary-http-protocols','pluginprotocol-module-version','v1-scope'],admin:['admin-surface-contract']};
for(const [category,files] of Object.entries(categories))for(const f of files){let short=f.replace(/-runtime$|-contract$/,'').replace(/^site-/,'').replace(/^http-/,'').replace(/^settings-/,'').replace(/-cross-process-recovery$/,'-recovery').replace(/-cross-process$/,'-race');if(f==='site-manifest-runtime')short='manifest-runtime';if(category==='settings'&&f==='settings-schema')short='schema';move(`tests/${f}.test.ts`,`tests/${category}/${short}.test.ts`);}
const contractsGroups={admin:['admin-actions','admin-actions.schema','admin-query','admin-query.schema','admin-surface-vectors','admin-surface','admin-surface.schema'],http:['http-dispatch','http-response-action.schema','http-stream-vectors','http-stream','http-stream.schema'],settings:['settings-semantics','settings-vectors','settings.schema'],site:['artifact-metadata.schema','artifact-operation-result.schema','site-manifest-semantics','site-manifest-vectors','site-manifest.schema','site-publish-manifest'],plugin:['plugin']};
const defFunctions=new Map();
for(const [group,files] of Object.entries(contractsGroups))for(const f of files){
 const old=`contracts/definitions/${f.replaceAll('-','_').replaceAll('.','_')}.go`; // schema filenames retained a dot by seed
 const source=fs.existsSync(old)?old:old.replace('_schema.go','.schema.go');
 const text=fs.readFileSync(source,'utf8');const functionName=text.match(/func (define\w+)\(/)[1];
 const short=f.replace(/^admin-|^http-|^settings-|^site-/,'').replace('artifact-operation-result','receipt').replace('artifact-metadata','metadata').replace('publish-manifest','publish').replace('-vectors','_vectors').replace('-semantics','_semantics').replace('response-action','response').replaceAll('.','_');
 move(source,`contracts/definitions/${group}/${short}.go`);defFunctions.set(functionName,{group,name:functionName.replace(/^define/,'')});
}
move('contracts/definitions/schema.go','contracts/schema/model.go');
function walk(dir){return fs.readdirSync(dir,{withFileTypes:true}).flatMap(e=>e.isDirectory()?walk(path.join(dir,e.name)):[path.join(dir,e.name)]);}
for(const file of [...walk('internal'),...walk('cmd'),...walk('contracts'),...walk('tests')].filter(f=>/\.(go|ts)$/.test(f))){
 let s=fs.readFileSync(file,'utf8');const old=s;
 if(file.endsWith('.go')){
  const groups=new Set();s=s.replace(/models\.(\w+)/g,(m,name)=>{const group=symbols.get(name);if(!group)return m;groups.add(group);return `${group}model.${name}`;});
  if(groups.size)s=s.replace('"liapoldus.local/server-plugin/internal/domain/models"',[...groups].map(g=>`${g}model "liapoldus.local/server-plugin/internal/domain/models/${g}"`).join('\n'));
  const model=file.match(/^internal\/domain\/models\/(\w+)\//);if(model)s=s.replace(/^package models/m,'package '+model[1]);
  const app=file.match(/^internal\/application\/(\w+)\//);if(app)s=s.replace(/^package application/m,'package '+app[1]);
  if(s.includes('"liapoldus.local/server-plugin/internal/application"')){
   const uses=new Set();s=s.replace(/application\.(\w+)/g,(m,name)=>{const g=/^(Configuration|NewConfiguration|ErrInvalidConfiguration|ErrCandidateRejected|ErrActivationFailed|ErrRevision)/.test(name)?'settings':'site';uses.add(g);return `${g}app.${name}`;});
   s=s.replace('"liapoldus.local/server-plugin/internal/application"',[...uses].map(g=>`${g}app "liapoldus.local/server-plugin/internal/application/${g}"`).join('\n'));
  }
  const def=file.match(/^contracts\/definitions\/(\w+)\//);
  if(def){s=s.replace(/^package definitions/m,'package '+def[1]);s=s.replace(/\bvaluePtr/g,'schema.Value').replace(/\bSchema\b(?!:|\s+string|"|,)/g,'schema.Definition');
   // Schema is also a struct field name and must not be rewritten in literals.
   s=s.replace(/schema\.Definition:/g,'Schema:');
   s=s.replace(/func (define\w+)\(/g,(_,f)=>`func ${defFunctions.get(f).name}(`);
   if(s.includes('schema.'))s=s.replace(/^package \w+\n/,'$&\nimport "liapoldus.local/server-plugin/contracts/schema"\n');
  }
  if(file==='contracts/schema/model.go')s=s.replace('package definitions','package schema').replace(/\bSchema\b/g,'Definition')+'\nfunc Value[T any](value T) *T { return &value }\n';
 }
 if(file.endsWith('.ts')&&/^tests\/[^/]+\/[^/]+\.test.ts$/.test(file))s=s.replaceAll("'./support/","'../support/").replace('new URL("..", import.meta.url)','new URL("../..", import.meta.url)').replace(')), "..")',')), "../..")');
 // Source references in build tests must follow the new ownership paths.
 for(const [from,to] of moves)s=s.replaceAll(from,to);
 if(s!==old)fs.writeFileSync(file,s);
}
// Registry wrappers are replaced by imports of declarations at their owners.
let registry=fs.readFileSync('contracts/definitions/documents.go','utf8');
const imports=[...new Set([...defFunctions.values()].map(v=>v.group))].map(g=>`${g}def "liapoldus.local/server-plugin/contracts/definitions/${g}"`).join('\n');
registry=registry.replace('import "encoding/json"',`import ("encoding/json"\n${imports}\n)`);
registry=registry.replace(/func valuePtr[^\n]*\n/,'').replace(/func (AdminActions|AdminQuery|Dispatch|Stream|SiteLink)\([^]*?\n/g,'');
for(const [fn,{group,name}]of defFunctions)registry=registry.replaceAll(fn+'()',group+'def.'+name+'()');
fs.writeFileSync('contracts/definitions/documents.go',registry);
for(const file of walk('contracts').filter(f=>f.endsWith('.go')&&!f.includes('/definitions/'))){let s=fs.readFileSync(file,'utf8');const needed=new Set();for(const [fn,group,name]of [['AdminActions','admin','AdminActions'],['AdminQuery','admin','AdminQuery'],['Dispatch','http','HTTPDispatch'],['Stream','http','HTTPStream'],['SiteLink','site','SitePublishManifest']]){if(s.includes('definitions.'+fn+'(')){needed.add(group);s=s.replaceAll('definitions.'+fn+'(',group+'def.'+name+'(');}}if(needed.size){s=s.replace('import (','import (\n'+[...needed].map(g=>`${g}def "liapoldus.local/server-plugin/contracts/definitions/${g}"`).join('\n'));}fs.writeFileSync(file,s);}
let test=fs.readFileSync('contracts/definitions/documents_test.go','utf8');test=test.replace('import (','import (\nadmin "liapoldus.local/server-plugin/contracts/definitions/admin"\nhttpdef "liapoldus.local/server-plugin/contracts/definitions/http"').replaceAll('AdminActions()','admin.AdminActions()').replaceAll('Dispatch()','httpdef.HTTPDispatch()');fs.writeFileSync('contracts/definitions/documents_test.go',test);
console.log([...moves].map(([a,b])=>a+' -> '+b).join('\n'));
