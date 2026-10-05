import {api,downloadBinary,mutation,uploadBinary} from './api';

export interface Workflow{
  id:string;name:string;approval_required:boolean;allowed_types:string[];max_bytes:number;
  retention_days:number;destination:string;enabled:boolean;revision:number;created_at:string;updated_at:string;
}
export interface Upload{
  id:string;workflow_id:string;customer_bot_id?:string;state:string;name:string;content_type:string;
  size_bytes:number;sha256?:string;expires_at:string;error_code?:string;revision:number;created_at:string;updated_at:string;
}
export const createWorkflow=(body:Record<string,unknown>)=>mutation<Workflow>('/uploader/workflows',body);
export const createUpload=(file:File,workflow_id:string,customer_bot_id='')=>mutation<Upload>('/uploader/uploads',{
  workflow_id,customer_bot_id,name:file.name,content_type:file.type||'application/octet-stream',size_bytes:file.size
});
export const putUpload=(upload:string,file:File)=>uploadBinary<Upload>('/uploader/uploads/'+upload+'/content',file);
export const uploadAction=(upload:string,action:string,expected_revision:number)=>api<Upload>('/uploader/uploads/'+upload+'/actions','POST',{action,expected_revision});
export const handoffUpload=(upload:string,bot_id:string,title:string)=>mutation('/uploader/uploads/'+upload+'/file-manager',{bot_id,title});
export async function saveDownloaded(upload:string,name:string){const blob=await downloadBinary('/uploader/uploads/'+upload+'/content');const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),0);}
