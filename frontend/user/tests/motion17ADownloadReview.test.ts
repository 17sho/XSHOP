import test from 'node:test'
import assert from 'node:assert/strict'
import { harness, settle, response } from './helpers/motion17AHarness.ts'
const calls=(h:any,kind:string)=>h.calls.filter((c:any)=>c.kind===kind)
const delivered=(no='A')=>response({order_no:no,status:'delivered'})
for(const name of ['useOrderDetail','useGuestOrderDetail']) for(const refresh of [false,true]) for(const outcome of ['success','error']) test(`A-R5 ${name} refresh=${refresh} download ${outcome} releases its busy ownership`,async()=>{
  const h=harness(name); h.mount(); const files:any[]=[]
  Object.assign(globalThis,{document:{createElement:()=>({href:'',download:'',click(){files.push(this.download)}})}})
  try {
    calls(h,'detail')[0].resolve(delivered()); await settle()
    void h.state.handleDownloadFulfillment('A-child'); await settle()
    if(refresh) {
      if(name==='useOrderDetail') void h.state.debouncedLoadOrder(); else void h.state.handleAuthSubmit()
      await settle(); calls(h,'detail').at(-1).resolve(delivered()); await settle()
    }
    assert.equal(h.state.fulfillmentDownloading.value,true)
    if(outcome==='success') calls(h,'download')[0].resolve({data:'synthetic fulfillment'})
    else calls(h,'download')[0].reject(new Error('synthetic download failure'))
    await settle()
    assert.deepEqual(files,outcome==='success'?['fulfillment-A-child.txt']:[])
    assert.equal(h.state.fulfillmentDownloading.value,false)
    void h.state.handleDownloadFulfillment('A-child'); await settle()
    assert.equal(calls(h,'download').length,2,'retry remains enabled')
    assert.equal(calls(h,'download')[1].args[0],'A-child','supplied child identity preserved')
  } finally {h.unmount()}
})

for(const name of ['useOrderDetail','useGuestOrderDetail']) test(`A-R5 ${name}: old download cannot release new identity operation`,async()=>{
  const h=harness(name); h.mount(); let downloaded=0
  Object.assign(globalThis,{document:{createElement:()=>({click(){downloaded++}})}})
  try {
    calls(h,'detail')[0].resolve(delivered()); await settle()
    void h.state.handleDownloadFulfillment('A'); await settle()
    h.route.params.order_no='B'; await settle()
    h.route.params.order_no='A'; await settle()
    calls(h,'detail').at(-1).resolve(delivered()); await settle()
    void h.state.handleDownloadFulfillment('A'); await settle()
    calls(h,'download')[0].resolve({data:'old'}); await settle()
    assert.equal(downloaded,0)
    assert.equal(h.state.fulfillmentDownloading.value,true)
    calls(h,'download')[1].resolve({data:'new'}); await settle()
    assert.equal(downloaded,1); assert.equal(h.state.fulfillmentDownloading.value,false)
  } finally {h.unmount()}
})

test('A-R5 guest credential edit suppresses old payload but releases owned busy flag',async()=>{
  const h=harness('useGuestOrderDetail'); h.mount(); let downloaded=0
  Object.assign(globalThis,{document:{createElement:()=>({click(){downloaded++}})}})
  try {
    calls(h,'detail')[0].resolve(delivered()); await settle()
    void h.state.handleDownloadFulfillment('A'); await settle()
    h.state.auth.value.email='changed@example.invalid'
    calls(h,'download')[0].resolve({data:'old credentials'}); await settle()
    assert.equal(downloaded,0); assert.equal(h.state.fulfillmentDownloading.value,false)
  } finally {h.unmount()}
})
