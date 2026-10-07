/* Dependency-free presentation. No backend requests or live operational data. */
(() => {
  'use strict';
  const {stateAt,chapters,duration}=globalThis.PyckStory;
  const $=id=>document.getElementById(id);
  const canvas=$('warehouse'), ctx=canvas.getContext('2d');
  const buttons=Array.from(document.querySelectorAll('[data-chapter]'));
  const reducedMotion=matchMedia('(prefers-reduced-motion: reduce)');
  let time=0,playing=false,speed=1,last=null,frame=null,lastText='';
  const copy=[
    ['DESIGNED TO EXECUTE','One process. A clear path.','The BPMN compiler validates the picking process and generates a Go workflow plus an Ent task-receipt schema.'],
    ['ORCHESTRATED BY TEMPORAL','The process becomes movement.','Picking tasks run in sequence on A01. Each completed movement emits telemetry, giving the analyzer a view of aisle traversal time.'],
    ['OBSERVE. COMPARE. DETECT.','A slowdown becomes a signal.','The fourth movement takes 30 seconds. The latest three samples average 17 seconds, crossing the configured 15-second threshold.'],
    ['FINISH THIS TASK. ADAPT THE NEXT.','Same wave. A better next move.','The alert is persisted through an outbox and delivered to this workflow run. Before the next task, Temporal selects the process-approved alternative, A02.'],
    ['PROCESS COMPLETE','Nine tasks. One adaptive flow.','Four picks on A01, four on A02, then express packing. Duplicate telemetry is ignored, and exactly one reroute alert is accepted.']
  ];
  const stops=[
    [0,0,3],[8,0,3],[12,1.5,3],[16,3.5,3],[20,5.5,3],[26,7.5,3],
    [31,7.5,3],[32,11,3],[33,11,6],[36,9.5,6],[39,7.5,6],[42,5.5,6],[45,3.5,6],
    [47,11,6],[49,11,8],[50,11.7,8],[54,11.7,8]
  ];
  function position(t){
    for(let i=1;i<stops.length;i++){
      if(t<=stops[i][0]){
        const a=stops[i-1],b=stops[i],p=Math.max(0,(t-a[0])/(b[0]-a[0]));
        const ease=p*p*(3-2*p);
        return [a[1]+(b[1]-a[1])*ease,a[2]+(b[2]-a[2])*ease];
      }
    }
    return stops[stops.length-1].slice(1);
  }
  const project=(x,y,z=0)=>[376+(x-y)*31,62+(x+y)*14.8-z*30];
  function polygon(points,fill,stroke){
    ctx.beginPath();points.forEach((p,i)=>i?ctx.lineTo(...p):ctx.moveTo(...p));ctx.closePath();
    if(fill){ctx.fillStyle=fill;ctx.fill();}if(stroke){ctx.strokeStyle=stroke;ctx.lineWidth=1;ctx.stroke();}
  }
  function line(points,color,width=1,dash=[]){
    ctx.beginPath();points.forEach((p,i)=>i?ctx.lineTo(...p):ctx.moveTo(...p));
    ctx.strokeStyle=color;ctx.lineWidth=width;ctx.lineJoin='round';ctx.lineCap='round';ctx.setLineDash(dash);ctx.stroke();ctx.setLineDash([]);
  }
  function box(x,y,w,d,h,colors,z=0){
    const a=project(x,y,z+h),b=project(x+w,y,z+h),c=project(x+w,y+d,z+h),e=project(x,y+d,z+h);
    polygon([e,c,project(x+w,y+d,z),project(x,y+d,z)],colors[1],'#485574');
    polygon([b,c,project(x+w,y+d,z),project(x+w,y,z)],colors[2],'#485574');
    polygon([a,b,c,e],colors[0],'#788399');
  }
  function text(label,x,y,size=11,color='#c1c8df',align='center'){
    ctx.fillStyle=color;ctx.font=`500 ${size}px "Segoe UI",Arial,sans-serif`;ctx.textAlign=align;ctx.fillText(label,x,y);
  }
  function pill(label,x,y,color='#19fff6'){
    ctx.font='600 10px "Segoe UI",Arial,sans-serif';const w=ctx.measureText(label).width+22;
    ctx.fillStyle='#20222d';ctx.strokeStyle='#46546d';ctx.lineWidth=1;
    ctx.beginPath();ctx.roundRect(x-w/2,y-13,w,25,5);ctx.fill();ctx.stroke();text(label,x,y+3,10,color);
  }
  function draw(s){
    const rect=canvas.getBoundingClientRect();if(!rect.width||!ctx)return;
    const dpr=Math.min(devicePixelRatio||1,2),w=Math.round(rect.width*dpr),h=Math.round(rect.height*dpr);
    if(canvas.width!==w||canvas.height!==h){canvas.width=w;canvas.height=h;}
    ctx.setTransform(dpr,0,0,dpr,0,0);ctx.clearRect(0,0,rect.width,rect.height);
    // On phones use a tighter virtual view so aisle labels retain readable size.
    const viewWidth=rect.width<500?650:820;
    const scale=Math.min(rect.width/viewWidth,rect.height/390);
    ctx.translate((rect.width-820*scale)/2,(rect.height-390*scale)/2);ctx.scale(scale,scale);
    polygon([project(-.6,-.5,-.23),project(12.8,-.5,-.23),project(12.8,9,-.23),project(-.6,9,-.23)],'#14151f');
    polygon([project(-.6,-.5),project(12.8,-.5),project(12.8,9),project(-.6,9)],'#3b414e','#626b80');
    for(let x=0;x<=12;x++)line([project(x,0),project(x,8.7)],'#454d5d');
    for(let y=0;y<=8;y++)line([project(0,y),project(12.5,y)],'#454d5d');
    // Low cutaway walls, striped safety edges and packing conveyor ground the scene.
    polygon([project(-.6,-.5),project(12.8,-.5),project(12.8,-.5,1.2),project(-.6,-.5,1.2)],'#515a70','#8994ab');
    line([project(-.6,-.5,.12),project(12.8,-.5,.12)],'#19fff6',1.5);
    line([project(-.4,8.7),project(10,8.7)],'#c9cb64',2,[8,6]);
    polygon([project(10.8,7.25),project(12.3,7.25),project(12.3,8.7),project(10.8,8.7)],'#595271','#877aa9');
    for(let y=7.35;y<8.6;y+=.2)line([project(10.9,y,.2),project(12.2,y,.2)],'#99a1b8',2);
    // Wide soft highlights keep routes visibly separate from the storage racks.
    line([project(0,3),project(11,3)],s.alert?'#634b2c':'#365866',17);
    line([project(0,6),project(11,6)],s.accepted?'#236a71':'#434555',17);
    line([project(0,3),project(11,3)],s.alert?'#e6a75a':'#19fff6',1.5,[5,7]);
    line([project(0,6),project(11,6)],s.accepted?'#19fff6':'#8184a4',1.5,[5,7]);
    if(s.accepted)line([project(7.5,3),project(11,3),project(11,6),project(3.5,6)],'#19fff6',3,[7,5]);
    if(s.time>=46)line([project(3.5,6),project(11,6),project(11,8),project(11.7,8)],'#19fff6',3,[7,5]);
    const objects=[];
    for(const y of [.4,3.8,6.8])for(const x of [1,3,5,7,9]){
      objects.push({depth:x+y,paint:()=>{
        box(x,y,1.55,.8,.15,['#7c8fa3','#485e7a','#30455d']);
        box(x+.06,y+.04,1.43,.7,.64,['#b9b19a','#857e70','#676777'],.15);
        box(x+.06,y+.04,1.43,.7,.07,['#a0acc2','#4c6c8f','#30455d'],.83);
        box(x+.12,y+.1,.57,.56,.45,['#d3c6a8','#aa957c','#8c7b70'],.91);
        box(x+.82,y+.1,.55,.56,.45,['#bdb5a4','#948979','#726c71'],.91);
        line([project(x+.75,y+.82,.24),project(x+.75,y+.82,.75)],'#c2c5cc',1);
        for(const px of [x,x+1.55])line([project(px,y+.8,0),project(px,y+.8,1.55)],'#509dc0',2);
      }});
    }
    const pos=position(s.time);
    objects.push({depth:pos[0]+pos[1]+.5,paint:()=>{
      const p=project(...pos);
      ctx.save();ctx.translate(...p);ctx.scale(1,.48);ctx.beginPath();ctx.arc(0,0,17,0,Math.PI*2);ctx.fillStyle='#19fff633';ctx.fill();ctx.restore();
      box(pos[0]-.24,pos[1]-.23,.48,.46,.48,['#c9fffc','#03b0e9','#5459a7']);
      const head=project(pos[0],pos[1],.94);ctx.beginPath();ctx.arc(...head,5.2,0,Math.PI*2);ctx.fillStyle='#c9fffc';ctx.fill();
      line([project(pos[0],pos[1],.48),project(pos[0],pos[1],.76)],'#19fff6',5);
    }});
    objects.push({depth:19,paint:()=>{
      box(11.2,7.4,1.1,1.1,.35,['#6a6093','#504b70','#343b58']);
      box(11.5,7.6,.5,.5,.4,['#d2ba85','#a38b5c','#7e6741'],.35);
    }});
    objects.sort((a,b)=>a.depth-b.depth).forEach(o=>o.paint());
    const a=project(-.4,3),b=project(-.4,6),pack=project(11.8,8.8);
    pill('A01',a[0]-9,a[1]+10,s.alert?'#f4b765':'#d2deef');pill('A02',b[0]-9,b[1]+10);
    pill('EXPRESS',pack[0]+18,pack[1]+17,s.finished?'#19fff6':'#c5d5e9');
    const picker=project(...pos,1.3);
    pill(s.finished?'WAVE COMPLETE':s.time<8?'PICKING READY':s.time<31?'PICKER · A01':s.time<33?'REROUTE ACCEPTED':s.time<46?'PICKER · A02':'EXPRESS PACKING',picker[0],picker[1]-10);
    if(s.alert){const alert=project(6.8,2.7,1.8);pill('!  A01 CONGESTION',alert[0],alert[1]-13,'#f4b765');}
    // Visible packet travels only during playback time; pausing freezes all motion.
    if(s.time>=12&&s.time<46){
      const packet=project(12.25,1+(s.time%2)/2*5,.2);
      ctx.beginPath();ctx.arc(...packet,3.5,0,Math.PI*2);ctx.fillStyle=s.alert?'#f4b765':'#19fff6';ctx.fill();
    }
    text('WAREHOUSE MOVEMENT',650,67,9,'#a6b2ca');
    text(s.time<8?'Validated process · approved alternatives':s.finished?'9 completed tasks · 1 accepted alert':'Telemetry informs the next task',650,83,9,'#a6b2ca');
    if(s.time<8){
      pill(s.time<3?'01  VALIDATE BPMN':s.time<5?'02  GENERATE GO WORKFLOW':'03  GENERATE ENT SCHEMA',390,355);
    }
  }
  function evidence(s){
    if(s.chapter===0)return '<span class="evidence-label">COMPILER OUTPUT</span><div class="compile-flow"><span>BPMN</span><b>→</b><span>Go + Ent</span></div>';
    if(s.chapter===1)return `<span class="evidence-label">COMPLETED MOVEMENT DURATIONS</span><div class="compile-flow"><span>${s.completed.map(t=>t.duration+' s').join(' · ')||'Awaiting first movement'}</span><b>OTLP →</b></div>`;
    if(s.chapter===2)return `<span class="evidence-label">${s.alert?'ROLLING MEAN 17 s > THRESHOLD 15 s':'A01 · FOURTH MOVEMENT IN PROGRESS'}</span><div class="sample-bars"><div style="height:33%">10 s</div><div style="height:37%">11 s</div><div class="slow" style="height:100%">${s.alert?'30 s':'…'}</div></div>`;
    if(s.chapter===3)return `<span class="evidence-label">${s.accepted?'RUN-SPECIFIC SIGNAL ACCEPTED':'ALERT COMMITTED · DISPATCHING'}</span><div class="compile-flow"><span>A01</span><b>→ approved alternative →</b><span>A02</span></div>`;
    return `<span class="evidence-label">${s.finished?'COMPLETED ROUTE':'EXPRESS BRANCH SELECTED'}</span><div class="compile-flow"><span>4 × A01</span><b>→</b><span>4 × A02</span><b>→</b><span>EXPRESS</span></div>`;
  }
  function render(){
    const s=stateAt(time);draw(s);
    $('timeline').value=String(time);$('timeline').setAttribute('aria-valuetext',`${Math.floor(time)} of 54 seconds`);
    $('time').textContent=`00:${String(Math.floor(time)).padStart(2,'0')} / 00:54`;
    $('play').innerHTML=playing?'Pause <span aria-hidden="true">Ⅱ</span>':time>=duration?'Replay <span aria-hidden="true">↺</span>':'Play animation <span aria-hidden="true">▶</span>';
    $('completed').innerHTML=`${s.completed.length} <small>/ 9</small>`;
    $('aisle').textContent=s.aisle;$('alerts').textContent=s.accepted?'1':'0';
    $('scene-status').textContent=s.finished?'Wave complete':s.time>=46?'Express packing':s.time>=33?'Alternative route active':s.accepted?'Reroute accepted':s.persisted?'Alert persisted in outbox':s.alert?'Congestion detected':s.time>=8?'Picking in progress':'Process ready';
    const textKey=`${s.chapter}:${s.completed.length}:${s.accepted}:${s.finished}`;
    if(textKey!==lastText){
      const c=copy[s.chapter];$('detail-kicker').textContent=c[0];$('detail-title').textContent=c[1];$('detail-copy').textContent=c[2];
      $('chapter-count').textContent=`0${s.chapter+1} / 05`;$('evidence').innerHTML=evidence(s);
      buttons.forEach((button,i)=>{if(i===s.chapter)button.setAttribute('aria-current','step');else button.removeAttribute('aria-current');button.classList.toggle('done',i<s.chapter);});
      lastText=textKey;
    }
  }
  function tick(now){
    frame=null;if(!playing)return;
    if(last!==null)time=Math.min(duration,time+Math.min((now-last)/1000,.15)*speed);
    last=now;if(time>=duration)playing=false;render();if(playing)frame=requestAnimationFrame(tick);
  }
  function setPlaying(value){
    playing=value;last=null;if(frame!==null)cancelAnimationFrame(frame);frame=null;
    if(playing)frame=requestAnimationFrame(tick);render();
  }
  $('play').addEventListener('click',()=>{if(time>=duration)time=0;setPlaying(!playing);});
  $('restart').addEventListener('click',()=>{time=0;last=null;render();});
  $('timeline').addEventListener('input',e=>{time=Number(e.target.value);last=null;render();});
  $('speed').addEventListener('change',e=>{speed=Number(e.target.value);last=null;});
  buttons.forEach((button,i)=>button.addEventListener('click',()=>{time=chapters[i];last=null;render();}));
  document.addEventListener('visibilitychange',()=>{if(document.hidden)setPlaying(false);});
  reducedMotion.addEventListener('change',()=>{if(reducedMotion.matches)setPlaying(false);});
  new ResizeObserver(()=>render()).observe(canvas);
  setPlaying(!reducedMotion.matches);
})();

