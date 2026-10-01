import test from 'node:test'
import assert from 'node:assert/strict'
import { debounceAsync } from '../src/utils/debounce.ts'
import { harness, settle, response, deferred } from './helpers/motion17AHarness.ts'
const calls = (h:any, kind:string) => h.calls.filter((c:any) => c.kind === kind)
const pendingOrder = (no='A') => ({ order_no:no, status:'pending_payment', total_amount:'10.00' })
const flushDelay = async (h:any, delay:number) => {
  for(const [id,fn] of [...h.timers]) if ((fn as any).delay === delay) {h.timers.delete(id); void fn()}
  await settle()
}

for (const provider of ['stripe','paypal']) for (const cancelQueuedRead of [false,true]) test(`A-R3 REAL debounce: ${provider} reconciles before cleanup (cancel=${cancelQueuedRead})`, async()=>{
  const h=harness('usePayment',{debounceAsync},{order_no:'A'}); h.mount()
  try {
    calls(h,'detail')[0].resolve(response(pendingOrder())); await settle()
    calls(h,'latest')[0].resolve(response({payment_id:91,provider_type:'official',channel_type:provider,pay_url:'https://synthetic.invalid/pay'})); await settle()
    const marker=provider==='stripe'?'stripe_return':'pp_return'
    h.route.query={order_no:'A',[marker]:'1'}; h.route.fullPath='/pay?return'; await settle()
    calls(h,'capture')[0].resolve(response({committed:true})); await settle()
    assert.equal(calls(h,'detail').length,1,'real 250ms debounce has not dispatched')
    if(cancelQueuedRead) {h.state.handleChangePaymentMethod(); await settle()}
    else await flushDelay(h,250)
    assert.equal(h.route.query[marker],'1','no cleanup until a successful authoritative read')
    assert.equal(calls(h,'detail').length,2,'canceled debounce is recovered with an explicit read')
    calls(h,'detail')[1].resolve(response({order_no:'A',status:'paid'})); await settle()
    assert.equal(h.state.order.value.status,'paid')
    assert.equal(h.route.query[marker],undefined)
    assert.equal(calls(h,'capture').length,1)
    assert.equal(calls(h,'create').length,0)
    assert.equal(calls(h,'cancel').length,0)
  } finally {h.unmount()}
})

test('A-R3 REAL debounce: failed authoritative refresh retains return markers', async()=>{
  const h=harness('usePayment',{debounceAsync},{order_no:'A'}); h.mount()
  try {
    calls(h,'detail')[0].resolve(response(pendingOrder())); await settle()
    calls(h,'latest')[0].resolve(response({payment_id:91,provider_type:'official',channel_type:'stripe',pay_url:'https://synthetic.invalid/pay'})); await settle()
    h.route.query={order_no:'A',stripe_return:'1'}; h.route.fullPath='/pay?return'; await settle()
    calls(h,'capture')[0].resolve(response({committed:true})); await settle()
    h.state.handleChangePaymentMethod(); await settle()
    assert.equal(calls(h,'detail').length,2)
    calls(h,'detail')[1].reject(new Error('synthetic refresh failure')); await settle()
    assert.equal(h.route.query.stripe_return,'1')
    assert.equal(h.redirects.length,0)
  } finally {h.unmount()}
})

test('A-R2: returning to an identical marker value does not revive an older cleanup owner', async()=>{
  const h=harness('usePayment',{}, {order_no:'A'}); h.mount()
  try {
    calls(h,'detail')[0].resolve(response(pendingOrder())); await settle()
    calls(h,'latest')[0].resolve(response(null)); await settle()
    h.route.query={order_no:'A',epay_return:'1'}; h.route.fullPath='/pay?first'; await settle()
    const read=calls(h,'detail').at(-1)
    h.route.query={order_no:'A'}; h.route.fullPath='/pay?none'; await settle()
    h.route.query={order_no:'A',epay_return:'1'}; h.route.fullPath='/pay?new'; await settle()
    read.resolve(response(pendingOrder())); await settle()
    calls(h,'latest').at(-1).resolve(response(null)); await settle()
    assert.equal(h.route.query.epay_return,'1','new arrival owns even an identical marker value')
  } finally {h.unmount()}
})

