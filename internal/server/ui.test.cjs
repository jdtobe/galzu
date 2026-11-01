const {test} = require('node:test');
const assert = require('node:assert/strict');
const ui = require('./web/ui.js');

test('public and direct filters use routing, including private help requests', () => {
  const messages = [{id:1,kind:'message'}, {id:2,kind:'help'}, {id:3,recipient_id:'a'}, {id:4,kind:'help',recipient_id:'b'}];
  assert.deepEqual(messages.filter(m => ui.visibleMessage(m,'all')).map(m=>m.id), [1,2,3,4]);
  assert.deepEqual(messages.filter(m => ui.visibleMessage(m,'public')).map(m=>m.id), [1,2]);
  assert.deepEqual(messages.filter(m => ui.visibleMessage(m,'direct')).map(m=>m.id), [3,4]);
});
test('rest, queue, inference, retry and failure remain distinct', () => {
  const agent = {status:'ready'};
  assert.equal(ui.agentState(agent,{last_outcome:'rested'}).label,'Resting');
  assert.equal(ui.agentState(agent,{state:'queued',last_outcome:'rested'}).label,'Queued');
  assert.equal(ui.agentState(agent,{state:'inference'},true).label,'In inference');
  assert.equal(ui.agentState(agent,{state:'queued'},true).label,'Paused · queued');
  assert.equal(ui.agentState(agent,{state:'retrying'}).label,'Retrying');
  assert.equal(ui.agentState({status:'error'},{state:'inference'}).label,'Needs attention');
});
test('delivery preserves unknown migration boundary and completed checkpoint semantics', () => {
  const ready={status:'ready'}, failed={status:'error'}, message={id:5};
  assert.equal(ui.deliveryState(message,ready), 'unknown');
  assert.equal(ui.deliveryState(message,ready,{baseline:5,checkpoint:10}), 'unknown');
  assert.equal(ui.deliveryState(message,failed,{baseline:0,checkpoint:5}), 'processed');
  assert.equal(ui.deliveryState(message,failed,{baseline:0,checkpoint:4}), 'needs attention');
  assert.equal(ui.deliveryState(message,ready,{baseline:0,checkpoint:null}), 'pending');
});
test('duration labels do not conflate bytes, tokens, or elapsed time', () => {
  assert.equal(ui.duration(12400),'12.4 s');
  assert.equal(ui.duration(23),'23 ms');
  assert.equal(ui.duration(undefined),'0 ms');
});
