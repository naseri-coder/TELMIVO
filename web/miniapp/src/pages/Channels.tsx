import {useState} from 'react';
import {Link,useParams} from 'react-router-dom';
import {api} from '../services/api';
import {useCustomer} from '../services/customer';
import type {Channel,ChannelPost,ChannelPostHistory} from '../services/channels';
import {Badge,Button,Card,Empty,ErrorState,Field,Loading,useMutation} from '../components/ui';
import {t} from '../locales';
import {operationKey,completeOperation} from '../services/operation';

async function createPost<T>(path:string,body:Record<string,unknown>):Promise<T>{
  const idempotency_key=await operationKey(path,body);
  const out=await api<T>(path,'POST',{...body,idempotency_key});
  completeOperation(path);
  return out;
}

export default function Channels(){
  const data=useCustomer<Channel[]>('/channels');
  const [chat,setChat]=useState('');
  const busy=useMutation();
  async function connect(){
    const n=Number(chat);
    if(!Number.isSafeInteger(n)||n>=0)throw new Error(t('channelChatInvalid'));
    await api('/channels/connect','POST',{chat_id:n});
    setChat('');
    data.retry();
  }
  return <><h1>{t('channelTitle')}</h1><p>{t('channelIntro')}</p>{busy.error&&<ErrorState error={busy.error}/>}<Card><form onSubmit={e=>{e.preventDefault();void busy.run(connect)}}><Field label={t('channelChatID')}><input inputMode="numeric" required value={chat} onChange={e=>setChat(e.target.value)} placeholder="-1001234567890"/></Field><p>{t('channelConnectHelp')}</p><Button loading={busy.loading}>{t('channelConnect')}</Button></form></Card>{data.error?<ErrorState error={data.error} retry={data.retry}/>:!data.data?<Loading/>:!data.data.length?<Card><Empty/></Card>:<div className="stats-grid">{data.data.map(c=><Card key={c.id}><h2>{c.title}</h2><p><Badge>{t('channelStatus'+c.status)}</Badge> · <Badge>{t('channelAuthority'+c.authority_health)}</Badge></p><p>{c.telegram_chat_id}</p><Link className="button" to={'/channels/'+c.id}>{t('channelManage')}</Link></Card>)}</div>}</>;
}

export function ChannelDetail(){
  const {channel}=useParams();
  const channels=useCustomer<Channel[]>('/channels');
  const posts=useCustomer<ChannelPost[]>('/channels/'+channel+'/posts');
  const current=channels.data?.find(c=>c.id===channel);
  const [body,setBody]=useState('');
  const [scheduled,setScheduled]=useState('');
  const busy=useMutation();
  async function make(schedule:boolean){
    const payload:Record<string,unknown>={body};
    if(schedule){
      if(!scheduled)throw new Error(t('channelScheduleRequired'));
      payload.scheduled_at=new Date(scheduled).toISOString();
    }
    await createPost('/channels/'+channel+'/posts',payload);
    setBody('');
    setScheduled('');
    posts.retry();
  }
  async function act(p:ChannelPost,action:string){
    const payload:Record<string,unknown>={action,expected_revision:p.revision};
    if(action==='SCHEDULE'){
      if(!scheduled)throw new Error(t('channelScheduleRequired'));
      payload.scheduled_at=new Date(scheduled).toISOString();
    }
    await api('/channels/'+channel+'/posts/'+p.id+'/actions','POST',payload);
    posts.retry();
  }
  if(channels.error)return <ErrorState error={channels.error} retry={channels.retry}/>;
  if(!channels.data)return <Loading/>;
  if(!current)return <ErrorState error={t('noData')}/>;
  return <><Link to="/channels">{t('channelBack')}</Link><h1>{current.title}</h1><Card><p><Badge>{t('channelStatus'+current.status)}</Badge> · <Badge>{t('channelAuthority'+current.authority_health)}</Badge></p><p>{t('channelChatID')}: <code>{current.telegram_chat_id}</code></p><p>{current.bot_can_post?t('channelCanPost'):t('channelCannotPost')}</p>{busy.error&&<ErrorState error={busy.error}/>}<Button loading={busy.loading} onClick={()=>busy.run(async()=>{await api('/channels/'+channel+'/verify','POST',{});channels.retry();})}>{t('channelVerify')}</Button>{current.status==='CONNECTED'&&<Button className="ghost" loading={busy.loading} onClick={()=>busy.run(async()=>{await api('/channels/'+channel+'/disconnect','POST',{});channels.retry();})}>{t('channelDisconnect')}</Button>}</Card><Card><h2>{t('channelNewPost')}</h2><Field label={t('channelPostBody')}><textarea required maxLength={4096} value={body} onChange={e=>setBody(e.target.value)}/></Field><Field label={t('channelScheduleAt')}><input type="datetime-local" value={scheduled} onChange={e=>setScheduled(e.target.value)}/></Field><Button disabled={!body.trim()} loading={busy.loading} onClick={()=>busy.run(()=>make(false))}>{t('channelSaveDraft')}</Button><Button disabled={!body.trim()||!scheduled||current.status!=='CONNECTED'} loading={busy.loading} onClick={()=>busy.run(()=>make(true))}>{t('channelSchedule')}</Button></Card><h2>{t('channelPosts')}</h2>{posts.error?<ErrorState error={posts.error} retry={posts.retry}/>:!posts.data?<Loading/>:!posts.data.length?<Card><Empty/></Card>:posts.data.map(p=><PostCard key={p.id} post={p} channel={channel!} schedule={scheduled} busy={busy} act={act}/>)}</>;
}

function PostCard({post:p,channel,busy,act}:{post:ChannelPost;channel:string;schedule:string;busy:ReturnType<typeof useMutation>;act:(p:ChannelPost,action:string)=>Promise<void>}){
  const [showHistory,setShowHistory]=useState(false);
  const history=useCustomer<ChannelPostHistory[]>('/channels/'+channel+'/posts/'+p.id+'/history');
  return <Card><h3>{t('channelPost')}</h3><p>{p.body}</p><p><Badge>{t('channelPostState'+p.state)}</Badge>{p.outcome_code&&<> · <code>{p.outcome_code}</code></>}</p>{p.scheduled_at&&<p>{t('channelScheduleAt')}: {new Date(p.scheduled_at).toLocaleString('fa-IR')}</p>}{p.state==='DRAFT'&&<><Button loading={busy.loading} onClick={()=>busy.run(()=>act(p,'PUBLISH_NOW'))}>{t('channelPublishNow')}</Button><Button className="ghost" loading={busy.loading} onClick={()=>busy.run(()=>act(p,'CANCEL'))}>{t('channelCancel')}</Button></>}{p.state==='SCHEDULED'&&<Button className="ghost" loading={busy.loading} onClick={()=>busy.run(()=>act(p,'CANCEL'))}>{t('channelCancel')}</Button>}<Button className="ghost" onClick={()=>setShowHistory(!showHistory)}>{t('channelHistory')}</Button>{showHistory&&(history.error?<ErrorState error={history.error} retry={history.retry}/>:!history.data?<Loading/>:<div>{history.data.map(h=><p key={h.revision}>#{h.revision} · {t('channelPostState'+h.state)}{h.outcome_code?' · '+h.outcome_code:''}</p>)}</div>)}</Card>;
}