for(const guest of [true,false]) test(`A-R1 control REAL debounce: current ${guest?'guest':'member'} create opens returned link exactly once`,async()=>{
  const h=harness('usePayment',{debounceAsync},{order_no:'A',...(guest?{guest:'1'}:{})}); h.mount()
  try {
    calls(h,'detail')[0].resolve(response(pendingOrder())); await settle()
    calls(h,'latest')[0].resolve(response(null)); await settle()
    h.state.selectedChannelId.value=7; void h.state.handlePayment(); void h.state.handlePayment()
    await flushDelay(h,200)
    assert.equal(calls(h,'create').length,1)
    calls(h,'create')[0].resolve(response({payment_id:92,channel_id:7,pay_url:'https://synthetic.invalid/current-A',interaction_mode:'redirect'})); await settle()
    for(const call of calls(h,'wallet')) call.resolve(response({balance:'0'})); await settle()
    assert.equal(h.state.paymentResult.value.payment_id,92)
    assert.deepEqual(h.redirects,['https://synthetic.invalid/current-A'])
    assert.equal(h.state.submitting.value,false)
    assert.equal(calls(h,'cancel').length,0)
  } finally {h.unmount()}
})

for (const provider of ['stripe', 'paypal']) test(`A-R2: generic return sync cannot consume newly arrived ${provider} markers`, async () => {
  const h=harness('usePayment',{}, {order_no:'A'}); h.mount()
  try {
    calls(h,'detail')[0].resolve(response(pendingOrder())); await settle()
    calls(h,'latest')[0].resolve(response(null)); await settle()
    h.route.query={order_no:'A',epay_return:'1'}; h.route.fullPath='/pay?epay_return=1'; await settle()
    const genericRead=calls(h,'detail').at(-1)
    const markers=provider==='stripe'?{stripe_return:'1',session_id:'new-session'}:{pp_return:'1',PayerID:'new-payer'}
    h.route.query={order_no:'A',...markers}; h.route.fullPath='/pay?new-provider'; await settle()
    genericRead.resolve(response(pendingOrder())); await settle()
    calls(h,'latest').at(-1).resolve(response({payment_id:72,provider_type:'official',channel_type:provider,pay_url:'https://synthetic.invalid/pay'})); await settle()
    assert.equal(calls(h,'capture').length,1)
    for(const [key,value] of Object.entries(markers)) assert.equal(h.route.query[key],value,'pending provider capture owns its markers')
    assert.equal(h.redirects.length,0)
  } finally {h.unmount()}
})

test('A-R2 control: unchanged generic return owner consumes only its own markers', async () => {
  const h=harness('usePayment',{}, {order_no:'A'}); h.mount()
  try {
    calls(h,'detail')[0].resolve(response(pendingOrder())); await settle()
    calls(h,'latest')[0].resolve(response(null)); await settle()
    h.route.query={order_no:'A',epay_return:'1',tracking:'keep'}; h.route.fullPath='/pay?epay_return=1'; await settle()
    calls(h,'detail').at(-1).resolve(response(pendingOrder())); await settle()
    calls(h,'latest').at(-1).resolve(response(null)); await settle()
    assert.equal(h.route.query.epay_return,undefined)
    assert.equal(h.route.query.tracking,'keep')
    assert.equal(h.redirects.length,1)
  } finally {h.unmount()}
})

for (const provider of ['stripe','paypal']) test(`A-R2: ${provider} capture cannot consume a replacement return token`, async()=>{
  const h=harness('usePayment',{}, {order_no:'A'}); h.mount()
  try {
    calls(h,'detail')[0].resolve(response(pendingOrder())); await settle()
    calls(h,'latest')[0].resolve(response(null)); await settle()
    h.state.paymentResult.value={payment_id:91,provider_type:'official',channel_type:provider}; await settle()
    const key=provider==='stripe'?'session_id':'token'
    h.route.query={order_no:'A',[key]:'first'}; h.route.fullPath='/pay?first'; await settle()
    h.route.query={order_no:'A',[key]:'second'}; h.route.fullPath='/pay?second'; await settle()
    calls(h,'capture')[0].resolve(response({committed:true})); await settle()
    calls(h,'detail').at(-1).resolve(response(pendingOrder())); await settle()
    assert.equal(h.route.query[key],'second')
    assert.equal(h.redirects.length,0)
  } finally {h.unmount()}
})

