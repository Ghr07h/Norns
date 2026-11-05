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
const checks=new Function('assert',addNode+String.raw` + "`" + `
let focused=null,clickTarget=null,detachments=0,selectedNodeId=null;
class Element{
 constructor(tag,attributes={}){this.tag=tag;this.attributes=attributes;this.dataset={};this.children=[];this.handlers=new Map();this.classList={add(){}}}
 setAttribute(name,value){this.attributes[name]=value}
 append(...children){for(const child of children){if(child.parentNode){detachments++;focused=null;clickTarget=null;child.parentNode.children=child.parentNode.children.filter(item=>item!==child)}child.parentNode=this;this.children.push(child)}}
 cloneNode(){return new Element(this.tag,{...this.attributes})}
 addEventListener(name,handler){this.handlers.set(name,handler)}
 dispatch(name,event={}){this.handlers.get(name)?.(event)}
}
const graphView='knowledge',nodeElements=new Map(),nodePortElements=new Map(),previews=[],activations=[];
const svgElement=(tag,attributes)=>new Element(tag,attributes),document={createElementNS:(_,tag)=>new Element(tag)},t=key=>key;
const cognitionVariant=()=> 'task-cognition',cognitionLabel=()=> 'Task-Init Cognition',knowledgeNodeShape=()=>svgElement('circle'),createGraphIcon=()=>svgElement('svg');
function appendKnowledgeLabel(){}
function previewKnowledgeNode(id){previews.push(id)}
function focusNode(id){selectedNodeId=id;activations.push(id)}
const layer=new Element('g'),node={id:'initial',kind:'cognition',origin:'task',title:'Subject information'},position={x:0,y:0,width:184,height:184,root:true};
addNode(node,position,layer);
const item=nodeElements.get(node.id);assert.equal(layer.children[0],item);
item.dispatch('pointerenter');
focused=item;clickTarget=item;item.dispatch('focus');item.dispatch('pointerenter');
assert.equal(detachments,0,'hover must not detach the SVG node and cancel the native click sequence');
assert.equal(focused,item);assert.equal(clickTarget,item);
let stopped=0,prevented=0;
if(clickTarget===item)item.dispatch('click',{stopPropagation(){stopped++}});
assert.equal(selectedNodeId,node.id);assert.equal(stopped,1);assert.deepEqual(activations,[node.id]);
for(const key of ['Enter',' '])item.dispatch('keydown',{key,preventDefault(){prevented++}});
assert.equal(prevented,2);assert.deepEqual(activations,[node.id,node.id,node.id]);
item.dispatch('pointerleave');item.dispatch('blur');assert.deepEqual(previews.slice(-2),[null,null]);
` + "`" + `);
checks(assert);
`
	command := exec.Command(node, "-e", script)
	command.Stdin = strings.NewReader(string(html))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Visual knowledge graph pointer interaction failed: %v\n%s", err, output)
	}
}
