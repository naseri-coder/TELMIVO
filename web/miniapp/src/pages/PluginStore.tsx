import {ConfigFields} from './Bots';
import {useState} from 'react';
import {Link,useParams} from 'react-router-dom';
import {api} from '../services/api';
import {useCustomer} from '../services/customer';
import {commercialMoney,defaultConfiguration,type Review} from '../services/botfactory';
import {operationalLabel,trialLabel,type StorePlugin} from '../services/store';
import {Badge,Button,Card,ErrorState,Field,Loading,Modal,useMutation} from '../components/ui';
import {t} from '../locales';
export default function PluginStore(){
 const {bot,plugin}=useParams();const [search,setSearch]=useState(''),[category,setCategory]=useState(''),[status,setStatus]=useState(''),[cursor,setCursor]=useState('');
 const path='/customer-bots/'+bot+'/store';
 const list=useCustomer<{items:StorePlugin[];next_cursor:string}>(path+'?'+new URLSearchParams({search,category,status,cursor}));
 if(plugin)return <StoreDetailPage bot={bot!} plugin={plugin}/>;
 return <><h1>فروشگاه پلاگین</h1><p>قیمت، دسترسی و وضعیت نصب مستقل‌اند. خرید آنلاین هنوز ارائه نشده است.</p>
 <Field label="جستجوی پلاگین"><input maxLength={100} value={search} onChange={e=>{setSearch(e.target.value);setCursor('')}}/></Field>
 <Field label="دسته‌بندی"><input placeholder="general" maxLength={100} value={category} onChange={e=>{setCategory(e.target.value);setCursor('')}}/></Field>
 <Field label="وضعیت تجاری"><select value={status} onChange={e=>{setStatus(e.target.value);setCursor('')}}><option value="">همه</option>{['INCLUDED','ALREADY_ENTITLED','PAID','SALE_DISABLED','TRIAL_AVAILABLE','UNAVAILABLE'].map(s=><option key={s} value={s}>{t('factory'+s)}</option>)}</select></Field>
 {list.error?<ErrorState error={list.error} retry={list.retry}/>:!list.data?<Loading/>:<div className="stats-grid">{list.data.items.map(p=><Card key={p.id}><h2>{p.name}</h2><p>{p.store.metadata.summary||p.description}</p><Badge>{t('factory'+p.commercial_status)}</Badge> <Badge>{operationalLabel(p.store.metadata.operational)}</Badge><p>{p.store.metadata.category} · {p.version}</p><Link to={path+'/'+p.id}>جزئیات {p.name}</Link></Card>)}</div>}
 {list.data?.items.length===0&&<p>پلاگینی یافت نشد.</p>}{cursor&&<Button onClick={()=>setCursor('')}>صفحه اول</Button>}{list.data?.next_cursor&&<Button onClick={()=>setCursor(list.data!.next_cursor)}>صفحه بعد</Button>}
 </>;
}
function PluginDetail({p,bot,refresh}:{p:StorePlugin;bot:string;refresh:()=>void}){
 const [review,setReview]=useState<Review>(),[action,setAction]=useState(''),[configuration,setConfiguration]=useState<Record<string,unknown>>(()=>defaultConfiguration(p));const op=useMutation();const base=`/customer-bots/${bot}/plugins/${p.id}`;
 const available=p.installable&&p.store.metadata.operational==='OPERATIONAL';
 async function preview(a:string){await op.run(async()=>{setAction(a);setReview(await api<Review>(base+'/impact','POST',{action:a,expected_revision:p.revision,...(a==='INSTALL'?{configuration}:{})}));});}
 return <><h1>{p.name}</h1><Card><p>{p.description}</p><p>{p.store.metadata.features}</p><Badge>{t('factory'+p.commercial_status)}</Badge> <Badge>{t('factoryState'+p.state)}</Badge>
 <p>دسترسی: {p.entitled?'مجاز':'نیازمند دسترسی'} · پلن: {p.plan} · شامل پلن: {p.included?'بله':'خیر'}</p><p>فروش: {p.store.sale_enabled?'باز':'بسته'} · اجرا: {operationalLabel(p.store.metadata.operational)}</p><p>آزمایشی: {trialLabel(p.store.trial_state)}</p>
 {p.prices&&<p>روزانه: {commercialMoney(p.prices.daily)} · ماهانه: {commercialMoney(p.prices.monthly)}</p>}
 {!p.entitled&&<p role="status">این پلاگین دسترسی فعالی ندارد؛ اعطای دسترسی نیازمند سیاست معتبر پلتفرم است.</p>}
 <p>وابستگی‌ها: {p.dependencies.join('، ')||'ندارد'} · تداخل‌ها: {p.conflicts.join('، ')||'ندارد'}</p><p>وابستگی فعال ناقص: {p.dependency_blocked?'بله':'خیر'} · تداخل فعال: {p.conflict_blocked?'بله':'خیر'}</p>
 <p>منابع دسترسی: {p.store.decision.sources.join('، ')||'ندارد'}</p>
 {Object.entries(p.store.quotas).map(([metric,q])=><p key={metric}>{metric}: مصرف {q.usage} / {q.limit.unlimited?'نامحدود':q.limit.value} · {q.allowed?'سهمیه موجود':'سهمیه تمام شده'}</p>)}
 {op.error&&<ErrorState error={op.error}/>}
 {p.state==='AVAILABLE'&&<><ConfigFields plugin={p} config={configuration} change={setConfiguration}/><Button disabled={!available} loading={op.loading} onClick={()=>preview('INSTALL')}>بررسی نصب</Button></>}
 {['INSTALLED','DISABLED'].includes(p.state)&&<Button disabled={!available} loading={op.loading} onClick={()=>preview('ACTIVATE')}>فعال‌سازی</Button>}
 {['ACTIVE','INSTALLED'].includes(p.state)&&<Button loading={op.loading} onClick={()=>preview('DISABLE')}>غیرفعال‌سازی</Button>}
 <p>غیرفعال‌سازی تنظیمات و سوابق را حفظ می‌کند.</p></Card>
 {review&&<Modal title="بررسی عملیات پلاگین" onClose={()=>setReview(undefined)}><p>عملیات: {action}</p>{review.issues.map((i,n)=><p role="alert" key={n}>{i.code}: {i.plugin} {i.other}</p>)}<p>وابستگی‌های لازم: {review.required.join('، ')||'ندارد'}</p><Button disabled={!review.ready} loading={op.loading} onClick={async()=>{if(await op.run(()=>api(base+'/actions','POST',{action,expected_revision:p.revision,...(action==='INSTALL'?{configuration}:{})}))){setReview(undefined);refresh();}}}>تأیید عملیات</Button></Modal>}
 </>;
}

function StoreDetailPage({bot,plugin}:{bot:string;plugin:string}){const path='/customer-bots/'+bot+'/store';const detail=useCustomer<StorePlugin>(path+'/'+plugin);return <><Link to={path}>بازگشت به فروشگاه پلاگین</Link>{detail.error?<ErrorState error={detail.error} retry={detail.retry}/>:!detail.data?<Loading/>:<PluginDetail key={plugin} p={detail.data} bot={bot} refresh={detail.retry}/>}</>;}
