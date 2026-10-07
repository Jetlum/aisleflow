// Executes the actual browser controller with DOM/canvas adapters. Browser layout
// and pixel appearance require separate visual inspection.
const {test}=require('node:test');
const assert=require('node:assert/strict');
const vm=require('node:vm');
const fs=require('node:fs');
const path=require('node:path');
function mount(reduced=false,width=900){
  const callbacks=new Map();let serial=0;let drawCalls=0;
  const element=()=>({value:'',textContent:'',innerHTML:'',attrs:{},events:{},classList:{toggle(){}},setAttribute(k,v){this.attrs[k]=v;},removeAttribute(k){delete this.attrs[k];},addEventListener(k,fn){this.events[k]=fn;}});
  const html=fs.readFileSync(path.join(__dirname,'index.html'),'utf8');
  const elements=Object.fromEntries([...html.matchAll(/id="([^"]+)"/g)].map(m=>[m[1],element()]));
  const canvas=new Proxy({measureText:text=>({width:text.length*6})},{get:(target,key)=>target[key]||(()=>{drawCalls++;}),set:(target,key,value)=>{target[key]=value;return true;}});
  elements.warehouse.getContext=()=>canvas;elements.warehouse.getBoundingClientRect=()=>({width,height:380});
  const buttons=Array.from({length:5},element);
  const document={getElementById:id=>{assert.ok(elements[id],`Missing DOM element ${id}`);return elements[id];},querySelectorAll:()=>buttons,addEventListener(k,fn){this[k]=fn;},hidden:false};
  const context=vm.createContext({document,matchMedia:()=>({matches:reduced,addEventListener(){}}),devicePixelRatio:1,ResizeObserver:class{observe(){}},requestAnimationFrame:fn=>{callbacks.set(++serial,fn);return serial;},cancelAnimationFrame:id=>callbacks.delete(id)});
  for(const file of ['story.js','animation.js'])vm.runInContext(fs.readFileSync(path.join(__dirname,file),'utf8'),context);
  const fire=(id,event,value)=>{if(value!==undefined)elements[id].value=value;elements[id].events[event]({target:elements[id]});};
  const tick=now=>{const jobs=[...callbacks.values()];callbacks.clear();jobs.forEach(fn=>fn(now));};
  return {elements,buttons,document,callbacks,fire,tick,get drawCalls(){return drawCalls;}};
}
test('real controller mounts, animates, pauses, seeks and restarts',()=>{
  const app=mount();assert.ok(app.drawCalls>0);assert.equal(app.callbacks.size,1);
  app.tick(0);app.tick(100);assert.ok(Number(app.elements.timeline.value)>0);
  app.fire('play','click');assert.equal(app.callbacks.size,0);
  app.fire('timeline','input','36');assert.equal(app.elements.aisle.textContent,'A02');assert.equal(app.elements.alerts.textContent,'1');
  assert.match(app.elements.completed.innerHTML,/^5 /);
  app.fire('restart','click');assert.equal(app.elements.timeline.value,'0');assert.equal(app.elements.alerts.textContent,'0');
  app.buttons[4].events.click();assert.equal(app.elements.timeline.value,'46');
  app.fire('timeline','input','53.9');app.fire('play','click');app.tick(200);app.tick(400);
  assert.equal(app.elements.timeline.value,'54');assert.match(app.elements.play.innerHTML,/Replay/);
  app.fire('play','click');assert.equal(app.elements.timeline.value,'0');assert.equal(app.callbacks.size,1);
  app.document.hidden=true;app.document.visibilitychange();assert.equal(app.callbacks.size,0);
});
test('reduced-motion mode stays paused; chapter buttons work at mobile drawing size',()=>{
  const app=mount(true,320);assert.equal(app.callbacks.size,0);assert.ok(app.drawCalls>0);
  app.buttons[3].events.click();assert.equal(app.elements.timeline.value,'30');assert.equal(app.callbacks.size,0);
  app.fire('speed','change','2');app.fire('play','click');app.tick(0);app.tick(100);
  assert.equal(app.elements.timeline.value,'30.2');
});
