import fs from 'node:fs';
const {fixture:f}=JSON.parse(fs.readFileSync('artifacts/stage9/restore-fixture.json'));
const root='http://localhost:8080';
const login=await fetch(root+'/api/v1/auth/login',{method:'POST',headers:{Origin:'http://localhost:5173','X-CSRF-Protection':'1','Content-Type':'application/json'},body:JSON.stringify({email:f.email,password:'local-load-fixture-password'})});
if(!login.ok)throw Error('Restored login failed');
const Cookie=login.headers.get('set-cookie').split(';')[0];
for(const p of [`/organizations/${f.organization}`,`/organizations/${f.organization}/assistants/${f.assistant}/preview/conversations/${f.conversation}`,`/organizations/${f.organization}/leads/${f.lead}`,`/organizations/${f.organization}/assistants/${f.assistant}/knowledge/${f.article}`]){
 const r=await fetch(root+'/api/v1'+p,{headers:{Cookie}});if(!r.ok)throw Error('Restored record missing');await r.text();
}
console.log('PASS restored login, organization, conversation, lead and knowledge through Go API');
