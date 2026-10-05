import {useState} from 'react';
import {Link,useParams} from 'react-router-dom';
import {api,mutation} from '../services/api';
import {useCustomer} from '../services/customer';
import {Button,Card,ErrorState,Field,Loading,useMutation} from '../components/ui';

type Action={type:string;value:string;field?:string};
type Condition={field:string;op:string;values:string[]};
type Schedule={kind?:string;cron?:string;timezone?:string;sla_seconds?:number;ticket?:string;contact?:string;user?:number};
interface Workflow{id:string;name:string;description:string;enabled:boolean;trigger:string;conditions:{mode:string;items:Condition[]};actions:Action[];delay_seconds:number;max_attempts:number;schedule?:Schedule;revision:number}
interface Execution{id:string;state:string;code:string;action_index:number;attempt:number;available_at:string;started_at?:string;completed_at?:string}
const triggers=['manual','schedule.cron','crm.contact.created','crm.contact.updated','crm.tag.added','ticket.created','ticket.customer_message','ticket.agent_message','ticket.status_changed','ticket.priority_changed','ticket.assigned','file.delivery_allowed','file.delivery_denied','broadcast.started','broadcast.completed','broadcast.cancelled','broadcast.status_changed','form.submitted','commerce.order.created','commerce.order.status_changed'];
const actions=['crm.add_tag','crm.remove_tag','crm.set_field','ticket.assign','ticket.priority','ticket.status','ticket.note','ticket.reply','telegram.send','ai.invoke','ai.store_result'];
const fields=['ticket.priority','ticket.status','ticket.department','contact.tags','file.reference','ai.priority','ai.category','form.id','order.id','order.status','campaign.id','campaign.status','schedule.kind','schedule.fire_at','schedule.occurrence'];
const empty:Workflow={id:'',name:'',description:'',enabled:false,trigger:'manual',conditions:{mode:'ALL',items:[]},actions:[{type:'ticket.priority',value:'HIGH'}],delay_seconds:0,max_attempts:1,schedule:{kind:'EVENT'},revision:0};
const scheduleKind=(w:Workflow)=>w.schedule?.kind||'EVENT';

