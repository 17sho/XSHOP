import test from 'node:test'
import assert from 'node:assert/strict'
import { harness, settle, deferred } from './helpers/motion17AHarness.ts'
const variants = [
  {name:'usePayment', method:'handleCopyPayLink', flag:'copied', field:'pay_url'},
  {name:'usePayment', method:'handleCopyWalletAddress', flag:'walletAddressCopied', field:'wallet_address'},
  {name:'useRechargeOrderDetail', method:'handleCopyWalletAddress', flag:'walletAddressCopied', field:'wallet_address'},
]
const replace = (h:any,name:string) => {if(name==='usePayment') h.route.query={order_no:'B'}; else h.route.params.recharge_no='B'}
const setValue = (h:any,v:any,text:string) => {(v.name==='usePayment'?h.state.paymentResult:h.state.payment).value={[v.field]:text}}
const copyTimers = (h:any) => [...h.timers.values()].filter((fn:any)=>fn.delay===1500)
for(const v of variants) for(const boundary of ['replace','unmount']) for(const outcome of ['success','error']) test(`A-R4 ${v.name}/${v.method}: pending ${outcome} after ${boundary} is inert`,async()=>{
  const copy=deferred(); const errors:any[]=[]
  const h=harness(v.name,{copyText:()=>copy.promise},{order_no:'A'}); h.mount()
  Object.assign(window,{alert:(message:any)=>errors.push(message)})
  try {
    setValue(h,v,'synthetic-A'); await settle()
    const operation=h.state[v.method](); await settle()
    if(boundary==='replace') replace(h,v.name); else h.unmount()
    await settle()
    if(outcome==='success') copy.resolve(undefined); else copy.reject(new Error('synthetic copy failure'))
    await operation; await settle()
    assert.equal(h.state[v.flag].value,false)
    assert.equal(copyTimers(h).length,0)
    assert.deepEqual(errors,[])
    if(v.name==='usePayment') assert.equal(h.state.paymentAlert.value?.message || '','')
  } finally {h.unmount()}
})

for(const v of variants) test(`A-R4 ${v.name}/${v.method}: timer boundary and new copy own feedback`,async()=>{
  const copies:any[]=[]
  const h=harness(v.name,{copyText:()=>{const d=deferred(); copies.push(d); return d.promise}},{order_no:'A'}); h.mount()
  try {
    setValue(h,v,'synthetic-A'); await settle(); void h.state[v.method](); copies[0].resolve(undefined); await settle()
    assert.equal(h.state[v.flag].value,true)
    const oldTimer=copyTimers(h)[0]; assert.ok(oldTimer)
    replace(h,v.name); await settle()
    assert.equal(h.state[v.flag].value,false,'entity change clears old feedback')
    assert.equal(copyTimers(h).length,0,'entity change cancels old timer')
    setValue(h,v,'synthetic-B'); await settle(); void h.state[v.method](); copies[1].resolve(undefined); await settle()
    const newTimer=copyTimers(h)[0]
    oldTimer(); assert.equal(h.state[v.flag].value,true,'already queued old timer cannot clear new feedback')
    assert.equal(copyTimers(h).length,1)
    for(const [id,fn] of h.timers) if(fn===newTimer) h.timers.delete(id)
    newTimer(); assert.equal(h.state[v.flag].value,false,'current timer clears current feedback')
    h.unmount(); assert.equal(copyTimers(h).length,0)
  } finally {h.unmount()}
})

for(const v of variants) test(`A-R4 ${v.name}/${v.method}: newest same-entity copy owns completion`,async()=>{
  const copies:any[]=[]; const errors:any[]=[]
  const h=harness(v.name,{copyText:()=>{const d=deferred(); copies.push(d); return d.promise}},{order_no:'A'}); h.mount()
  Object.assign(window,{alert:(message:any)=>errors.push(message)})
  try {
    setValue(h,v,'synthetic-A'); await settle()
    void h.state[v.method](); void h.state[v.method]()
    copies[1].resolve(undefined); await settle()
    const timer=copyTimers(h)[0]
    copies[0].reject(new Error('older failed copy')); await settle()
    assert.equal(h.state[v.flag].value,true)
    assert.deepEqual(errors,[])
    if(v.name==='usePayment') assert.equal(h.state.paymentAlert.value?.message || '','')
    assert.deepEqual(copyTimers(h),[timer])
  } finally {h.unmount()}
})

for(const v of variants) test(`A-R4 ${v.name}/${v.method}: a failed newer copy cannot strand old success feedback`,async()=>{
  const copies:any[]=[]
  const h=harness(v.name,{copyText:()=>{const d=deferred(); copies.push(d); return d.promise}},{order_no:'A'}); h.mount()
  Object.assign(window,{alert:()=>{}})
  try {
    setValue(h,v,'synthetic-A'); await settle()
    void h.state[v.method](); copies[0].resolve(undefined); await settle()
    assert.equal(h.state[v.flag].value,true)
    const oldTimer=copyTimers(h)[0]
    void h.state[v.method](); copies[1].reject(new Error('new copy failed')); await settle()
    assert.equal(h.state[v.flag].value,false)
    assert.equal(copyTimers(h).length,0)
    oldTimer(); assert.equal(h.state[v.flag].value,false)
  } finally {h.unmount()}
})

for(const v of variants) test(`A-R4 control ${v.name}/${v.method}: current rejection reports error without timer`,async()=>{
  const errors:any[]=[]; const h=harness(v.name,{copyText:async()=>{throw new Error('current copy failure')}},{order_no:'A'}); h.mount()
  Object.assign(window,{alert:(message:any)=>errors.push(message)})
  try {
    setValue(h,v,'synthetic-A'); await settle(); await h.state[v.method](); await settle()
    assert.equal(h.state[v.flag].value,false); assert.equal(copyTimers(h).length,0)
    if(v.name==='usePayment') assert.equal(h.state.paymentAlert.value?.message || '','current copy failure')
    else assert.deepEqual(errors,['payment.copyFailed'])
  } finally {h.unmount()}
})
