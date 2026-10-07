/* Timings are presentation seconds, not measured warehouse durations. */
(function (root) {
  'use strict';
  const chapters = [0, 8, 22, 30, 46];
  const tasks = [
    {id:'pick_01',at:12,aisle:'A01',duration:9},
    {id:'pick_02',at:16,aisle:'A01',duration:10},
    {id:'pick_03',at:20,aisle:'A01',duration:11},
    {id:'pick_04',at:26,aisle:'A01',duration:30},
    {id:'pick_05',at:36,aisle:'A02',duration:10},
    {id:'pick_06',at:39,aisle:'A02',duration:10},
    {id:'pick_07',at:42,aisle:'A02',duration:10},
    {id:'pick_08',at:45,aisle:'A02',duration:10},
    {id:'pack_express',at:50,aisle:'EXPRESS',duration:10}
  ];
  function stateAt(value) {
    const time = Math.max(0,Math.min(54,Number(value)||0));
    const completed = tasks.filter(task=>task.at<=time);
    return {time,chapter:chapters.reduce((c,start,i)=>time>=start?i:c,0),completed,
      aisle:time>=46?'EXPRESS':time>=33?'A02':'A01',
      alert:time>=26,persisted:time>=28,accepted:time>=31,
      mean:time>=26?17:time>=20?10:null,finished:time>=50};
  }
  const api={chapters,tasks,stateAt,duration:54};
  if(typeof module!=='undefined' && module.exports) module.exports=api;
  else root.PyckStory=api;
})(globalThis);