export function Automation(){
  const {bot}=useParams();
  const base=`/customer-bots/${bot}/automation`;
  const options=useCustomer<{triggers:string[];actions:string[]}>(base+'/options');
  const list=useCustomer<Workflow[]>(base+'/workflows');
  const [cursor,setCursor]=useState('');
  const history=useCustomer<Execution[]>(base+'/history?cursor='+cursor);
  const [w,setW]=useState<Workflow>(empty);
  const [ticket,setTicket]=useState('');
  const [review,setReview]=useState('');
  const busy=useMutation();
  const kind=scheduleKind(w);
  const action=(i:number,a:Action)=>setW({...w,actions:w.actions.map((v,n)=>i===n?a:v)});
  const condition=(i:number,c:Condition)=>setW({...w,conditions:{...w.conditions,items:w.conditions.items.map((v,n)=>i===n?c:v)}});
  const patchSchedule=(v:Partial<Schedule>)=>setW({...w,schedule:{...(w.schedule??{}),...v}});
  const changeKind=(next:string)=>{
    if(next==='CRON'){setW({...w,trigger:'schedule.cron',delay_seconds:0,schedule:{kind:'CRON',cron:w.schedule?.cron||'0 * * * *',timezone:w.schedule?.timezone||'UTC',ticket:'',contact:'',user:0}});return}
    if(next==='SLA'){setW({...w,trigger:w.trigger.startsWith('ticket.')?w.trigger:'ticket.created',delay_seconds:0,schedule:{kind:'SLA',sla_seconds:w.schedule?.sla_seconds||300}});return}
    setW({...w,trigger:w.trigger==='schedule.cron'?'manual':w.trigger,schedule:{kind:'EVENT'}});
  };
  const visibleTriggers=triggers.filter(v=>{
    if(kind==='CRON')return v==='schedule.cron';
    if(kind==='SLA')return v.startsWith('ticket.');
    return v!=='schedule.cron'&&(options.data?.triggers.includes(v)||v===w.trigger);
  });
  return <><Link to={`/customer-bots/${bot}`}>بازگشت به افزونه‌ها</Link><h1>اتوماسیون</h1>{busy.error&&<ErrorState error={busy.error}/>}
    <Card><form onSubmit={async e=>{e.preventDefault();if(await busy.run(()=>mutation(base+'/workflows'+(w.id?'/'+w.id:''),{workflow:w,expected_revision:w.revision}))){setW(empty);list.retry();}}}>
      <Field label="نام گردش کار"><input required maxLength={100} value={w.name} onChange={e=>setW({...w,name:e.target.value})}/></Field>
      <Field label="توضیح"><textarea maxLength={500} value={w.description} onChange={e=>setW({...w,description:e.target.value})}/></Field>
      <Field label="نوع زمان‌بندی"><select value={kind} onChange={e=>changeKind(e.target.value)}><option value="EVENT">رویدادی / تأخیری</option><option value="CRON">Cron دوره‌ای</option><option value="SLA">SLA Timer</option></select></Field>
      <Field label="رویداد"><select disabled={kind==='CRON'} value={w.trigger} onChange={e=>setW({...w,trigger:e.target.value})}>{visibleTriggers.map(v=><option key={v}>{v}</option>)}</select></Field>
      {kind==='EVENT'&&<Field label="تأخیر (ثانیه)"><input type="number" min={0} max={2592000} value={w.delay_seconds} onChange={e=>setW({...w,delay_seconds:Number(e.target.value)})}/></Field>}
      {kind==='CRON'&&<Card><Field label="Cron expression"><input required maxLength={128} value={w.schedule?.cron??''} onChange={e=>patchSchedule({cron:e.target.value})}/></Field><Field label="Timezone"><input required maxLength={64} value={w.schedule?.timezone||'UTC'} onChange={e=>patchSchedule({timezone:e.target.value})}/></Field><Field label="مرجع تیکت هدف"><input maxLength={100} value={w.schedule?.ticket??''} onChange={e=>patchSchedule({ticket:e.target.value})}/></Field><Field label="شناسه Contact هدف"><input maxLength={100} value={w.schedule?.contact??''} onChange={e=>patchSchedule({contact:e.target.value})}/></Field><Field label="شناسه تلگرام هدف"><input type="number" min={0} value={w.schedule?.user??0} onChange={e=>patchSchedule({user:Number(e.target.value)})}/></Field><p>هدف Cron محدود به منابع صریح همین Workspace است؛ webhook، shell و SQL دلخواه پشتیبانی نمی‌شود.</p></Card>}
      {kind==='SLA'&&<Field label="مهلت SLA (ثانیه)"><input type="number" min={1} max={31536000} value={w.schedule?.sla_seconds??300} onChange={e=>patchSchedule({sla_seconds:Number(e.target.value)})}/></Field>}
      <Field label="حالت شرط"><select value={w.conditions.mode} onChange={e=>setW({...w,conditions:{...w.conditions,mode:e.target.value}})}><option>ALL</option><option>ANY</option></select></Field>
      {w.conditions.items.map((c,i)=><Card key={i}><Field label="فیلد شرط"><select value={c.field} onChange={e=>condition(i,{...c,field:e.target.value})}>{fields.map(v=><option key={v}>{v}</option>)}</select></Field><Field label="مقایسه"><select value={c.op} onChange={e=>condition(i,{...c,op:e.target.value,values:['EXISTS','NOT_EXISTS'].includes(e.target.value)?[]:['HIGH']})}>{['EQUALS','NOT_EQUALS','IN','NOT_IN','EXISTS','NOT_EXISTS','CONTAINS','GREATER_THAN','LESS_THAN'].map(v=><option key={v}>{v}</option>)}</select></Field><Field label="مقدار شرط"><input disabled={['EXISTS','NOT_EXISTS'].includes(c.op)} maxLength={200} value={c.values.join(',')} onChange={e=>condition(i,{...c,values:e.target.value.split(',')})}/></Field><Button type="button" onClick={()=>setW({...w,conditions:{...w.conditions,items:w.conditions.items.filter((_,n)=>i!==n)}})}>حذف شرط</Button></Card>)}
      <Button type="button" disabled={w.conditions.items.length>=16} onClick={()=>setW({...w,conditions:{...w.conditions,items:[...w.conditions.items,{field:'ticket.priority',op:'EQUALS',values:['HIGH']}]}})}>افزودن شرط</Button>
      <h2>اقدامات به ترتیب اجرا</h2>{w.actions.map((a,i)=><Card key={i}><Field label="نوع اقدام"><select value={a.type} onChange={e=>action(i,{...a,type:e.target.value})}>{actions.filter(v=>options.data?.actions.includes(v)||v===a.type).map(v=><option key={v}>{v}</option>)}</select></Field><Field label="مقدار اقدام"><textarea maxLength={4000} value={a.value} onChange={e=>action(i,{...a,value:e.target.value})}/></Field><Field label="فیلد مقصد"><input maxLength={64} value={a.field??''} onChange={e=>action(i,{...a,field:e.target.value})}/></Field><Button type="button" disabled={w.actions.length===1} onClick={()=>setW({...w,actions:w.actions.filter((_,n)=>n!==i)})}>حذف اقدام</Button></Card>)}
      <Button type="button" disabled={w.actions.length>=8} onClick={()=>setW({...w,actions:[...w.actions,{type:'crm.add_tag',value:'support'}]})}>افزودن اقدام</Button>
      <Field label="حداکثر تلاش"><input type="number" min={1} max={2} value={w.max_attempts} onChange={e=>setW({...w,max_attempts:Number(e.target.value)})}/></Field>
      <Field label="فعال"><input type="checkbox" checked={w.enabled} onChange={e=>setW({...w,enabled:e.target.checked})}/></Field>
      <Button loading={busy.loading}>ذخیره گردش کار</Button><Button type="button" loading={busy.loading} onClick={()=>busy.run(async()=>setReview(JSON.stringify(await api(base+'/preview','POST',{workflow:w,data:{}}))))}>Preview بدون تغییر</Button><p>Preview داده‌ای را تغییر نمی‌دهد و اثری اجرا نمی‌کند.</p>{review&&<p role="status">{review}</p>}
    </form></Card>
    {list.error?<ErrorState error={list.error} retry={list.retry}/>:!list.data?<Loading/>:list.data.map(v=><Card key={v.id}><h2>{v.name}</h2><p>{v.trigger} — {scheduleKind(v)} — {v.enabled?'فعال':'غیرفعال'}</p><Button onClick={()=>setW({...v,schedule:v.schedule??{kind:'EVENT'}})}>ویرایش</Button>{v.trigger==='manual'&&<><Field label="مرجع تیکت"><input value={ticket} onChange={e=>setTicket(e.target.value)}/></Field><Button loading={busy.loading} onClick={()=>busy.run(async()=>{const r=await mutation<{execution_id:string}>(base+'/workflows/'+v.id+'/run',{ticket,contact:''});await api(base+'/executions/'+r.execution_id+'/run','POST',{});history.retry();})}>اجرای دستی</Button></>}</Card>)}
    <h2>تاریخچه اجرا</h2>{history.error?<ErrorState error={history.error} retry={history.retry}/>:history.data?.map(v=><Card key={v.id}><p>{v.state} — {v.code||'—'}</p><p>موعد: {v.available_at} · شروع: {v.started_at||'—'} · پایان: {v.completed_at||'—'} · {v.action_index} · {v.attempt}</p></Card>)}
    {history.data?.length===50&&<Button onClick={()=>setCursor(history.data!.at(-1)!.id)}>صفحه بعد</Button>}{cursor&&<Button onClick={()=>setCursor('')}>صفحه اول</Button>}
  </>;
}
interface Config{name:string;model:string;instructions:string;language:string;mode:string;tools:string[];write_enabled:boolean;memory:boolean;recent:number;output_tokens:number;retrieval_mode:string;max_knowledge_chunks:number;max_cost_microunits:number;knowledge:string[];sources:string[];files:string[];revision:number}
interface Model{id:string;name:string;provider:string;tools:boolean;structured:boolean;max_input:number;max_output:number;input_microunits_per_million:number;output_microunits_per_million:number}
interface Knowledge{id:string;name:string;body:string;enabled:boolean;source_kind:string;source_ref:string;revision:number}
interface Usage{id:string;state:string;code:string;model:string;input_tokens:number;output_tokens:number;tool_calls:number;cost_microunits:number;retrieved_chunks:number;latency_ms:number}
interface Result{text:string;state:string;code:string}
const read=['contact.summary','ticket.context','file.metadata','access.evaluate'];const write=['contact.add_tag','ticket.priority','ticket.note','ticket.assign','ticket.status'];
export function Assistant(){
  const {bot}=useParams();const base=`/customer-bots/${bot}/assistant`;
  const data=useCustomer<Config>(base);const models=useCustomer<Model[]>(base+'/models');const sources=useCustomer<Knowledge[]>(base+'/knowledge');const usage=useCustomer<Usage[]>(base+'/usage');
  const [edit,setEdit]=useState<Config>();const [source,setSource]=useState({name:'',body:''});const [ingest,setIngest]=useState({source_kind:'FILE_MANAGER',reference:'',name:''});const busy=useMutation();
  if(data.error)return <ErrorState error={data.error} retry={data.retry}/>;if(!data.data)return <Loading/>;
  const c={retrieval_mode:'HYBRID',max_knowledge_chunks:4,max_cost_microunits:0,sources:[],...(edit??data.data)};
  const set=(v:Partial<Config>)=>setEdit({...c,...v});
  const selected=models.data?.find(m=>m.id===c.model);
  const manual=sources.data?.filter(k=>k.source_kind==='MANUAL')??[];
  const imported=sources.data?.filter(k=>k.source_kind!=='MANUAL'&&k.source_ref)??[];
  return <><Link to={`/customer-bots/${bot}`}>بازگشت به افزونه‌ها</Link><h1>دستیار هوش مصنوعی</h1><p>Assistant 2.0 · Hybrid retrieval · bounded cost controls</p>{busy.error&&<ErrorState error={busy.error}/>}
    <Card><form onSubmit={async e=>{e.preventDefault();await busy.run(async()=>setEdit(await api<Config>(base,'POST',{configuration:c,expected_revision:c.revision})))}}>
      <Field label="نام دستیار"><input required maxLength={100} value={c.name} onChange={e=>set({name:e.target.value})}/></Field>
      <Field label="مدل تأییدشده"><select required value={c.model} onChange={e=>set({model:e.target.value})}><option value="">انتخاب مدل</option>{models.data?.map(m=><option key={m.id} value={m.id}>{m.name}</option>)}</select></Field>
      {selected&&<p>ورودی/خروجی: {selected.max_input}/{selected.max_output} · Tools: {selected.tools?'بله':'خیر'} · Structured: {selected.structured?'بله':'خیر'} · هزینه هر یک‌میلیون توکن: {selected.input_microunits_per_million}/{selected.output_microunits_per_million} microunit</p>}
      {models.data?.length===0&&<p>اپراتور باید یک مدل مجاز را پیکربندی کند.</p>}
      <Field label="دستورالعمل"><textarea maxLength={4096} value={c.instructions} onChange={e=>set({instructions:e.target.value})}/></Field>
      <Field label="زبان"><input maxLength={32} value={c.language} onChange={e=>set({language:e.target.value})}/></Field>
      <Field label="حالت پاسخ"><select value={c.mode} onChange={e=>set({mode:e.target.value})}>{['AI_ONLY','AI_WITH_HANDOFF','AGENT_ASSIST'].map(v=><option key={v}>{v}</option>)}</select></Field>
      <Field label="روش بازیابی دانش"><select value={c.retrieval_mode||'HYBRID'} onChange={e=>set({retrieval_mode:e.target.value})}><option value="HYBRID">Hybrid</option><option value="LEXICAL">Lexical</option></select></Field>
      <Field label="حداکثر قطعه دانش در هر درخواست"><input type="number" min={1} max={8} value={c.max_knowledge_chunks||4} onChange={e=>set({max_knowledge_chunks:Number(e.target.value)})}/></Field>
      <Field label="سقف هزینه هر درخواست (microunit، صفر = بدون سقف محلی)"><input type="number" min={0} value={c.max_cost_microunits||0} onChange={e=>set({max_cost_microunits:Number(e.target.value)})}/></Field>
      <Field label="حافظه اخیر"><input type="checkbox" checked={c.memory} onChange={e=>set({memory:e.target.checked})}/></Field>
      <Field label="تعداد پیام حافظه"><input type="number" min={0} max={12} value={c.recent} onChange={e=>set({recent:Number(e.target.value)})}/></Field>
      <Field label="حداکثر توکن پاسخ"><input type="number" min={1} max={2048} value={c.output_tokens} onChange={e=>set({output_tokens:Number(e.target.value)})}/></Field>
      {read.map(v=><Field key={v} label={v}><input type="checkbox" checked={c.tools.includes(v)} onChange={e=>set({tools:e.target.checked?[...c.tools,v]:c.tools.filter(x=>x!==v)})}/></Field>)}
      <Field label="اجازه ابزارهای نوشتن"><input type="checkbox" checked={c.write_enabled} onChange={e=>set({write_enabled:e.target.checked,tools:e.target.checked?c.tools:c.tools.filter(v=>!write.includes(v))})}/></Field>
      <p>فعال‌کردن این گزینه به‌تنهایی اجازه اجرای write نمی‌دهد؛ هر write همچنان نیازمند authorization صریح درخواست است.</p>
      {write.map(v=><Field key={v} label={v}><input type="checkbox" disabled={!c.write_enabled} checked={c.tools.includes(v)} onChange={e=>set({tools:e.target.checked?[...c.tools,v]:c.tools.filter(x=>x!==v)})}/></Field>)}
      <h2>دانش دستی مجاز</h2>{manual.map(k=><Field key={k.id} label={k.name}><input type="checkbox" checked={c.knowledge.includes(k.id)} onChange={e=>set({knowledge:e.target.checked?[...c.knowledge,k.id]:c.knowledge.filter(v=>v!==k.id)})}/></Field>)}
      <h2>منابع واردشده مجاز</h2>{imported.map(k=>{const selector=`${k.source_kind}:${k.source_ref}`;return <Field key={k.id} label={`${k.name} · ${k.source_kind} · ${k.source_ref}`}><input type="checkbox" checked={c.sources.includes(selector)} onChange={e=>set({sources:e.target.checked?[...c.sources,selector]:c.sources.filter(v=>v!==selector)})}/></Field>})}
      <Field label="مراجع مجاز برای ابزار file.metadata (هر خط یک reference)"><textarea value={c.files.join('\n')} onChange={e=>set({files:e.target.value.split('\n').map(v=>v.trim()).filter(Boolean)})}/></Field>
      <Button loading={busy.loading}>ذخیره دستیار</Button>
    </form></Card>
    <Card><form onSubmit={async e=>{e.preventDefault();if(await busy.run(()=>mutation(base+'/knowledge',{knowledge:{...source,id:'',enabled:true,source_kind:'MANUAL',source_ref:'',revision:0},expected_revision:0}))){setSource({name:'',body:''});sources.retry()}}}>
      <h2>دانش دستی</h2><Field label="نام منبع"><input required maxLength={100} value={source.name} onChange={e=>setSource({...source,name:e.target.value})}/></Field><Field label="متن دانش"><textarea required maxLength={4096} value={source.body} onChange={e=>setSource({...source,body:e.target.value})}/></Field><Button loading={busy.loading}>افزودن منبع</Button>
    </form></Card>
    <Card><form onSubmit={async e=>{e.preventDefault();if(await busy.run(()=>mutation(base+'/ingest',ingest))){setIngest({...ingest,reference:'',name:''});sources.retry()}}}>
      <h2>ورود دانش از TELMIVO</h2><Field label="نوع منبع"><select value={ingest.source_kind} onChange={e=>setIngest({...ingest,source_kind:e.target.value})}><option value="FILE_MANAGER">File Manager</option><option value="UPLOADER">Uploader</option></select></Field><Field label="مرجع منبع"><input required maxLength={200} value={ingest.reference} onChange={e=>setIngest({...ingest,reference:e.target.value})}/></Field><Field label="نام اختیاری"><input maxLength={100} value={ingest.name} onChange={e=>setIngest({...ingest,name:e.target.value})}/></Field><p>Uploader فقط فایل‌های متنی ذخیره‌شده و کوچک را می‌پذیرد؛ URL، فایل باینری، shell و parser دلخواه اجرا نمی‌شود.</p><Button loading={busy.loading}>ورود و ایندکس</Button>
    </form></Card>
    <AIPlayground bot={bot!} disabled={busy.loading}/><h2>مصرف</h2>{usage.data?.map(u=><p key={u.id}>{u.state} · {u.model} · tokens {u.input_tokens}/{u.output_tokens} · tools {u.tool_calls} · chunks {u.retrieved_chunks} · cost {u.cost_microunits} µ · {u.latency_ms} ms</p>)}
  </>;
}
export function AIPlayground({bot,ticket,onUse,disabled=false}:{bot:string;ticket?:string;onUse?:(text:string)=>void;disabled?:boolean}){const [text,setText]=useState('');const [result,setResult]=useState<Result>();const busy=useMutation();return <Card><h2>{ticket?'پیشنهاد دستیار برای کارشناس':'آزمایش دستیار'}</h2><p>پیشنهاد به مشتری ارسال نمی‌شود.</p>{busy.error&&<ErrorState error={busy.error}/>}<form onSubmit={async e=>{e.preventDefault();await busy.run(async()=>setResult(await mutation<Result>(`/customer-bots/${bot}/assistant/chat`,{text,ticket:ticket??'',confirm_writes:false})))}}><Field label="درخواست از دستیار"><textarea required maxLength={4000} value={text} onChange={e=>setText(e.target.value)}/></Field><Button loading={busy.loading} disabled={disabled}>دریافت پیشنهاد</Button></form>{result&&<div role="status"><p>{result.state} — {result.code}</p><p style={{whiteSpace:'pre-wrap'}}>{result.text}</p>{onUse&&result.state==='SUCCEEDED'&&<Button onClick={()=>onUse(result.text)}>انتقال به پیش‌نویس پاسخ</Button>}</div>}</Card>}