test('A-R1: pending creation A-B-A reconciles authoritative link without duplicate creation', async () => {
  const h = harness('usePayment', { useAppStore: () => ({locale:'en-US',loading:false,config:{payment_channels:[{id:7,provider_type:'epay',channel_type:'alipay'}]},getServerTime:()=>Date.now()}) }, {order_no:'A',guest:'1'}); h.mount()
  try {
    calls(h,'detail')[0].resolve(response(pendingOrder())); await settle()
    calls(h,'latest')[0].resolve(response(null)); await settle()
    h.state.selectedChannelId.value=7; void h.state.handlePayment(); await settle()
    const first = calls(h,'create')[0]; assert.equal(first.args[0].order_no, 'A')
    h.route.query={order_no:'B',guest:'1'}; await settle()
    h.route.query={order_no:'A',guest:'1'}; await settle()
    calls(h,'detail').at(-1).resolve(response(pendingOrder())); await settle()
    calls(h,'latest').at(-1).resolve(response(null)); await settle()
    h.state.selectedChannelId.value=7; void h.state.handlePayment(); await settle()
    assert.equal(calls(h,'create').length, 1, 'pending duplicate is correctly suppressed')
    first.resolve(response({payment_id:71,channel_id:7,pay_url:'https://synthetic.invalid/authoritative',interaction_mode:'redirect'})); await settle()
    assert.equal(h.state.paymentResult.value?.payment_id, 71)
    assert.equal(h.state.cachedPayment.value?.payment_id, 71)
    assert.equal(h.redirects.length, 0)
    const latestCount=calls(h,'latest').length
    void h.state.handleRefresh(); await settle()
    calls(h,'detail').at(-1).resolve(response(pendingOrder())); await settle()
    assert.equal(calls(h,'latest').length, latestCount, 'refresh does not reconcile discarded outcome')
    h.state.selectedChannelId.value=7; void h.state.handlePayment(); await settle()
    assert.equal(calls(h,'create').length, 1, 'resume settled authoritative creation')
    assert.equal(calls(h,'cancel').length,0)
  } finally { h.unmount() }
})


for (const guest of [true, false]) for (const settleOn of ['A', 'B']) test(`A-R1: ${guest ? 'guest' : 'member'} creation settles on ${settleOn} and resumes A only`, async () => {
  const h=harness('usePayment', {useAppStore:()=>({locale:'en-US',loading:false,config:{payment_channels:[{id:7,provider_type:'epay',channel_type:'alipay'}]},getServerTime:()=>Date.now()})}, {order_no:'A', ...(guest ? {guest:'1'} : {})}); h.mount()
  const query=(no:string)=>({order_no:no,...(guest?{guest:'1'}:{})})
  try {
    calls(h,'detail')[0].resolve(response(pendingOrder())); await settle()
    calls(h,'latest')[0].resolve(response(null)); await settle()
    h.state.selectedChannelId.value=7; void h.state.handlePayment(); await settle()
    h.route.query=query('B'); await settle()
    if(settleOn==='A') {h.route.query=query('A'); await settle(); calls(h,'detail').at(-1).resolve(response(pendingOrder())); await settle(); calls(h,'latest').at(-1).resolve(response(null)); await settle()}
    calls(h,'create')[0].resolve(response({payment_id:73,channel_id:7,pay_url:'https://synthetic.invalid/settled-A',interaction_mode:'redirect'})); await settle()
    if(settleOn==='B') {
      assert.equal(h.state.paymentResult.value,null); assert.equal(h.redirects.length,0)
      h.route.query=query('A'); await settle(); calls(h,'detail').at(-1).resolve(response(pendingOrder())); await settle()
      for(const c of calls(h,'latest')) c.resolve(response(null)); await settle()
    }
    assert.equal(h.state.paymentResult.value?.payment_id,73)
    assert.equal(h.state.cachedPayment.value?.payment_id,73)
    assert.equal(h.redirects.length,0,'reconciliation never replays original automatic navigation')
    h.state.selectedChannelId.value=7; void h.state.handlePayment(); await settle()
    assert.equal(calls(h,'create').length,1)
    assert.equal(calls(h,'cancel').length,0)
    assert.deepEqual(h.redirects,['https://synthetic.invalid/settled-A'],'explicit resume opens only A')
  } finally {h.unmount()}
})
