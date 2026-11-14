import fs from 'node:fs';
import {performance} from 'node:perf_hooks';
import {randomUUID,createHmac} from 'node:crypto';
import http from 'node:http';
const base=process.env.LOAD_URL||'http://localhost:8080';
if(!/^http:\/\/(localhost|127\.0\.0\.1):\d+$/.test(base)||process.env.LOAD_TEST_DATABASE!=='confirmed_test')throw Error('Local URL and explicit test-database acknowledgement required');
let cookie='';const timings=[],failures={};
async function request(p,body,headers={}){const start=performance.now();const r=await fetch(base+p,{method:body===undefined?'GET':'POST',headers:{Origin:'http://localhost:5173','X-CSRF-Protection':'1','Content-Type':'application/json',Cookie:cookie,...headers},body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(150000)});const text=await r.text();timings.push(performance.now()-start);if(!r.ok)failures[r.status]=(failures[r.status]||0)+1;if(r.headers.get('set-cookie'))cookie=r.headers.get('set-cookie').split(';')[0];return {status:r.status,text,json:()=>JSON.parse(text)}}
const email=`load-${Date.now()}@example.com`,password='local-load-fixture-password';
await request('/api/v1/auth/register',{email,password,preferred_locale:'en'});const login=await request('/api/v1/auth/login',{email,password});if(login.status!==200)throw Error('Login rejected: '+login.status);
const org=(await request('/api/v1/organizations',{name:'Local load fixture',country:'CZ'})).json();
const a=(await request(`/api/v1/organizations/${org.id}/assistants`,{name:'Load assistant'})).json();
const site=(await request(`/api/v1/organizations/${org.id}/sites`,{name:'Local load',domain:'load.example',assistant_id:a.id})).json();
const admin=`/api/v1/organizations/${org.id}/sites/${site.id}/widget`;
const w=(await request(admin,{})).json().widget;
await fetch(base+admin,{method:'PATCH',headers:{Origin:'http://localhost:5173','X-CSRF-Protection':'1','Content-Type':'application/json',Cookie:cookie},body:JSON.stringify({enabled:true,origins:['http://localhost:4173'],settings:w.settings})});
const publicPath=`/api/v1/public/widgets/${w.id}`;
const asst=`/api/v1/organizations/${org.id}/assistants/${a.id}`;
const article=(await request(asst+'/knowledge/articles',{title:'Local recovery knowledge',content:'Local recovery fixture: accounting support costs 450 EUR monthly.',language:'en'})).json();
const conversation=(await request(asst+'/preview/conversations',{})).json();
await request(asst+`/preview/conversations/${conversation.id}/messages`,{request_id:randomUUID(),content:'Local recovery fixture question'});
const key=(await request(`/api/v1/organizations/${org.id}/integrations/keys`,{name:'Local fixture',days:1,scopes:['leads:read','leads:write']})).json();
let signing='',deliveries=0,badSignatures=0;
const receiver=http.createServer(async(req,res)=>{const chunks=[];for await(const c of req)chunks.push(c);const b=Buffer.concat(chunks),signature='v1='+createHmac('sha256',signing).update(String(req.headers['x-siteassistant-timestamp'])+'.').update(b).digest('hex');if(req.headers['x-siteassistant-signature']!==signature){badSignatures++;res.writeHead(401)}else{deliveries++;res.writeHead(204)}res.end()});
await new Promise((resolve,reject)=>{receiver.once('error',reject);receiver.listen(18086,'127.0.0.1',resolve)});
const endpoint=(await request(`/api/v1/organizations/${org.id}/integrations/webhooks`,{url:'http://127.0.0.1:18086/webhook'})).json();signing=endpoint.secret;
const lead=(await request('/api/v1/external/leads',{assistant_id:a.id,values:{full_name:'Recovery fixture',email:'restore@example.com'}},{Authorization:'Bearer '+key.token,'Idempotency-Key':randomUUID()})).json();
const reports=[];
// Functional mixed concurrency sample. Safety caps are not bypassed.
const mixed=[];
for(const [name,operation] of [
 ['sessions_sse_replay',async()=>{const r=await request(publicPath+'/sessions',{}, {Origin:'http://localhost:4173'});if(r.status!==201)return;const s=r.json(),h={Origin:'http://localhost:4173',Authorization:'Bearer '+s.token};const body={request_id:randomUUID(),content:'What are the accounting service terms?'};await request(publicPath+`/sessions/${s.id}/messages`,body,h);await request(publicPath+`/sessions/${s.id}/messages`,body,h)}],
 ['operator_history_take_reply',async()=>{const list=(await request(`/api/v1/organizations/${org.id}/conversations`)).json();if(list.items?.length){const c=list.items[0],p=`/api/v1/organizations/${org.id}/conversations/${c.id}`;await request(p+'/messages?after=0');const take=await request(p+'/take',{request_id:randomUUID(),version:c.state_version});if(take.status===200){const current=take.json();await request(p+'/messages',{request_id:randomUUID(),version:current.state_version,content:'Local operator reply'})}}}],
 ['external_idempotency_feed',async()=>{const h={Authorization:'Bearer '+key.token,'Idempotency-Key':randomUUID()},body={assistant_id:a.id,values:{full_name:'Local fixture',email:'fixture@example.com'}};await request('/api/v1/external/leads',body,h);await request('/api/v1/external/leads',body,h);await request('/api/v1/external/leads?limit=10',undefined,h)}],
 ['knowledge_search',async()=>{await request(asst+'/knowledge/search',{query:'accounting support cost',mode:'hybrid'})}],
]){timings.length=0;for(const k of Object.keys(failures))delete failures[k];const start=performance.now();await Promise.all(Array.from({length:10},operation));timings.sort((a,b)=>a-b);mixed.push({name,concurrency:10,requests:timings.length,rps:timings.length/((performance.now()-start)/1000),p50_ms:timings[Math.floor(timings.length*.5)],p95_ms:timings[Math.floor(timings.length*.95)],p99_ms:timings[timings.length-1],http_errors:{...failures}})}
for(const concurrency of [10,25,50,100]){
 timings.length=0;for(const k of Object.keys(failures))delete failures[k];const started=performance.now();
 await Promise.all(Array.from({length:concurrency},async()=>{await request(publicPath+'/config',undefined,{Origin:'http://localhost:4173'});await request(`/api/v1/organizations/${org.id}/conversations`)}));
 timings.sort((a,b)=>a-b);const q=n=>timings[Math.min(timings.length-1,Math.floor(timings.length*n))];reports.push({concurrency,requests:timings.length,rps:timings.length/((performance.now()-started)/1000),p50_ms:q(.5),p95_ms:q(.95),p99_ms:q(.99),http_errors:{...failures}});
}
await new Promise(resolve=>setTimeout(resolve,1500));await new Promise(resolve=>receiver.close(resolve));
const result={time:new Date().toISOString(),scope:'Mixed 10-client sample plus configuration/list ramp; not GPU capacity or endurance acceptance',mixed,reports,webhooks:{deliveries,badSignatures},client_rss_bytes:process.memoryUsage().rss,fixture:{email,organization:org.id,assistant:a.id,article:article.id,conversation:conversation.id,lead:lead.id},limitations:['No built-in API process CPU/RSS sampler; collect separate observations','No long-duration leak claim','Protective 429 responses are retained in results']};
fs.mkdirSync('artifacts/stage9',{recursive:true});fs.writeFileSync('artifacts/stage9/load.json',JSON.stringify(result,null,2));console.log(JSON.stringify(result,null,2));
