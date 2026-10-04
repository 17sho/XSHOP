import {createApp,nextTick} from 'vue'
import {createI18n} from 'vue-i18n'
import {it,expect,vi,afterEach} from 'vitest'
import Tab from './views/admin/components/SettingsCaptchaTab.vue'
const api=vi.hoisted(()=>({updateCaptchaSettings:vi.fn()}))
vi.mock('@/api/admin',()=>({adminAPI:api}))
vi.mock('@/utils/notify',()=>({notifyError:vi.fn(),notifySuccess:vi.fn()}))
let app:any;let el:HTMLElement
const data=()=>({provider:'image',scenes:{login:true,register:false,register_send_code:false,reset_send_code:false,guest_create_order:true,gift_card_redeem:false},image:{character_type:'digits',length:4,width:240,height:80,noise_count:2,show_line:10,expire_seconds:300,max_store:10240},turnstile:{site_key:'',secret_key:'',has_secret:false,verify_url:'https://challenges.cloudflare.com/turnstile/v0/siteverify',timeout_ms:2000}})
async function flush(){await nextTick();await Promise.resolve();await nextTick()}
afterEach(()=>{app?.unmount();el?.remove();vi.clearAllMocks()})
it('renders bitmask checkboxes preserving legacy combinations and saves length four',async()=>{
 api.updateCaptchaSettings.mockResolvedValue({data:{data:{}}})
 el=document.createElement('div');document.body.append(el)
 app=createApp(Tab,{loaded:true,data:data()});app.use(createI18n({legacy:false,locale:'en',missingWarn:false,fallbackWarn:false,messages:{en:{}}}));const vm=app.mount(el) as any;await flush()
 const checks=[...el.querySelectorAll<HTMLInputElement>('input[type=checkbox]')]
 expect(checks.map(i=>i.checked)).toEqual([true,false,true])
 checks[1]!.checked=true;checks[1]!.dispatchEvent(new Event('change',{bubbles:true}));await flush()
 await vm.save();expect(api.updateCaptchaSettings.mock.calls[0]![0].image).toMatchObject({length:4,show_line:14,noise_count:2})
 const length=el.querySelector<HTMLInputElement>('input[min="4"][max="8"]')!
 expect(length).not.toBeNull();length.value='3';length.dispatchEvent(new Event('input',{bubbles:true}));await flush();await vm.save();expect(api.updateCaptchaSettings).toHaveBeenCalledTimes(1)
})
