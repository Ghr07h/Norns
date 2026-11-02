package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVisualKnowledgeGraphGeometry(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required to exercise generated Visual JavaScript")
	}
	html, err := visualHTML([]byte(`{}`), "en")
	if err != nil {
		t.Fatal(err)
	}
	const script = `
const assert=require('node:assert/strict'),vm=require('node:vm'),html=require('node:fs').readFileSync(0,'utf8');
const script=html.slice(html.indexOf('<script>')+8,html.lastIndexOf('</script>'));
new vm.Script(script);
const algorithms=script.slice(script.indexOf('function knowledgeNodeSize'),script.indexOf('function updateGraphViewControl'));
const checks=new Function('assert', 'let stageWidth=0,stageHeight=0;const graphArrowGap=8,byId=new Map();function graphNodeCompare(first,second){return String(first.created_at).localeCompare(String(second.created_at))||first.id.localeCompare(second.id)}function graphPathNumber(value){return Math.round(value*10)/10}\n'+algorithms+'\n'+String.raw` + "`" + `
const makeNode=(id,kind='cognition',origin='exploration')=>({id,kind,origin,created_at:id,title:id});
const center=position=>({x:position.x+position.width/2,y:position.y+position.height/2});
const distance=(first,second)=>Math.hypot(first.x-second.x,first.y-second.y);
assert.equal(knowledgeGraphLayout([],[]).positions.size,0);
assert.equal(stageWidth,880);
const root={...makeNode('root','cognition','task'),created_at:'0000'};
const nodes=[root,makeNode('explore','exploration'),makeNode('result'),makeNode('other','exploration'),makeNode('island'),makeNode('cycle')];
for(const node of nodes)byId.set(node.id,node);
const edges=[{from:'root',to:'explore',kind:'source'},{from:'explore',to:'result',kind:'output'},{from:'root',to:'other',kind:'source'},{from:'other',to:'result',kind:'output'},{from:'result',to:'cycle',kind:'extends'},{from:'cycle',to:'result',kind:'extends'},{from:'missing',to:'root',kind:'source'}];
const layout=knowledgeGraphLayout(nodes,edges),rootPosition=layout.positions.get(root.id);
assert.equal(layout.positions.size,nodes.length);
assert.deepEqual(center(rootPosition),{x:stageWidth/2,y:stageHeight/2});
assert.equal(rootPosition.root,true);
assert.ok(rootPosition.width>layout.positions.get('result').width);
for(const [id,position] of layout.positions){assert.ok(Number.isFinite(position.x)&&Number.isFinite(position.y));if(id!==root.id)assert.ok(distance(center(rootPosition),center(position))>200)}
const entries=[...layout.positions.values()];for(let first=0;first<entries.length;first++)for(let second=first+1;second<entries.length;second++)assert.ok(distance(center(entries[first]),center(entries[second]))>(entries[first].width+entries[second].width)/2+20,'nodes overlap');
assert.deepEqual(knowledgeGraphLayout([...nodes].reverse(),[...edges].reverse()).positions,layout.positions);
const circle={id:'root',x:0,y:0,width:184,height:184};
assert.deepEqual(knowledgeNodeBoundary(circle,{x:1000,y:92}),{x:179,y:92});
assert.deepEqual(knowledgeNodeBoundary(circle,{x:92,y:-1000}),{x:92,y:5});
const hexagon={id:'explore',x:0,y:0,width:144,height:144};
assert.equal(knowledgeHexagon(hexagon,5).length,6);
assert.ok(Math.abs(knowledgeNodeBoundary(hexagon,{x:72,y:-1000}).y-(72-67*Math.sin(Math.PI/3)))<.001);
const routes=knowledgeGraphRoutes([...edges,{from:'root',to:'explore',kind:'retry'},{from:'root',to:'root',kind:'extends'}],layout.positions);
assert.equal(routes.length,edges.length+1);
for(const route of routes)assert.ok(!/NaN|Infinity/.test(route.path),route.path);
assert.equal(new Set(routes.map(route=>route.path)).size,routes.length);
for(const route of routes)for(const [id,position]of layout.positions){if(id===route.edge.from||id===route.edge.to)continue;for(let index=1;index<route.samples.length;index++)assert.ok(knowledgeSegmentDistance(center(position),route.samples[index-1],route.samples[index])>position.width/2-5,'route intersects a node')}
assert.ok(routes.find(route=>route.edge.from===route.edge.to).samples.length>3,'self-loop must have visible geometry');
const obstacles=new Map([['root',{x:0,y:0,width:132,height:132}],['result',{x:600,y:0,width:132,height:132}],['other',{x:300,y:0,width:144,height:144}]]);
const avoided=knowledgeGraphRoutes([{from:'root',to:'result',kind:'source'}],obstacles)[0];
const obstacle=center(obstacles.get('other'));
assert.ok(avoided.samples.every(point=>distance(point,obstacle)>86),'route crosses an unrelated node');
const emptyGrid=knowledgeRoutingGrid(new Map()),smoothEdge={from:'smooth-from',to:'smooth-to',kind:'source'};
const wideTurn=knowledgeRoundedPath([{x:100,y:500},{x:500,y:500},{x:500,y:100}],emptyGrid,smoothEdge);
assert.ok(wideTurn.samples.every(point=>distance(point,{x:500,y:500})>45),'long bends must not retain tiny fixed-radius elbows');
assert.deepEqual(wideTurn.samples[0],{x:100,y:500});assert.deepEqual(wideTurn.samples.at(-1),{x:500,y:100});
const shortBend=[{x:100,y:200},{x:300,y:200},{x:318,y:182},{x:600,y:400}];
assert.equal((knowledgeRoundedPath(shortBend,emptyGrid,smoothEdge).path.match(/ Q /g)||[]).length,1,'short grid zigzags should merge');
assert.equal(shortBend.length,4,'smoothing must not mutate the routed points');
assert.equal((knowledgeRoundedPath(shortBend.map((point,index)=>({...point,anchor:index===1})),emptyGrid,smoothEdge).path.match(/ Q /g)||[]).length,2,'parallel-edge lane anchors must survive smoothing');
const closeObstacle={x:420,y:420,width:40,height:40},closeGrid=knowledgeRoutingGrid(new Map([['smooth-obstacle',closeObstacle]]));
const tightTurn=knowledgeRoundedPath([{x:100,y:500},{x:500,y:500},{x:500,y:100}],closeGrid,smoothEdge);
assert.match(tightTurn.path,/ Q /,'obstacle avoidance must retain a smooth turn');
assert.notEqual(tightTurn.path,wideTurn.path,'rounding must adapt to nearby obstacles');
for(let index=1;index<tightTurn.samples.length;index++)assert.ok(knowledgeSegmentDistance(center(closeObstacle),tightTurn.samples[index-1],tightTurn.samples[index])>=closeObstacle.width/2+15,'smoothed curve must preserve obstacle clearance');
const alternate={...makeNode('later-task','cognition','task'),created_at:'9999'};
assert.equal([...knowledgeGraphLayout([...nodes,alternate],edges).positions].find(([,position])=>position.root)[0],root.id);
assert.equal(knowledgeGraphLayout([makeNode('fallback','exploration')],[]).positions.get('fallback').root,true);
stageWidth=1280;stageHeight=1000;
const wall=new Map([['left',{x:40,y:400,width:132,height:132}],['right',{x:1020,y:400,width:132,height:132}],...Array.from({length:4},(_,index)=>['wall-'+index,{x:500,y:140+index*170,width:144,height:144}])]);
for(const id of wall.keys())byId.set(id,makeNode(id));
const detours=knowledgeGraphRoutes([{from:'left',to:'right',kind:'source'},{from:'left',to:'right',kind:'retry'},{from:'right',to:'left',kind:'extends'}],wall);
assert.equal(new Set(detours.map(route=>route.path)).size,3);
for(const route of detours)for(const [id,position]of wall){if(id===route.edge.from||id===route.edge.to)continue;for(let index=1;index<route.samples.length;index++)assert.ok(knowledgeSegmentDistance(center(position),route.samples[index-1],route.samples[index])>position.width/2-5,'detour intersects obstacle wall')}
for(const count of [1,12,80]){const fan=[root,...Array.from({length:count},(_,index)=>makeNode('fan-'+index,'exploration'))],links=fan.slice(1).map(node=>({from:root.id,to:node.id,kind:'source'}));const result=knowledgeGraphLayout(fan,links);assert.deepEqual(center(result.positions.get(root.id)),{x:stageWidth/2,y:stageHeight/2});const values=[...result.positions.values()];for(let first=0;first<values.length;first++)for(let second=first+1;second<values.length;second++)assert.ok(distance(center(values[first]),center(values[second]))>(values[first].width+values[second].width)/2+20,'fan overlaps')}
console.log('Knowledge graph geometry checks passed');
` + "`" + `);
checks(assert);
`
	command := exec.Command(node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual knowledge graph geometry failed: %v\n%s", err, output)
	}
}
