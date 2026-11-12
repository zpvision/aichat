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
writeFileSync(new URL('../docs/openapi.json',import.meta.url),JSON.stringify({openapi:'3.0.3',info:{title:'site-assistant',version:'0.1.0',description:'Stage 1. Same-origin cookie API; all writes require exact Origin, JSON Content-Type and X-CSRF-Protection: 1. No CORS. Login/register: 10 attempts per peer IP per 15 minutes, 4 concurrent password operations per process. Error messages are localized by clients from error.code.'},servers:[{url:'http://localhost:8080'}],paths,components:{securitySchemes:{session:{type:'apiKey',in:'cookie',name:'session'}},schemas}},null,2)+'\n');
