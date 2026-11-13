// Local demonstration only. Never deploy this development receiver to production.
import http from 'node:http';
import {createHmac,timingSafeEqual} from 'node:crypto';
const secret=process.env.WEBHOOK_SIGNING_SECRET;
if(!secret)throw Error('Set WEBHOOK_SIGNING_SECRET to the one-time webhook secret shown in the dashboard');
const seen=new Set();
http.createServer(async(req,res)=>{
 if(req.method!=='POST'||req.url!=='/webhook'){res.writeHead(404).end();return;}
 let body=Buffer.alloc(0);for await(const b of req){body=Buffer.concat([body,b]);if(body.length>4096){res.writeHead(413).end();return;}}
 const stamp=req.headers['x-siteassistant-timestamp'],sig=req.headers['x-siteassistant-signature'];
 const expected='v1='+createHmac('sha256',secret).update(stamp+'.').update(body).digest('hex');
 if(!stamp||!/^\d{1,12}$/.test(stamp)||Math.abs(Date.now()/1000-Number(stamp))>300||!sig||Buffer.byteLength(sig)!==expected.length||!timingSafeEqual(Buffer.from(sig),Buffer.from(expected))){res.writeHead(401).end();return;}
 try{const e=JSON.parse(body);if(!e.event_id||e.event_id!==req.headers['x-siteassistant-event-id']||!['lead.created','webhook.test'].includes(e.event_type))throw Error();if(!seen.has(e.event_id)){seen.add(e.event_id);console.log(JSON.stringify({event_id:e.event_id,event_type:e.event_type,lead_id:e.lead_id,created_at:e.created_at}));}res.writeHead(204).end();}catch{res.writeHead(400).end();}
}).listen(18086,'127.0.0.1',()=>console.log('Local receiver: http://127.0.0.1:18086/webhook'));
