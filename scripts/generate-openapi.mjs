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
const color={type:'string',pattern:'^#[0-9a-fA-F]{6}$'};
schemas.WidgetCopy=object({title:{...string,maxLength:60},subtitle:{...string,maxLength:140},greeting:{...string,maxLength:400}});
schemas.WidgetTheme=object({texts:{type:'object',properties:Object.fromEntries(['ru','en','cs','de'].map(l=>[l,ref('WidgetCopy')])),additionalProperties:false},primary:color,secondary:color,user_color:color,launcher_color:color,position:{type:'string',enum:['left','right']},offset_x:{type:'integer',minimum:8,maximum:80},offset_y:{type:'integer',minimum:8,maximum:80},show_launcher:{type:'boolean'},remember_session:{type:'boolean'},allow_new:{type:'boolean'},default_locale:locale,auto_locale:{type:'boolean'},privacy_url:{...string,description:'Empty or HTTPS company privacy policy URL'}});
schemas.Widget=object({id:uuid,organization_id:uuid,assistant_id:uuid,site_id:uuid,domain:string,enabled:{type:'boolean'},origins:{type:'array',items:string,maxItems:10},settings:ref('WidgetTheme'),verification_token:string,verified_at:{...date,nullable:true},created_at:date});
schemas.WidgetAdmin=object({widget:ref('Widget'),snippet:string,public_url:string,logo_url:string,verification_path:string,verification_content:string,local_development:{type:'boolean'}});
schemas.WidgetPatch=object({enabled:{type:'boolean'},origins:{type:'array',items:string,minItems:1,maxItems:10},settings:ref('WidgetTheme')},[]);
schemas.PublicWidgetConfig=object({id:uuid,settings:ref('WidgetTheme'),logo_url:string,provider:{type:'string',enum:['disabled','mock','ollama']},demo:{type:'boolean'},available:{type:'boolean'},max_message_chars:{type:'integer'}});
schemas.VisitorSession=object({id:uuid,token:{type:'string',pattern:'^[A-Za-z0-9_-]{43}$',description:'Secret returned once. Keep in parent sessionStorage, never URL/logs.'},expires_at:date});
schemas.PublicMessage=object({id:uuid,request_id:uuid,role:{type:'string',enum:['user','assistant']},content:string,status:{type:'string',enum:['processing','completed','failed','canceled']},created_at:date});
schemas.PublicHistory=object({messages:{type:'array',items:ref('PublicMessage'),maxItems:100}});
const widgetAdmin=base+'/sites/{siteId}/widget';
add('get',widgetAdmin,'Read widget settings and installation code (members)','WidgetAdmin');
add('post',widgetAdmin,'Provision disabled widget, idempotent per site (owner/admin)','WidgetAdmin',{status:201});
add('patch',widgetAdmin,'Update widget/origins; production enable requires domain verification (owner/admin)','WidgetAdmin',{input:'WidgetPatch'});
add('post',widgetAdmin+'/verify','Fetch fixed HTTPS ownership challenge using SSRF-safe importer (owner/admin)','WidgetAdmin');
add('post',widgetAdmin+'/logo','Upload PNG/JPEG logo, 256 KiB and 512x512 max; re-encoded PNG (owner/admin)','WidgetAdmin');
paths[widgetAdmin+'/logo'].post.requestBody={required:true,content:{'multipart/form-data':{schema:object({file:{type:'string',format:'binary'}})}}};
const publicWidget='/api/v1/public/widgets/{widgetId}';
add('get',publicWidget+'/config','Public appearance/provider status; enabled widget and exact allowed Origin','PublicWidgetConfig',{auth:false});
add('post',publicWidget+'/sessions','Create anonymous session and website conversation; no cookies','VisitorSession',{status:201,auth:false});
add('get',publicWidget+'/sessions/{sessionId}/messages','Visitor history, token + widget + session + origin + expiry checked','PublicHistory',{auth:false});
add('post',publicWidget+'/sessions/{sessionId}/messages','Idempotent shared AI pipeline, POST SSE (start/delta/complete/error)','PublicHistory',{input:'MessageInput',auth:false});
add('post',publicWidget+'/sessions/{sessionId}/cancel','Cancel visitor generation; same token/origin scope','OK',{auth:false});
add('get',publicWidget+'/logo','Public company branding image, no visitor/tenant information','OK',{auth:false});
paths[publicWidget+'/logo'].get.responses[200]={description:'Sanitized PNG company logo',content:{'image/png':{schema:{type:'string',format:'binary'}}}};
for(const [path,ops]of Object.entries(paths).filter(([p])=>p.startsWith(publicWidget))){for(const op of Object.values(ops)){
 op.parameters=(op.parameters||[]).filter(p=>p.in==='path');
 if(!path.endsWith('/logo'))op.parameters.push({name:'Origin',in:'header',required:false,schema:string,description:'Direct clients: exact allowed parent Origin. Iframe requests: WIDGET_PUBLIC_URL with X-Widget-Origin. Same-origin GET may omit Origin only with Sec-Fetch-Site: same-origin.'},{name:'X-Widget-Origin',in:'header',required:false,schema:string,description:'Iframe only: actual parent origin validated by loader/postMessage and CSP frame-ancestors.'});
 if(path.includes('/sessions/{'))op.security=[{visitorBearer:[]}];
 for(const [status,description]of [[502,'Invalid/incomplete inference'],[503,'AI disabled, unavailable or globally busy'],[504,'Generation timeout']])op.responses[status]={description,content:{'application/json':{schema:ref('Error')}}};
}}
paths[publicWidget+'/sessions/{sessionId}/messages'].post.responses[200]={description:'SSE start={request_id}; delta={text}; complete={request_id,messages:PublicMessage[]}; error={code}. Incomplete output never persisted as completed. JSON errors before streaming.',content:{'text/event-stream':{schema:string}}};
schemas.ConversationMessage.properties.author_user_id={...uuid,nullable:true};
schemas.AIResult.properties.manual={type:'boolean',description:'Website-only manual acceptance; no AI execution/assistant message.'};
schemas.ConversationMessage.properties.request_id={...string,description:'UUID for visitor/staff request; empty on an AI response in preview.'};
schemas.ConversationMessage.properties.change_seq={type:'integer',format:'int64'};
schemas.PublicMessage.properties.role.enum=['user','assistant','operator'];
schemas.PublicMessage.properties.sequence={type:'integer',format:'int64'};schemas.PublicMessage.properties.change_seq={type:'integer',format:'int64'};
const mode={type:'string',enum:['ai_active','human_requested','human_active','closed']};
schemas.InboxConversation=object({id:uuid,organization_id:uuid,assistant_id:uuid,site_id:uuid,control_mode:mode,state_version:{type:'integer',format:'int64'},assigned_user_id:{...uuid,nullable:true},site:string,assistant:string,assignee:string,last_content:string,unread:{type:'integer'},generating:{type:'boolean'},created_at:date,updated_at:date,last_message_at:{...date,nullable:true},last_visitor_at:{...date,nullable:true},requested_at:{...date,nullable:true},closed_at:{...date,nullable:true}});
schemas.InboxMessage=object({...schemas.ConversationMessage.properties,execution_id:{...string,description:'AI execution UUID or empty for manual messages'},role:{type:'string',enum:['user','assistant','operator']}});
schemas.InboxEvent=object({id:uuid,sequence:{type:'integer',format:'int64'},kind:{type:'string',enum:['human_requested','human_active','ai_active','closed','assigned','note']},actor_id:{...uuid,nullable:true},target_id:{...uuid,nullable:true},content:string,created_at:date});
schemas.InboxPage=object({conversation:ref('InboxConversation'),messages:{type:'array',items:ref('InboxMessage')},events:{type:'array',items:ref('InboxEvent')},cursor:{type:'integer',format:'int64'},next_before:{type:'integer',format:'int64'},more:{type:'boolean'}});
schemas.InboxList=object({items:{type:'array',items:ref('InboxConversation'),maxItems:50},next_cursor:string});
schemas.InboxAction=object({request_id:uuid,version:{type:'integer',format:'int64',minimum:1},content:{type:'string',maxLength:4000},assignee:{...string,description:'Member UUID, or empty to unassign (admin/owner only)'}},['request_id','version']);
schemas.StaffReply=object({request_id:uuid,version:{type:'integer',format:'int64',minimum:1},content:{type:'string',minLength:1,maxLength:2000}});
schemas.ReadInput=object({sequence:{type:'integer',format:'int64',minimum:0}});
schemas.InboxSummary=object({unread:{type:'integer'},requested:{type:'integer'}});
schemas.Member=object({id:uuid,name:string,role:{type:'string',enum:['owner','admin','operator']}});
schemas.Invitation=object({id:uuid,organization_id:uuid,role:{type:'string',enum:['admin','operator']},expires_at:date,revoked_at:{...date,nullable:true},accepted_at:{...date,nullable:true},url:{...string,description:'One-time secret fragment link, returned only on creation'}},['id','organization_id','role','expires_at','revoked_at','accepted_at']);
schemas.InviteInput=object({role:{type:'string',enum:['admin','operator']}});
schemas.AcceptInput=object({token:{type:'string',minLength:43,maxLength:43}});schemas.AcceptedInvitation=object({organization_id:uuid});
const inboxBase=base+'/conversations',inboxItem=inboxBase+'/{conversationId}';
add('get',inboxBase,'Website inbox only. Members; operators see own and unassigned open conversations','InboxList');
for(const name of ['mode','site','assistant','assigned','q','from','to','cursor','limit'])paths[inboxBase].get.parameters.push({name,in:'query',required:false,schema:name==='limit'?{type:'integer',minimum:1,maximum:50,default:30}:string,description:({mode:'Control mode; empty means all',assigned:'Member UUID or me',q:'Literal substring, up to 200 Unicode characters, completed message text only',from:'YYYY-MM-DD UTC inclusive',to:'YYYY-MM-DD UTC inclusive',cursor:'Opaque keyset pagination token'})[name]||name});
for(const path of [inboxItem,inboxItem+'/messages']){add('get',path,'Conversation changes: initial latest 50; after cursor up to 100 changes; before older page. Merge by ID/order by message sequence.','InboxPage');for(const name of ['after','before'])paths[path].get.parameters.push({name,in:'query',required:false,schema:{type:'integer',format:'int64',minimum:0}})}
for(const action of ['take','assign','resume-ai','close','notes'])add('post',inboxItem+'/'+action,'Version checked, idempotent '+action+'; DB-fenced control transitions. Operator only own except take unassigned; assign admin/owner only.','InboxConversation',{input:'InboxAction'});
add('post',inboxItem+'/messages','Staff reply only in human_active, serialized with handoff/resume. Operator only own.','InboxMessage',{input:'StaffReply'});
add('post',inboxItem+'/read','Mark up to supplied message sequence read for the current member, monotonic and clamped','OK',{input:'ReadInput'});
add('get',base+'/inbox-summary','Visible unread visitor messages and human requests for current member','InboxSummary');
add('get',base+'/members','Member names/roles, no emails or secrets','Member',{array:true});
add('get',base+'/invitations','Latest 100 invitations, no tokens (owner/admin)','Invitation',{array:true});
add('post',base+'/invitations','Single-use 7-day invitation; max 50 pending; secret returned once (owner/admin)','Invitation',{input:'InviteInput',status:201});
add('delete',base+'/invitations/{invitationId}','Revoke unused invitation, idempotent (owner/admin)','OK');
add('post','/api/v1/invitations/accept','Authenticated acceptance. Same-user retry is idempotent; existing membership never changed.','AcceptedInvitation',{input:'AcceptInput'});
schemas.VisitorEvent=object({id:uuid,sequence:{type:'integer',format:'int64'},kind:{type:'string',enum:['human_requested','human_active','ai_active','closed']},created_at:date});
schemas.PublicHistory=object({messages:{type:'array',items:ref('PublicMessage')},events:{type:'array',items:ref('VisitorEvent')},control_mode:mode,state_version:{type:'integer',format:'int64'},cursor:{type:'integer',format:'int64'},next_before:{type:'integer',format:'int64'},more:{type:'boolean'}});
schemas.HumanRequest=object({request_id:uuid});schemas.PublicMode=object({control_mode:mode,state_version:{type:'integer',format:'int64'}});
add('post',publicWidget+'/sessions/{sessionId}/request-human','Stop AI atomically and persist staff request; visitor bearer/origin required','PublicMode',{input:'HumanRequest',auth:false});
const humanOp=paths[publicWidget+'/sessions/{sessionId}/request-human'].post;humanOp.security=[{visitorBearer:[]}];humanOp.parameters=humanOp.parameters.filter(p=>p.in==='path').concat(paths[publicWidget+'/sessions/{sessionId}/cancel'].post.parameters.filter(p=>p.in==='header'));
for(const name of ['after','before'])paths[publicWidget+'/sessions/{sessionId}/messages'].get.parameters.push({name,in:'query',required:false,schema:{type:'integer',format:'int64',minimum:0}});
schemas.PublicWidgetConfig.properties.available.description='Chat availability: manual messages remain possible when AI provider is disabled. Provider field describes AI mode.';

