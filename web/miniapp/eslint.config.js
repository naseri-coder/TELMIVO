import ts from 'typescript-eslint';
export default ts.config({ignores:['dist']},...ts.configs.recommended,{files:['src/**/*.{ts,tsx}'],rules:{'@typescript-eslint/no-explicit-any':'error'}});
