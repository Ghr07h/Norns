package app

import (
	"os/exec"
	"strings"
	"testing"
)

func TestVisualKnowledgePointerEventsKeepClickTarget(t *testing.T) {
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
const addNode=html.slice(html.indexOf('function addNode('),html.indexOf('function nodePortX('));
const themes=html.slice(html.indexOf('function cognitionVariant('),html.indexOf('function cognitionLabel('));
const checks=new Function('assert',themes+addNode+String.raw` + "`" + `
let focused=null,clickTarget=null,detachments=0,selectedNodeId=null;
class Element{
 constructor(tag,attributes={}){this.tag=tag;this.attributes=attributes;this.dataset={};this.children=[];this.handlers=new Map();this.classList={add(){},toggle(){}}}
 setAttribute(name,value){this.attributes[name]=value}
 append(...children){for(const child of children){if(child.parentNode){detachments++;focused=null;clickTarget=null;child.parentNode.children=child.parentNode.children.filter(item=>item!==child)}child.parentNode=this;this.children.push(child)}}
 cloneNode(){return new Element(this.tag,{...this.attributes})}
 addEventListener(name,handler){this.handlers.set(name,handler)}
 dispatch(name,event={}){this.handlers.get(name)?.(event)}
}
const graphView='knowledge',nodeElements=new Map(),nodePortElements=new Map(),previews=[],activations=[];
const knowledgeState={branchOf:new Map(),depth:new Map(),parent:new Map(),root:'initial'};
const svgElement=(tag,attributes)=>new Element(tag,attributes),document={createElementNS:(_,tag)=>new Element(tag)},t=key=>key;
const cognitionLabel=()=> 'Task-Init Cognition',knowledgeNodeShape=()=>svgElement('circle'),createGraphIcon=()=>svgElement('svg');
function appendKnowledgeLabel(){}
function previewKnowledgeNode(id){previews.push(id)}
function focusNode(id){selectedNodeId=id;activations.push(id)}
const layer=new Element('g'),node={id:'initial',kind:'cognition',origin:'task',title:'Subject information'},position={x:0,y:0,width:184,height:184,root:true};
addNode(node,position,layer);
const item=nodeElements.get(node.id);assert.equal(layer.children[0],item);
assert.ok(item.children.some(child=>child.attributes.style==='stroke:url(#cognition-bronze)'));
assert.equal(item.handlers.has('pointerenter'),false,'delegated hover must not react to label relayout');
assert.equal(item.handlers.has('pointerleave'),false,'delegated hover must not react to label relayout');
item.dispatch('pointerenter');
focused=item;clickTarget=item;item.dispatch('focus');item.dispatch('pointerenter');
assert.equal(detachments,0,'hover must not detach the SVG node and cancel the native click sequence');
assert.equal(focused,item);assert.equal(clickTarget,item);
let stopped=0,prevented=0;
if(clickTarget===item)item.dispatch('click',{stopPropagation(){stopped++}});
assert.equal(selectedNodeId,node.id);assert.equal(stopped,1);assert.deepEqual(activations,[node.id]);
for(const key of ['Enter',' '])item.dispatch('keydown',{key,preventDefault(){prevented++}});
assert.equal(prevented,2);assert.deepEqual(activations,[node.id,node.id,node.id]);
item.dispatch('pointerleave');item.dispatch('blur');assert.deepEqual(previews,[node.id,null]);
for(const [id,properties,metal] of [
 ['task',{origin:'task'},'cognition-bronze'],
 ['result',{task_purpose_relevant:true},'cognition-gold'],
 ['regular',{},'cognition-silver']
]){
 const cognition={id,kind:'cognition',title:id,...properties};
 addNode(cognition,{...position,root:false},layer);
 assert.ok(nodeElements.get(id).children.some(child=>child.attributes.style==='stroke:url(#'+metal+')'),'non-root cognition must retain its metallic gradient');
}
` + "`" + `);
checks(assert);
`
	command := exec.Command(node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual knowledge graph pointer interaction failed: %v\n%s", err, output)
	}
}

func TestVisualKnowledgeHoverRetainsOriginalLabelHitRegion(t *testing.T) {
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
const hover=html.slice(html.indexOf('function branchClearHover('),html.indexOf('function branchLabelCandidates('));
const checks=new Function('assert',hover+String.raw` + "`" + `
let graphView='knowledge',labelRect={x:120,y:120,width:140,height:28};
const knowledgeState={hoveredId:null,hoverAnchor:null},previews=[];
const document={activeElement:null},el=()=>({getBoundingClientRect:()=>({left:20,top:20})});
const label={getAttribute:()=> 'visible',querySelector:()=>({getBoundingClientRect:()=>labelRect})};
const item={dataset:{id:'root'},querySelector:selector=>selector==='.knowledge-label'?label:{getBoundingClientRect:()=>({x:70,y:70,width:30,height:30})}};
const nodeElements=new Map([['root',item]]),background={closest:()=>null},target={closest:()=>item};
function previewKnowledgeNode(id){previews.push(id)}
const move=(clientX,clientY,eventTarget=background,extra={})=>branchPointerMove({clientX,clientY,target:eventTarget,buttons:0,pointerType:'mouse',...extra});
move(140,130,target);
assert.equal(knowledgeState.hoveredId,'root');assert.deepEqual(previews,['root']);
labelRect={x:300,y:300,width:220,height:90};
for(let index=0;index<30;index++)move(140+index%2,130);
assert.deepEqual(previews,['root'],'expanded title relayout and tiny pointer movement must not toggle hover');
move(80,80);assert.deepEqual(previews,['root'],'node shape must retain hover');
move(320,330);assert.deepEqual(previews,['root'],'expanded title must retain hover');
move(600,600);move(601,600);assert.deepEqual(previews,['root',null]);
assert.equal(knowledgeState.hoverAnchor,null);
move(320,330,target);move(320,330,target,{buttons:1});
assert.equal(knowledgeState.hoveredId,null,'drag must clear hover');
move(320,330,target);move(320,330,target,{pointerType:'touch'});
assert.equal(knowledgeState.hoveredId,null,'touch must clear hover');
document.activeElement={closest:()=>item};
move(320,330,target);branchClearHover();assert.equal(previews.at(-1),'root','keyboard focus must survive hover exit');
const count=previews.length;graphView='plane';move(320,330,target);assert.equal(previews.length,count);
` + "`" + `);
checks(assert);
`
	command := exec.Command(node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual delegated hover failed: %v\n%s", err, output)
	}
}
