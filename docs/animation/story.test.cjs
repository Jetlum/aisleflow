const {test}=require('node:test');
const assert=require('node:assert/strict');
const {stateAt,tasks,chapters}=require('./story.js');

test('the controlled story reproduces the verified closed-loop route',()=>{
  assert.deepEqual(stateAt(54).completed.map(t=>t.aisle),['A01','A01','A01','A01','A02','A02','A02','A02','EXPRESS']);
  assert.equal(stateAt(54).completed.length,9);
  assert.equal(stateAt(54).accepted,true);
  assert.equal(stateAt(54).finished,true);
});
test('the current task finishes before persistence, signal acceptance and rerouting',()=>{
  assert.equal(stateAt(25.9).alert,false);
  assert.equal(stateAt(26).completed.at(-1).id,'pick_04');
  assert.equal(stateAt(26).mean,(10+11+30)/3);
  assert.equal(stateAt(26).persisted,false);
  assert.equal(stateAt(28).persisted,true);
  assert.equal(stateAt(30.9).accepted,false);
  assert.equal(stateAt(31).accepted,true);
  assert.equal(stateAt(31).aisle,'A01');
  assert.equal(stateAt(33).aisle,'A02');
  assert.equal(stateAt(35.9).completed.length,4);
  assert.equal(stateAt(36).completed.at(-1).id,'pick_05');
});
test('scrubbing backwards restores the state without retained alerts or completed tasks',()=>{
  stateAt(54);
  assert.equal(stateAt(0).completed.length,0);
  assert.equal(stateAt(0).accepted,false);
  assert.equal(stateAt(0).aisle,'A01');
  assert.equal(stateAt(20).mean,10);
  chapters.forEach((t,i)=>assert.equal(stateAt(t).chapter,i));
  assert.equal(stateAt(-99).time,0);
  assert.equal(stateAt(999).time,54);
  assert.equal(tasks.length,9);
});
