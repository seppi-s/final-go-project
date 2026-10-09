const $=s=>document.querySelector(s);
const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const money=c=>new Intl.NumberFormat('en-US',{style:'currency',currency:'USD'}).format(c/100);
let selected=null, students=[];
async function api(path,options={}){const r=await fetch('/api'+path,options);const b=await r.json();if(!r.ok)throw new Error(b.error||'Request failed');return b;}
const post=(p,b)=>api(p,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(b)});
function toast(message){$('#toast').textContent=message;$('#toast').style.display='block';clearTimeout(window.toastTimer);window.toastTimer=setTimeout(()=>$('#toast').style.display='none',4500);}
function avatar(s){return s.photo_url?`<img class="avatar" src="${esc(s.photo_url)}" alt="Student photo">`:`<span class="avatar">${esc(s.first_name[0]+s.last_name[0])}</span>`;}
function renderList(){const q=$('#search').value.toLowerCase();$('#students').innerHTML=students.filter(s=>`${s.first_name} ${s.last_name} ${s.national_id}`.toLowerCase().includes(q)).map(s=>`<button class="student-row ${selected===s.id?'selected':''}" data-id="${s.id}">${avatar(s)}<span>${esc(s.first_name)} ${esc(s.last_name)}<small>ID ${esc(s.national_id)} · ${s.age} years</small></span><span class="arrow">↗</span></button>`).join('')||'<p>No students found. Add your first student.</p>';document.querySelectorAll('[data-id]').forEach(el=>el.onclick=()=>showStudent(el.dataset.id).catch(e=>toast(e.message)));}
async function refresh(){const [s,r,p]=await Promise.all([api('/students'),api('/registrations'),api('/payments')]);students=s;$('#student-count').textContent=s.length;$('#registration-count').textContent=r.length;$('#payment-total').textContent=money(p.reduce((a,v)=>a+v.amount_cents,0));renderList();}
async function showStudent(id){const s=await api('/students/'+id);selected=id;renderList();$('#detail').innerHTML=`<div class="identity">${avatar(s)}<div><h2>${esc(s.first_name)} ${esc(s.last_name)}</h2><p>Student profile · ${esc(s.national_id)}</p></div></div><div class="info"><div><span>AGE</span>${s.age} years</div><div><span>PHONE</span>${esc(s.phone)||'—'}</div><div><span>EMAIL</span>${esc(s.email)||'—'}</div><div><span>JOINED</span>${new Date(s.created_at).toLocaleDateString()}</div></div><label>Update photo<input id="photo-update" type="file" accept="image/png,image/jpeg"></label><h3>Registrations</h3>${s.registrations.map(r=>`<div class="record">${esc(r.course)} <span class="badge">${esc(r.status)}</span><small>${esc(r.semester)} · Paid ${money(r.paid_cents)} of ${money(r.fee_cents)} · Due ${money(r.fee_cents-r.paid_cents)}</small></div>`).join('')||'<p>No registrations yet.</p>'}<form id="registration-form"><div class="form-grid"><label>Course<input name="course" required maxlength="120" placeholder="e.g. Computer Science"></label><label>Semester<input name="semester" required maxlength="120" placeholder="e.g. Fall 2026"></label></div><label>Registration fee (USD)<input name="fee" type="number" min="0" max="1000000" step="0.01" required></label><button type="submit" class="ghost">＋ Register student</button></form><h3>Record a payment</h3><form id="payment-form"><label>Registration<select name="registration_id" required><option value="">Choose registration</option>${s.registrations.filter(r=>r.paid_cents<r.fee_cents).map(r=>`<option value="${r.id}">${esc(r.course)} · ${money(r.fee_cents-r.paid_cents)} due</option>`).join('')}</select></label><div class="form-grid"><label>Amount (USD)<input name="amount" type="number" min="0.01" max="1000000" step="0.01" required></label><label>Method<select name="method"><option value="bank_transfer">Bank transfer</option><option value="cash">Cash</option><option value="card">Card</option></select></label></div><label>Unique reference<input name="reference" required maxlength="120"></label><button class="ghost" type="submit">＋ Record payment</button></form><h3>Payment history</h3>${s.payments.map(p=>`<div class="record">${money(p.amount_cents)} · ${esc(p.method.replace('_',' '))}<small>${esc(p.reference)} · ${new Date(p.created_at).toLocaleDateString()}</small></div>`).join('')||'<p>No payments recorded.</p>'}`;
$('#photo-update').onchange=async e=>{if(!e.target.files[0])return;try{await upload(id,e.target.files[0]);await refresh();await showStudent(id);toast('Photo updated');}catch(e){toast(e.message);}};
$('#registration-form').onsubmit=e=>submit(e,async f=>{await post('/registrations',{student_id:id,course:f.get('course'),semester:f.get('semester'),fee_cents:Math.round(Number(f.get('fee'))*100)});});
$('#payment-form').onsubmit=e=>submit(e,async f=>{await post('/payments',{registration_id:f.get('registration_id'),amount_cents:Math.round(Number(f.get('amount'))*100),method:f.get('method'),reference:f.get('reference')});});}
async function submit(e,action){e.preventDefault();const btn=e.target.querySelector('button[type=submit]');btn.disabled=true;try{await action(new FormData(e.target));await refresh();await showStudent(selected);toast('Record saved');}catch(err){toast(err.message);btn.disabled=false;}}
async function upload(id,file){if(file.size>5*1024*1024-1000)throw new Error('Photo must be smaller than 5 MB.');const f=new FormData();f.append('photo',file);return api('/students/'+id+'/photo',{method:'POST',body:f});}
$('#new-student').onclick=()=>{$('#form-error').textContent='';$('#student-dialog').showModal();};$('#close-dialog').onclick=()=>$('#student-dialog').close();$('#search').oninput=renderList;
$('#student-form').onsubmit=async e=>{e.preventDefault();const f=new FormData(e.target),btn=e.target.querySelector('[type=submit]');btn.disabled=true;let created;try{created=await post('/students',{first_name:f.get('first_name'),last_name:f.get('last_name'),age:Number(f.get('age')),national_id:f.get('national_id'),email:f.get('email'),phone:f.get('phone')});const photo=f.get('photo');if(photo.size){try{await upload(created.id,photo);}catch(err){toast('Student saved; photo failed: '+err.message);}}$('#student-dialog').close();e.target.reset();await refresh();await showStudent(created.id);}catch(err){$('#form-error').textContent=created?'Student saved. Refresh to view it. '+err.message:err.message;}finally{btn.disabled=false;}};
(async()=>{try{await api('/health');$('#health').textContent='API connected';await refresh();}catch(e){$('#health').textContent='API unavailable';toast(e.message);}})();

