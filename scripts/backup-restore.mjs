// Offline snapshot: stop API/workers and all writers first. Never stores application secrets.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {spawnSync} from 'node:child_process';
const [action, directory, database] = process.argv.slice(2);
if(!['backup','restore','verify'].includes(action)||!directory) throw Error('Usage: node scripts/backup-restore.mjs backup|restore|verify ABSOLUTE_DIRECTORY DATABASE');
if(!path.isAbsolute(directory))throw Error('Absolute backup directory required');
const hash=b=>crypto.createHash('sha256').update(b).digest('hex');
function pg(args, input){const r=spawnSync('docker',['compose','exec','-T','postgres',...args],{input,maxBuffer:512*1024*1024});if(r.status!==0)throw Error('PostgreSQL command failed');return r.stdout;}
function walk(root,prefix=''){let out=[];for(const name of fs.readdirSync(path.join(root,prefix))){const rel=path.join(prefix,name),f=path.join(root,rel),s=fs.lstatSync(f);if(s.isSymbolicLink())throw Error('Symlinks forbidden in snapshot');if(s.isDirectory())out.push(...walk(root,rel));else out.push(rel)}return out}
if(action==='backup'){
 if(!/^[a-z][a-z0-9_]+$/.test(database||'')||process.env.BACKUP_WRITERS_STOPPED!=='yes')throw Error('Explicit DB and BACKUP_WRITERS_STOPPED=yes required');
 if(fs.existsSync(directory))throw Error('Destination already exists');fs.mkdirSync(directory,{recursive:true,mode:0o700});
 fs.writeFileSync(path.join(directory,'database.dump'),pg(['pg_dump','-U','site_assistant','-Fc',database]),{mode:0o600});
 for(const [name,key] of [['knowledge','KNOWLEDGE_FILE_ROOT'],['widget','WIDGET_FILE_ROOT']]){const root=process.env[key];if(!root||!fs.existsSync(root))throw Error('Explicit storage roots required');walk(root);fs.cpSync(root,path.join(directory,name),{recursive:true})}
 const files={};for(const rel of walk(directory))files[rel]=hash(fs.readFileSync(path.join(directory,rel)));
 fs.writeFileSync(path.join(directory,'manifest.json'),JSON.stringify({version:1,created_at:new Date().toISOString(),files},null,2),{mode:0o600});
}else{
 const m=JSON.parse(fs.readFileSync(path.join(directory,'manifest.json')));
 for(const [rel,digest]of Object.entries(m.files)){const f=path.resolve(directory,rel);if(!f.startsWith(directory+path.sep)||hash(fs.readFileSync(f))!==digest)throw Error('Snapshot integrity failed')}
 if(action==='restore'){
  if(!/^[a-z][a-z0-9_]+_test$/.test(database||''))throw Error('Restore requires NEW database ending _test');
  pg(['createdb','-U','site_assistant',database]); // Deliberately fails if database exists.
  pg(['pg_restore','-U','site_assistant','--exit-on-error','--no-owner','-d',database],fs.readFileSync(path.join(directory,'database.dump')));
  console.log('Restored new test DB. Point storage roots at snapshot knowledge/widget directories. Do not start delivery workers until outbound endpoints have been reviewed.');
 }
}
console.log(action+' complete');
