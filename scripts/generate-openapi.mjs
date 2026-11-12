import {writeFileSync} from 'node:fs';
const string={type:'string'},uuid={type:'string',format:'uuid'},date={type:'string',format:'date-time'},locale={type:'string',enum:['ru','en','cs','de']};
const object=(properties,required=Object.keys(properties))=>({type:'object',additionalProperties:false,properties,required});
const ref=n=>({$ref:'#/components/schemas/'+n});
const schemas={
 Error:object({error:object({code:string})}),OK:object({ok:{type:'boolean'}}),
 User:object({id:uuid,email:{type:'string',format:'email'},name:string,preferred_locale:locale,created_at:date}),
 Organization:object({id:uuid,name:string,country:{type:'string',pattern:'^[A-Z]{2}$'},timezone:string,default_locale:locale,role:{type:'string',enum:['owner','admin','operator']},created_at:date},['id','name','country','timezone','default_locale','created_at']),
 Assistant:object({id:uuid,organization_id:uuid,name:string,description:string,default_locale:locale,language_policy:{type:'string',enum:['visitor','fixed']},status:{type:'string',enum:['draft']},created_at:date,updated_at:date}),
 Site:object({id:uuid,organization_id:uuid,assistant_id:uuid,name:string,domain:string,status:{type:'string',enum:['unverified']},created_at:date}),
 Register:object({email:{type:'string',format:'email',maxLength:254},password:{type:'string',minLength:12,maxLength:128},preferred_locale:locale}),
 Login:object({email:string,password:{type:'string',maxLength:128}}),
 ProfilePatch:object({name:{type:'string',maxLength:120},preferred_locale:locale},[]),
 OrganizationCreate:object({name:{type:'string',minLength:1,maxLength:160},country:{type:'string',pattern:'^[A-Za-z]{2}$'},timezone:{...string,default:'UTC'},default_locale:{...locale,default:'en'}},['name','country']),
 AssistantCreate:object({name:{type:'string',minLength:1,maxLength:160},description:{type:'string',maxLength:4000},default_locale:locale,language_policy:{type:'string',enum:['visitor','fixed'],default:'visitor'},status:{type:'string',enum:['draft']}},['name']),
 SiteCreate:object({name:{type:'string',minLength:1,maxLength:160},domain:{type:'string',maxLength:253,description:'Bare DNS domain; IDN supported. No URL, port, IP, wildcard or path.'},assistant_id:uuid}),
 Config:object({brand_name:string,region:{type:'string',enum:['ru','eu']},default_locale:locale}),Health:object({status:string})
};
schemas.OrganizationPatch={...schemas.OrganizationCreate,required:[]};schemas.AssistantPatch={...schemas.AssistantCreate,required:[]};
const paths={};let count=0;
function add(method,path,summary,response,{input,status=200,auth=true,array=false}={}){const operation={operationId:'operation'+(++count),summary,security:auth?[{session:[]}]:[],responses:{[status]:{description:'Success',content:{'application/json':{schema:array?{type:'array',items:ref(response)}:ref(response)}}}}};for(const code of [401,403,404,409,422,429,500])operation.responses[code]={description:({401:'Authentication required',403:'Permission or CSRF denied',404:'Resource not found (also returned to nonmembers)',409:'Conflict',422:'Validation failed',429:'Rate limit',500:'Internal error'})[code],content:{'application/json':{schema:ref('Error')}}};
 const params=[...path.matchAll(/\{(\w+)\}/g)].map(m=>({name:m[1],in:'path',required:true,schema:uuid}));if(method!=='get'){params.push({name:'Origin',in:'header',required:true,schema:string,description:'Must equal PUBLIC_APP_URL'},{name:'X-CSRF-Protection',in:'header',required:true,schema:{type:'string',enum:['1']}});operation.requestBody={required:true,content:{'application/json':{schema:input?ref(input):object({},[])}}}}if(params.length)operation.parameters=params;
 (paths[path]??={})[method]=operation;
}
add('get','/healthz','Liveness','Health',{auth:false});add('get','/readyz','Database readiness','Health',{auth:false});paths['/readyz'].get.responses[503]={description:'Database unavailable',content:{'application/json':{schema:ref('Error')}}};
add('get','/api/v1/config','Public regional branding','Config',{auth:false});
add('post','/api/v1/auth/register','Register; identical response for existing email, no session','OK',{auth:false,input:'Register',status:202});
add('post','/api/v1/auth/login','Sign in; sets HttpOnly session cookie','User',{auth:false,input:'Login'});
add('post','/api/v1/auth/logout','Revoke current session and clear cookie','OK');
add('get','/api/v1/auth/me','Current user','User');add('get','/api/v1/profile','Current profile','User');add('patch','/api/v1/profile','Update name or locale','User',{input:'ProfilePatch'});
add('get','/api/v1/organizations','Memberships of current user','Organization',{array:true});add('post','/api/v1/organizations','Create organization and owner membership atomically','Organization',{input:'OrganizationCreate',status:201});
const base='/api/v1/organizations/{id}';add('get',base,'Organization accessible to member','Organization');add('patch',base,'Update organization (owner only)','Organization',{input:'OrganizationPatch'});
add('get',base+'/assistants','List assistants (any member)','Assistant',{array:true});add('post',base+'/assistants','Create draft assistant (owner/admin)','Assistant',{input:'AssistantCreate',status:201});add('get',base+'/assistants/{assistantId}','Get assistant (any member)','Assistant');add('patch',base+'/assistants/{assistantId}','Update assistant (owner/admin); active rejected','Assistant',{input:'AssistantPatch'});
add('get',base+'/sites','List sites (any member)','Site',{array:true});add('post',base+'/sites','Add unverified site (owner/admin)','Site',{input:'SiteCreate',status:201});add('delete',base+'/sites/{siteId}','Delete site (owner/admin)','OK');
const states={type:'string',enum:['pending','processing','ready','failed']};
schemas.KnowledgeInput=object({title:{type:'string',maxLength:240},content:{type:'string',maxLength:2097152},language:{type:'string',enum:['','ru','en','cs','de']},enabled:{type:'boolean'}},[]);
schemas.ArticleCreate={...schemas.KnowledgeInput,required:['title','content']};
schemas.KnowledgeSource=object({id:uuid,organization_id:uuid,assistant_id:uuid,kind:{type:'string',enum:['article','document','web']},title:string,content:{...string,description:'Extracted plain text, omitted as empty string in list responses'},language:string,url:string,format:string,enabled:{type:'boolean'},checksum:string,revision:{type:'integer'},active_revision:{type:'integer',nullable:true},status:states,error_code:string,crawl:{type:'boolean'},imported_pages:{type:'integer'},created_at:date,updated_at:date});
schemas.ImportURL=object({url:{type:'string',format:'uri',maxLength:2000},crawl:{type:'boolean',default:false}},['url']);
schemas.SearchInput=object({query:{type:'string',minLength:1,maxLength:2000},mode:{type:'string',enum:['hybrid','semantic'],default:'hybrid'}},['query']);
schemas.SearchHit=object({chunk_id:uuid,document_id:uuid,title:string,content:string,url:string,page:{type:'integer',nullable:true},score:{type:'number',description:'RRF score for hybrid, cosine similarity for semantic. Not a probability.'},similarity:{type:'number'},index_version:{type:'integer'},model:string,model_version:string,language:string});
const kb=base+'/assistants/{assistantId}/knowledge';
add('get',kb,'List assistant knowledge (members; content fetched separately)','KnowledgeSource',{array:true});
add('post',kb+'/articles','Create article and queue indexing (owner/admin)','KnowledgeSource',{input:'ArticleCreate',status:202});
add('get',kb+'/{documentId}','Read extracted plain text (members)','KnowledgeSource');
add('patch',kb+'/{documentId}','Update metadata/enabled; content editable only for articles; queue new revision','KnowledgeSource',{input:'KnowledgeInput',status:202});
add('delete',kb+'/{documentId}','Delete source, jobs and all chunks (owner/admin)','OK');
add('post',kb+'/{documentId}/reindex','Reindex or manually refresh web source (owner/admin)','KnowledgeSource',{status:202});
add('post',kb+'/import-url','Queue public HTML import, optional same-host crawl up to configured cap','KnowledgeSource',{input:'ImportURL',status:202});
add('post',kb+'/upload','Upload one original document and queue extraction/indexing','KnowledgeSource',{status:202});
paths[kb+'/upload'].post.requestBody={required:true,content:{'multipart/form-data':{schema:object({file:{type:'string',format:'binary',description:'One TXT/MD/PDF/DOCX file, default max 10 MiB'}})}}};
add('post',kb+'/search','Retrieve tenant/assistant-scoped passages; requires available embedding model','SearchHit',{input:'SearchInput',array:true});
const optionalTokens={type:'integer',format:'int64',nullable:true};
const aiProps={display_name:{type:'string',maxLength:120},role:{type:'string',maxLength:200},company_description:{type:'string',maxLength:2000},style:{type:'string',maxLength:300},instructions:{type:'string',maxLength:3000},no_answer:{type:'string',maxLength:600},language_policy:{type:'string',enum:['auto','fixed','default']},response_locale:locale,offer_operator:{type:'boolean'}};
schemas.AISettings=object(aiProps);schemas.AISettingsPatch=object(aiProps,[]);
schemas.AISettingsResponse=object({settings:ref('AISettings'),provider:{type:'string',enum:['disabled','mock','ollama']},model:string,demo:{type:'boolean'},max_message_chars:{type:'integer'}});
schemas.Conversation=object({id:uuid,organization_id:uuid,assistant_id:uuid,kind:{type:'string',enum:['preview']},status:{type:'string',enum:['open','closed']},created_at:date,updated_at:date});
schemas.ConversationMessage=object({id:uuid,execution_id:uuid,sequence:{type:'integer',format:'int64'},role:{type:'string',enum:['user','assistant']},content:string,language:string,status:{type:'string',enum:['processing','completed','failed','canceled']},created_at:date});
schemas.AIExecution=object({id:uuid,request_id:uuid,provider:string,model:string,status:{type:'string',enum:['processing','completed','failed','canceled']},duration_ms:{type:'integer',format:'int64'},input_tokens:optionalTokens,output_tokens:optionalTokens,provider_duration_ns:optionalTokens,error_code:string,sources:{type:'array',items:ref('SearchHit')},response_language:string,policy_fallback:{type:'boolean'},created_at:date});
schemas.ConversationDetail=object({conversation:ref('Conversation'),messages:{type:'array',items:ref('ConversationMessage')},executions:{type:'array',items:ref('AIExecution')},next_before:optionalTokens});
schemas.MessageInput=object({request_id:{...uuid,description:'Client UUID idempotency key. Same key/body replays result; changed body conflicts.'},content:{type:'string',minLength:1,maxLength:2000}});
schemas.AIResult=object({execution:ref('AIExecution'),user_message:ref('ConversationMessage'),assistant_message:ref('ConversationMessage'),replay:{type:'boolean'}});
const aiBase=base+'/assistants/{assistantId}';const conversations=aiBase+'/preview/conversations';
add('get',aiBase+'/ai/settings','Read assistant AI settings and configured provider mode (member)','AISettingsResponse');
add('patch',aiBase+'/ai/settings','Update subordinate instructions (owner/admin)','AISettingsResponse',{input:'AISettingsPatch'});
add('get',conversations,'List up to 100 latest preview conversations (member)','Conversation',{array:true});
add('post',conversations,'Create preview conversation (owner/admin)','Conversation',{status:201});
add('get',conversations+'/{conversationId}','History, 100 latest messages with before cursor (member)','ConversationDetail');
paths[conversations+'/{conversationId}'].get.parameters.push({name:'before',in:'query',required:false,schema:{type:'integer',format:'int64',minimum:1}});
add('delete',conversations+'/{conversationId}','Cancel and delete preview, messages and executions (owner/admin)','OK');
add('post',conversations+'/{conversationId}/cancel','Cancel the active execution, idempotent (owner/admin)','OK');
add('post',conversations+'/{conversationId}/messages','Generate a non-streaming reply, server-side history and RAG (owner/admin)','AIResult',{input:'MessageInput'});
add('post',conversations+'/{conversationId}/messages/stream','POST SSE: start, delta, complete, error. Same cookie/Origin/CSRF as JSON requests. No URL tokens.','AIResult',{input:'MessageInput'});
paths[conversations+'/{conversationId}/messages/stream'].post.responses[200]={description:'SSE start data=AIResult (processing), delta data={text}, complete data=AIResult (including replay), error data={code}. Partial text is never persisted as completed. HTTP errors before the stream remain JSON.',content:{'text/event-stream':{schema:{type:'string'}}}};
for(const p of Object.keys(paths).filter(p=>p.includes('/preview/')||p.endsWith('/ai/settings')))for(const op of Object.values(paths[p]))for(const [status,description]of [[502,'Invalid or incomplete inference response'],[503,'Disabled/unavailable provider or generation capacity exhausted'],[504,'Generation timeout']])op.responses[status]={description,content:{'application/json':{schema:ref('Error')}}};
for(const path of Object.keys(paths).filter(p=>p.includes('/knowledge')))for(const op of Object.values(paths[path])){for(const [status,description]of [[413,'Upload, archive, extraction or text size exceeded'],[503,'Embedding or extractor unavailable/invalid']])op.responses[status]={description,content:{'application/json':{schema:ref('Error')}}}}
writeFileSync(new URL('../docs/openapi.json',import.meta.url),JSON.stringify({openapi:'3.0.3',info:{title:'site-assistant',version:'0.3.0',description:'Stages 1–3. Same-origin cookie API; writes require exact Origin, JSON Content-Type (multipart only for knowledge/upload) and X-CSRF-Protection: 1. No CORS. Login/register: 10 attempts per peer IP per 15 minutes, 4 concurrent password operations per process. Error messages are localized by clients from error.code.'},servers:[{url:'http://localhost:8080'}],paths,components:{securitySchemes:{session:{type:'apiKey',in:'cookie',name:'session'}},schemas}},null,2)+'\n');