// Same-origin REST request playground. Responses are rendered as text.
const examples={
 health:['GET','/api/health',null],
 students:['GET','/api/students',null],
 create:['POST','/api/students',{first_name:'Test',last_name:'Student',age:20,national_id:'TEST-'+Date.now(),email:'test@example.com',phone:''}],
 profile:['GET','/api/students/STUDENT_ID',null],
 registrations:['GET','/api/registrations',null],
 register:['POST','/api/registrations',{student_id:'STUDENT_ID',course:'Computer Science',semester:'Fall 2026',fee_cents:10000}],
 payments:['GET','/api/payments',null],
 pay:['POST','/api/payments',{registration_id:'REGISTRATION_ID',amount_cents:4000,method:'bank_transfer',reference:'TEST-PAY-'+Date.now()}]
};
function bodyVisibility(){$('#api-body-label').hidden=$('#api-method').value==='GET';}
function chooseExample(){const [method,path,body]=examples[$('#api-example').value];$('#api-method').value=method;$('#api-path').value=path;$('#api-body').value=body?JSON.stringify(body,null,2):'';bodyVisibility();}
$('#api-example').onchange=chooseExample;$('#api-method').onchange=bodyVisibility;chooseExample();
$('#api-form').onsubmit=async e=>{
 e.preventDefault();const btn=$('#api-send'),status=$('#api-status'),output=$('#api-response');status.classList.remove('failed');
 try{
  const path=$('#api-path').value.trim();const url=new URL(path,location.origin);
  if(!path.startsWith('/api/')||url.origin!==location.origin||!url.pathname.startsWith('/api/')||url.hash)throw new Error('Use an /api/ endpoint on this server.');
  const method=$('#api-method').value;const options={method};
  if(method==='POST'){const parsed=JSON.parse($('#api-body').value);if(!parsed||typeof parsed!=='object'||Array.isArray(parsed))throw new Error('Request body must be a JSON object.');options.headers={'Content-Type':'application/json'};options.body=JSON.stringify(parsed);}
  btn.disabled=true;status.textContent='Sending…';output.textContent='Waiting for response…';const start=performance.now();
  const response=await fetch(url,options);const raw=await response.text();status.textContent=`${response.status} ${response.statusText} · ${Math.round(performance.now()-start)} ms`;status.classList.toggle('failed',!response.ok);
  let formatted=raw;try{formatted=JSON.stringify(JSON.parse(raw),null,2);}catch{}
  output.textContent=`${method} ${url.pathname}${url.search}\nContent-Type: ${response.headers.get('content-type')||'unknown'}\n\n${formatted}`;
  if(method==='POST'&&response.ok){try{await refresh();if(selected)await showStudent(selected);}catch(err){toast('Request saved; dashboard refresh failed: '+err.message);}}
 }catch(err){status.textContent='Request error';status.classList.add('failed');output.textContent=err.message;}finally{btn.disabled=false;}
};
