import {useState} from 'react';
import {useCustomer} from '../services/customer';
import type {Bot} from '../services/botfactory';
import {createUpload,createWorkflow,handoffUpload,putUpload,saveDownloaded,uploadAction,type Upload,type Workflow} from '../services/uploader';
import {Badge,Button,Card,Empty,ErrorState,Field,Loading,useMutation} from '../components/ui';
import {t} from '../locales';

function stateTone(state:string){return state==='STORED'?'green':state==='FAILED'||state==='REJECTED'?'red':'neutral';}
function bytes(n:number){if(n<1024)return n+' B';if(n<1024*1024)return (n/1024).toFixed(1)+' KB';return (n/(1024*1024)).toFixed(1)+' MB';}

export default function Uploader(){
  const workflows=useCustomer<Workflow[]>('/uploader/workflows');
  const uploads=useCustomer<Upload[]>('/uploader/uploads');
  const bots=useCustomer<Bot[]>('/customer-bots');
  const busy=useMutation();
  const [workflow,setWorkflow]=useState('');
  const [file,setFile]=useState<File>();
  const [workflowName,setWorkflowName]=useState('');
  const [types,setTypes]=useState('application/pdf,image/jpeg,image/png,text/plain');
  const [maxBytes,setMaxBytes]=useState(10*1024*1024);
  const [retention,setRetention]=useState(30);
  const [approval,setApproval]=useState(true);
  const [handoff,setHandoff]=useState<Record<string,string>>({});

  async function addWorkflow(){
    await createWorkflow({name:workflowName,approval_required:approval,allowed_types:types.split(',').map(v=>v.trim()).filter(Boolean),max_bytes:maxBytes,retention_days:retention,destination:'OBJECT_STORAGE',enabled:true,expected_revision:0});
    setWorkflowName('');
    workflows.retry();
  }
  async function send(){
    if(!file||!workflow)throw new Error(t('uploaderChooseRequired'));
    const created=await createUpload(file,workflow);
    await putUpload(created.id,file);
    setFile(undefined);
    uploads.retry();
  }

  return <><h1>{t('uploaderTitle')}</h1><p>{t('uploaderIntro')}</p>{busy.error&&<ErrorState error={busy.error} retry={()=>{workflows.retry();uploads.retry();}}/>}
    <Card><h2>{t('uploaderNewWorkflow')}</h2>
      <Field label={t('uploaderWorkflowName')}><input maxLength={120} required value={workflowName} onChange={e=>setWorkflowName(e.target.value)}/></Field>
      <Field label={t('uploaderAllowedTypes')}><input value={types} onChange={e=>setTypes(e.target.value)}/></Field>
      <Field label={t('uploaderMaxBytes')}><input type="number" min={1} max={5000000000} value={maxBytes} onChange={e=>setMaxBytes(Number(e.target.value))}/></Field>
      <Field label={t('uploaderRetention')}><input type="number" min={1} max={3650} value={retention} onChange={e=>setRetention(Number(e.target.value))}/></Field>
      <Field label={t('uploaderApproval')}><input type="checkbox" checked={approval} onChange={e=>setApproval(e.target.checked)}/></Field>
      <Button disabled={!workflowName.trim()} loading={busy.loading} onClick={()=>busy.run(addWorkflow)}>{t('uploaderSaveWorkflow')}</Button>
    </Card>
    <Card><h2>{t('uploaderNewUpload')}</h2>
      {workflows.error?<ErrorState error={workflows.error} retry={workflows.retry}/>:!workflows.data?<Loading/>:
        <Field label={t('uploaderWorkflow')}><select value={workflow} onChange={e=>setWorkflow(e.target.value)}><option value="">{t('uploaderChoose')}</option>{workflows.data.filter(w=>w.enabled).map(w=><option key={w.id} value={w.id}>{w.name}</option>)}</select></Field>}
      <Field label={t('uploaderFile')}><input type="file" onChange={e=>setFile(e.target.files?.[0])}/></Field>
      {file&&<p>{file.name} · {bytes(file.size)}</p>}
      <Button disabled={!file||!workflow} loading={busy.loading} onClick={()=>busy.run(send)}>{t('uploaderStart')}</Button>
    </Card>
    <h2>{t('uploaderUploads')}</h2>
    {uploads.error?<ErrorState error={uploads.error} retry={uploads.retry}/>:!uploads.data?<Loading/>:!uploads.data.length?<Card><Empty/></Card>:<div className="stats-grid">{uploads.data.map(u=><Card key={u.id}>
      <h3>{u.name}</h3><p><Badge tone={stateTone(u.state)}>{t('uploaderState'+u.state)}</Badge> · {bytes(u.size_bytes)}</p>
      <p>{u.content_type}</p>{u.sha256&&<p><code>{u.sha256.slice(0,16)}…</code></p>}
      {u.state==='PENDING_APPROVAL'&&<><Button loading={busy.loading} onClick={()=>busy.run(async()=>{await uploadAction(u.id,'APPROVE',u.revision);uploads.retry();})}>{t('uploaderApprove')}</Button><Button className="ghost" loading={busy.loading} onClick={()=>busy.run(async()=>{await uploadAction(u.id,'REJECT',u.revision);uploads.retry();})}>{t('uploaderReject')}</Button></>}
      {u.state==='STORED'&&<><Button loading={busy.loading} onClick={()=>busy.run(()=>saveDownloaded(u.id,u.name))}>{t('uploaderDownload')}</Button><Button className="ghost" loading={busy.loading} onClick={()=>busy.run(async()=>{await uploadAction(u.id,'DELETE',u.revision);uploads.retry();})}>{t('uploaderDelete')}</Button>
        {bots.data&&bots.data.length>0&&<><Field label={t('uploaderBot')}><select value={handoff[u.id]??''} onChange={e=>setHandoff({...handoff,[u.id]:e.target.value})}><option value="">{t('uploaderChoose')}</option>{bots.data.map(b=><option key={b.id} value={b.id}>{b.name}</option>)}</select></Field><Button className="ghost" disabled={!handoff[u.id]} loading={busy.loading} onClick={()=>busy.run(()=>handoffUpload(u.id,handoff[u.id],u.name))}>{t('uploaderFileManager')}</Button></>}
      </>}
      {u.error_code&&<p role="alert">{u.error_code}</p>}
    </Card>)}</div>}
  </>;
}
