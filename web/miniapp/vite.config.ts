import {defineConfig} from 'vitest/config';
import react from '@vitejs/plugin-react';
export default defineConfig({plugins:[react()],server:{host:'127.0.0.1',proxy:{'/api':'http://127.0.0.1:8080'}},test:{environment:'jsdom',include:['src/**/*.test.ts','src/**/*.test.tsx']}});