const mapOf=(v)=>({type:'object',additionalProperties:v});
schemas.LeadField=object({key:{type:'string',pattern:'^[a-z][a-z0-9_]{0,39}$'},label:string,labels:{...mapOf(string),nullable:true},description:string,type:{type:'string',enum:['text','email','phone','number','select','inn_ru']},required:{type:'boolean'},active:{type:'boolean'},position:{type:'integer',minimum:0,maximum:1000},options:{type:'array',items:string,maxItems:30,nullable:true},min_length:{type:'integer'},max_length:{type:'integer',maximum:2000},min:{type:'number',nullable:true},max:{type:'number',nullable:true},integer:{type:'boolean'}});
schemas.LeadSettings=object({enabled:{type:'boolean'},revision:{type:'integer',minimum:1},title:string,goal:{type:'string',maxLength:1500},require_contact:{type:'boolean'},success_messages:mapOf(string),fields:{type:'array',items:ref('LeadField'),maxItems:12}});
schemas.LeadValue=object({value:{oneOf:[string,{type:'number'}],nullable:true},valid:{type:'boolean'},error_code:string,source:string,message_id:uuid,sequence:{type:'integer'},updated_at:date},['value','valid','error_code','source','updated_at']);
schemas.LeadCollection=object({state:{type:'string',enum:['idle','collecting','confirmation','submitted','canceled']},version:{type:'integer'},conversation_version:{type:'integer'},values:mapOf(ref('LeadValue')),fields:{type:'array',items:ref('LeadField')},missing:{type:'array',items:string},paused:{type:'boolean'},enabled:{type:'boolean'},privacy_url:string,success_messages:mapOf(string),error_code:string,demo:{type:'boolean'},lead_id:uuid},['state','version','conversation_version','values','fields','missing','paused','enabled','privacy_url','success_messages','error_code','demo']);
schemas.CollectionInput=object({version:{type:'integer',minimum:0},conversation_version:{type:'integer',minimum:1},values:mapOf({oneOf:[string,{type:'number'}],nullable:true})},['version','conversation_version']);
schemas.Lead=object({id:uuid,organization_id:uuid,assistant_id:uuid,site_id:uuid,conversation_id:uuid,name:string,email:string,phone:string,data:mapOf({oneOf:[string,{type:'number'}]}),provenance:mapOf(ref('LeadValue')),contact_keys:mapOf(string),snapshot:ref('LeadSettings'),values:mapOf(ref('LeadValue')),privacy_url:string,status:{type:'string',enum:['new','in_progress','won','lost']},assigned_user_id:{...uuid,nullable:true},source:{type:'string',enum:['website','staff']},version:{type:'integer'},site:string,created_at:date,updated_at:date,can_open_conversation:{type:'boolean'}});
schemas.LeadEvent=object({id:uuid,kind:string,actor_id:{...uuid,nullable:true},target_id:{...uuid,nullable:true},status:string,note:string,created_at:date});
schemas.LeadDetail=object({lead:ref('Lead'),events:{type:'array',items:ref('LeadEvent'),maxItems:100}});
schemas.LeadList=object({items:{type:'array',items:ref('Lead'),maxItems:50},next_cursor:string});
schemas.LeadPatch=object({version:{type:'integer',minimum:1},status:{type:'string',enum:['new','in_progress','won','lost']},assigned_user_id:string,values:schemas.CollectionInput.properties.values,note:{type:'string',maxLength:4000}},['version']);
const leadSettings=base+'/assistants/{assistantId}/lead-settings';
add('get',leadSettings,'Owner/admin: scenario and configurable fields','LeadSettings');add('put',leadSettings,'Owner/admin: replace settings with revision CAS; active collections retain snapshot','LeadSettings',{input:'LeadSettings'});
const leadsBase=base+'/leads';add('get',leadsBase,'Company scoped keyset list. Operator: own or unassigned open leads','LeadList');
for(const name of ['status','site','assistant','assigned','from','to','cursor','limit'])paths[leadsBase].get.parameters.push({name,in:'query',required:false,schema:name==='limit'?{type:'integer',minimum:1,maximum:50,default:30}:string});
add('get',leadsBase+'/{leadId}','Role/assignment scoped lead with latest 100 events','LeadDetail');add('patch',leadsBase+'/{leadId}','Version CAS. Operators claim unassigned, update own status/notes; admin/owner contacts/assignments','LeadDetail',{input:'LeadPatch'});add('delete',leadsBase+'/{leadId}','Owner/admin delete lead/events. Submitted collection remains a tombstone; chat remains','OK');
const collection=inboxItem+'/collection';add('get',collection,'Website conversation access; frozen collection DTO','LeadCollection');for(const action of ['start','edit','cancel','submit'])add('post',collection+'/'+action,'Staff '+action+'. Writes require human_active and own conversation for operators. Explicit submit creates one lead.','LeadCollection',{input:'CollectionInput'});
const publicCollection=publicWidget+'/sessions/{sessionId}/collection';
for(const [method,path]of [['get',publicCollection],...['start','edit','cancel','submit'].map(a=>['post',publicCollection+'/'+a])]){add(method,path,'Visitor bearer/origin required. Version checked; submit requires confirmation. No admin lead ID or internal notes.','LeadCollection',{auth:false,...(method==='post'?{input:'CollectionInput'}:{})});const op=paths[path][method];op.security=[{visitorBearer:[]}];op.parameters=op.parameters.filter(p=>p.in==='path').concat(humanOp.parameters.filter(p=>p.in==='header'));}
schemas.PublicWidgetConfig.properties.lead_enabled={type:'boolean'};

