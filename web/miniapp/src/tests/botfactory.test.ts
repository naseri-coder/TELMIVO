import {describe,it,expect} from 'vitest';
import {commercialMoney} from '../services/botfactory';
describe('commercial denomination rendering',()=>{it('uses exact large atomic integers and catalog scale',()=>{expect(commercialMoney({unit:'USD',scale:2,amount:'9007199254740993'})).toBe('90071992547409.93 USD');expect(commercialMoney({unit:'IRR',scale:0,amount:'0'})).toBe('0 IRR');expect(commercialMoney({unit:'CUSTOM',scale:3,amount:'5'})).toBe('0.005 CUSTOM');});});
