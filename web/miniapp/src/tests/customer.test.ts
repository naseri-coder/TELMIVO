import {describe,it,expect} from 'vitest';
import {formatAtomic} from '../locales';
describe('customer atomic money formatting',()=>{it('preserves large integer precision',()=>expect(formatAtomic('900719925474099199','USD')).toBe('۹٬۰۰۷٬۱۹۹٬۲۵۴٬۷۴۰٬۹۹۱.99'));it('formats supported asset scales',()=>{expect(formatAtomic('1234567','USDT_TON')).toBe('۱.234567');expect(formatAtomic('1234567890','TON')).toBe('۱.234567890');expect(formatAtomic('-1','USD')).toBe('-۰.01');expect(formatAtomic('12','XTR')).toBe('۱۲');});});
