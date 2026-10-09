import fs from 'node:fs';
const base='/Users/docup/Projects/Liapoldus Engine/plugins/server/';
let patch='*** Begin Patch\n';
function edit(file, transform) {const old=fs.readFileSync(file,'utf8'); const next=transform(old); if(next!==old)patch+=`*** Update File: ${base+file}\n@@\n`+old.trimEnd().split('\n').map(l=>'-'+l).join('\n')+'\n'+next.trimEnd().split('\n').map(l=>'+'+l).join('\n')+'\n';}
function replaceFunction(s,name,body) { const re=new RegExp(`func ${name}\\([^]*?^}`, 'm'); if(!re.test(s))throw Error(name); return s.replace(re,body); }
function defs(s) {return s.replace('import (','import (\n"liapoldus.local/server-plugin/contracts/definitions"');}
for(const file of ['assets.go','plugin_manifest.go','site_artifact.go','site_manifest.go','admin_queries.go','dispatch.go','site_archive.go']) {
 edit('contracts/'+file,s=>{
  s=defs(s).replace(/\n\s*"embed"/g,'').replace(/\n\s*"io\/fs"/g,file==='site_manifest.go'?'\n"io/fs"':'');
  s=s.replace(/\/\/go:embed[^\n]*\nvar \w+ embed.FS\n/g,'');
  s=s.replace(/fs.ReadFile\(files, "v1\/([^"\n]+)"\)/g,'definitions.Bytes("$1")');
  s=s.replace(/fs.ReadFile\(siteManifestFiles, schemaPath\)/g,'definitions.Bytes(strings.TrimPrefix(schemaPath, "v1/"))');
  if(file==='plugin_manifest.go')return 'package contracts\n\nimport "liapoldus.local/server-plugin/contracts/definitions"\n\nfunc PluginManifest() ([]byte,error) { return definitions.Bytes("plugin.json") }\n';
  if(file==='site_archive.go')s=replaceFunction(s,'LoadSiteArchiveLimits',`func LoadSiteArchiveLimits() (SiteArchiveLimits,error) {\n v := definitions.AdminActions().Archive\n return SiteArchiveLimits{MinArtifactBytes:int64(v.MinArtifactBytes), ArtifactBytes:int64(v.ArtifactBytes), MetadataBytes:int64(v.MetadataBytes), MultipartOverhead:int64(v.MultipartOverheadBytes), ExpandedBytes:int64(v.ExpandedBytes), MaxEntries:int64(v.MaxFiles), MaxCompressionRatio:int64(v.MaxCompressionRatio)},nil\n}`).replace(/\n\s*"encoding\/json"/g,'');
  if(file==='site_manifest.go')s=replaceFunction(s,'loadSitePublishManifestLink',`func loadSitePublishManifestLink() (sitePublishManifestLink,error) {\n v := definitions.SiteLink()\n return sitePublishManifestLink{ArchiveEntry:v.ArchiveEntry,Schema:v.Schema,PublishCapability:v.PublishCapability,RollbackCapability:v.RollbackCapability},nil\n}`);
  if(file==='admin_queries.go') {
   for(const [fn,type,expr] of [['AdminQueryDefaultLimit','int','DefaultLimit'],['AdminQueryFallbackCapability','string','FallbackCapability'],['AdminQueryActionID','string','ActionID'],['CertificateStatusCapability','string','CertificateStatusCapability']]) s=replaceFunction(s,fn,`func ${fn}() (${type},error) { return definitions.AdminQuery().${expr},nil }`);
   s=replaceFunction(s,'AdminActionSuccessStatus',`func AdminActionSuccessStatus(capability string) (int,error) {\n operation,exists := definitions.AdminActions().Operations[capability]\n if !exists || operation.HTTPStatus==nil || *operation.HTTPStatus<200 || *operation.HTTPStatus>299 { return 0,ErrInvalidAssets }\n return *operation.HTTPStatus,nil\n}`);
   s=replaceFunction(s,'AdminProblem',`func AdminProblem(capability,category string) (int,string,error) {\n operation,exists := definitions.AdminActions().Operations[capability]\n if !exists { return 0,"",ErrInvalidAssets }\n problem,exists := operation.Errors[category]\n if !exists || problem.HTTP<400 || problem.HTTP>599 { return 0,"",ErrInvalidAssets }\n return problem.HTTP,category,nil\n}`);
   s=s.replace(/\tqueryCatalog, err := definitions.Bytes\("admin-query.json"\)[^]*?\tcapability, exists := catalog.Resources/m,'\tcatalog := definitions.AdminQuery()\n\tcapability, exists := catalog.Resources');
   s=replaceFunction(s,'AdminActionCapability',`func AdminActionCapability(pageID,actionID string) (string,error) {\n for capability,operation := range definitions.AdminActions().Operations {\n if operation.SurfaceBinding!=nil && operation.SurfaceBinding.PageID==pageID && operation.SurfaceBinding.ActionID==actionID { return capability,nil }\n }\n return "",ErrInvalidAssets\n}`);
   s=replaceFunction(s,'validAdminQueryPayload',`func validAdminQueryPayload(capability string,payload []byte) bool { return validateCapabilityRequest(capability,payload)==nil }`);
   s=replaceFunction(s,'validateCapabilityRequest',`func validateCapabilityRequest(capability string,payload []byte) error {\n operation,exists:=definitions.AdminActions().Operations[capability]\n if !exists || operation.RequestSchema==nil { return ErrInvalidAssets }\n schema,err:=json.Marshal(operation.RequestSchema)\n if err!=nil { return ErrInvalidAssets }\n return validateAgainstSchema(schema,payload,capability)\n}`);
  }
  if(file==='site_artifact.go') {
   s=replaceFunction(s,'sitePublishProblem',`func sitePublishProblem(code string) (int,string,error) { return AdminProblem(definitions.SiteLink().PublishCapability,code) }`);
   s=replaceFunction(s,'AdminActionProblem',`func AdminActionProblem(pageID,actionID,category string) (int,string,error) {\n capability,err:=AdminActionCapability(pageID,actionID)\n if err!=nil { return 0,"",err }\n return AdminProblem(capability,category)\n}`);
   s=s.replace(/\tcontents, err := definitions.Bytes\("admin-actions.json"\)[^]*?\toperation, exists := document.Operations/m,'\tdocument := definitions.AdminActions()\n\toperation, exists := document.Operations');
   s=s.replace(/if !exists \|\| (operation|statusOperation|rollbackOperation)\.SurfaceBinding\.PageID/g,'if !exists || $1.SurfaceBinding == nil || $1.SurfaceBinding.PageID');
  }
  if(file==='dispatch.go') {
   // Keep the public runtime types; construct them from typed declarations.
   const types=[...s.matchAll(/type (\w+) struct \{([^]*?)\n}/g)];
   const structs=new Map(types.map(m=>[m[1],[...m[2].matchAll(/^\s*(\w+)\s+(\w+)\s+`json:"([^" ]+)"`/gm)].map(x=>({field:x[1],type:x[2],json:x[3]}))]));
   const goField=k=>k[0].toUpperCase()+k.slice(1);
   function fields(type,src){return structs.get(type).filter(f=>f.field!=='Stream').map(f=>{let key=f.field; if(f.field==='SSE')key='Sse';if(f.field==='WebSocket')key='Websocket';if(f.field==='IDField')key='IdField';const value=['HTTPResponseCookies','HTTPStreamWebSocket','HTTPStreamSSE'].includes(f.type)?`${f.type}{${fields(f.type,src+'.'+key)}}`:f.type==='int64'?`int64(${src}.${key})`:`${src}.${key}`;return `${f.field}: ${value},`;}).join('\n');}
   // Slice fields are not captured by the simple scalar matcher.
   s=replaceFunction(s,'LoadHTTPDispatch',`func LoadHTTPDispatch() (HTTPDispatch,error) {\n v:=definitions.Dispatch()\n stream,err:=LoadHTTPStream()\n if err!=nil { return HTTPDispatch{},err }\n return HTTPDispatch{${fields('HTTPDispatch','v')}\nBlockedHeaders: v.BlockedHeaders, Stream:stream},nil\n}`);
   s=s.replace('ValidateAllBeforeHeaders: v.ResponseCookies.ValidateAllBeforeHeaders,','ValidateAllBeforeHeaders: v.ResponseCookies.ValidateAllBeforeHeaders, SameSiteValues:v.ResponseCookies.SameSiteValues,');
   s=replaceFunction(s,'LoadHTTPStream',`func LoadHTTPStream() (HTTPStream,error) {\n v:=definitions.Stream()\n return HTTPStream{${fields('HTTPStream','v')}},nil\n}`).replace(/\n\s*"encoding\/json"/g,'');
  }
  return s;
 });
}
console.log(patch+'*** End Patch');
