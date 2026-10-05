import {api} from './api';
export interface Channel{id:string;telegram_chat_id:number;title:string;status:string;authority_health:string;bot_can_post:boolean;checked_at?:string}
export interface ChannelPost{id:string;channel_id:string;body:string;state:string;scheduled_at?:string;attempted_at?:string;delivered_at?:string;telegram_message_id?:number;outcome_code:string;revision:number;created_at:string;updated_at:string}
export interface ChannelPostHistory{revision:number;state:string;outcome_code:string;created_at:string}
export const listChannels=()=>api<Channel[]>('/channels');
