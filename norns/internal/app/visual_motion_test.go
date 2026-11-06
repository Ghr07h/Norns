package app

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestVisualSoftOrbsRespectGraphStateAndCamera(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required to exercise generated Visual JavaScript")
	}
	html, err := visualHTML([]byte(`{}`), "en")
	if err != nil {
		t.Fatal(err)
	}
	const script = `
const assert=require('node:assert/strict'),html=require('node:fs').readFileSync(0,'utf8');
const source=html.slice(html.indexOf('function initKnowledgeMotion('),html.indexOf('function initKnowledgeGraph('));
const themes=html.slice(html.indexOf('function cognitionVariant('),html.indexOf('function cognitionLabel('));
const check=new Function('assert',themes+source+String.raw` + "`" + `
class Element {
 constructor(tag,attributes={}){
  this.tag=tag;this.attributes={...attributes};this.children=[];this.parentNode=null;
  this.classList={values:new Set((attributes.class||'').split(' ').filter(Boolean)),contains(name){return this.values.has(name)},add(name){this.values.add(name)},remove(...names){for(const name of names)this.values.delete(name)}};
 }
 get isConnected(){return this.attached||Boolean(this.parentNode?.isConnected)}
 setAttribute(name,value){this.attributes[name]=String(value)}
 getAttribute(name){return this.attributes[name]}
 append(...children){for(const child of children){child.remove();child.parentNode=this;this.children.push(child)}}
 after(child){const parent=this.parentNode;child.remove();child.parentNode=parent;parent.children.splice(parent.children.indexOf(this)+1,0,child)}
 remove(){if(this.parentNode){const parent=this.parentNode;parent.children=parent.children.filter(child=>child!==this);this.parentNode=null}}
 replaceChildren(...children){for(const child of [...this.children])child.remove();this.append(...children)}
 querySelector(selector){return this.querySelectorAll(selector)[0]||null}
 querySelectorAll(selector){const matches=[];for(const child of this.children){if(selector.startsWith('.')?child.classList.contains(selector.slice(1)):selector.startsWith('#')?child.attributes.id===selector.slice(1):child.tag===selector)matches.push(child);matches.push(...child.querySelectorAll(selector))}return matches}
}
const svgElement=(tag,attributes)=>new Element(tag,attributes),svg=svgElement('svg');svg.attached=true;
const viewport={clientWidth:800,clientHeight:500},shell={getBoundingClientRect:()=>({top:0,bottom:500})},stage={};
const el=id=>id==='edges'?svg:id==='viewport'?viewport:id==='graph-shell'?shell:stage;
let graphView='plane',scale=1,panX=0,panY=0;
const first={from:'root',to:'result',kind:'source'},cross={from:'root',to:'other',kind:'output'};
const byId=new Map([['result',{kind:'exploration',status:'finished'}],['other',{kind:'cognition',task_purpose_relevant:true}]]);
let edgeElements=[{edge:first,path:svgElement('path',{class:'edge source backbone'})},{edge:cross,path:svgElement('path',{class:'edge output cross-edge'})}];
const knowledgeState={allRelations:false,routes:[{edge:first,samples:[{x:100,y:100},{x:300,y:100}]},{edge:cross,samples:[{x:100,y:200},{x:300,y:200}]}]};
const definitions=svgElement('defs'),edgeLayer=svgElement('g',{class:'edge-layer'});svg.append(definitions,edgeLayer);
const frames=new Map(),observers=[],intersections=[],listeners=new Map();let frameID=0;
const requestAnimationFrame=callback=>{frames.set(++frameID,callback);return frameID},cancelAnimationFrame=id=>frames.delete(id);
const step=timestamp=>{const callbacks=[...frames.values()];frames.clear();for(const callback of callbacks)callback(timestamp)};
class MutationObserver{constructor(callback){this.callback=callback;observers.push(this)}observe(){}}
class IntersectionObserver{constructor(callback){this.callback=callback;intersections.push(this)}observe(){}}
const preference={matches:false,addEventListener(name,callback){this.change=callback}};
const document={hidden:false,addEventListener:(name,callback)=>listeners.set(name,callback)};
const window={innerHeight:500,matchMedia:()=>preference,addEventListener(){}};
initKnowledgeMotion();
const refresh=observers[0].callback;
assert.equal(frames.size,0,'planar view must not schedule motion');
assert.equal(svg.querySelectorAll('.knowledge-particles').length,0);
graphView='knowledge';refresh();
assert.equal(frames.size,1);assert.equal(definitions.children.length,6);
assert.equal(svg.querySelectorAll('.knowledge-particle').length,1,'cross links must stay quiet by default');
const bead=svg.querySelector('.knowledge-particle'),core=bead.querySelector('.knowledge-particle-core'),glow=bead.querySelector('.knowledge-particle-glow');
const diameter=()=>Number(core.getAttribute('r'))*scale*2;
const point=()=>bead.getAttribute('transform').match(/-?\d+(?:\.\d+)?/g).map(Number);
assert.equal(diameter(),3.2);assert.equal(Number(glow.getAttribute('r'))*scale*2,9.2);
assert.equal(core.getAttribute('fill'),'#17684e');
assert.equal(glow.getAttribute('fill'),'url(#knowledge-motion-glow-finished)');
step(100);const start=point();step(150);
assert.ok(Math.abs((point()[0]-start[0])-1.3)<.002,'motion must travel 26 screen pixels per second');
const beforeZoom=point();scale=.5;refresh();
assert.deepEqual(point(),beforeZoom,'zoom must not reset particle phase');assert.equal(diameter(),3.2);
const scaledStart=point();step(200);
assert.ok(Math.abs((point()[0]-scaledStart[0])*scale-1.3)<.002,'screen speed must remain constant after zoom');
edgeElements[0].path.classList.add('preview-related');refresh();
assert.equal(diameter(),3.9);assert.equal(Number(glow.getAttribute('r'))*scale*2,11);
assert.equal(bead.getAttribute('opacity'),'0.98');
edgeElements[0].path.classList.remove('preview-related');
for(const hiddenClass of ['unrelated','preview-unrelated','branch-inactive']){
 edgeElements[0].path.classList.add(hiddenClass);refresh();
 assert.equal(bead.getAttribute('visibility'),'hidden');assert.equal(frames.size,0);
 edgeElements[0].path.classList.remove(hiddenClass);refresh();assert.equal(frames.size,1);
}
knowledgeState.allRelations=true;refresh();
assert.equal(svg.querySelectorAll('.knowledge-particle').length,2);
const crossBead=svg.querySelectorAll('.knowledge-particle')[1];
assert.equal(crossBead.getAttribute('visibility'),'visible');
assert.equal(crossBead.querySelector('.knowledge-particle-core').getAttribute('fill'),'#9a7414');
assert.equal(crossBead.querySelector('.knowledge-particle-glow').getAttribute('fill'),'url(#knowledge-motion-glow-result-cognition)');
knowledgeState.allRelations=false;refresh();assert.equal(crossBead.getAttribute('visibility'),'hidden');
edgeElements[1].path.classList.add('related');refresh();assert.equal(crossBead.getAttribute('visibility'),'visible');
edgeElements[1].path.classList.remove('related');refresh();
document.hidden=true;listeners.get('visibilitychange')();assert.equal(frames.size,0);
const suspended=point();step(5000);assert.deepEqual(point(),suspended);
scale=.25;refresh();assert.equal(diameter(),3.2);assert.deepEqual(point(),suspended,'camera updates while paused must not advance motion');
document.hidden=false;listeners.get('visibilitychange')();step(10000);
assert.deepEqual(point(),suspended,'resuming from background must not jump forward');step(10050);assert.notDeepEqual(point(),suspended);
intersections[0].callback([{isIntersecting:false}]);assert.equal(frames.size,0);
intersections[0].callback([{isIntersecting:true}]);assert.equal(frames.size,1);
preference.matches=true;preference.change();assert.equal(frames.size,0);
const reduced=point();step(20000);assert.deepEqual(point(),reduced);
preference.matches=false;preference.change();assert.equal(frames.size,1);
panX=-1000;refresh();assert.equal(frames.size,0,'off-canvas relations must not keep an animation loop alive');
panX=0;refresh();assert.equal(frames.size,1);
for(let index=0;index<10;index++)refresh();
assert.equal(frames.size,1);assert.equal(svg.querySelectorAll('.knowledge-particles').length,1);assert.equal(definitions.children.length,6);
graphView='plane';refresh();assert.equal(frames.size,0);assert.equal(svg.querySelectorAll('.knowledge-particles').length,0);
graphView='knowledge';
for(const [theme,node,color] of [
 ['task-cognition',{kind:'cognition',origin:'task'},'#955d30'],
 ['result-cognition',{kind:'cognition',task_purpose_relevant:true},'#9a7414'],
 ['regular',{kind:'cognition'},'#7b858c'],
 ['todo',{kind:'exploration',status:'todo'},'#d4882f'],
 ['running',{kind:'exploration',status:'running'},'#477dbd'],
 ['finished',{kind:'exploration',status:'finished'},'#17684e']
]){
 byId.set(first.to,node);edgeElements=[{edge:first,path:svgElement('path',{class:'edge source backbone'})}];
 svg.replaceChildren(svgElement('defs'),svgElement('g',{class:'edge-layer'}));refresh();
 const particle=svg.querySelector('.knowledge-particle');
 assert.equal(particle.querySelector('.knowledge-particle-core').getAttribute('fill'),color);
 assert.equal(particle.querySelector('.knowledge-particle-glow').getAttribute('fill'),'url(#knowledge-motion-glow-'+theme+')');
 assert.equal(svg.querySelector('#knowledge-motion-glow-'+theme).children[0].getAttribute('stop-color'),color);
}
graphView='knowledge';knowledgeState.routes=[];edgeElements=[];
svg.replaceChildren(svgElement('defs'),svgElement('g',{class:'edge-layer'}));refresh();
assert.equal(frames.size,0,'empty graphs must not schedule motion');assert.equal(svg.querySelectorAll('.knowledge-particle').length,0);
console.log('Soft orb state, geometry, speed, visibility and lifecycle checks passed');
` + "`" + `);
check(assert);
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual soft orb checks failed: %v\n%s", err, output)
	}
}
