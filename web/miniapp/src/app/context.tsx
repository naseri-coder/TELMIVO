import {createContext,useContext} from 'react';
import type {User,Workspace,Decision} from '../services/api';
export const SessionContext=createContext<{user:User;workspace:Workspace;renameWorkspace:(name:string)=>void;access:Record<string,Decision>}>({} as {user:User;workspace:Workspace;renameWorkspace:(name:string)=>void;access:Record<string,Decision>});
export const useSession=()=>useContext(SessionContext);
