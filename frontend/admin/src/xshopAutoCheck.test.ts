import { createApp, nextTick } from 'vue'
import { createPinia,setActivePinia } from 'pinia'
import { useAdminAuthStore } from './stores/auth'
import {it,expect,vi,afterEach} from 'vitest'
import Panel from './components/XshopVersionBadge.vue'
const api=vi.hoisted(()=>({get:vi.fn(),post:vi.fn()}))
vi.mock('@/api/client',()=>({api}))
let app:any;let el:HTMLElement
async function flush(){for(let i=0;i<10;i++){await Promise.resolve();await nextTick()}}
afterEach(()=>{app?.unmount();el?.remove();vi.useRealTimers();vi.restoreAllMocks();vi.clearAllMocks()})
it('automatically checks visible admin session and shows red dot without downloading',async()=>{
 vi.useFakeTimers();vi.spyOn(document,'visibilityState','get').mockReturnValue('visible');
 api.get.mockResolvedValue({data:{data:{state:'idle'}}});api.post.mockImplementation(async()=>{api.get.mockResolvedValue({data:{data:{state:'available',version:'v2',digest:'signed'}}});return {data:{data:{}}}});
 const pinia=createPinia();setActivePinia(pinia);const auth=useAdminAuthStore();auth.token='synthetic';auth.isSuper=true;
 el=document.createElement('div');document.body.append(el);app=createApp(Panel);app.use(pinia);app.mount(el);await flush();
 expect(api.post).toHaveBeenCalledTimes(1);expect(api.post).toHaveBeenCalledWith('/admin/xshop-upgrade/check');expect(el.querySelector('[aria-label="有更新待处理"]')?.classList.contains('bg-red-500')).toBe(true);expect(el.querySelector('[role="dialog"]')).toBeNull();
 await vi.advanceTimersByTimeAsync(600000);await flush();expect(api.post).toHaveBeenCalledTimes(2);
 auth.isSuper=false;await flush();await vi.advanceTimersByTimeAsync(600000);expect(api.post).toHaveBeenCalledTimes(2);
})