// Universal company API and durable outbound webhooks. No CRM-specific destination.
schemas.APIKeyInput=object({name:{type:'string',minLength:1,maxLength:100},scopes:{type:'array',items:{type:'string',enum:['leads:read','leads:write']},minItems:1,maxItems:2,uniqueItems:true},days:{type:'integer',minimum:1,maximum:365}});
schemas.APIKeyCreated=object({id:uuid,token:{...string,description:'Shown once. Store server-side; only SHA-256 digest persists.'}});
schemas.APIKey=object({id:uuid,organization_id:uuid,name:string,scopes:{type:'array',items:string},created_at:date,expires_at:date,revoked_at:{...date,nullable:true},last_used_at:{...date,nullable:true}});
schemas.WebhookInput=object({url:{type:'string',format:'uri',maxLength:2000,description:'Public HTTPS, no query/auth/redirects. Exact configured loopback exception in development only.'}});
schemas.WebhookCreated=object({id:uuid,secret:{...string,description:'Shown once, stored encrypted using WEBHOOK_ENCRYPTION_KEY.'}});schemas.WebhookSecret=object({secret:string});
schemas.Webhook=object({id:uuid,organization_id:uuid,url:string,enabled:{type:'boolean'},version:{type:'integer'},created_at:date});
schemas.WebhookToggle=object({enabled:{type:'boolean'}});
schemas.WebhookDelivery=object({id:uuid,organization_id:uuid,endpoint_id:uuid,event_id:uuid,status:{type:'string',enum:['pending','processing','delivered','failed','canceled']},attempts:{type:'integer'},max_attempts:{type:'integer'},next_attempt_at:date,lease_token:{...uuid,nullable:true},lease_until:{...date,nullable:true},last_code:string,http_status:{type:'integer'},created_at:date,updated_at:date});
schemas.WebhookAttempt=object({id:uuid,organization_id:uuid,delivery_id:uuid,attempt:{type:'integer'},code:string,http_status:{type:'integer'},duration_ms:{type:'integer'},created_at:date});schemas.TestDelivery=object({delivery_id:uuid});
schemas.WebhookEvent=object({event_id:uuid,event_type:{type:'string',enum:['lead.created','webhook.test']},lead_id:{...uuid,nullable:true},created_at:date});
schemas.ExternalLead=object({id:uuid,assistant_id:uuid,site_id:{...uuid,nullable:true},conversation_id:{...uuid,nullable:true},name:string,email:string,phone:string,data:mapOf({oneOf:[string,{type:'number'}]}),status:{type:'string',enum:['new','in_progress','won','lost']},version:{type:'integer'},created_at:date,updated_at:date});
schemas.ExternalLeadInput=object({assistant_id:uuid,site_id:uuid,values:mapOf({oneOf:[string,{type:'number'}]})},['assistant_id','values']);
schemas.ExternalFeed=object({items:{type:'array',maxItems:100,items:object({event_id:uuid,lead_id:uuid,created_at:date,deleted:{type:'boolean'},lead:{allOf:[ref('ExternalLead')],nullable:true}})},next_cursor:string,has_more:{type:'boolean'}});
const ib=base+'/integrations';
for(const [kind,response]of [['keys','APIKey'],['webhooks','Webhook'],['deliveries','WebhookDelivery']]){add('get',ib+'/'+kind,'Owner/admin only: company '+kind,response,{array:true});if(kind==='deliveries')paths[ib+'/'+kind].get.parameters.push({name:'before',in:'query',required:false,schema:uuid,description:'Last delivery UUID from previous page; 100 items per page'});}
add('post',ib+'/keys','Owner/admin create expiring scoped API key. Returned secret is shown once.','APIKeyCreated',{input:'APIKeyInput',status:201});add('delete',ib+'/keys/{itemId}','Owner/admin revoke key immediately','OK');
add('post',ib+'/webhooks','Owner/admin create signed metadata-only lead.created webhook','WebhookCreated',{input:'WebhookInput',status:201});add('patch',ib+'/webhooks/{itemId}','Owner/admin enable/disable endpoint','OK',{input:'WebhookToggle'});add('post',ib+'/webhooks/{itemId}/rotate','Owner/admin rotate signing secret; coordinate receiver deployment','WebhookSecret');add('post',ib+'/webhooks/{itemId}/test','Owner/admin enqueue signed webhook.test with null lead ID','TestDelivery',{status:202});add('post',ib+'/deliveries/{itemId}/retry','Owner/admin retry failed/canceled delivery with same event ID','OK',{status:202});add('get',ib+'/deliveries/{itemId}/attempts','Owner/admin: latest 100 attempts, without response bodies','WebhookAttempt',{array:true});
const ext='/api/v1/external/leads';add('get',ext,'API key leads:read. Ascending commit-safe creation feed including tombstones; persist returned cursor','ExternalFeed',{auth:false});paths[ext].get.parameters=[];for(const [name,schema]of [['cursor',string],['limit',{type:'integer',minimum:1,maximum:100,default:50}]])paths[ext].get.parameters.push({name,in:'query',required:false,schema});
add('get',ext+'/{leadId}','API key leads:read; company inferred exclusively from key','ExternalLead',{auth:false});add('post',ext,'API key leads:write. Explicit standalone creation, validates assistant fields; no fake conversation. Identical Idempotency-Key replay returns same ID.','ExternalLead',{auth:false,input:'ExternalLeadInput',status:201});
for(const path of [ext,ext+'/{leadId}'])for(const [method,op]of Object.entries(paths[path])){op.security=[{companyAPIKey:[]}];op.parameters=(op.parameters||[]).filter(p=>p.in!=='header');if(method==='post')op.parameters.push({name:'Idempotency-Key',in:'header',required:true,schema:{type:'string',pattern:'^[A-Za-z0-9_-]{8,100}$'}});for(const code of [410,503])op.responses[code]={description:code===410?'Previously created lead was deleted':'Service unavailable',content:{'application/json':{schema:ref('Error')}}};}
schemas.Lead.properties.site_id.nullable=true;schemas.Lead.properties.conversation_id.nullable=true;schemas.Lead.properties.source.enum.push('api');
writeFileSync(new URL('../docs/openapi.json',import.meta.url),JSON.stringify({openapi:'3.0.3',info:{title:'site-assistant',version:'0.7.0',description:'Stages 1-7. Universal integrations; see docs/finkoper-integration.md. Versioned lead collection and explicit confirmation; see docs/leads.md. Cabinet: cookie auth, exact PUBLIC_APP_URL Origin and X-CSRF-Protection: 1 on writes, JSON (multipart knowledge/upload and widget/logo). Public widget: separate exact-origin CORS, visitor bearer tokens, no cookies. Inbox: website only, assignment/role scoped, version/idempotency checks, DB-fenced AI handoff. Errors use stable error.code. See docs/operator-inbox.md and docs/widget.md.'},servers:[{url:'http://localhost:8080'}],paths,components:{securitySchemes:{companyAPIKey:{type:'http',scheme:'bearer',description:'Company API key sa_...; scopes leads:read/leads:write; not a user cookie or JWT.'},session:{type:'apiKey',in:'cookie',name:'session'},visitorBearer:{type:'http',scheme:'bearer',description:'Opaque anonymous session secret, not JWT. Bound to widget/session/origin; SHA-256 digest stored server-side.'}},schemas}},null,2)+'\n');
