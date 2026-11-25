// Code generated from api/config/crd/bases. DO NOT EDIT.

import { formats } from "@kubernetes-models/validate";
export const validate = validate16;
const schema12 = {"type":"object","properties":{"apiVersion":{"$ref":"rgSv5V2hoWAdXQ_XErlwxUiy6phhz4ZEBCjtqeXtVec"},"kind":{"$ref":"JNQhjbvdoLOKq5EQgy-RNoHS1eKG3X7HqQDB3yaxhiM"},"metadata":{"$ref":"P3srTZpWadQH4s4N9KfJR0iCqydQo9YAz7xMB2qf0GQ"},"spec":{"$ref":"3RA_R3k5V-8yMChJYZdIsTMu_Spo4kmUAU7Lon8weZk"},"status":{"$ref":"zNm-daf2j-hB_Dni5dSzqLIwLX8cI-m5xUSROGEEQ3U"}},"required":["apiVersion","kind"],"$id":"rlinf.io.v1alpha1.Node"};
const schema13 = {"type":"string","enum":["rlinf.io/v1alpha1"]};

function validate17(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(typeof data !== "string"){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const vSchema0 = schema13.enum;
if(!(data === "rlinf.io/v1alpha1")){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema13.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate17.errors = vErrors;
return errors === 0;
}

const schema14 = {"type":"string","enum":["Node"]};

function validate19(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(typeof data !== "string"){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const vSchema0 = schema14.enum;
if(!(data === "Node")){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema14.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate19.errors = vErrors;
return errors === 0;
}

const schema15 = {"nullableRef":"io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta#"};
const schema16 = {};

import { validate as validate22 } from "@kubernetes-models/apimachinery/_schemas/IoK8sApimachineryPkgApisMetaV1ObjectMeta";


function validate21(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(data !== null){
if(!(validate22(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate22.errors : vErrors.concat(validate22.errors);
errors = vErrors.length;
}
else {
}
}
validate21.errors = vErrors;
return errors === 0;
}

const schema17 = {"properties":{"agentType":{"$ref":"l_DJxe5Cy2XbpJh4rH5_qZiSOSpZjC3fwneDYujliJg"},"identity":{"$ref":"FEAfVSsDW2NtuEGYrD4eB1ucuIcFpcg9jD3lT4FZwEI"},"operations":{"$ref":"chX1GLGOWX2pGMpLltnnNP-4bTs9vbMyhtWz7ApEhhA"},"runtime":{"$ref":"fsUmk8hbigW3EI-D0FACCFSMbxooozt1kHNp-gPlBEk"},"unschedulable":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"type":"object","nullable":true};
const schema1 = {"enum":["Kubernetes","Docker","Raw"],"type":"string","nullable":true};

function validate1(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((typeof data !== "string") && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
const vSchema0 = schema1.enum;
if(!(((data === "Kubernetes") || (data === "Docker")) || (data === "Raw"))){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema1.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate1.errors = vErrors;
return errors === 0;
}

const schema18 = {"properties":{"domain":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"id":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"owner":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"tenant":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};
const schema0 = {"type":"string","nullable":true};

function validate0(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((typeof data !== "string") && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
validate0.errors = vErrors;
return errors === 0;
}


function validate27(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.domain !== undefined){
let data0 = data.domain;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/domain",parentData:data,parentDataProperty:"domain",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.id !== undefined){
let data1 = data.id;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/id",parentData:data,parentDataProperty:"id",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.owner !== undefined){
let data2 = data.owner;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/owner",parentData:data,parentDataProperty:"owner",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.tenant !== undefined){
let data3 = data.tenant;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/tenant",parentData:data,parentDataProperty:"tenant",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate27.errors = vErrors;
return errors === 0;
}

const schema19 = {"items":{"$ref":"-3Vt-XhIseRGXALf-wqQgiaF7vut_mQ_p_JWj5HdQY0"},"type":"array","nullable":true};
const schema20 = {"properties":{"id":{"$ref":"2tG51vkrD8fXpcnHZtqV2HvjVdK98owFTjt5N5f3XEo"},"type":{"$ref":"K7YdC3JHfm45sDnv_N9huiHH2vYhgHiXcepbVDUkPMM"}},"required":["id","type"],"type":"object"};
const schema21 = {"minLength":1,"type":"string"};
import func0 from "@kubernetes-models/validate/runtime/ucs2length";

function validate35(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(typeof data === "string"){
if(func0(data) < 1){
const err0 = {instancePath,schemaPath:"#/minLength",keyword:"minLength",params:{limit: 1}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
}
else {
const err1 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate35.errors = vErrors;
return errors === 0;
}

const schema22 = {"enum":["Drain","EnterMaintenance"],"type":"string"};

function validate37(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(typeof data !== "string"){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const vSchema0 = schema22.enum;
if(!((data === "Drain") || (data === "EnterMaintenance"))){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema22.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate37.errors = vErrors;
return errors === 0;
}


function validate34(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.id === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "id"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.type === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "type"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.id !== undefined){
let data0 = data.id;
const _errs1 = errors;
if(!(validate35(data0, {instancePath:instancePath+"/id",parentData:data,parentDataProperty:"id",rootData}))){
vErrors = vErrors === null ? validate35.errors : vErrors.concat(validate35.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.type !== undefined){
let data1 = data.type;
const _errs2 = errors;
if(!(validate37(data1, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate37.errors : vErrors.concat(validate37.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
else {
const err2 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
validate34.errors = vErrors;
return errors === 0;
}


function validate33(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs2 = errors;
if(!(validate34(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate34.errors : vErrors.concat(validate34.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
if(!valid0){
valid1 = false;
}
}
}
validate33.errors = vErrors;
return errors === 0;
}

const schema23 = {"properties":{"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"type":{"$ref":"l_DJxe5Cy2XbpJh4rH5_qZiSOSpZjC3fwneDYujliJg"}},"type":"object","nullable":true};

function validate41(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name !== undefined){
let data0 = data.name;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.type !== undefined){
let data1 = data.type;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate41.errors = vErrors;
return errors === 0;
}

const schema4 = {"type":"boolean","nullable":true};

function validate4(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((typeof data !== "boolean") && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "boolean"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
validate4.errors = vErrors;
return errors === 0;
}


function validate25(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.agentType !== undefined){
let data0 = data.agentType;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/agentType",parentData:data,parentDataProperty:"agentType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.identity !== undefined){
let data1 = data.identity;
const _errs3 = errors;
if(!(validate27(data1, {instancePath:instancePath+"/identity",parentData:data,parentDataProperty:"identity",rootData}))){
vErrors = vErrors === null ? validate27.errors : vErrors.concat(validate27.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.operations !== undefined){
let data2 = data.operations;
const _errs4 = errors;
if(!(validate33(data2, {instancePath:instancePath+"/operations",parentData:data,parentDataProperty:"operations",rootData}))){
vErrors = vErrors === null ? validate33.errors : vErrors.concat(validate33.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.runtime !== undefined){
let data3 = data.runtime;
const _errs5 = errors;
if(!(validate41(data3, {instancePath:instancePath+"/runtime",parentData:data,parentDataProperty:"runtime",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.unschedulable !== undefined){
let data4 = data.unschedulable;
const _errs6 = errors;
if(!(validate4(data4, {instancePath:instancePath+"/unschedulable",parentData:data,parentDataProperty:"unschedulable",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate25.errors = vErrors;
return errors === 0;
}

const schema24 = {"properties":{"activeWorkloads":{"$ref":"mUiiaWnIPkdeoEyPf3FfjJ0Y-XZl60UPv2HaEjV53ik"},"addresses":{"$ref":"vdaQwCx5ZyA1TmTZlGbhVsIjxQHj2PssnXOl-Qvc8dE"},"allocatable":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"},"capabilitySnapshot":{"$ref":"NX-YFoeG9Uz7Dkql2-RxqsuCyHbT_SG5v3DO7g1IuTw"},"capacity":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"},"conditions":{"$ref":"hhAxL57cvApnLYfNWQbGuX0ymd4M4buVmG7H0q63I0w"},"diskPressure":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"drainOperations":{"$ref":"4QjPT_jkSooG-HwbZUG6BbIEPlUp4mru6PI9ZhlEW0s"},"events":{"$ref":"Ju-wMSQJ-nWv9bz8kP8PuWhur4odJBfn2n0aWeAvRzo"},"kubernetesCordoned":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"kubernetesNodeUID":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"kubernetesReady":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"lastHeartbeatTime":{"$ref":"YL2AaRx5mZZL3drs48WsIqN7ZQe66kWzKXDNrUwOgoc"},"lastStatusChangeTime":{"$ref":"YL2AaRx5mZZL3drs48WsIqN7ZQe66kWzKXDNrUwOgoc"},"nodeInfo":{"$ref":"frgKKNmrwfxggpfZpLUgIbjHptJm-rtlhn0BVHi1n2Y"},"observedNodeLabels":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"},"phase":{"$ref":"HTmxul1jSZTMoFP06KaVGww93WPYdn55Tt7zuQvTs4M"},"pullProgress":{"$ref":"gDMAFrrisSGbwGplQDJI2kIWbFoO35CEb_0uFAdTsQ8"},"storage":{"$ref":"OIbjNBNM55W4mfhobLlKUNjpogQAuW7e4iGx5SQom-8"},"used":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"}},"type":"object","nullable":true};
const schema25 = {"properties":{"pending":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"rlarkManaged":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"running":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"stopping":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"total":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"}},"type":"object","nullable":true};
const schema2 = {"format":"int32","type":"integer","nullable":true};
const formats0 = formats.int32;

function validate2(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(((typeof data == "number") && (!(data % 1) && !isNaN(data))) && (isFinite(data)))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "integer"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if((typeof data == "number") && (isFinite(data))){
if(!(formats0.validate(data))){
const err1 = {instancePath,schemaPath:"#/format",keyword:"format",params:{format: "int32"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
}
if(typeof data === "string"){
}
validate2.errors = vErrors;
return errors === 0;
}


function validate48(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.pending !== undefined){
let data0 = data.pending;
const _errs2 = errors;
if(!(validate2(data0, {instancePath:instancePath+"/pending",parentData:data,parentDataProperty:"pending",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.rlarkManaged !== undefined){
let data1 = data.rlarkManaged;
const _errs3 = errors;
if(!(validate2(data1, {instancePath:instancePath+"/rlarkManaged",parentData:data,parentDataProperty:"rlarkManaged",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.running !== undefined){
let data2 = data.running;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/running",parentData:data,parentDataProperty:"running",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.stopping !== undefined){
let data3 = data.stopping;
const _errs5 = errors;
if(!(validate2(data3, {instancePath:instancePath+"/stopping",parentData:data,parentDataProperty:"stopping",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.total !== undefined){
let data4 = data.total;
const _errs6 = errors;
if(!(validate2(data4, {instancePath:instancePath+"/total",parentData:data,parentDataProperty:"total",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate48.errors = vErrors;
return errors === 0;
}

const schema26 = {"items":{"$ref":"K3T-T2ukXhcPE9us1NDKTbY9H1n_wuSG7yIeYLvzqRY"},"type":"array","nullable":true};
const schema27 = {"properties":{"address":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["address","type"],"type":"object"};
const schema3 = {"type":"string"};

function validate3(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(typeof data !== "string"){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
validate3.errors = vErrors;
return errors === 0;
}


function validate56(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.address === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "address"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.type === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "type"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.address !== undefined){
let data0 = data.address;
const _errs1 = errors;
if(!(validate3(data0, {instancePath:instancePath+"/address",parentData:data,parentDataProperty:"address",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.type !== undefined){
let data1 = data.type;
const _errs2 = errors;
if(!(validate3(data1, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
else {
const err2 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
validate56.errors = vErrors;
return errors === 0;
}


function validate55(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs2 = errors;
if(!(validate56(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate56.errors : vErrors.concat(validate56.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
if(!valid0){
valid1 = false;
}
}
}
validate55.errors = vErrors;
return errors === 0;
}

const schema6 = {"additionalProperties":{"$ref":"3dmDSoHI3Hv64ppBBbqAEh3HzVwqzFWb_nrM2BhHTgs"},"type":"object","properties":{},"nullable":true};
const schema7 = {"anyOf":[{"$ref":"laYO4ZujiLzTOvPomluoS9e439ISWe2RqSMRunp3ReI"},{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}],"pattern":"^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"};
const schema8 = {"type":"integer"};

function validate9(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(!(((typeof data == "number") && (!(data % 1) && !isNaN(data))) && (isFinite(data)))){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "integer"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
validate9.errors = vErrors;
return errors === 0;
}

const pattern0 = new RegExp("^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$", "u");

function validate8(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
let valid0 = false;
const _errs1 = errors;
if(!(validate9(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var _valid0 = _errs1 === errors;
valid0 = valid0 || _valid0;
if(!valid0){
const _errs2 = errors;
if(!(validate3(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var _valid0 = _errs2 === errors;
valid0 = valid0 || _valid0;
if(!valid0){
}
}
if(!valid0){
const err0 = {instancePath,schemaPath:"#/anyOf",keyword:"anyOf",params:{}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
else {
errors = _errs0;
if(vErrors !== null){
if(_errs0){
vErrors.length = _errs0;
}
else {
vErrors = null;
}
}
}
if(typeof data === "string"){
if(!pattern0.test(data)){
const err1 = {instancePath,schemaPath:"#/pattern",keyword:"pattern",params:{pattern: "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
}
validate8.errors = vErrors;
return errors === 0;
}


function validate7(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
const _errs2 = errors;
for(const key0 in data){
let data0 = data[key0];
const _errs3 = errors;
if(!(validate8(data0, {instancePath:instancePath+"/" + key0.replace(/~/g, "~0").replace(/\//g, "~1"),parentData:data,parentDataProperty:key0,rootData}))){
vErrors = vErrors === null ? validate8.errors : vErrors.concat(validate8.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate7.errors = vErrors;
return errors === 0;
}

const schema28 = {"properties":{"devices":{"$ref":"TIbWWpK2yyQAi5K8qAbO-q3uzUytMaqI7qFZ5KB6YlU"},"network":{"$ref":"e3KMD9aKGWtn2RDl1-7JFzwf_6NHrKmGUJSz4iks0fw"},"operations":{"$ref":"ExuTPEY0niRzCVARKZVvbsv56IfgciL-Msm1yUwtJgs"},"runtime":{"$ref":"L-LnP0x7SHqmwtDm9Fr-FMFnwjAPPimy0z1u8Dj01XE"},"storage":{"$ref":"VgcdnkNosEJuY8FrI8Sdmzw6W5Z-FyFsig2MDiDuj6I"}},"type":"object","nullable":true};
const schema29 = {"items":{"$ref":"q9BF57pYUSZIpDqT4kf2LhLUDAXvbF0ASL_t4Xf7udE"},"type":"array","nullable":true};
const schema30 = {"properties":{"available":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"category":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"count":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"}},"required":["category"],"type":"object"};

function validate64(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.category === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "category"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.available !== undefined){
let data0 = data.available;
const _errs1 = errors;
if(!(validate2(data0, {instancePath:instancePath+"/available",parentData:data,parentDataProperty:"available",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.category !== undefined){
let data1 = data.category;
const _errs2 = errors;
if(!(validate3(data1, {instancePath:instancePath+"/category",parentData:data,parentDataProperty:"category",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.count !== undefined){
let data2 = data.count;
const _errs3 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/count",parentData:data,parentDataProperty:"count",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
else {
const err1 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate64.errors = vErrors;
return errors === 0;
}


function validate63(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs2 = errors;
if(!(validate64(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate64.errors : vErrors.concat(validate64.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
if(!valid0){
valid1 = false;
}
}
}
validate63.errors = vErrors;
return errors === 0;
}

const schema31 = {"properties":{"crossCluster":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"hostNetwork":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"serviceExposure":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"type":"object","nullable":true};

function validate70(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.crossCluster !== undefined){
let data0 = data.crossCluster;
const _errs2 = errors;
if(!(validate4(data0, {instancePath:instancePath+"/crossCluster",parentData:data,parentDataProperty:"crossCluster",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.hostNetwork !== undefined){
let data1 = data.hostNetwork;
const _errs3 = errors;
if(!(validate4(data1, {instancePath:instancePath+"/hostNetwork",parentData:data,parentDataProperty:"hostNetwork",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.serviceExposure !== undefined){
let data2 = data.serviceExposure;
const _errs4 = errors;
if(!(validate4(data2, {instancePath:instancePath+"/serviceExposure",parentData:data,parentDataProperty:"serviceExposure",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate70.errors = vErrors;
return errors === 0;
}

const schema32 = {"properties":{"exec":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"logs":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"terminal":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"type":"object","nullable":true};

function validate75(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.exec !== undefined){
let data0 = data.exec;
const _errs2 = errors;
if(!(validate4(data0, {instancePath:instancePath+"/exec",parentData:data,parentDataProperty:"exec",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.logs !== undefined){
let data1 = data.logs;
const _errs3 = errors;
if(!(validate4(data1, {instancePath:instancePath+"/logs",parentData:data,parentDataProperty:"logs",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.terminal !== undefined){
let data2 = data.terminal;
const _errs4 = errors;
if(!(validate4(data2, {instancePath:instancePath+"/terminal",parentData:data,parentDataProperty:"terminal",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate75.errors = vErrors;
return errors === 0;
}

const schema33 = {"properties":{"type":{"$ref":"l_DJxe5Cy2XbpJh4rH5_qZiSOSpZjC3fwneDYujliJg"},"version":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"workloadKinds":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};
const schema5 = {"items":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":"array","nullable":true};

function validate5(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs2 = errors;
if(!(validate3(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
if(!valid0){
valid1 = false;
}
}
}
validate5.errors = vErrors;
return errors === 0;
}


function validate80(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.type !== undefined){
let data0 = data.type;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.version !== undefined){
let data1 = data.version;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/version",parentData:data,parentDataProperty:"version",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.workloadKinds !== undefined){
let data2 = data.workloadKinds;
const _errs4 = errors;
if(!(validate5(data2, {instancePath:instancePath+"/workloadKinds",parentData:data,parentDataProperty:"workloadKinds",rootData}))){
vErrors = vErrors === null ? validate5.errors : vErrors.concat(validate5.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate80.errors = vErrors;
return errors === 0;
}

const schema34 = {"properties":{"storageClasses":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"volumeTypes":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};

function validate85(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.storageClasses !== undefined){
let data0 = data.storageClasses;
const _errs2 = errors;
if(!(validate5(data0, {instancePath:instancePath+"/storageClasses",parentData:data,parentDataProperty:"storageClasses",rootData}))){
vErrors = vErrors === null ? validate5.errors : vErrors.concat(validate5.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.volumeTypes !== undefined){
let data1 = data.volumeTypes;
const _errs3 = errors;
if(!(validate5(data1, {instancePath:instancePath+"/volumeTypes",parentData:data,parentDataProperty:"volumeTypes",rootData}))){
vErrors = vErrors === null ? validate5.errors : vErrors.concat(validate5.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate85.errors = vErrors;
return errors === 0;
}


function validate62(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.devices !== undefined){
let data0 = data.devices;
const _errs2 = errors;
if(!(validate63(data0, {instancePath:instancePath+"/devices",parentData:data,parentDataProperty:"devices",rootData}))){
vErrors = vErrors === null ? validate63.errors : vErrors.concat(validate63.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.network !== undefined){
let data1 = data.network;
const _errs3 = errors;
if(!(validate70(data1, {instancePath:instancePath+"/network",parentData:data,parentDataProperty:"network",rootData}))){
vErrors = vErrors === null ? validate70.errors : vErrors.concat(validate70.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.operations !== undefined){
let data2 = data.operations;
const _errs4 = errors;
if(!(validate75(data2, {instancePath:instancePath+"/operations",parentData:data,parentDataProperty:"operations",rootData}))){
vErrors = vErrors === null ? validate75.errors : vErrors.concat(validate75.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.runtime !== undefined){
let data3 = data.runtime;
const _errs5 = errors;
if(!(validate80(data3, {instancePath:instancePath+"/runtime",parentData:data,parentDataProperty:"runtime",rootData}))){
vErrors = vErrors === null ? validate80.errors : vErrors.concat(validate80.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.storage !== undefined){
let data4 = data.storage;
const _errs6 = errors;
if(!(validate85(data4, {instancePath:instancePath+"/storage",parentData:data,parentDataProperty:"storage",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate62.errors = vErrors;
return errors === 0;
}

const schema35 = {"items":{"$ref":"dZ13d79yfL0AfKGv8XJr0-7aalgpv7dwgkV5EU9HxwM"},"type":"array","nullable":true};
const schema36 = {"properties":{"lastTransitionTime":{"$ref":"nWP098h5Flk6EOMa_OWYW4EnhyCrisyvgJPgk0WtC9c"},"message":{"$ref":"CG805GD3KzsipQt5EKLOQw9SjU_4PVFnnbOjDh0LzJQ"},"observedGeneration":{"$ref":"Tv0gqme9PlVaT-l831KRjUe4u9reG1m_llVaPqPtqUs"},"reason":{"$ref":"UPBfG4e9zcqG6mx3tN1LWinVzYKOFfIO2VnMgg6lAWI"},"status":{"$ref":"UcCRuQZyUlftnmEwuGDZcciwLAdzzRj4HOLofWAwyW8"},"type":{"$ref":"grctw_GwW6iDVZUzowft1rMz55_d64qiJ_wOfyk2Qoo"}},"required":["lastTransitionTime","message","reason","status","type"],"type":"object"};
const schema37 = {"format":"date-time","type":"string"};
const formats2 = formats["date-time"];

function validate93(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if((typeof data == "number") && (isFinite(data))){
}
if(typeof data === "string"){
if(!(formats2.validate(data))){
const err0 = {instancePath,schemaPath:"#/format",keyword:"format",params:{format: "date-time"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
}
else {
const err1 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate93.errors = vErrors;
return errors === 0;
}

const schema38 = {"maxLength":32768,"type":"string"};

function validate95(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(typeof data === "string"){
if(func0(data) > 32768){
const err0 = {instancePath,schemaPath:"#/maxLength",keyword:"maxLength",params:{limit: 32768}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
}
else {
const err1 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate95.errors = vErrors;
return errors === 0;
}

const schema39 = {"format":"int64","type":"integer","minimum":0,"nullable":true};
const formats4 = formats.int64;

function validate97(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(((typeof data == "number") && (!(data % 1) && !isNaN(data))) && (isFinite(data)))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "integer"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if((typeof data == "number") && (isFinite(data))){
if(data < 0 || isNaN(data)){
const err1 = {instancePath,schemaPath:"#/minimum",keyword:"minimum",params:{comparison: ">=", limit: 0}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(!(formats4.validate(data))){
const err2 = {instancePath,schemaPath:"#/format",keyword:"format",params:{format: "int64"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
}
if(typeof data === "string"){
}
validate97.errors = vErrors;
return errors === 0;
}

const schema40 = {"maxLength":1024,"minLength":1,"pattern":"^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$","type":"string"};
const pattern1 = new RegExp("^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$", "u");

function validate99(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(typeof data === "string"){
if(func0(data) > 1024){
const err0 = {instancePath,schemaPath:"#/maxLength",keyword:"maxLength",params:{limit: 1024}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(func0(data) < 1){
const err1 = {instancePath,schemaPath:"#/minLength",keyword:"minLength",params:{limit: 1}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(!pattern1.test(data)){
const err2 = {instancePath,schemaPath:"#/pattern",keyword:"pattern",params:{pattern: "^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
}
else {
const err3 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err3];
}
else {
vErrors.push(err3);
}
errors++;
}
validate99.errors = vErrors;
return errors === 0;
}

const schema41 = {"enum":["True","False","Unknown"],"type":"string"};

function validate101(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(typeof data !== "string"){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const vSchema0 = schema41.enum;
if(!(((data === "True") || (data === "False")) || (data === "Unknown"))){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema41.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate101.errors = vErrors;
return errors === 0;
}

const schema42 = {"maxLength":316,"pattern":"^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$","type":"string"};
const pattern2 = new RegExp("^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$", "u");

function validate103(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(typeof data === "string"){
if(func0(data) > 316){
const err0 = {instancePath,schemaPath:"#/maxLength",keyword:"maxLength",params:{limit: 316}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(!pattern2.test(data)){
const err1 = {instancePath,schemaPath:"#/pattern",keyword:"pattern",params:{pattern: "^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
}
else {
const err2 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
validate103.errors = vErrors;
return errors === 0;
}


function validate92(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.lastTransitionTime === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "lastTransitionTime"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.message === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "message"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.reason === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "reason"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.status === undefined){
const err3 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "status"}};
if(vErrors === null){
vErrors = [err3];
}
else {
vErrors.push(err3);
}
errors++;
}
if(data.type === undefined){
const err4 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "type"}};
if(vErrors === null){
vErrors = [err4];
}
else {
vErrors.push(err4);
}
errors++;
}
if(data.lastTransitionTime !== undefined){
let data0 = data.lastTransitionTime;
const _errs1 = errors;
if(!(validate93(data0, {instancePath:instancePath+"/lastTransitionTime",parentData:data,parentDataProperty:"lastTransitionTime",rootData}))){
vErrors = vErrors === null ? validate93.errors : vErrors.concat(validate93.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.message !== undefined){
let data1 = data.message;
const _errs2 = errors;
if(!(validate95(data1, {instancePath:instancePath+"/message",parentData:data,parentDataProperty:"message",rootData}))){
vErrors = vErrors === null ? validate95.errors : vErrors.concat(validate95.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.observedGeneration !== undefined){
let data2 = data.observedGeneration;
const _errs3 = errors;
if(!(validate97(data2, {instancePath:instancePath+"/observedGeneration",parentData:data,parentDataProperty:"observedGeneration",rootData}))){
vErrors = vErrors === null ? validate97.errors : vErrors.concat(validate97.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.reason !== undefined){
let data3 = data.reason;
const _errs4 = errors;
if(!(validate99(data3, {instancePath:instancePath+"/reason",parentData:data,parentDataProperty:"reason",rootData}))){
vErrors = vErrors === null ? validate99.errors : vErrors.concat(validate99.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.status !== undefined){
let data4 = data.status;
const _errs5 = errors;
if(!(validate101(data4, {instancePath:instancePath+"/status",parentData:data,parentDataProperty:"status",rootData}))){
vErrors = vErrors === null ? validate101.errors : vErrors.concat(validate101.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.type !== undefined){
let data5 = data.type;
const _errs6 = errors;
if(!(validate103(data5, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate103.errors : vErrors.concat(validate103.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
else {
const err5 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err5];
}
else {
vErrors.push(err5);
}
errors++;
}
validate92.errors = vErrors;
return errors === 0;
}


function validate91(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs2 = errors;
if(!(validate92(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate92.errors : vErrors.concat(validate92.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
if(!valid0){
valid1 = false;
}
}
}
validate91.errors = vErrors;
return errors === 0;
}

const schema43 = {"items":{"$ref":"_tmnEtOhLqmPVP7MWajHmAkjQC-3meHwE213540D5FY"},"type":"array","nullable":true};
const schema44 = {"properties":{"blocked":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"completed":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"operationID":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"waiting":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"}},"required":["operationID"],"type":"object"};

function validate109(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.operationID === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "operationID"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.blocked !== undefined){
let data0 = data.blocked;
const _errs1 = errors;
if(!(validate2(data0, {instancePath:instancePath+"/blocked",parentData:data,parentDataProperty:"blocked",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.completed !== undefined){
let data1 = data.completed;
const _errs2 = errors;
if(!(validate4(data1, {instancePath:instancePath+"/completed",parentData:data,parentDataProperty:"completed",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.operationID !== undefined){
let data2 = data.operationID;
const _errs3 = errors;
if(!(validate3(data2, {instancePath:instancePath+"/operationID",parentData:data,parentDataProperty:"operationID",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.waiting !== undefined){
let data3 = data.waiting;
const _errs4 = errors;
if(!(validate2(data3, {instancePath:instancePath+"/waiting",parentData:data,parentDataProperty:"waiting",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
else {
const err1 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate109.errors = vErrors;
return errors === 0;
}


function validate108(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs2 = errors;
if(!(validate109(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate109.errors : vErrors.concat(validate109.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
if(!valid0){
valid1 = false;
}
}
}
validate108.errors = vErrors;
return errors === 0;
}

const schema45 = {"items":{"$ref":"psMXK1PdmbXnnoGH_RHVnCc5FjNkMdWz1aoeVYVaq1s"},"type":"array","nullable":true};
const schema46 = {"properties":{"count":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"lastTime":{"$ref":"YL2AaRx5mZZL3drs48WsIqN7ZQe66kWzKXDNrUwOgoc"},"message":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"objectKind":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"objectName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"reason":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"source":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"type":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["reason","type"],"type":"object"};
const schema9 = {"format":"date-time","type":"string","nullable":true};

function validate13(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((typeof data !== "string") && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if((typeof data == "number") && (isFinite(data))){
}
if(typeof data === "string"){
if(!(formats2.validate(data))){
const err1 = {instancePath,schemaPath:"#/format",keyword:"format",params:{format: "date-time"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
}
validate13.errors = vErrors;
return errors === 0;
}


function validate117(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.reason === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "reason"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.type === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "type"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.count !== undefined){
let data0 = data.count;
const _errs1 = errors;
if(!(validate2(data0, {instancePath:instancePath+"/count",parentData:data,parentDataProperty:"count",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.lastTime !== undefined){
let data1 = data.lastTime;
const _errs2 = errors;
if(!(validate13(data1, {instancePath:instancePath+"/lastTime",parentData:data,parentDataProperty:"lastTime",rootData}))){
vErrors = vErrors === null ? validate13.errors : vErrors.concat(validate13.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.message !== undefined){
let data2 = data.message;
const _errs3 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/message",parentData:data,parentDataProperty:"message",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.objectKind !== undefined){
let data3 = data.objectKind;
const _errs4 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/objectKind",parentData:data,parentDataProperty:"objectKind",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.objectName !== undefined){
let data4 = data.objectName;
const _errs5 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/objectName",parentData:data,parentDataProperty:"objectName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.reason !== undefined){
let data5 = data.reason;
const _errs6 = errors;
if(!(validate3(data5, {instancePath:instancePath+"/reason",parentData:data,parentDataProperty:"reason",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.source !== undefined){
let data6 = data.source;
const _errs7 = errors;
if(!(validate0(data6, {instancePath:instancePath+"/source",parentData:data,parentDataProperty:"source",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.type !== undefined){
let data7 = data.type;
const _errs8 = errors;
if(!(validate3(data7, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
}
else {
const err2 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
validate117.errors = vErrors;
return errors === 0;
}


function validate116(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs2 = errors;
if(!(validate117(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate117.errors : vErrors.concat(validate117.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
if(!valid0){
valid1 = false;
}
}
}
validate116.errors = vErrors;
return errors === 0;
}

const schema47 = {"properties":{"agentVersion":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"architecture":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"kernelVersion":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"operatingSystem":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"runtimeVersion":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate133(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.agentVersion !== undefined){
let data0 = data.agentVersion;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/agentVersion",parentData:data,parentDataProperty:"agentVersion",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.architecture !== undefined){
let data1 = data.architecture;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/architecture",parentData:data,parentDataProperty:"architecture",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.kernelVersion !== undefined){
let data2 = data.kernelVersion;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/kernelVersion",parentData:data,parentDataProperty:"kernelVersion",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.operatingSystem !== undefined){
let data3 = data.operatingSystem;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/operatingSystem",parentData:data,parentDataProperty:"operatingSystem",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.runtimeVersion !== undefined){
let data4 = data.runtimeVersion;
const _errs6 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/runtimeVersion",parentData:data,parentDataProperty:"runtimeVersion",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate133.errors = vErrors;
return errors === 0;
}

const schema48 = {"additionalProperties":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":"object","properties":{},"nullable":true};

function validate140(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
const _errs2 = errors;
for(const key0 in data){
let data0 = data[key0];
const _errs3 = errors;
if(!(validate3(data0, {instancePath:instancePath+"/" + key0.replace(/~/g, "~0").replace(/\//g, "~1"),parentData:data,parentDataProperty:key0,rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate140.errors = vErrors;
return errors === 0;
}

const schema49 = {"enum":["Registering","Online","Degraded","Offline","Cordoned","Draining"],"type":"string","nullable":true};

function validate143(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((typeof data !== "string") && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "string"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
const vSchema0 = schema49.enum;
if(!((((((data === "Registering") || (data === "Online")) || (data === "Degraded")) || (data === "Offline")) || (data === "Cordoned")) || (data === "Draining"))){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema49.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate143.errors = vErrors;
return errors === 0;
}

const schema50 = {"items":{"$ref":"eXqsosMmVlqiYyBADb2y8LOSfkVbCL3AlfIBXQ87hmE"},"type":"array","nullable":true};
const schema51 = {"properties":{"downloaded":{"$ref":"0YQm7z5aN2fg7S8XlbxrquRU6pdIr97uCbtQYyjv3S4"},"image":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"message":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"speed":{"$ref":"0hFN2lc8EZNwdRPFUdVm0IS7gM8AwqK_YhN0yw_gZLU"},"status":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"total":{"$ref":"0YQm7z5aN2fg7S8XlbxrquRU6pdIr97uCbtQYyjv3S4"}},"required":["downloaded","image","speed","status","total"],"type":"object"};
const schema10 = {"format":"int64","type":"integer"};

function validate14(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(!(((typeof data == "number") && (!(data % 1) && !isNaN(data))) && (isFinite(data)))){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "integer"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
if((typeof data == "number") && (isFinite(data))){
if(!(formats4.validate(data))){
const err1 = {instancePath,schemaPath:"#/format",keyword:"format",params:{format: "int64"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
}
if(typeof data === "string"){
}
validate14.errors = vErrors;
return errors === 0;
}

const schema52 = {"type":"number"};

function validate150(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(!((typeof data == "number") && (isFinite(data)))){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "number"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
validate150.errors = vErrors;
return errors === 0;
}


function validate146(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.downloaded === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "downloaded"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.image === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "image"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.speed === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "speed"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.status === undefined){
const err3 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "status"}};
if(vErrors === null){
vErrors = [err3];
}
else {
vErrors.push(err3);
}
errors++;
}
if(data.total === undefined){
const err4 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "total"}};
if(vErrors === null){
vErrors = [err4];
}
else {
vErrors.push(err4);
}
errors++;
}
if(data.downloaded !== undefined){
let data0 = data.downloaded;
const _errs1 = errors;
if(!(validate14(data0, {instancePath:instancePath+"/downloaded",parentData:data,parentDataProperty:"downloaded",rootData}))){
vErrors = vErrors === null ? validate14.errors : vErrors.concat(validate14.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.image !== undefined){
let data1 = data.image;
const _errs2 = errors;
if(!(validate3(data1, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.message !== undefined){
let data2 = data.message;
const _errs3 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/message",parentData:data,parentDataProperty:"message",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.speed !== undefined){
let data3 = data.speed;
const _errs4 = errors;
if(!(validate150(data3, {instancePath:instancePath+"/speed",parentData:data,parentDataProperty:"speed",rootData}))){
vErrors = vErrors === null ? validate150.errors : vErrors.concat(validate150.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.status !== undefined){
let data4 = data.status;
const _errs5 = errors;
if(!(validate3(data4, {instancePath:instancePath+"/status",parentData:data,parentDataProperty:"status",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.total !== undefined){
let data5 = data.total;
const _errs6 = errors;
if(!(validate14(data5, {instancePath:instancePath+"/total",parentData:data,parentDataProperty:"total",rootData}))){
vErrors = vErrors === null ? validate14.errors : vErrors.concat(validate14.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
else {
const err5 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err5];
}
else {
vErrors.push(err5);
}
errors++;
}
validate146.errors = vErrors;
return errors === 0;
}


function validate145(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs2 = errors;
if(!(validate146(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate146.errors : vErrors.concat(validate146.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
if(!valid0){
valid1 = false;
}
}
}
validate145.errors = vErrors;
return errors === 0;
}

const schema53 = {"properties":{"availableBytes":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"capacityBytes":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"usedBytes":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"}},"type":"object","nullable":true};
const schema11 = {"format":"int64","type":"integer","nullable":true};

function validate15(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(((typeof data == "number") && (!(data % 1) && !isNaN(data))) && (isFinite(data)))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "integer"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if((typeof data == "number") && (isFinite(data))){
if(!(formats4.validate(data))){
const err1 = {instancePath,schemaPath:"#/format",keyword:"format",params:{format: "int64"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
}
if(typeof data === "string"){
}
validate15.errors = vErrors;
return errors === 0;
}


function validate156(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.availableBytes !== undefined){
let data0 = data.availableBytes;
const _errs2 = errors;
if(!(validate15(data0, {instancePath:instancePath+"/availableBytes",parentData:data,parentDataProperty:"availableBytes",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.capacityBytes !== undefined){
let data1 = data.capacityBytes;
const _errs3 = errors;
if(!(validate15(data1, {instancePath:instancePath+"/capacityBytes",parentData:data,parentDataProperty:"capacityBytes",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.usedBytes !== undefined){
let data2 = data.usedBytes;
const _errs4 = errors;
if(!(validate15(data2, {instancePath:instancePath+"/usedBytes",parentData:data,parentDataProperty:"usedBytes",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate156.errors = vErrors;
return errors === 0;
}


function validate47(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if((!(data && typeof data == "object" && !Array.isArray(data))) && (data !== null)){
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
const _errs0 = errors;
const _errs1 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.activeWorkloads !== undefined){
let data0 = data.activeWorkloads;
const _errs2 = errors;
if(!(validate48(data0, {instancePath:instancePath+"/activeWorkloads",parentData:data,parentDataProperty:"activeWorkloads",rootData}))){
vErrors = vErrors === null ? validate48.errors : vErrors.concat(validate48.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.addresses !== undefined){
let data1 = data.addresses;
const _errs3 = errors;
if(!(validate55(data1, {instancePath:instancePath+"/addresses",parentData:data,parentDataProperty:"addresses",rootData}))){
vErrors = vErrors === null ? validate55.errors : vErrors.concat(validate55.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.allocatable !== undefined){
let data2 = data.allocatable;
const _errs4 = errors;
if(!(validate7(data2, {instancePath:instancePath+"/allocatable",parentData:data,parentDataProperty:"allocatable",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.capabilitySnapshot !== undefined){
let data3 = data.capabilitySnapshot;
const _errs5 = errors;
if(!(validate62(data3, {instancePath:instancePath+"/capabilitySnapshot",parentData:data,parentDataProperty:"capabilitySnapshot",rootData}))){
vErrors = vErrors === null ? validate62.errors : vErrors.concat(validate62.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.capacity !== undefined){
let data4 = data.capacity;
const _errs6 = errors;
if(!(validate7(data4, {instancePath:instancePath+"/capacity",parentData:data,parentDataProperty:"capacity",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.conditions !== undefined){
let data5 = data.conditions;
const _errs7 = errors;
if(!(validate91(data5, {instancePath:instancePath+"/conditions",parentData:data,parentDataProperty:"conditions",rootData}))){
vErrors = vErrors === null ? validate91.errors : vErrors.concat(validate91.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.diskPressure !== undefined){
let data6 = data.diskPressure;
const _errs8 = errors;
if(!(validate4(data6, {instancePath:instancePath+"/diskPressure",parentData:data,parentDataProperty:"diskPressure",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.drainOperations !== undefined){
let data7 = data.drainOperations;
const _errs9 = errors;
if(!(validate108(data7, {instancePath:instancePath+"/drainOperations",parentData:data,parentDataProperty:"drainOperations",rootData}))){
vErrors = vErrors === null ? validate108.errors : vErrors.concat(validate108.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.events !== undefined){
let data8 = data.events;
const _errs10 = errors;
if(!(validate116(data8, {instancePath:instancePath+"/events",parentData:data,parentDataProperty:"events",rootData}))){
vErrors = vErrors === null ? validate116.errors : vErrors.concat(validate116.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.kubernetesCordoned !== undefined){
let data9 = data.kubernetesCordoned;
const _errs11 = errors;
if(!(validate4(data9, {instancePath:instancePath+"/kubernetesCordoned",parentData:data,parentDataProperty:"kubernetesCordoned",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.kubernetesNodeUID !== undefined){
let data10 = data.kubernetesNodeUID;
const _errs12 = errors;
if(!(validate0(data10, {instancePath:instancePath+"/kubernetesNodeUID",parentData:data,parentDataProperty:"kubernetesNodeUID",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.kubernetesReady !== undefined){
let data11 = data.kubernetesReady;
const _errs13 = errors;
if(!(validate4(data11, {instancePath:instancePath+"/kubernetesReady",parentData:data,parentDataProperty:"kubernetesReady",rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.lastHeartbeatTime !== undefined){
let data12 = data.lastHeartbeatTime;
const _errs14 = errors;
if(!(validate13(data12, {instancePath:instancePath+"/lastHeartbeatTime",parentData:data,parentDataProperty:"lastHeartbeatTime",rootData}))){
vErrors = vErrors === null ? validate13.errors : vErrors.concat(validate13.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
if(data.lastStatusChangeTime !== undefined){
let data13 = data.lastStatusChangeTime;
const _errs15 = errors;
if(!(validate13(data13, {instancePath:instancePath+"/lastStatusChangeTime",parentData:data,parentDataProperty:"lastStatusChangeTime",rootData}))){
vErrors = vErrors === null ? validate13.errors : vErrors.concat(validate13.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs15 === errors;
}
if(data.nodeInfo !== undefined){
let data14 = data.nodeInfo;
const _errs16 = errors;
if(!(validate133(data14, {instancePath:instancePath+"/nodeInfo",parentData:data,parentDataProperty:"nodeInfo",rootData}))){
vErrors = vErrors === null ? validate133.errors : vErrors.concat(validate133.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs16 === errors;
}
if(data.observedNodeLabels !== undefined){
let data15 = data.observedNodeLabels;
const _errs17 = errors;
if(!(validate140(data15, {instancePath:instancePath+"/observedNodeLabels",parentData:data,parentDataProperty:"observedNodeLabels",rootData}))){
vErrors = vErrors === null ? validate140.errors : vErrors.concat(validate140.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs17 === errors;
}
if(data.phase !== undefined){
let data16 = data.phase;
const _errs18 = errors;
if(!(validate143(data16, {instancePath:instancePath+"/phase",parentData:data,parentDataProperty:"phase",rootData}))){
vErrors = vErrors === null ? validate143.errors : vErrors.concat(validate143.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs18 === errors;
}
if(data.pullProgress !== undefined){
let data17 = data.pullProgress;
const _errs19 = errors;
if(!(validate145(data17, {instancePath:instancePath+"/pullProgress",parentData:data,parentDataProperty:"pullProgress",rootData}))){
vErrors = vErrors === null ? validate145.errors : vErrors.concat(validate145.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs19 === errors;
}
if(data.storage !== undefined){
let data18 = data.storage;
const _errs20 = errors;
if(!(validate156(data18, {instancePath:instancePath+"/storage",parentData:data,parentDataProperty:"storage",rootData}))){
vErrors = vErrors === null ? validate156.errors : vErrors.concat(validate156.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs20 === errors;
}
if(data.used !== undefined){
let data19 = data.used;
const _errs21 = errors;
if(!(validate7(data19, {instancePath:instancePath+"/used",parentData:data,parentDataProperty:"used",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs21 === errors;
}
}
validate47.errors = vErrors;
return errors === 0;
}


function validate16(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
/*# sourceURL="rlinf.io.v1alpha1.Node" */;
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.apiVersion === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "apiVersion"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.kind === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "kind"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.apiVersion !== undefined){
let data0 = data.apiVersion;
const _errs1 = errors;
if(!(validate17(data0, {instancePath:instancePath+"/apiVersion",parentData:data,parentDataProperty:"apiVersion",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.kind !== undefined){
let data1 = data.kind;
const _errs2 = errors;
if(!(validate19(data1, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate19.errors : vErrors.concat(validate19.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.metadata !== undefined){
let data2 = data.metadata;
const _errs3 = errors;
if(!(validate21(data2, {instancePath:instancePath+"/metadata",parentData:data,parentDataProperty:"metadata",rootData}))){
vErrors = vErrors === null ? validate21.errors : vErrors.concat(validate21.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.spec !== undefined){
let data3 = data.spec;
const _errs4 = errors;
if(!(validate25(data3, {instancePath:instancePath+"/spec",parentData:data,parentDataProperty:"spec",rootData}))){
vErrors = vErrors === null ? validate25.errors : vErrors.concat(validate25.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.status !== undefined){
let data4 = data.status;
const _errs5 = errors;
if(!(validate47(data4, {instancePath:instancePath+"/status",parentData:data,parentDataProperty:"status",rootData}))){
vErrors = vErrors === null ? validate47.errors : vErrors.concat(validate47.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
else {
const err2 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
validate16.errors = vErrors;
return errors === 0;
}
