// Code generated from api/config/crd/bases. DO NOT EDIT.

import { formats } from "@kubernetes-models/validate";
export const validate = validate273;
const schema81 = {"type":"object","properties":{"apiVersion":{"$ref":"rgSv5V2hoWAdXQ_XErlwxUiy6phhz4ZEBCjtqeXtVec"},"kind":{"$ref":"Rb8e8Lq3bawVJxqI-VfN_BtOGo9S-Pfl13_yf3-qx7c"},"metadata":{"$ref":"P3srTZpWadQH4s4N9KfJR0iCqydQo9YAz7xMB2qf0GQ"},"spec":{"$ref":"1RPfEr3TChGW8QWSdVasGbuLgs2WLFDPuVyW_YzI5eY"},"status":{"$ref":"2eBADeLr4H53oHOwnXqCDV52YLB7ePbzDiNobIyO_R8"}},"required":["apiVersion","kind"],"$id":"rlinf.io.v1alpha1.Job"};
const schema82 = {"type":"string","enum":["rlinf.io/v1alpha1"]};

function validate274(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
const vSchema0 = schema82.enum;
if(!(data === "rlinf.io/v1alpha1")){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema82.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate274.errors = vErrors;
return errors === 0;
}

const schema83 = {"type":"string","enum":["Job"]};

function validate276(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
const vSchema0 = schema83.enum;
if(!(data === "Job")){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema83.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate276.errors = vErrors;
return errors === 0;
}

const schema84 = {"nullableRef":"io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta#"};
const schema85 = {};

import { validate as validate279 } from "@kubernetes-models/apimachinery/_schemas/IoK8sApimachineryPkgApisMetaV1ObjectMeta";


function validate278(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(data !== null){
if(!(validate279(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate279.errors : vErrors.concat(validate279.errors);
errors = vErrors.length;
}
else {
}
}
validate278.errors = vErrors;
return errors === 0;
}

const schema86 = {"properties":{"domain":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"sshPublicKey":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"suspended":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"tags":{"$ref":"AZUBfZk2hZ5WZyE5h97-9eYFBx85ELOoaLL9epVNPfg"},"tasks":{"$ref":"yF6tj4Mc8vlebo565NjbEmgNlJkl9B1OBmuCmgvPnLE"}},"type":"object","nullable":true};
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

const schema2 = {"type":"boolean","nullable":true};

function validate2(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate2.errors = vErrors;
return errors === 0;
}

const schema3 = {"items":{"$ref":"V89VA1AXeRuHheKt9ipXKQFE-H5_Z5APxb-xlMIJVqg"},"type":"array","nullable":true};
const schema4 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"values":{"$ref":"uqUtVyG9D2UAWDvqosEgVMtwK5kI3TMy2eLbDZB452I"}},"required":["key","values"],"type":"object"};
const schema1 = {"type":"string"};

function validate1(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate1.errors = vErrors;
return errors === 0;
}

const schema5 = {"items":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":"array"};

function validate6(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
if(!valid0){
valid1 = false;
}
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate6.errors = vErrors;
return errors === 0;
}


function validate4(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.key === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "key"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.values === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "values"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.key !== undefined){
let data0 = data.key;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.values !== undefined){
let data1 = data.values;
const _errs2 = errors;
if(!(validate6(data1, {instancePath:instancePath+"/values",parentData:data,parentDataProperty:"values",rootData}))){
vErrors = vErrors === null ? validate6.errors : vErrors.concat(validate6.errors);
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
validate4.errors = vErrors;
return errors === 0;
}


function validate3(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate4(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate4.errors : vErrors.concat(validate4.errors);
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
validate3.errors = vErrors;
return errors === 0;
}

const schema87 = {"items":{"$ref":"MzJ1uFU1gzQryymWAIf-T2WOgI5YUwc0Arp5L6E8W6E"},"type":"array","nullable":true};
const schema88 = {"properties":{"downstreamName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"environments":{"$ref":"ER8EfLI4tSlEHvLQ8_Z43cyggmMtWivyTo46m-KzNZE"},"head":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"placement":{"$ref":"KUhfmrayW79_k19wFilmt4-fZzov5TFSde5mJCEwwZY"},"role":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"runtime":{"$ref":"GpIVJdoOsXHFetByy5WgIi3GHmwWTCpFhj_OcVo99fg"},"suspended":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"tags":{"$ref":"AZUBfZk2hZ5WZyE5h97-9eYFBx85ELOoaLL9epVNPfg"},"workload":{"$ref":"8viUqQ1oh4U-mS7-fVETX0nTT-7IJNuBfApiHkLWISg"}},"required":["placement","role","runtime","workload"],"type":"object"};
const schema89 = {"items":{"$ref":"EBBQtu2Gx9J6fkKNZQZN30EEJK6IhvRmiXH6HGsPPik"},"type":"array","nullable":true};
const schema6 = {"properties":{"parameters":{"$ref":"RBNvo1WzZ4oRRq0W9-hknpT7T8If536DEMBg9hyq_4o"},"type":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"version":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["type","version"],"type":"object"};
const schema7 = {};

function validate11(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
validate11.errors = null;
return true;
}


function validate10(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.type === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "type"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.version === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "version"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.parameters !== undefined){
let data0 = data.parameters;
const _errs1 = errors;
if(!(validate11(data0, {instancePath:instancePath+"/parameters",parentData:data,parentDataProperty:"parameters",rootData}))){
vErrors = vErrors === null ? validate11.errors : vErrors.concat(validate11.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.type !== undefined){
let data1 = data.type;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.version !== undefined){
let data2 = data.version;
const _errs3 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/version",parentData:data,parentDataProperty:"version",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
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
validate10.errors = vErrors;
return errors === 0;
}


function validate290(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate10(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate10.errors : vErrors.concat(validate10.errors);
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
validate290.errors = vErrors;
return errors === 0;
}

const schema90 = {"properties":{"nodeSelector":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"}},"type":"object"};
const schema8 = {"additionalProperties":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":"object","properties":{},"nullable":true};

function validate15(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/" + key0.replace(/~/g, "~0").replace(/\//g, "~1"),parentData:data,parentDataProperty:key0,rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate15.errors = vErrors;
return errors === 0;
}


function validate295(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.nodeSelector !== undefined){
let data0 = data.nodeSelector;
const _errs1 = errors;
if(!(validate15(data0, {instancePath:instancePath+"/nodeSelector",parentData:data,parentDataProperty:"nodeSelector",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate295.errors = vErrors;
return errors === 0;
}

const schema91 = {"properties":{"kind":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["kind"],"type":"object"};

function validate299(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.kind === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "kind"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.kind !== undefined){
let data0 = data.kind;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.name !== undefined){
let data1 = data.name;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
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
validate299.errors = vErrors;
return errors === 0;
}

const schema92 = {"properties":{"carrier":{"$ref":"EBBQtu2Gx9J6fkKNZQZN30EEJK6IhvRmiXH6HGsPPik"},"kind":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"parameters":{"$ref":"RBNvo1WzZ4oRRq0W9-hknpT7T8If536DEMBg9hyq_4o"},"template":{"$ref":"xcL4NGh_zPZzHwtqXxR_gWzuA1WPWoXAZzpBvyzgDsw"},"version":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["carrier","version"],"type":"object"};
const schema93 = {"properties":{"metadata":{"$ref":"9xOEryzozGanNCCz7goq0K4hzMvoOusQQ4iDJ494Bhw"},"spec":{"$ref":"9ELI21-RY5S8b2WTyT-3A4ABzk4xYvXNgu9N8BXlru8"}},"type":"object","nullable":true};
const schema72 = {"properties":{"annotations":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"},"finalizers":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"labels":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"namespace":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};
const schema9 = {"items":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":"array","nullable":true};

function validate17(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate17.errors = vErrors;
return errors === 0;
}


function validate246(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.annotations !== undefined){
let data0 = data.annotations;
const _errs2 = errors;
if(!(validate15(data0, {instancePath:instancePath+"/annotations",parentData:data,parentDataProperty:"annotations",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.finalizers !== undefined){
let data1 = data.finalizers;
const _errs3 = errors;
if(!(validate17(data1, {instancePath:instancePath+"/finalizers",parentData:data,parentDataProperty:"finalizers",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.labels !== undefined){
let data2 = data.labels;
const _errs4 = errors;
if(!(validate15(data2, {instancePath:instancePath+"/labels",parentData:data,parentDataProperty:"labels",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.name !== undefined){
let data3 = data.name;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.namespace !== undefined){
let data4 = data.namespace;
const _errs6 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/namespace",parentData:data,parentDataProperty:"namespace",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate246.errors = vErrors;
return errors === 0;
}

const schema94 = {"properties":{"activeDeadlineSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"affinity":{"$ref":"xEzn8Yn5UZwX8tOMdJxI_vy9KYtT_0JX0A88KaKocSo"},"automountServiceAccountToken":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"containers":{"$ref":"vPve3gHxxNN3Sm6pVDxHpN1aCLg_ph9lZEklsRQDPgo"},"dnsConfig":{"$ref":"FyA4xmwX0zynVwRI119QOxQbFJN1tctRkE7b2kzXuc4"},"dnsPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"enableServiceLinks":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"ephemeralContainers":{"$ref":"yKmjfLtm3wPQY2AVwwdCnmvEKUbRA-4_LyiuVKGendA"},"hostAliases":{"$ref":"tZ9rYgYSvnaUwreY3KeGfBGexu5Ycj74ilyAZUawHog"},"hostIPC":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"hostNetwork":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"hostPID":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"hostUsers":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"hostname":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"hostnameOverride":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"imagePullSecrets":{"$ref":"X1TOUYMQSutUYQ6hvqIzZbwk5omDcBNTKfnB0-fL2Z8"},"initContainers":{"$ref":"x3_xyk8cJDC-WRl-Q3Vnzp0T2UpwLUhDnhFB1cxjhUw"},"nodeName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"nodeSelector":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"},"os":{"$ref":"E97dOchMV0AJMEW2BvWE8x-plhaqw9P894BmF33ZGUY"},"overhead":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"},"preemptionPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"priority":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"priorityClassName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readinessGates":{"$ref":"tNwOOZ3NvCXgx0K8ciAkE_yKAsFotR0cS8KzXPkjiQA"},"resourceClaims":{"$ref":"0tfYwcjr1H_7z3CEXMv8E4O0eK7miDdm2dJX8axskZo"},"resources":{"$ref":"FhMRS9yiPxYjQhkDxbgv-jQ4jx9VLxRdMLNZtrOnwhI"},"restartPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"runtimeClassName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"schedulerName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"schedulingGates":{"$ref":"m4uoxKUfX1hC4hxhIVYqmLIHdW2quxepD9tixEKMfno"},"schedulingGroup":{"$ref":"_zmNSOGPBS_eId1M6G5RIekFmO_DVXdtDZe9BX5KXas"},"securityContext":{"$ref":"Q_ytIvl2Z5eadDvs5pDNIRSSXo4ZRIiK5GaG-32_Hu0"},"serviceAccount":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"serviceAccountName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"setHostnameAsFQDN":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"shareProcessNamespace":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"subdomain":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"terminationGracePeriodSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"tolerations":{"$ref":"_Va30eZa87-ZEGMNNRXBis4QY2hj9ZNqFG0WocRDLmw"},"topologySpreadConstraints":{"$ref":"nQdehtf0Mv4PatJ6VO_CgOf95dDX90Z5ka6yfaxVtLU"},"volumes":{"$ref":"6MEj9x1EdleTLWxH3GxOdDVugNWE_cdNoiAgUzIbfCY"}},"required":["containers"],"type":"object","nullable":true};
const schema34 = {"format":"int64","type":"integer","nullable":true};
const formats2 = formats.int64;

function validate86(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(formats2.validate(data))){
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
validate86.errors = vErrors;
return errors === 0;
}

const schema95 = {"properties":{"nodeAffinity":{"$ref":"IM3S0i8cVDyz5lYunR5j9kbhD7Xy_8b7_y3KRQ-p_lg"},"podAffinity":{"$ref":"O-egicA4MB8uPievojYXDHsa_7eltZLPehir6Y4ISbM"},"podAntiAffinity":{"$ref":"O-egicA4MB8uPievojYXDHsa_7eltZLPehir6Y4ISbM"}},"type":"object","nullable":true};
const schema96 = {"properties":{"preferredDuringSchedulingIgnoredDuringExecution":{"$ref":"6e5xkVuknB576DQB84VZ02uf6mAhcmFIEb0YZ5ZeoME"},"requiredDuringSchedulingIgnoredDuringExecution":{"$ref":"dsUswhQImzOLseDnHGq7bSBqTb8SnnotFBkumLi5HQQ"}},"type":"object","nullable":true};
const schema97 = {"items":{"$ref":"0GU7bbB8-3A4umzJ_Vhsp6Y__EiDmpASVQZgVjw3dmQ"},"type":"array","nullable":true};
const schema98 = {"properties":{"preference":{"$ref":"bPs4NJDgemKZPXkEsB4N2vMyTIvZBXESTOmRdaBhgzY"},"weight":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"}},"required":["preference","weight"],"type":"object"};
const schema12 = {"properties":{"matchExpressions":{"$ref":"ayKrb4PFrKLMYqvjiutPxrbz750hbVgxft904f5rFtQ"},"matchFields":{"$ref":"ayKrb4PFrKLMYqvjiutPxrbz750hbVgxft904f5rFtQ"}},"type":"object"};
const schema10 = {"items":{"$ref":"9NBsm8QGHPFL3UnZD7QzZl0zjxatAalAe2kAVgrnyrc"},"type":"array","nullable":true};
const schema11 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"operator":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"values":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"required":["key","operator"],"type":"object"};

function validate20(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.key === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "key"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.operator === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "operator"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.key !== undefined){
let data0 = data.key;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.operator !== undefined){
let data1 = data.operator;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/operator",parentData:data,parentDataProperty:"operator",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.values !== undefined){
let data2 = data.values;
const _errs3 = errors;
if(!(validate17(data2, {instancePath:instancePath+"/values",parentData:data,parentDataProperty:"values",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
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
validate20.errors = vErrors;
return errors === 0;
}


function validate19(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate20(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate20.errors : vErrors.concat(validate20.errors);
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
validate19.errors = vErrors;
return errors === 0;
}


function validate25(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.matchExpressions !== undefined){
let data0 = data.matchExpressions;
const _errs1 = errors;
if(!(validate19(data0, {instancePath:instancePath+"/matchExpressions",parentData:data,parentDataProperty:"matchExpressions",rootData}))){
vErrors = vErrors === null ? validate19.errors : vErrors.concat(validate19.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.matchFields !== undefined){
let data1 = data.matchFields;
const _errs2 = errors;
if(!(validate19(data1, {instancePath:instancePath+"/matchFields",parentData:data,parentDataProperty:"matchFields",rootData}))){
vErrors = vErrors === null ? validate19.errors : vErrors.concat(validate19.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate25.errors = vErrors;
return errors === 0;
}

const schema14 = {"format":"int32","type":"integer"};
const formats0 = formats.int32;

function validate31(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate31.errors = vErrors;
return errors === 0;
}


function validate316(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.preference === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "preference"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.weight === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "weight"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.preference !== undefined){
let data0 = data.preference;
const _errs1 = errors;
if(!(validate25(data0, {instancePath:instancePath+"/preference",parentData:data,parentDataProperty:"preference",rootData}))){
vErrors = vErrors === null ? validate25.errors : vErrors.concat(validate25.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.weight !== undefined){
let data1 = data.weight;
const _errs2 = errors;
if(!(validate31(data1, {instancePath:instancePath+"/weight",parentData:data,parentDataProperty:"weight",rootData}))){
vErrors = vErrors === null ? validate31.errors : vErrors.concat(validate31.errors);
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
validate316.errors = vErrors;
return errors === 0;
}


function validate315(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate316(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate316.errors : vErrors.concat(validate316.errors);
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
validate315.errors = vErrors;
return errors === 0;
}

const schema99 = {"properties":{"nodeSelectorTerms":{"$ref":"WceES8jSl1b6JeqUZQ18OOMpo-zexwPEiCeLvTA69Xg"}},"required":["nodeSelectorTerms"],"type":"object","nullable":true};
const schema100 = {"items":{"$ref":"bPs4NJDgemKZPXkEsB4N2vMyTIvZBXESTOmRdaBhgzY"},"type":"array"};

function validate322(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs1 = errors;
if(!(validate25(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate25.errors : vErrors.concat(validate25.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
if(!valid0){
valid1 = false;
}
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate322.errors = vErrors;
return errors === 0;
}


function validate321(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.nodeSelectorTerms === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "nodeSelectorTerms"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.nodeSelectorTerms !== undefined){
let data0 = data.nodeSelectorTerms;
const _errs2 = errors;
if(!(validate322(data0, {instancePath:instancePath+"/nodeSelectorTerms",parentData:data,parentDataProperty:"nodeSelectorTerms",rootData}))){
vErrors = vErrors === null ? validate322.errors : vErrors.concat(validate322.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate321.errors = vErrors;
return errors === 0;
}


function validate314(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.preferredDuringSchedulingIgnoredDuringExecution !== undefined){
let data0 = data.preferredDuringSchedulingIgnoredDuringExecution;
const _errs2 = errors;
if(!(validate315(data0, {instancePath:instancePath+"/preferredDuringSchedulingIgnoredDuringExecution",parentData:data,parentDataProperty:"preferredDuringSchedulingIgnoredDuringExecution",rootData}))){
vErrors = vErrors === null ? validate315.errors : vErrors.concat(validate315.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.requiredDuringSchedulingIgnoredDuringExecution !== undefined){
let data1 = data.requiredDuringSchedulingIgnoredDuringExecution;
const _errs3 = errors;
if(!(validate321(data1, {instancePath:instancePath+"/requiredDuringSchedulingIgnoredDuringExecution",parentData:data,parentDataProperty:"requiredDuringSchedulingIgnoredDuringExecution",rootData}))){
vErrors = vErrors === null ? validate321.errors : vErrors.concat(validate321.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate314.errors = vErrors;
return errors === 0;
}

const schema16 = {"properties":{"preferredDuringSchedulingIgnoredDuringExecution":{"$ref":"Gv8Hvk-YMeAbi17zcXvGdYz35Trv28l0k6de2GFzrpY"},"requiredDuringSchedulingIgnoredDuringExecution":{"$ref":"x-4CXre8Q_6LB7j_yEJTNRttDcJly0o6xUOEkjdRAMg"}},"type":"object","nullable":true};
const schema17 = {"items":{"$ref":"9Yx6B--m2eQFYkNbnIcDLOTpZxndM94PyYZq_-kDybs"},"type":"array","nullable":true};
const schema18 = {"properties":{"podAffinityTerm":{"$ref":"8t0N05ceNTQObj1ufnNXouWS88GDJF66xuctgeD6LLg"},"weight":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"}},"required":["podAffinityTerm","weight"],"type":"object"};
const schema15 = {"properties":{"labelSelector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"matchLabelKeys":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"mismatchLabelKeys":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"namespaceSelector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"namespaces":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"topologyKey":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["topologyKey"],"type":"object"};
const schema13 = {"properties":{"matchExpressions":{"$ref":"ayKrb4PFrKLMYqvjiutPxrbz750hbVgxft904f5rFtQ"},"matchLabels":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"}},"type":"object","nullable":true};

function validate28(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.matchExpressions !== undefined){
let data0 = data.matchExpressions;
const _errs2 = errors;
if(!(validate19(data0, {instancePath:instancePath+"/matchExpressions",parentData:data,parentDataProperty:"matchExpressions",rootData}))){
vErrors = vErrors === null ? validate19.errors : vErrors.concat(validate19.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.matchLabels !== undefined){
let data1 = data.matchLabels;
const _errs3 = errors;
if(!(validate15(data1, {instancePath:instancePath+"/matchLabels",parentData:data,parentDataProperty:"matchLabels",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate28.errors = vErrors;
return errors === 0;
}


function validate32(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.topologyKey === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "topologyKey"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.labelSelector !== undefined){
let data0 = data.labelSelector;
const _errs1 = errors;
if(!(validate28(data0, {instancePath:instancePath+"/labelSelector",parentData:data,parentDataProperty:"labelSelector",rootData}))){
vErrors = vErrors === null ? validate28.errors : vErrors.concat(validate28.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.matchLabelKeys !== undefined){
let data1 = data.matchLabelKeys;
const _errs2 = errors;
if(!(validate17(data1, {instancePath:instancePath+"/matchLabelKeys",parentData:data,parentDataProperty:"matchLabelKeys",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.mismatchLabelKeys !== undefined){
let data2 = data.mismatchLabelKeys;
const _errs3 = errors;
if(!(validate17(data2, {instancePath:instancePath+"/mismatchLabelKeys",parentData:data,parentDataProperty:"mismatchLabelKeys",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.namespaceSelector !== undefined){
let data3 = data.namespaceSelector;
const _errs4 = errors;
if(!(validate28(data3, {instancePath:instancePath+"/namespaceSelector",parentData:data,parentDataProperty:"namespaceSelector",rootData}))){
vErrors = vErrors === null ? validate28.errors : vErrors.concat(validate28.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.namespaces !== undefined){
let data4 = data.namespaces;
const _errs5 = errors;
if(!(validate17(data4, {instancePath:instancePath+"/namespaces",parentData:data,parentDataProperty:"namespaces",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.topologyKey !== undefined){
let data5 = data.topologyKey;
const _errs6 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/topologyKey",parentData:data,parentDataProperty:"topologyKey",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
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
validate32.errors = vErrors;
return errors === 0;
}


function validate41(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.podAffinityTerm === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "podAffinityTerm"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.weight === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "weight"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.podAffinityTerm !== undefined){
let data0 = data.podAffinityTerm;
const _errs1 = errors;
if(!(validate32(data0, {instancePath:instancePath+"/podAffinityTerm",parentData:data,parentDataProperty:"podAffinityTerm",rootData}))){
vErrors = vErrors === null ? validate32.errors : vErrors.concat(validate32.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.weight !== undefined){
let data1 = data.weight;
const _errs2 = errors;
if(!(validate31(data1, {instancePath:instancePath+"/weight",parentData:data,parentDataProperty:"weight",rootData}))){
vErrors = vErrors === null ? validate31.errors : vErrors.concat(validate31.errors);
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
validate41.errors = vErrors;
return errors === 0;
}


function validate40(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate41(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
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
validate40.errors = vErrors;
return errors === 0;
}

const schema19 = {"items":{"$ref":"8t0N05ceNTQObj1ufnNXouWS88GDJF66xuctgeD6LLg"},"type":"array","nullable":true};

function validate46(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate32(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate32.errors : vErrors.concat(validate32.errors);
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
validate46.errors = vErrors;
return errors === 0;
}


function validate39(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.preferredDuringSchedulingIgnoredDuringExecution !== undefined){
let data0 = data.preferredDuringSchedulingIgnoredDuringExecution;
const _errs2 = errors;
if(!(validate40(data0, {instancePath:instancePath+"/preferredDuringSchedulingIgnoredDuringExecution",parentData:data,parentDataProperty:"preferredDuringSchedulingIgnoredDuringExecution",rootData}))){
vErrors = vErrors === null ? validate40.errors : vErrors.concat(validate40.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.requiredDuringSchedulingIgnoredDuringExecution !== undefined){
let data1 = data.requiredDuringSchedulingIgnoredDuringExecution;
const _errs3 = errors;
if(!(validate46(data1, {instancePath:instancePath+"/requiredDuringSchedulingIgnoredDuringExecution",parentData:data,parentDataProperty:"requiredDuringSchedulingIgnoredDuringExecution",rootData}))){
vErrors = vErrors === null ? validate46.errors : vErrors.concat(validate46.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate39.errors = vErrors;
return errors === 0;
}


function validate313(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.nodeAffinity !== undefined){
let data0 = data.nodeAffinity;
const _errs2 = errors;
if(!(validate314(data0, {instancePath:instancePath+"/nodeAffinity",parentData:data,parentDataProperty:"nodeAffinity",rootData}))){
vErrors = vErrors === null ? validate314.errors : vErrors.concat(validate314.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.podAffinity !== undefined){
let data1 = data.podAffinity;
const _errs3 = errors;
if(!(validate39(data1, {instancePath:instancePath+"/podAffinity",parentData:data,parentDataProperty:"podAffinity",rootData}))){
vErrors = vErrors === null ? validate39.errors : vErrors.concat(validate39.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.podAntiAffinity !== undefined){
let data2 = data.podAntiAffinity;
const _errs4 = errors;
if(!(validate39(data2, {instancePath:instancePath+"/podAntiAffinity",parentData:data,parentDataProperty:"podAntiAffinity",rootData}))){
vErrors = vErrors === null ? validate39.errors : vErrors.concat(validate39.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate313.errors = vErrors;
return errors === 0;
}

const schema101 = {"items":{"$ref":"chE8IbiwOYvrhp6jxf4CeNPe4xowDfDQCt1Zhaa9Prk"},"type":"array"};
const schema70 = {"properties":{"args":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"command":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"env":{"$ref":"QK6qdOVPcIj3ymOL58J6NgOdTW_Xbf1pI79iGILi8uU"},"envFrom":{"$ref":"ZRw41W3mWwhKksdKQqy6gdGI0VqZa0mht_DKnlcF-lw"},"image":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"imagePullPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"lifecycle":{"$ref":"2GolHCLsHBmPr5h5-SqeL3kwFl1yyHXgHxOurrzuSpc"},"livenessProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"ports":{"$ref":"QdA0VP-JsKHKjWu-1Hu8oyYcguMz4UHlXTFbpvKweaM"},"readinessProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"resizePolicy":{"$ref":"6FxzbMy3fQv1gheBeNadVqOl2xbXIqiFGG5KNo9kRcw"},"resources":{"$ref":"FhMRS9yiPxYjQhkDxbgv-jQ4jx9VLxRdMLNZtrOnwhI"},"restartPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"restartPolicyRules":{"$ref":"bn1AnMh-2dkJyL1LY5A_xgvSFgNvRm9Wd4RL1NkeXRU"},"securityContext":{"$ref":"ppbGhvBFiV7RuYhp7GW5xoG5M7lU4kbUlUSlBvBbdzE"},"startupProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"stdin":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"stdinOnce":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"terminationMessagePath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"terminationMessagePolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"tty":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeDevices":{"$ref":"yySLqkz1jc_2D5zmwBnHmcS-y-0ogKvDTXKQGVruH8A"},"volumeMounts":{"$ref":"K3z2jQ4aYZcOrB0gSTeLWvm5O5c50b2P06GZmb9a8OM"},"workingDir":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["name"],"type":"object"};
const schema40 = {"items":{"$ref":"oh8pSA2ZWw7M7xqiGJZa1l-wmNwDLQ-3XcRRVCMnyvA"},"type":"array","nullable":true};
const schema41 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"value":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"valueFrom":{"$ref":"3Om5CxocdGRSXm0woiJiUBiNiuG5fJ71SR9MY4QT4dM"}},"required":["name"],"type":"object"};
const schema42 = {"properties":{"configMapKeyRef":{"$ref":"Ld9L-NJLX9DB_iAQL99ICueNKI_kx3YsLhY4cC2SPx8"},"fieldRef":{"$ref":"HmyyyuOupyisSiKp0rvRRJtbCek8WlFUsfGxXzcVsrg"},"fileKeyRef":{"$ref":"n4gXUrpmTuMnrzObLAcsnh7fAb5vuSAR-bi24ZVKK7o"},"resourceFieldRef":{"$ref":"lUaYvKMK_XnIMZc4H9_ERGObFXszqs8GfnzmBfc3Oh8"},"secretKeyRef":{"$ref":"Ld9L-NJLX9DB_iAQL99ICueNKI_kx3YsLhY4cC2SPx8"}},"type":"object","nullable":true};
const schema20 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"required":["key"],"type":"object","nullable":true};
const schema21 = {"default":"","type":"string","nullable":true};

function validate51(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate51.errors = vErrors;
return errors === 0;
}


function validate49(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.key === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "key"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.key !== undefined){
let data0 = data.key;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.name !== undefined){
let data1 = data.name;
const _errs3 = errors;
if(!(validate51(data1, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate51.errors : vErrors.concat(validate51.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.optional !== undefined){
let data2 = data.optional;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate49.errors = vErrors;
return errors === 0;
}

const schema43 = {"properties":{"apiVersion":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"fieldPath":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["fieldPath"],"type":"object","nullable":true};

function validate115(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.fieldPath === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "fieldPath"}};
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
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/apiVersion",parentData:data,parentDataProperty:"apiVersion",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fieldPath !== undefined){
let data1 = data.fieldPath;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/fieldPath",parentData:data,parentDataProperty:"fieldPath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate115.errors = vErrors;
return errors === 0;
}

const schema44 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"optional":{"$ref":"PxmoJm38YOn1dCbyB5PLDDQXV2B2rqFhpzNXj0_ibe4"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"volumeName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["key","path","volumeName"],"type":"object","nullable":true};
const schema45 = {"default":false,"type":"boolean","nullable":true};

function validate121(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate121.errors = vErrors;
return errors === 0;
}


function validate119(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.key === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "key"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.path === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "path"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.volumeName === undefined){
const err3 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "volumeName"}};
if(vErrors === null){
vErrors = [err3];
}
else {
vErrors.push(err3);
}
errors++;
}
if(data.key !== undefined){
let data0 = data.key;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.optional !== undefined){
let data1 = data.optional;
const _errs3 = errors;
if(!(validate121(data1, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate121.errors : vErrors.concat(validate121.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.path !== undefined){
let data2 = data.path;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.volumeName !== undefined){
let data3 = data.volumeName;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/volumeName",parentData:data,parentDataProperty:"volumeName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate119.errors = vErrors;
return errors === 0;
}

const schema46 = {"properties":{"containerName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"divisor":{"$ref":"3dmDSoHI3Hv64ppBBbqAEh3HzVwqzFWb_nrM2BhHTgs"},"resource":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["resource"],"type":"object","nullable":true};
const schema37 = {"anyOf":[{"$ref":"laYO4ZujiLzTOvPomluoS9e439ISWe2RqSMRunp3ReI"},{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}],"pattern":"^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"};
const schema23 = {"type":"integer"};

function validate57(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate57.errors = vErrors;
return errors === 0;
}

const pattern0 = new RegExp("^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$", "u");

function validate101(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
let valid0 = false;
const _errs1 = errors;
if(!(validate57(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate57.errors : vErrors.concat(validate57.errors);
errors = vErrors.length;
}
else {
}
var _valid0 = _errs1 === errors;
valid0 = valid0 || _valid0;
if(!valid0){
const _errs2 = errors;
if(!(validate1(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate101.errors = vErrors;
return errors === 0;
}


function validate126(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.resource === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "resource"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.containerName !== undefined){
let data0 = data.containerName;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/containerName",parentData:data,parentDataProperty:"containerName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.divisor !== undefined){
let data1 = data.divisor;
const _errs3 = errors;
if(!(validate101(data1, {instancePath:instancePath+"/divisor",parentData:data,parentDataProperty:"divisor",rootData}))){
vErrors = vErrors === null ? validate101.errors : vErrors.concat(validate101.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.resource !== undefined){
let data2 = data.resource;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/resource",parentData:data,parentDataProperty:"resource",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate126.errors = vErrors;
return errors === 0;
}


function validate113(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.configMapKeyRef !== undefined){
let data0 = data.configMapKeyRef;
const _errs2 = errors;
if(!(validate49(data0, {instancePath:instancePath+"/configMapKeyRef",parentData:data,parentDataProperty:"configMapKeyRef",rootData}))){
vErrors = vErrors === null ? validate49.errors : vErrors.concat(validate49.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fieldRef !== undefined){
let data1 = data.fieldRef;
const _errs3 = errors;
if(!(validate115(data1, {instancePath:instancePath+"/fieldRef",parentData:data,parentDataProperty:"fieldRef",rootData}))){
vErrors = vErrors === null ? validate115.errors : vErrors.concat(validate115.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.fileKeyRef !== undefined){
let data2 = data.fileKeyRef;
const _errs4 = errors;
if(!(validate119(data2, {instancePath:instancePath+"/fileKeyRef",parentData:data,parentDataProperty:"fileKeyRef",rootData}))){
vErrors = vErrors === null ? validate119.errors : vErrors.concat(validate119.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.resourceFieldRef !== undefined){
let data3 = data.resourceFieldRef;
const _errs5 = errors;
if(!(validate126(data3, {instancePath:instancePath+"/resourceFieldRef",parentData:data,parentDataProperty:"resourceFieldRef",rootData}))){
vErrors = vErrors === null ? validate126.errors : vErrors.concat(validate126.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.secretKeyRef !== undefined){
let data4 = data.secretKeyRef;
const _errs6 = errors;
if(!(validate49(data4, {instancePath:instancePath+"/secretKeyRef",parentData:data,parentDataProperty:"secretKeyRef",rootData}))){
vErrors = vErrors === null ? validate49.errors : vErrors.concat(validate49.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate113.errors = vErrors;
return errors === 0;
}


function validate110(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.name !== undefined){
let data0 = data.name;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.value !== undefined){
let data1 = data.value;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/value",parentData:data,parentDataProperty:"value",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.valueFrom !== undefined){
let data2 = data.valueFrom;
const _errs3 = errors;
if(!(validate113(data2, {instancePath:instancePath+"/valueFrom",parentData:data,parentDataProperty:"valueFrom",rootData}))){
vErrors = vErrors === null ? validate113.errors : vErrors.concat(validate113.errors);
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
validate110.errors = vErrors;
return errors === 0;
}


function validate109(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate110(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate110.errors : vErrors.concat(validate110.errors);
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
validate109.errors = vErrors;
return errors === 0;
}

const schema47 = {"items":{"$ref":"HHPN67ks7XM2URGVvIHxDM_VLhU9JKL3x7CLOTcCXdk"},"type":"array","nullable":true};
const schema48 = {"properties":{"configMapRef":{"$ref":"u-QkpzKhtzKCL4fqRT1udeFHaK0vv8tqmNxAND2hxII"},"prefix":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"secretRef":{"$ref":"u-QkpzKhtzKCL4fqRT1udeFHaK0vv8tqmNxAND2hxII"}},"type":"object"};
const schema22 = {"properties":{"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"type":"object","nullable":true};

function validate54(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate51(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate51.errors : vErrors.concat(validate51.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.optional !== undefined){
let data1 = data.optional;
const _errs3 = errors;
if(!(validate2(data1, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate54.errors = vErrors;
return errors === 0;
}


function validate135(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.configMapRef !== undefined){
let data0 = data.configMapRef;
const _errs1 = errors;
if(!(validate54(data0, {instancePath:instancePath+"/configMapRef",parentData:data,parentDataProperty:"configMapRef",rootData}))){
vErrors = vErrors === null ? validate54.errors : vErrors.concat(validate54.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.prefix !== undefined){
let data1 = data.prefix;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/prefix",parentData:data,parentDataProperty:"prefix",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.secretRef !== undefined){
let data2 = data.secretRef;
const _errs3 = errors;
if(!(validate54(data2, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate54.errors : vErrors.concat(validate54.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate135.errors = vErrors;
return errors === 0;
}


function validate134(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate135(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate135.errors : vErrors.concat(validate135.errors);
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
validate134.errors = vErrors;
return errors === 0;
}

const schema49 = {"properties":{"postStart":{"$ref":"bI1FYAC1lVqZU-yavNa7Tnj1QxPBg6vSykf7ECTWEso"},"preStop":{"$ref":"bI1FYAC1lVqZU-yavNa7Tnj1QxPBg6vSykf7ECTWEso"},"stopSignal":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};
const schema25 = {"properties":{"exec":{"$ref":"9C0OixFARi4hI87PTOihZwsBCgmS5p3ncDGfVlnGaWE"},"httpGet":{"$ref":"AvymdmXaS2uRanVsPLWMQgXBYDVVb7RMCKXx61FTpv8"},"sleep":{"$ref":"O9c6rU4oPqcIn0IAR6GJa06bNcsqOvXcnvmWF6hjOas"},"tcpSocket":{"$ref":"kUbDk1_JH4_cSX_6OddmnZt6rvEGMGnt00AZtEGZzIE"}},"type":"object","nullable":true};
const schema26 = {"properties":{"command":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};

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
if(data.command !== undefined){
let data0 = data.command;
const _errs2 = errors;
if(!(validate17(data0, {instancePath:instancePath+"/command",parentData:data,parentDataProperty:"command",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate62.errors = vErrors;
return errors === 0;
}

const schema27 = {"properties":{"host":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"httpHeaders":{"$ref":"MsS0666yf0HSUh6mG0cBqbw9ThqZk9-DowZaIzPios0"},"path":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"port":{"$ref":"ih-pWOC033UnwvHDNXL0jVD9FR7FP484ObfuKTqXkMU"},"scheme":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["port"],"type":"object","nullable":true};
const schema28 = {"items":{"$ref":"0EjQDQuCE_EGeSNjOUachoDmgPAdJKL5JCdMDcFxJHs"},"type":"array","nullable":true};
const schema29 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"value":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["name","value"],"type":"object"};

function validate68(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.value === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "value"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.name !== undefined){
let data0 = data.name;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.value !== undefined){
let data1 = data.value;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/value",parentData:data,parentDataProperty:"value",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate68.errors = vErrors;
return errors === 0;
}


function validate67(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate68(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate68.errors : vErrors.concat(validate68.errors);
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
validate67.errors = vErrors;
return errors === 0;
}

const schema24 = {"anyOf":[{"$ref":"laYO4ZujiLzTOvPomluoS9e439ISWe2RqSMRunp3ReI"},{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}]};

function validate58(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
let valid0 = false;
const _errs1 = errors;
if(!(validate57(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate57.errors : vErrors.concat(validate57.errors);
errors = vErrors.length;
}
else {
}
var _valid0 = _errs1 === errors;
valid0 = valid0 || _valid0;
if(!valid0){
const _errs2 = errors;
if(!(validate1(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate58.errors = vErrors;
return errors === 0;
}


function validate65(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.port === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "port"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.host !== undefined){
let data0 = data.host;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/host",parentData:data,parentDataProperty:"host",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.httpHeaders !== undefined){
let data1 = data.httpHeaders;
const _errs3 = errors;
if(!(validate67(data1, {instancePath:instancePath+"/httpHeaders",parentData:data,parentDataProperty:"httpHeaders",rootData}))){
vErrors = vErrors === null ? validate67.errors : vErrors.concat(validate67.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.path !== undefined){
let data2 = data.path;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.port !== undefined){
let data3 = data.port;
const _errs5 = errors;
if(!(validate58(data3, {instancePath:instancePath+"/port",parentData:data,parentDataProperty:"port",rootData}))){
vErrors = vErrors === null ? validate58.errors : vErrors.concat(validate58.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.scheme !== undefined){
let data4 = data.scheme;
const _errs6 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/scheme",parentData:data,parentDataProperty:"scheme",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate65.errors = vErrors;
return errors === 0;
}

const schema30 = {"properties":{"seconds":{"$ref":"0YQm7z5aN2fg7S8XlbxrquRU6pdIr97uCbtQYyjv3S4"}},"required":["seconds"],"type":"object","nullable":true};
const schema31 = {"format":"int64","type":"integer"};

function validate78(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(formats2.validate(data))){
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
validate78.errors = vErrors;
return errors === 0;
}


function validate77(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.seconds === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "seconds"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.seconds !== undefined){
let data0 = data.seconds;
const _errs2 = errors;
if(!(validate78(data0, {instancePath:instancePath+"/seconds",parentData:data,parentDataProperty:"seconds",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate77.errors = vErrors;
return errors === 0;
}

const schema32 = {"properties":{"host":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"port":{"$ref":"ih-pWOC033UnwvHDNXL0jVD9FR7FP484ObfuKTqXkMU"}},"required":["port"],"type":"object","nullable":true};

function validate81(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.port === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "port"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.host !== undefined){
let data0 = data.host;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/host",parentData:data,parentDataProperty:"host",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.port !== undefined){
let data1 = data.port;
const _errs3 = errors;
if(!(validate58(data1, {instancePath:instancePath+"/port",parentData:data,parentDataProperty:"port",rootData}))){
vErrors = vErrors === null ? validate58.errors : vErrors.concat(validate58.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate81.errors = vErrors;
return errors === 0;
}


function validate61(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate62(data0, {instancePath:instancePath+"/exec",parentData:data,parentDataProperty:"exec",rootData}))){
vErrors = vErrors === null ? validate62.errors : vErrors.concat(validate62.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.httpGet !== undefined){
let data1 = data.httpGet;
const _errs3 = errors;
if(!(validate65(data1, {instancePath:instancePath+"/httpGet",parentData:data,parentDataProperty:"httpGet",rootData}))){
vErrors = vErrors === null ? validate65.errors : vErrors.concat(validate65.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.sleep !== undefined){
let data2 = data.sleep;
const _errs4 = errors;
if(!(validate77(data2, {instancePath:instancePath+"/sleep",parentData:data,parentDataProperty:"sleep",rootData}))){
vErrors = vErrors === null ? validate77.errors : vErrors.concat(validate77.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.tcpSocket !== undefined){
let data3 = data.tcpSocket;
const _errs5 = errors;
if(!(validate81(data3, {instancePath:instancePath+"/tcpSocket",parentData:data,parentDataProperty:"tcpSocket",rootData}))){
vErrors = vErrors === null ? validate81.errors : vErrors.concat(validate81.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate61.errors = vErrors;
return errors === 0;
}


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
if(data.postStart !== undefined){
let data0 = data.postStart;
const _errs2 = errors;
if(!(validate61(data0, {instancePath:instancePath+"/postStart",parentData:data,parentDataProperty:"postStart",rootData}))){
vErrors = vErrors === null ? validate61.errors : vErrors.concat(validate61.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.preStop !== undefined){
let data1 = data.preStop;
const _errs3 = errors;
if(!(validate61(data1, {instancePath:instancePath+"/preStop",parentData:data,parentDataProperty:"preStop",rootData}))){
vErrors = vErrors === null ? validate61.errors : vErrors.concat(validate61.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.stopSignal !== undefined){
let data2 = data.stopSignal;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/stopSignal",parentData:data,parentDataProperty:"stopSignal",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate140.errors = vErrors;
return errors === 0;
}

const schema35 = {"properties":{"exec":{"$ref":"9C0OixFARi4hI87PTOihZwsBCgmS5p3ncDGfVlnGaWE"},"failureThreshold":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"grpc":{"$ref":"TPbd1UVnDN2xj-Iuf9RvwkhMgTdHQQ3lBf7qZ98euNo"},"httpGet":{"$ref":"AvymdmXaS2uRanVsPLWMQgXBYDVVb7RMCKXx61FTpv8"},"initialDelaySeconds":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"periodSeconds":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"successThreshold":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"tcpSocket":{"$ref":"kUbDk1_JH4_cSX_6OddmnZt6rvEGMGnt00AZtEGZzIE"},"terminationGracePeriodSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"timeoutSeconds":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"}},"type":"object","nullable":true};
const schema33 = {"format":"int32","type":"integer","nullable":true};

function validate85(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate85.errors = vErrors;
return errors === 0;
}

const schema36 = {"properties":{"port":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"service":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"}},"required":["port"],"type":"object","nullable":true};

function validate90(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.port === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "port"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.port !== undefined){
let data0 = data.port;
const _errs2 = errors;
if(!(validate31(data0, {instancePath:instancePath+"/port",parentData:data,parentDataProperty:"port",rootData}))){
vErrors = vErrors === null ? validate31.errors : vErrors.concat(validate31.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.service !== undefined){
let data1 = data.service;
const _errs3 = errors;
if(!(validate51(data1, {instancePath:instancePath+"/service",parentData:data,parentDataProperty:"service",rootData}))){
vErrors = vErrors === null ? validate51.errors : vErrors.concat(validate51.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate90.errors = vErrors;
return errors === 0;
}


function validate87(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate62(data0, {instancePath:instancePath+"/exec",parentData:data,parentDataProperty:"exec",rootData}))){
vErrors = vErrors === null ? validate62.errors : vErrors.concat(validate62.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.failureThreshold !== undefined){
let data1 = data.failureThreshold;
const _errs3 = errors;
if(!(validate85(data1, {instancePath:instancePath+"/failureThreshold",parentData:data,parentDataProperty:"failureThreshold",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.grpc !== undefined){
let data2 = data.grpc;
const _errs4 = errors;
if(!(validate90(data2, {instancePath:instancePath+"/grpc",parentData:data,parentDataProperty:"grpc",rootData}))){
vErrors = vErrors === null ? validate90.errors : vErrors.concat(validate90.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.httpGet !== undefined){
let data3 = data.httpGet;
const _errs5 = errors;
if(!(validate65(data3, {instancePath:instancePath+"/httpGet",parentData:data,parentDataProperty:"httpGet",rootData}))){
vErrors = vErrors === null ? validate65.errors : vErrors.concat(validate65.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.initialDelaySeconds !== undefined){
let data4 = data.initialDelaySeconds;
const _errs6 = errors;
if(!(validate85(data4, {instancePath:instancePath+"/initialDelaySeconds",parentData:data,parentDataProperty:"initialDelaySeconds",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.periodSeconds !== undefined){
let data5 = data.periodSeconds;
const _errs7 = errors;
if(!(validate85(data5, {instancePath:instancePath+"/periodSeconds",parentData:data,parentDataProperty:"periodSeconds",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.successThreshold !== undefined){
let data6 = data.successThreshold;
const _errs8 = errors;
if(!(validate85(data6, {instancePath:instancePath+"/successThreshold",parentData:data,parentDataProperty:"successThreshold",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.tcpSocket !== undefined){
let data7 = data.tcpSocket;
const _errs9 = errors;
if(!(validate81(data7, {instancePath:instancePath+"/tcpSocket",parentData:data,parentDataProperty:"tcpSocket",rootData}))){
vErrors = vErrors === null ? validate81.errors : vErrors.concat(validate81.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.terminationGracePeriodSeconds !== undefined){
let data8 = data.terminationGracePeriodSeconds;
const _errs10 = errors;
if(!(validate86(data8, {instancePath:instancePath+"/terminationGracePeriodSeconds",parentData:data,parentDataProperty:"terminationGracePeriodSeconds",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.timeoutSeconds !== undefined){
let data9 = data.timeoutSeconds;
const _errs11 = errors;
if(!(validate85(data9, {instancePath:instancePath+"/timeoutSeconds",parentData:data,parentDataProperty:"timeoutSeconds",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
}
validate87.errors = vErrors;
return errors === 0;
}

const schema50 = {"items":{"$ref":"W4Z_n5BnB66USjWh05DixBIRyKBkgB0p3XT4-b20o7U"},"type":"array","nullable":true};
const schema51 = {"properties":{"containerPort":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"hostIP":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"hostPort":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"protocol":{"$ref":"clxWPOfeflcI7GT0mKf9XOAew5S3Ye83ruoYctXlNVY"}},"required":["containerPort"],"type":"object"};
const schema52 = {"default":"TCP","type":"string","nullable":true};

function validate150(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate150.errors = vErrors;
return errors === 0;
}


function validate145(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.containerPort === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "containerPort"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.containerPort !== undefined){
let data0 = data.containerPort;
const _errs1 = errors;
if(!(validate31(data0, {instancePath:instancePath+"/containerPort",parentData:data,parentDataProperty:"containerPort",rootData}))){
vErrors = vErrors === null ? validate31.errors : vErrors.concat(validate31.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.hostIP !== undefined){
let data1 = data.hostIP;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/hostIP",parentData:data,parentDataProperty:"hostIP",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.hostPort !== undefined){
let data2 = data.hostPort;
const _errs3 = errors;
if(!(validate85(data2, {instancePath:instancePath+"/hostPort",parentData:data,parentDataProperty:"hostPort",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.name !== undefined){
let data3 = data.name;
const _errs4 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.protocol !== undefined){
let data4 = data.protocol;
const _errs5 = errors;
if(!(validate150(data4, {instancePath:instancePath+"/protocol",parentData:data,parentDataProperty:"protocol",rootData}))){
vErrors = vErrors === null ? validate150.errors : vErrors.concat(validate150.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
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
validate145.errors = vErrors;
return errors === 0;
}


function validate144(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate145(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate145.errors : vErrors.concat(validate145.errors);
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
validate144.errors = vErrors;
return errors === 0;
}

const schema53 = {"items":{"$ref":"7TSxkO796UTLbFFdnMD77Gk_wJkH64SQBmtdtNBI1DI"},"type":"array","nullable":true};
const schema54 = {"properties":{"resourceName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"restartPolicy":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["resourceName","restartPolicy"],"type":"object"};

function validate154(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.resourceName === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "resourceName"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.restartPolicy === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "restartPolicy"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.resourceName !== undefined){
let data0 = data.resourceName;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/resourceName",parentData:data,parentDataProperty:"resourceName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.restartPolicy !== undefined){
let data1 = data.restartPolicy;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/restartPolicy",parentData:data,parentDataProperty:"restartPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate154.errors = vErrors;
return errors === 0;
}


function validate153(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate154(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate154.errors : vErrors.concat(validate154.errors);
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
validate153.errors = vErrors;
return errors === 0;
}

const schema55 = {"properties":{"claims":{"$ref":"jARlz_jRCwuBtBAPv6O31_X-vye3IHpN8UfU2yAeEBc"},"limits":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"},"requests":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"}},"type":"object","nullable":true};
const schema56 = {"items":{"$ref":"fBVfIKFFliLviCbz7ba4Y3cixNwtkbdwlBfhQb5Wskg"},"type":"array","nullable":true};
const schema57 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"request":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["name"],"type":"object"};

function validate160(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.name !== undefined){
let data0 = data.name;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.request !== undefined){
let data1 = data.request;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/request",parentData:data,parentDataProperty:"request",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
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
validate160.errors = vErrors;
return errors === 0;
}


function validate159(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate160(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate160.errors : vErrors.concat(validate160.errors);
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
validate159.errors = vErrors;
return errors === 0;
}

const schema38 = {"additionalProperties":{"$ref":"3dmDSoHI3Hv64ppBBbqAEh3HzVwqzFWb_nrM2BhHTgs"},"type":"object","properties":{},"nullable":true};

function validate104(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate101(data0, {instancePath:instancePath+"/" + key0.replace(/~/g, "~0").replace(/\//g, "~1"),parentData:data,parentDataProperty:key0,rootData}))){
vErrors = vErrors === null ? validate101.errors : vErrors.concat(validate101.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate104.errors = vErrors;
return errors === 0;
}


function validate158(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.claims !== undefined){
let data0 = data.claims;
const _errs2 = errors;
if(!(validate159(data0, {instancePath:instancePath+"/claims",parentData:data,parentDataProperty:"claims",rootData}))){
vErrors = vErrors === null ? validate159.errors : vErrors.concat(validate159.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.limits !== undefined){
let data1 = data.limits;
const _errs3 = errors;
if(!(validate104(data1, {instancePath:instancePath+"/limits",parentData:data,parentDataProperty:"limits",rootData}))){
vErrors = vErrors === null ? validate104.errors : vErrors.concat(validate104.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.requests !== undefined){
let data2 = data.requests;
const _errs4 = errors;
if(!(validate104(data2, {instancePath:instancePath+"/requests",parentData:data,parentDataProperty:"requests",rootData}))){
vErrors = vErrors === null ? validate104.errors : vErrors.concat(validate104.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate158.errors = vErrors;
return errors === 0;
}

const schema58 = {"items":{"$ref":"tySkinukrNUkRdT5RTlRnJ6Q5BCp7PH4cvcjnXQi7qk"},"type":"array","nullable":true};
const schema59 = {"properties":{"action":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"exitCodes":{"$ref":"kC8sbNbSbDgzM2lKWJrZR2A3aIX9Qs7YPcegJUJ89mw"}},"required":["action"],"type":"object"};
const schema60 = {"properties":{"operator":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"values":{"$ref":"vFZBaVxDYEw8WDgeqavILxL3SULZ27GRcVk6BpSH3Rc"}},"required":["operator"],"type":"object","nullable":true};
const schema61 = {"items":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"type":"array","nullable":true};

function validate172(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate31(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate31.errors : vErrors.concat(validate31.errors);
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
validate172.errors = vErrors;
return errors === 0;
}


function validate170(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.operator === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "operator"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.operator !== undefined){
let data0 = data.operator;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/operator",parentData:data,parentDataProperty:"operator",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.values !== undefined){
let data1 = data.values;
const _errs3 = errors;
if(!(validate172(data1, {instancePath:instancePath+"/values",parentData:data,parentDataProperty:"values",rootData}))){
vErrors = vErrors === null ? validate172.errors : vErrors.concat(validate172.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate170.errors = vErrors;
return errors === 0;
}


function validate168(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.action === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "action"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.action !== undefined){
let data0 = data.action;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/action",parentData:data,parentDataProperty:"action",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.exitCodes !== undefined){
let data1 = data.exitCodes;
const _errs2 = errors;
if(!(validate170(data1, {instancePath:instancePath+"/exitCodes",parentData:data,parentDataProperty:"exitCodes",rootData}))){
vErrors = vErrors === null ? validate170.errors : vErrors.concat(validate170.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
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
validate168.errors = vErrors;
return errors === 0;
}


function validate167(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate168(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate168.errors : vErrors.concat(validate168.errors);
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
validate167.errors = vErrors;
return errors === 0;
}

const schema62 = {"properties":{"allowPrivilegeEscalation":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"appArmorProfile":{"$ref":"aMnE7Im_y76-Nz9q0hhh6yja57sQFej02mLc00OwhtE"},"capabilities":{"$ref":"ihLpIC0AJcbzOXQ910bylE0NrJtM_usuKt6HWO2C5R8"},"privileged":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"procMount":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnlyRootFilesystem":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"runAsGroup":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"runAsNonRoot":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"runAsUser":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"seLinuxOptions":{"$ref":"RAvVUd1y4DBTzL23WWML0HifcsQtvUbbJLKVUvzXXO4"},"seccompProfile":{"$ref":"aMnE7Im_y76-Nz9q0hhh6yja57sQFej02mLc00OwhtE"},"windowsOptions":{"$ref":"6z1i1A1Aq99luryppL8NHWpsKmtBfItfBpL24p91f_I"}},"type":"object","nullable":true};
const schema39 = {"properties":{"localhostProfile":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"type":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["type"],"type":"object","nullable":true};

function validate106(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.localhostProfile !== undefined){
let data0 = data.localhostProfile;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/localhostProfile",parentData:data,parentDataProperty:"localhostProfile",rootData}))){
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
validate106.errors = vErrors;
return errors === 0;
}

const schema63 = {"properties":{"add":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"drop":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};

function validate180(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.add !== undefined){
let data0 = data.add;
const _errs2 = errors;
if(!(validate17(data0, {instancePath:instancePath+"/add",parentData:data,parentDataProperty:"add",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.drop !== undefined){
let data1 = data.drop;
const _errs3 = errors;
if(!(validate17(data1, {instancePath:instancePath+"/drop",parentData:data,parentDataProperty:"drop",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate180.errors = vErrors;
return errors === 0;
}

const schema64 = {"properties":{"level":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"role":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"type":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"user":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate190(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.level !== undefined){
let data0 = data.level;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/level",parentData:data,parentDataProperty:"level",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.role !== undefined){
let data1 = data.role;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/role",parentData:data,parentDataProperty:"role",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.type !== undefined){
let data2 = data.type;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.user !== undefined){
let data3 = data.user;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/user",parentData:data,parentDataProperty:"user",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate190.errors = vErrors;
return errors === 0;
}

const schema65 = {"properties":{"gmsaCredentialSpec":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"gmsaCredentialSpecName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"hostProcess":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"runAsUserName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate197(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.gmsaCredentialSpec !== undefined){
let data0 = data.gmsaCredentialSpec;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/gmsaCredentialSpec",parentData:data,parentDataProperty:"gmsaCredentialSpec",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.gmsaCredentialSpecName !== undefined){
let data1 = data.gmsaCredentialSpecName;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/gmsaCredentialSpecName",parentData:data,parentDataProperty:"gmsaCredentialSpecName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.hostProcess !== undefined){
let data2 = data.hostProcess;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/hostProcess",parentData:data,parentDataProperty:"hostProcess",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.runAsUserName !== undefined){
let data3 = data.runAsUserName;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/runAsUserName",parentData:data,parentDataProperty:"runAsUserName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate197.errors = vErrors;
return errors === 0;
}


function validate177(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.allowPrivilegeEscalation !== undefined){
let data0 = data.allowPrivilegeEscalation;
const _errs2 = errors;
if(!(validate2(data0, {instancePath:instancePath+"/allowPrivilegeEscalation",parentData:data,parentDataProperty:"allowPrivilegeEscalation",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.appArmorProfile !== undefined){
let data1 = data.appArmorProfile;
const _errs3 = errors;
if(!(validate106(data1, {instancePath:instancePath+"/appArmorProfile",parentData:data,parentDataProperty:"appArmorProfile",rootData}))){
vErrors = vErrors === null ? validate106.errors : vErrors.concat(validate106.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.capabilities !== undefined){
let data2 = data.capabilities;
const _errs4 = errors;
if(!(validate180(data2, {instancePath:instancePath+"/capabilities",parentData:data,parentDataProperty:"capabilities",rootData}))){
vErrors = vErrors === null ? validate180.errors : vErrors.concat(validate180.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.privileged !== undefined){
let data3 = data.privileged;
const _errs5 = errors;
if(!(validate2(data3, {instancePath:instancePath+"/privileged",parentData:data,parentDataProperty:"privileged",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.procMount !== undefined){
let data4 = data.procMount;
const _errs6 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/procMount",parentData:data,parentDataProperty:"procMount",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.readOnlyRootFilesystem !== undefined){
let data5 = data.readOnlyRootFilesystem;
const _errs7 = errors;
if(!(validate2(data5, {instancePath:instancePath+"/readOnlyRootFilesystem",parentData:data,parentDataProperty:"readOnlyRootFilesystem",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.runAsGroup !== undefined){
let data6 = data.runAsGroup;
const _errs8 = errors;
if(!(validate86(data6, {instancePath:instancePath+"/runAsGroup",parentData:data,parentDataProperty:"runAsGroup",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.runAsNonRoot !== undefined){
let data7 = data.runAsNonRoot;
const _errs9 = errors;
if(!(validate2(data7, {instancePath:instancePath+"/runAsNonRoot",parentData:data,parentDataProperty:"runAsNonRoot",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.runAsUser !== undefined){
let data8 = data.runAsUser;
const _errs10 = errors;
if(!(validate86(data8, {instancePath:instancePath+"/runAsUser",parentData:data,parentDataProperty:"runAsUser",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.seLinuxOptions !== undefined){
let data9 = data.seLinuxOptions;
const _errs11 = errors;
if(!(validate190(data9, {instancePath:instancePath+"/seLinuxOptions",parentData:data,parentDataProperty:"seLinuxOptions",rootData}))){
vErrors = vErrors === null ? validate190.errors : vErrors.concat(validate190.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.seccompProfile !== undefined){
let data10 = data.seccompProfile;
const _errs12 = errors;
if(!(validate106(data10, {instancePath:instancePath+"/seccompProfile",parentData:data,parentDataProperty:"seccompProfile",rootData}))){
vErrors = vErrors === null ? validate106.errors : vErrors.concat(validate106.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.windowsOptions !== undefined){
let data11 = data.windowsOptions;
const _errs13 = errors;
if(!(validate197(data11, {instancePath:instancePath+"/windowsOptions",parentData:data,parentDataProperty:"windowsOptions",rootData}))){
vErrors = vErrors === null ? validate197.errors : vErrors.concat(validate197.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
}
validate177.errors = vErrors;
return errors === 0;
}

const schema66 = {"items":{"$ref":"OVr1UXw7GhHO0fgY9cIvIVSoMiw0xn85m1qUa_o9V8U"},"type":"array","nullable":true};
const schema67 = {"properties":{"devicePath":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["devicePath","name"],"type":"object"};

function validate204(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.devicePath === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "devicePath"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.name === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.devicePath !== undefined){
let data0 = data.devicePath;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/devicePath",parentData:data,parentDataProperty:"devicePath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.name !== undefined){
let data1 = data.name;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate204.errors = vErrors;
return errors === 0;
}


function validate203(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate204(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate204.errors : vErrors.concat(validate204.errors);
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
validate203.errors = vErrors;
return errors === 0;
}

const schema68 = {"items":{"$ref":"77fVc4wpT49BE5gRmrKBt_Stn6VnY9fBPevvzJEor0U"},"type":"array","nullable":true};
const schema69 = {"properties":{"mountPath":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"mountPropagation":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"recursiveReadOnly":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"subPath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"subPathExpr":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["mountPath","name"],"type":"object"};

function validate209(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.mountPath === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "mountPath"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.name === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.mountPath !== undefined){
let data0 = data.mountPath;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/mountPath",parentData:data,parentDataProperty:"mountPath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.mountPropagation !== undefined){
let data1 = data.mountPropagation;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/mountPropagation",parentData:data,parentDataProperty:"mountPropagation",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.name !== undefined){
let data2 = data.name;
const _errs3 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.readOnly !== undefined){
let data3 = data.readOnly;
const _errs4 = errors;
if(!(validate2(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.recursiveReadOnly !== undefined){
let data4 = data.recursiveReadOnly;
const _errs5 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/recursiveReadOnly",parentData:data,parentDataProperty:"recursiveReadOnly",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.subPath !== undefined){
let data5 = data.subPath;
const _errs6 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/subPath",parentData:data,parentDataProperty:"subPath",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.subPathExpr !== undefined){
let data6 = data.subPathExpr;
const _errs7 = errors;
if(!(validate0(data6, {instancePath:instancePath+"/subPathExpr",parentData:data,parentDataProperty:"subPathExpr",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
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
validate209.errors = vErrors;
return errors === 0;
}


function validate208(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate209(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate209.errors : vErrors.concat(validate209.errors);
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
validate208.errors = vErrors;
return errors === 0;
}


function validate218(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.args !== undefined){
let data0 = data.args;
const _errs1 = errors;
if(!(validate17(data0, {instancePath:instancePath+"/args",parentData:data,parentDataProperty:"args",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.command !== undefined){
let data1 = data.command;
const _errs2 = errors;
if(!(validate17(data1, {instancePath:instancePath+"/command",parentData:data,parentDataProperty:"command",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.env !== undefined){
let data2 = data.env;
const _errs3 = errors;
if(!(validate109(data2, {instancePath:instancePath+"/env",parentData:data,parentDataProperty:"env",rootData}))){
vErrors = vErrors === null ? validate109.errors : vErrors.concat(validate109.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.envFrom !== undefined){
let data3 = data.envFrom;
const _errs4 = errors;
if(!(validate134(data3, {instancePath:instancePath+"/envFrom",parentData:data,parentDataProperty:"envFrom",rootData}))){
vErrors = vErrors === null ? validate134.errors : vErrors.concat(validate134.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.image !== undefined){
let data4 = data.image;
const _errs5 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.imagePullPolicy !== undefined){
let data5 = data.imagePullPolicy;
const _errs6 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/imagePullPolicy",parentData:data,parentDataProperty:"imagePullPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.lifecycle !== undefined){
let data6 = data.lifecycle;
const _errs7 = errors;
if(!(validate140(data6, {instancePath:instancePath+"/lifecycle",parentData:data,parentDataProperty:"lifecycle",rootData}))){
vErrors = vErrors === null ? validate140.errors : vErrors.concat(validate140.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.livenessProbe !== undefined){
let data7 = data.livenessProbe;
const _errs8 = errors;
if(!(validate87(data7, {instancePath:instancePath+"/livenessProbe",parentData:data,parentDataProperty:"livenessProbe",rootData}))){
vErrors = vErrors === null ? validate87.errors : vErrors.concat(validate87.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.name !== undefined){
let data8 = data.name;
const _errs9 = errors;
if(!(validate1(data8, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.ports !== undefined){
let data9 = data.ports;
const _errs10 = errors;
if(!(validate144(data9, {instancePath:instancePath+"/ports",parentData:data,parentDataProperty:"ports",rootData}))){
vErrors = vErrors === null ? validate144.errors : vErrors.concat(validate144.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.readinessProbe !== undefined){
let data10 = data.readinessProbe;
const _errs11 = errors;
if(!(validate87(data10, {instancePath:instancePath+"/readinessProbe",parentData:data,parentDataProperty:"readinessProbe",rootData}))){
vErrors = vErrors === null ? validate87.errors : vErrors.concat(validate87.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.resizePolicy !== undefined){
let data11 = data.resizePolicy;
const _errs12 = errors;
if(!(validate153(data11, {instancePath:instancePath+"/resizePolicy",parentData:data,parentDataProperty:"resizePolicy",rootData}))){
vErrors = vErrors === null ? validate153.errors : vErrors.concat(validate153.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.resources !== undefined){
let data12 = data.resources;
const _errs13 = errors;
if(!(validate158(data12, {instancePath:instancePath+"/resources",parentData:data,parentDataProperty:"resources",rootData}))){
vErrors = vErrors === null ? validate158.errors : vErrors.concat(validate158.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.restartPolicy !== undefined){
let data13 = data.restartPolicy;
const _errs14 = errors;
if(!(validate0(data13, {instancePath:instancePath+"/restartPolicy",parentData:data,parentDataProperty:"restartPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
if(data.restartPolicyRules !== undefined){
let data14 = data.restartPolicyRules;
const _errs15 = errors;
if(!(validate167(data14, {instancePath:instancePath+"/restartPolicyRules",parentData:data,parentDataProperty:"restartPolicyRules",rootData}))){
vErrors = vErrors === null ? validate167.errors : vErrors.concat(validate167.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs15 === errors;
}
if(data.securityContext !== undefined){
let data15 = data.securityContext;
const _errs16 = errors;
if(!(validate177(data15, {instancePath:instancePath+"/securityContext",parentData:data,parentDataProperty:"securityContext",rootData}))){
vErrors = vErrors === null ? validate177.errors : vErrors.concat(validate177.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs16 === errors;
}
if(data.startupProbe !== undefined){
let data16 = data.startupProbe;
const _errs17 = errors;
if(!(validate87(data16, {instancePath:instancePath+"/startupProbe",parentData:data,parentDataProperty:"startupProbe",rootData}))){
vErrors = vErrors === null ? validate87.errors : vErrors.concat(validate87.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs17 === errors;
}
if(data.stdin !== undefined){
let data17 = data.stdin;
const _errs18 = errors;
if(!(validate2(data17, {instancePath:instancePath+"/stdin",parentData:data,parentDataProperty:"stdin",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs18 === errors;
}
if(data.stdinOnce !== undefined){
let data18 = data.stdinOnce;
const _errs19 = errors;
if(!(validate2(data18, {instancePath:instancePath+"/stdinOnce",parentData:data,parentDataProperty:"stdinOnce",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs19 === errors;
}
if(data.terminationMessagePath !== undefined){
let data19 = data.terminationMessagePath;
const _errs20 = errors;
if(!(validate0(data19, {instancePath:instancePath+"/terminationMessagePath",parentData:data,parentDataProperty:"terminationMessagePath",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs20 === errors;
}
if(data.terminationMessagePolicy !== undefined){
let data20 = data.terminationMessagePolicy;
const _errs21 = errors;
if(!(validate0(data20, {instancePath:instancePath+"/terminationMessagePolicy",parentData:data,parentDataProperty:"terminationMessagePolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs21 === errors;
}
if(data.tty !== undefined){
let data21 = data.tty;
const _errs22 = errors;
if(!(validate2(data21, {instancePath:instancePath+"/tty",parentData:data,parentDataProperty:"tty",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs22 === errors;
}
if(data.volumeDevices !== undefined){
let data22 = data.volumeDevices;
const _errs23 = errors;
if(!(validate203(data22, {instancePath:instancePath+"/volumeDevices",parentData:data,parentDataProperty:"volumeDevices",rootData}))){
vErrors = vErrors === null ? validate203.errors : vErrors.concat(validate203.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs23 === errors;
}
if(data.volumeMounts !== undefined){
let data23 = data.volumeMounts;
const _errs24 = errors;
if(!(validate208(data23, {instancePath:instancePath+"/volumeMounts",parentData:data,parentDataProperty:"volumeMounts",rootData}))){
vErrors = vErrors === null ? validate208.errors : vErrors.concat(validate208.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs24 === errors;
}
if(data.workingDir !== undefined){
let data24 = data.workingDir;
const _errs25 = errors;
if(!(validate0(data24, {instancePath:instancePath+"/workingDir",parentData:data,parentDataProperty:"workingDir",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs25 === errors;
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
validate218.errors = vErrors;
return errors === 0;
}


function validate331(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs1 = errors;
if(!(validate218(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate218.errors : vErrors.concat(validate218.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
if(!valid0){
valid1 = false;
}
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "array"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate331.errors = vErrors;
return errors === 0;
}

const schema102 = {"properties":{"nameservers":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"options":{"$ref":"3GiESGFQ1bt10pSKgDXOz2E8WZV5mfZ_lzY5h2ZooyI"},"searches":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};
const schema103 = {"items":{"$ref":"F4Xy5Fz36W_URgJ39USLrSRkrK9xMUDLknD7E0nmTrw"},"type":"array","nullable":true};
const schema104 = {"properties":{"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"value":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object"};

function validate337(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name !== undefined){
let data0 = data.name;
const _errs1 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.value !== undefined){
let data1 = data.value;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/value",parentData:data,parentDataProperty:"value",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate337.errors = vErrors;
return errors === 0;
}


function validate336(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate337(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate337.errors : vErrors.concat(validate337.errors);
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
validate336.errors = vErrors;
return errors === 0;
}


function validate334(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.nameservers !== undefined){
let data0 = data.nameservers;
const _errs2 = errors;
if(!(validate17(data0, {instancePath:instancePath+"/nameservers",parentData:data,parentDataProperty:"nameservers",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.options !== undefined){
let data1 = data.options;
const _errs3 = errors;
if(!(validate336(data1, {instancePath:instancePath+"/options",parentData:data,parentDataProperty:"options",rootData}))){
vErrors = vErrors === null ? validate336.errors : vErrors.concat(validate336.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.searches !== undefined){
let data2 = data.searches;
const _errs4 = errors;
if(!(validate17(data2, {instancePath:instancePath+"/searches",parentData:data,parentDataProperty:"searches",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate334.errors = vErrors;
return errors === 0;
}

const schema105 = {"items":{"$ref":"C-ceGOizjWksqCzap3EC-S8JeZSosW0aT9tkSJlW-hw"},"type":"array","nullable":true};
const schema106 = {"properties":{"args":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"command":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"env":{"$ref":"QK6qdOVPcIj3ymOL58J6NgOdTW_Xbf1pI79iGILi8uU"},"envFrom":{"$ref":"ZRw41W3mWwhKksdKQqy6gdGI0VqZa0mht_DKnlcF-lw"},"image":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"imagePullPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"lifecycle":{"$ref":"2GolHCLsHBmPr5h5-SqeL3kwFl1yyHXgHxOurrzuSpc"},"livenessProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"ports":{"$ref":"QdA0VP-JsKHKjWu-1Hu8oyYcguMz4UHlXTFbpvKweaM"},"readinessProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"resizePolicy":{"$ref":"6FxzbMy3fQv1gheBeNadVqOl2xbXIqiFGG5KNo9kRcw"},"resources":{"$ref":"FhMRS9yiPxYjQhkDxbgv-jQ4jx9VLxRdMLNZtrOnwhI"},"restartPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"restartPolicyRules":{"$ref":"bn1AnMh-2dkJyL1LY5A_xgvSFgNvRm9Wd4RL1NkeXRU"},"securityContext":{"$ref":"ppbGhvBFiV7RuYhp7GW5xoG5M7lU4kbUlUSlBvBbdzE"},"startupProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"stdin":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"stdinOnce":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"targetContainerName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"terminationMessagePath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"terminationMessagePolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"tty":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeDevices":{"$ref":"yySLqkz1jc_2D5zmwBnHmcS-y-0ogKvDTXKQGVruH8A"},"volumeMounts":{"$ref":"K3z2jQ4aYZcOrB0gSTeLWvm5O5c50b2P06GZmb9a8OM"},"workingDir":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["name"],"type":"object"};

function validate347(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.args !== undefined){
let data0 = data.args;
const _errs1 = errors;
if(!(validate17(data0, {instancePath:instancePath+"/args",parentData:data,parentDataProperty:"args",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.command !== undefined){
let data1 = data.command;
const _errs2 = errors;
if(!(validate17(data1, {instancePath:instancePath+"/command",parentData:data,parentDataProperty:"command",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.env !== undefined){
let data2 = data.env;
const _errs3 = errors;
if(!(validate109(data2, {instancePath:instancePath+"/env",parentData:data,parentDataProperty:"env",rootData}))){
vErrors = vErrors === null ? validate109.errors : vErrors.concat(validate109.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.envFrom !== undefined){
let data3 = data.envFrom;
const _errs4 = errors;
if(!(validate134(data3, {instancePath:instancePath+"/envFrom",parentData:data,parentDataProperty:"envFrom",rootData}))){
vErrors = vErrors === null ? validate134.errors : vErrors.concat(validate134.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.image !== undefined){
let data4 = data.image;
const _errs5 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.imagePullPolicy !== undefined){
let data5 = data.imagePullPolicy;
const _errs6 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/imagePullPolicy",parentData:data,parentDataProperty:"imagePullPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.lifecycle !== undefined){
let data6 = data.lifecycle;
const _errs7 = errors;
if(!(validate140(data6, {instancePath:instancePath+"/lifecycle",parentData:data,parentDataProperty:"lifecycle",rootData}))){
vErrors = vErrors === null ? validate140.errors : vErrors.concat(validate140.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.livenessProbe !== undefined){
let data7 = data.livenessProbe;
const _errs8 = errors;
if(!(validate87(data7, {instancePath:instancePath+"/livenessProbe",parentData:data,parentDataProperty:"livenessProbe",rootData}))){
vErrors = vErrors === null ? validate87.errors : vErrors.concat(validate87.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.name !== undefined){
let data8 = data.name;
const _errs9 = errors;
if(!(validate1(data8, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.ports !== undefined){
let data9 = data.ports;
const _errs10 = errors;
if(!(validate144(data9, {instancePath:instancePath+"/ports",parentData:data,parentDataProperty:"ports",rootData}))){
vErrors = vErrors === null ? validate144.errors : vErrors.concat(validate144.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.readinessProbe !== undefined){
let data10 = data.readinessProbe;
const _errs11 = errors;
if(!(validate87(data10, {instancePath:instancePath+"/readinessProbe",parentData:data,parentDataProperty:"readinessProbe",rootData}))){
vErrors = vErrors === null ? validate87.errors : vErrors.concat(validate87.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.resizePolicy !== undefined){
let data11 = data.resizePolicy;
const _errs12 = errors;
if(!(validate153(data11, {instancePath:instancePath+"/resizePolicy",parentData:data,parentDataProperty:"resizePolicy",rootData}))){
vErrors = vErrors === null ? validate153.errors : vErrors.concat(validate153.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.resources !== undefined){
let data12 = data.resources;
const _errs13 = errors;
if(!(validate158(data12, {instancePath:instancePath+"/resources",parentData:data,parentDataProperty:"resources",rootData}))){
vErrors = vErrors === null ? validate158.errors : vErrors.concat(validate158.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.restartPolicy !== undefined){
let data13 = data.restartPolicy;
const _errs14 = errors;
if(!(validate0(data13, {instancePath:instancePath+"/restartPolicy",parentData:data,parentDataProperty:"restartPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
if(data.restartPolicyRules !== undefined){
let data14 = data.restartPolicyRules;
const _errs15 = errors;
if(!(validate167(data14, {instancePath:instancePath+"/restartPolicyRules",parentData:data,parentDataProperty:"restartPolicyRules",rootData}))){
vErrors = vErrors === null ? validate167.errors : vErrors.concat(validate167.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs15 === errors;
}
if(data.securityContext !== undefined){
let data15 = data.securityContext;
const _errs16 = errors;
if(!(validate177(data15, {instancePath:instancePath+"/securityContext",parentData:data,parentDataProperty:"securityContext",rootData}))){
vErrors = vErrors === null ? validate177.errors : vErrors.concat(validate177.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs16 === errors;
}
if(data.startupProbe !== undefined){
let data16 = data.startupProbe;
const _errs17 = errors;
if(!(validate87(data16, {instancePath:instancePath+"/startupProbe",parentData:data,parentDataProperty:"startupProbe",rootData}))){
vErrors = vErrors === null ? validate87.errors : vErrors.concat(validate87.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs17 === errors;
}
if(data.stdin !== undefined){
let data17 = data.stdin;
const _errs18 = errors;
if(!(validate2(data17, {instancePath:instancePath+"/stdin",parentData:data,parentDataProperty:"stdin",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs18 === errors;
}
if(data.stdinOnce !== undefined){
let data18 = data.stdinOnce;
const _errs19 = errors;
if(!(validate2(data18, {instancePath:instancePath+"/stdinOnce",parentData:data,parentDataProperty:"stdinOnce",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs19 === errors;
}
if(data.targetContainerName !== undefined){
let data19 = data.targetContainerName;
const _errs20 = errors;
if(!(validate0(data19, {instancePath:instancePath+"/targetContainerName",parentData:data,parentDataProperty:"targetContainerName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs20 === errors;
}
if(data.terminationMessagePath !== undefined){
let data20 = data.terminationMessagePath;
const _errs21 = errors;
if(!(validate0(data20, {instancePath:instancePath+"/terminationMessagePath",parentData:data,parentDataProperty:"terminationMessagePath",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs21 === errors;
}
if(data.terminationMessagePolicy !== undefined){
let data21 = data.terminationMessagePolicy;
const _errs22 = errors;
if(!(validate0(data21, {instancePath:instancePath+"/terminationMessagePolicy",parentData:data,parentDataProperty:"terminationMessagePolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs22 === errors;
}
if(data.tty !== undefined){
let data22 = data.tty;
const _errs23 = errors;
if(!(validate2(data22, {instancePath:instancePath+"/tty",parentData:data,parentDataProperty:"tty",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs23 === errors;
}
if(data.volumeDevices !== undefined){
let data23 = data.volumeDevices;
const _errs24 = errors;
if(!(validate203(data23, {instancePath:instancePath+"/volumeDevices",parentData:data,parentDataProperty:"volumeDevices",rootData}))){
vErrors = vErrors === null ? validate203.errors : vErrors.concat(validate203.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs24 === errors;
}
if(data.volumeMounts !== undefined){
let data24 = data.volumeMounts;
const _errs25 = errors;
if(!(validate208(data24, {instancePath:instancePath+"/volumeMounts",parentData:data,parentDataProperty:"volumeMounts",rootData}))){
vErrors = vErrors === null ? validate208.errors : vErrors.concat(validate208.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs25 === errors;
}
if(data.workingDir !== undefined){
let data25 = data.workingDir;
const _errs26 = errors;
if(!(validate0(data25, {instancePath:instancePath+"/workingDir",parentData:data,parentDataProperty:"workingDir",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs26 === errors;
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
validate347.errors = vErrors;
return errors === 0;
}


function validate346(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate347(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate347.errors : vErrors.concat(validate347.errors);
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
validate346.errors = vErrors;
return errors === 0;
}

const schema107 = {"items":{"$ref":"1HespOXKrotYoqvobtjdYpFt-3Uj4rr3rOO-NdY0MJw"},"type":"array","nullable":true};
const schema108 = {"properties":{"hostnames":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"ip":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["ip"],"type":"object"};

function validate377(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.ip === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "ip"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.hostnames !== undefined){
let data0 = data.hostnames;
const _errs1 = errors;
if(!(validate17(data0, {instancePath:instancePath+"/hostnames",parentData:data,parentDataProperty:"hostnames",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.ip !== undefined){
let data1 = data.ip;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/ip",parentData:data,parentDataProperty:"ip",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
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
validate377.errors = vErrors;
return errors === 0;
}


function validate376(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate377(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate377.errors : vErrors.concat(validate377.errors);
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
validate376.errors = vErrors;
return errors === 0;
}

const schema109 = {"items":{"$ref":"CeclzNr20k5vr0FY8_Di-i5kv_hKd8rGtt9CG90tDrI"},"type":"array","nullable":true};
const schema78 = {"properties":{"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"}},"type":"object"};

function validate269(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name !== undefined){
let data0 = data.name;
const _errs1 = errors;
if(!(validate51(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate51.errors : vErrors.concat(validate51.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate269.errors = vErrors;
return errors === 0;
}


function validate388(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate269(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate269.errors : vErrors.concat(validate269.errors);
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
validate388.errors = vErrors;
return errors === 0;
}

const schema110 = {"items":{"$ref":"chE8IbiwOYvrhp6jxf4CeNPe4xowDfDQCt1Zhaa9Prk"},"type":"array","nullable":true};

function validate391(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate218(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate218.errors : vErrors.concat(validate218.errors);
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
validate391.errors = vErrors;
return errors === 0;
}

const schema111 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["name"],"type":"object","nullable":true};

function validate396(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.name === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.name !== undefined){
let data0 = data.name;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate396.errors = vErrors;
return errors === 0;
}

const schema112 = {"items":{"$ref":"7SJ8IHNZpr4K9bVQcc8BXyxEB7fvxKGrCHMOn4uuNjk"},"type":"array","nullable":true};
const schema113 = {"properties":{"conditionType":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["conditionType"],"type":"object"};

function validate404(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.conditionType === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "conditionType"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.conditionType !== undefined){
let data0 = data.conditionType;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/conditionType",parentData:data,parentDataProperty:"conditionType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
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
validate404.errors = vErrors;
return errors === 0;
}


function validate403(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate404(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate404.errors : vErrors.concat(validate404.errors);
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
validate403.errors = vErrors;
return errors === 0;
}

const schema114 = {"items":{"$ref":"8oMSVTgbQLfKCbtW7rBmcM71xueDXJZxnN_-a3YWUoE"},"type":"array","nullable":true};
const schema115 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"resourceClaimName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"resourceClaimTemplateName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["name"],"type":"object"};

function validate409(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.name !== undefined){
let data0 = data.name;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.resourceClaimName !== undefined){
let data1 = data.resourceClaimName;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/resourceClaimName",parentData:data,parentDataProperty:"resourceClaimName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.resourceClaimTemplateName !== undefined){
let data2 = data.resourceClaimTemplateName;
const _errs3 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/resourceClaimTemplateName",parentData:data,parentDataProperty:"resourceClaimTemplateName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate409.errors = vErrors;
return errors === 0;
}


function validate408(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate409(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate409.errors : vErrors.concat(validate409.errors);
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
validate408.errors = vErrors;
return errors === 0;
}

const schema116 = {"items":{"$ref":"Tv4xxEM4-rkWx429stngdyg1y-NXOJv9XnOY7jdo7LY"},"type":"array","nullable":true};
const schema117 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["name"],"type":"object"};

function validate420(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.name !== undefined){
let data0 = data.name;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
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
validate420.errors = vErrors;
return errors === 0;
}


function validate419(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate420(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate420.errors : vErrors.concat(validate420.errors);
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
validate419.errors = vErrors;
return errors === 0;
}

const schema118 = {"properties":{"podGroupName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate424(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.podGroupName !== undefined){
let data0 = data.podGroupName;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/podGroupName",parentData:data,parentDataProperty:"podGroupName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate424.errors = vErrors;
return errors === 0;
}

const schema119 = {"properties":{"appArmorProfile":{"$ref":"aMnE7Im_y76-Nz9q0hhh6yja57sQFej02mLc00OwhtE"},"fsGroup":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"fsGroupChangePolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"runAsGroup":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"runAsNonRoot":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"runAsUser":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"seLinuxChangePolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"seLinuxOptions":{"$ref":"RAvVUd1y4DBTzL23WWML0HifcsQtvUbbJLKVUvzXXO4"},"seccompProfile":{"$ref":"aMnE7Im_y76-Nz9q0hhh6yja57sQFej02mLc00OwhtE"},"supplementalGroups":{"$ref":"61CuU5PR0DTXgf6CSVY2YGQyMC0xseKwOl9CPTT5SMw"},"supplementalGroupsPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"sysctls":{"$ref":"MsS0666yf0HSUh6mG0cBqbw9ThqZk9-DowZaIzPios0"},"windowsOptions":{"$ref":"6z1i1A1Aq99luryppL8NHWpsKmtBfItfBpL24p91f_I"}},"type":"object","nullable":true};
const schema120 = {"items":{"$ref":"0YQm7z5aN2fg7S8XlbxrquRU6pdIr97uCbtQYyjv3S4"},"type":"array","nullable":true};

function validate437(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate78(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
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
validate437.errors = vErrors;
return errors === 0;
}


function validate427(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.appArmorProfile !== undefined){
let data0 = data.appArmorProfile;
const _errs2 = errors;
if(!(validate106(data0, {instancePath:instancePath+"/appArmorProfile",parentData:data,parentDataProperty:"appArmorProfile",rootData}))){
vErrors = vErrors === null ? validate106.errors : vErrors.concat(validate106.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fsGroup !== undefined){
let data1 = data.fsGroup;
const _errs3 = errors;
if(!(validate86(data1, {instancePath:instancePath+"/fsGroup",parentData:data,parentDataProperty:"fsGroup",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.fsGroupChangePolicy !== undefined){
let data2 = data.fsGroupChangePolicy;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/fsGroupChangePolicy",parentData:data,parentDataProperty:"fsGroupChangePolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.runAsGroup !== undefined){
let data3 = data.runAsGroup;
const _errs5 = errors;
if(!(validate86(data3, {instancePath:instancePath+"/runAsGroup",parentData:data,parentDataProperty:"runAsGroup",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.runAsNonRoot !== undefined){
let data4 = data.runAsNonRoot;
const _errs6 = errors;
if(!(validate2(data4, {instancePath:instancePath+"/runAsNonRoot",parentData:data,parentDataProperty:"runAsNonRoot",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.runAsUser !== undefined){
let data5 = data.runAsUser;
const _errs7 = errors;
if(!(validate86(data5, {instancePath:instancePath+"/runAsUser",parentData:data,parentDataProperty:"runAsUser",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.seLinuxChangePolicy !== undefined){
let data6 = data.seLinuxChangePolicy;
const _errs8 = errors;
if(!(validate0(data6, {instancePath:instancePath+"/seLinuxChangePolicy",parentData:data,parentDataProperty:"seLinuxChangePolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.seLinuxOptions !== undefined){
let data7 = data.seLinuxOptions;
const _errs9 = errors;
if(!(validate190(data7, {instancePath:instancePath+"/seLinuxOptions",parentData:data,parentDataProperty:"seLinuxOptions",rootData}))){
vErrors = vErrors === null ? validate190.errors : vErrors.concat(validate190.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.seccompProfile !== undefined){
let data8 = data.seccompProfile;
const _errs10 = errors;
if(!(validate106(data8, {instancePath:instancePath+"/seccompProfile",parentData:data,parentDataProperty:"seccompProfile",rootData}))){
vErrors = vErrors === null ? validate106.errors : vErrors.concat(validate106.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.supplementalGroups !== undefined){
let data9 = data.supplementalGroups;
const _errs11 = errors;
if(!(validate437(data9, {instancePath:instancePath+"/supplementalGroups",parentData:data,parentDataProperty:"supplementalGroups",rootData}))){
vErrors = vErrors === null ? validate437.errors : vErrors.concat(validate437.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.supplementalGroupsPolicy !== undefined){
let data10 = data.supplementalGroupsPolicy;
const _errs12 = errors;
if(!(validate0(data10, {instancePath:instancePath+"/supplementalGroupsPolicy",parentData:data,parentDataProperty:"supplementalGroupsPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.sysctls !== undefined){
let data11 = data.sysctls;
const _errs13 = errors;
if(!(validate67(data11, {instancePath:instancePath+"/sysctls",parentData:data,parentDataProperty:"sysctls",rootData}))){
vErrors = vErrors === null ? validate67.errors : vErrors.concat(validate67.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.windowsOptions !== undefined){
let data12 = data.windowsOptions;
const _errs14 = errors;
if(!(validate197(data12, {instancePath:instancePath+"/windowsOptions",parentData:data,parentDataProperty:"windowsOptions",rootData}))){
vErrors = vErrors === null ? validate197.errors : vErrors.concat(validate197.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
}
validate427.errors = vErrors;
return errors === 0;
}

const schema121 = {"items":{"$ref":"0ByfSGW7wvmVW7bPgwyiWBjeufbPURNaUIcLKKK35ZA"},"type":"array","nullable":true};
const schema122 = {"properties":{"effect":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"key":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"operator":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"tolerationSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"value":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object"};

function validate451(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.effect !== undefined){
let data0 = data.effect;
const _errs1 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/effect",parentData:data,parentDataProperty:"effect",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.key !== undefined){
let data1 = data.key;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.operator !== undefined){
let data2 = data.operator;
const _errs3 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/operator",parentData:data,parentDataProperty:"operator",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.tolerationSeconds !== undefined){
let data3 = data.tolerationSeconds;
const _errs4 = errors;
if(!(validate86(data3, {instancePath:instancePath+"/tolerationSeconds",parentData:data,parentDataProperty:"tolerationSeconds",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.value !== undefined){
let data4 = data.value;
const _errs5 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/value",parentData:data,parentDataProperty:"value",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate451.errors = vErrors;
return errors === 0;
}


function validate450(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate451(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate451.errors : vErrors.concat(validate451.errors);
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
validate450.errors = vErrors;
return errors === 0;
}

const schema123 = {"items":{"$ref":"StMxQItS9v0SfOwJaZyLYLt5kRuP512kSHqU4NYTwI0"},"type":"array","nullable":true};
const schema124 = {"properties":{"labelSelector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"matchLabelKeys":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"maxSkew":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"minDomains":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"nodeAffinityPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"nodeTaintsPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"topologyKey":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"whenUnsatisfiable":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["maxSkew","topologyKey","whenUnsatisfiable"],"type":"object"};

function validate460(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.maxSkew === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "maxSkew"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.topologyKey === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "topologyKey"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.whenUnsatisfiable === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "whenUnsatisfiable"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.labelSelector !== undefined){
let data0 = data.labelSelector;
const _errs1 = errors;
if(!(validate28(data0, {instancePath:instancePath+"/labelSelector",parentData:data,parentDataProperty:"labelSelector",rootData}))){
vErrors = vErrors === null ? validate28.errors : vErrors.concat(validate28.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.matchLabelKeys !== undefined){
let data1 = data.matchLabelKeys;
const _errs2 = errors;
if(!(validate17(data1, {instancePath:instancePath+"/matchLabelKeys",parentData:data,parentDataProperty:"matchLabelKeys",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.maxSkew !== undefined){
let data2 = data.maxSkew;
const _errs3 = errors;
if(!(validate31(data2, {instancePath:instancePath+"/maxSkew",parentData:data,parentDataProperty:"maxSkew",rootData}))){
vErrors = vErrors === null ? validate31.errors : vErrors.concat(validate31.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.minDomains !== undefined){
let data3 = data.minDomains;
const _errs4 = errors;
if(!(validate85(data3, {instancePath:instancePath+"/minDomains",parentData:data,parentDataProperty:"minDomains",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.nodeAffinityPolicy !== undefined){
let data4 = data.nodeAffinityPolicy;
const _errs5 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/nodeAffinityPolicy",parentData:data,parentDataProperty:"nodeAffinityPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.nodeTaintsPolicy !== undefined){
let data5 = data.nodeTaintsPolicy;
const _errs6 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/nodeTaintsPolicy",parentData:data,parentDataProperty:"nodeTaintsPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.topologyKey !== undefined){
let data6 = data.topologyKey;
const _errs7 = errors;
if(!(validate1(data6, {instancePath:instancePath+"/topologyKey",parentData:data,parentDataProperty:"topologyKey",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.whenUnsatisfiable !== undefined){
let data7 = data.whenUnsatisfiable;
const _errs8 = errors;
if(!(validate1(data7, {instancePath:instancePath+"/whenUnsatisfiable",parentData:data,parentDataProperty:"whenUnsatisfiable",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
}
else {
const err3 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err3];
}
else {
vErrors.push(err3);
}
errors++;
}
validate460.errors = vErrors;
return errors === 0;
}


function validate459(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate460(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate460.errors : vErrors.concat(validate460.errors);
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
validate459.errors = vErrors;
return errors === 0;
}

const schema125 = {"items":{"$ref":"Hj6zWhh70IB7X0q3eRZDjlHhz7sqbnQspRAima0qQSE"},"type":"array","nullable":true};
const schema126 = {"properties":{"awsElasticBlockStore":{"$ref":"vPA0bm54uQADvg8Ep2156ezWb9pvkjUSgi79O6AEzUs"},"azureDisk":{"$ref":"wFqLNNJgW_o5ObauitiCOyT0NGvkjQH59OgBfR3nx78"},"azureFile":{"$ref":"ra-qRI4a9EDFDlPkSamRu2Q2QFXgcgrTkMEaIAfIFlQ"},"cephfs":{"$ref":"hF2IjS1nzlRhEgH0aYEOkSyicJRz2WA5TydzEGoRLKg"},"cinder":{"$ref":"fNCUZPQc9iEaWmz8ai20p-XbVRg8tBJIhSF3TXq-YMs"},"configMap":{"$ref":"aviBc3qnN2f55E2vimjJLIzsV9DrIo0yQQb5hwqxUOc"},"csi":{"$ref":"oQ5udLLiWaZv5Xde630bmkROSPyJ-qElIPSHcGuPOY4"},"downwardAPI":{"$ref":"GMd9Y-VcblyvoFTlA1OqTa5xw10fjnz_tJ5HDF6AEOU"},"emptyDir":{"$ref":"dD_f54ZfU9CI7NKO29jcEDlRdCPgexeuapdopjIU01E"},"ephemeral":{"$ref":"eNrnIy2CikSgPB0aVTsii2hzgtDrTs6oGNyfcyCWEMs"},"fc":{"$ref":"4YMYCoeKb62-XCvxNQKZZg8ljPa2bImErUwuV6lfzMc"},"flexVolume":{"$ref":"87oLwN-NCLaqdDpK_Xpdr0U9qiabmcRmprmqZV3W2OM"},"flocker":{"$ref":"aKg6ZgUktk8XvWlSQnzXgMGusTLk-Y4v0E66MJqnxKM"},"gcePersistentDisk":{"$ref":"Nc8CY9PKVBuFiqw-t7h5cOVQLWFbxUZcHQ_nHz9xtm0"},"gitRepo":{"$ref":"rS29wFkNVu1ceZUQHqPBmyB8-Ms8_nnLBrD35UT73dI"},"glusterfs":{"$ref":"TpFF3OzeCU-chmDfffTsAmyFGMVfPLAPDphy4r3XKjg"},"hostPath":{"$ref":"EXTQAV0X282Zg3GxvBwRnhWqJJDwIVAFTHzBb8v3nlg"},"image":{"$ref":"5c0MzG5RcFc2MWsW2mHabMHz4ImtJyTyHppNKfKeOdQ"},"iscsi":{"$ref":"8HbKIUlglpSfrcdpgZeJFdBlpWGsIvExGUK1n48WUr0"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"nfs":{"$ref":"nwSeAMDcw-4i412pD0rNHf229BX9fN7BBcjt-3DdCKI"},"persistentVolumeClaim":{"$ref":"Nt1_4vgMqd6c7fTj2yEYei6As7a9E2OyyNV5h8I-T-w"},"photonPersistentDisk":{"$ref":"Ed4AAIR_qYXTjtKQpuCQkveQduEVxyKSFYufAbsWSJQ"},"portworxVolume":{"$ref":"_qkJOFjT4iVO-EThDJ3tJ8R8OjXW5ytgOhPUrts7VCw"},"projected":{"$ref":"7xCtd90hb1cpRlMmHzI_H6sbzbpkjpI3GAlSHRbg64E"},"quobyte":{"$ref":"ZXGgqPX-VDcSDf8w43imCiVzHKzcgfpUOl-yALusknA"},"rbd":{"$ref":"j-DO-_nrU_kl5foZpuWFww_njUnDUr4E1u8oCeqXJE4"},"scaleIO":{"$ref":"Pz5rdKKPt5xwuDA5BqivksbvxoLDreFxoGght9-tDko"},"secret":{"$ref":"p2IOGKuqym0QlcKZpielo4SFxm41yjJyrMQ9Pmr5A2A"},"storageos":{"$ref":"JLIMsOAl8xZaat8jT6oTbEVgrK7d76K8GSAAjC8omEk"},"vsphereVolume":{"$ref":"Yl_IkrhrBmYK2ezUSAmmTuS5zATmqlK0Gh1MlcOGsT8"}},"required":["name"],"type":"object"};
const schema127 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"partition":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeID":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["volumeID"],"type":"object","nullable":true};

function validate473(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.volumeID === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "volumeID"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.partition !== undefined){
let data1 = data.partition;
const _errs3 = errors;
if(!(validate85(data1, {instancePath:instancePath+"/partition",parentData:data,parentDataProperty:"partition",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.readOnly !== undefined){
let data2 = data.readOnly;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.volumeID !== undefined){
let data3 = data.volumeID;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/volumeID",parentData:data,parentDataProperty:"volumeID",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate473.errors = vErrors;
return errors === 0;
}

const schema128 = {"properties":{"cachingMode":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"diskName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"diskURI":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"fsType":{"$ref":"b7rsf0DzZXS6T136mheaM3HHbss23WkJGWNWmAb7obA"},"kind":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"PxmoJm38YOn1dCbyB5PLDDQXV2B2rqFhpzNXj0_ibe4"}},"required":["diskName","diskURI"],"type":"object","nullable":true};
const schema129 = {"default":"ext4","type":"string","nullable":true};

function validate483(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate483.errors = vErrors;
return errors === 0;
}


function validate479(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.diskName === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "diskName"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.diskURI === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "diskURI"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.cachingMode !== undefined){
let data0 = data.cachingMode;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/cachingMode",parentData:data,parentDataProperty:"cachingMode",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.diskName !== undefined){
let data1 = data.diskName;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/diskName",parentData:data,parentDataProperty:"diskName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.diskURI !== undefined){
let data2 = data.diskURI;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/diskURI",parentData:data,parentDataProperty:"diskURI",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.fsType !== undefined){
let data3 = data.fsType;
const _errs5 = errors;
if(!(validate483(data3, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate483.errors : vErrors.concat(validate483.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.kind !== undefined){
let data4 = data.kind;
const _errs6 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.readOnly !== undefined){
let data5 = data.readOnly;
const _errs7 = errors;
if(!(validate121(data5, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate121.errors : vErrors.concat(validate121.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
}
validate479.errors = vErrors;
return errors === 0;
}

const schema130 = {"properties":{"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"shareName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["secretName","shareName"],"type":"object","nullable":true};

function validate488(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.secretName === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "secretName"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.shareName === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "shareName"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.readOnly !== undefined){
let data0 = data.readOnly;
const _errs2 = errors;
if(!(validate2(data0, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.secretName !== undefined){
let data1 = data.secretName;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/secretName",parentData:data,parentDataProperty:"secretName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.shareName !== undefined){
let data2 = data.shareName;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/shareName",parentData:data,parentDataProperty:"shareName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate488.errors = vErrors;
return errors === 0;
}

const schema131 = {"properties":{"monitors":{"$ref":"uqUtVyG9D2UAWDvqosEgVMtwK5kI3TMy2eLbDZB452I"},"path":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretFile":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"user":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["monitors"],"type":"object","nullable":true};
const schema71 = {"properties":{"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"}},"type":"object","nullable":true};

function validate244(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate51(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate51.errors : vErrors.concat(validate51.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate244.errors = vErrors;
return errors === 0;
}


function validate493(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.monitors === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "monitors"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.monitors !== undefined){
let data0 = data.monitors;
const _errs2 = errors;
if(!(validate6(data0, {instancePath:instancePath+"/monitors",parentData:data,parentDataProperty:"monitors",rootData}))){
vErrors = vErrors === null ? validate6.errors : vErrors.concat(validate6.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.path !== undefined){
let data1 = data.path;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.readOnly !== undefined){
let data2 = data.readOnly;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.secretFile !== undefined){
let data3 = data.secretFile;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/secretFile",parentData:data,parentDataProperty:"secretFile",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.secretRef !== undefined){
let data4 = data.secretRef;
const _errs6 = errors;
if(!(validate244(data4, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate244.errors : vErrors.concat(validate244.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.user !== undefined){
let data5 = data.user;
const _errs7 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/user",parentData:data,parentDataProperty:"user",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
}
validate493.errors = vErrors;
return errors === 0;
}

const schema132 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"volumeID":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["volumeID"],"type":"object","nullable":true};

function validate501(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.volumeID === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "volumeID"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.readOnly !== undefined){
let data1 = data.readOnly;
const _errs3 = errors;
if(!(validate2(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.secretRef !== undefined){
let data2 = data.secretRef;
const _errs4 = errors;
if(!(validate244(data2, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate244.errors : vErrors.concat(validate244.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.volumeID !== undefined){
let data3 = data.volumeID;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/volumeID",parentData:data,parentDataProperty:"volumeID",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate501.errors = vErrors;
return errors === 0;
}

const schema133 = {"properties":{"defaultMode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"items":{"$ref":"gPtCVyFH2eXv-J44JI3istIuhC40N-NdI4mRYcFGaCI"},"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"type":"object","nullable":true};
const schema73 = {"items":{"$ref":"Mw85TCQKW0HVKrKU83sDokxzwBc5-5PPrcGiCJqci0s"},"type":"array","nullable":true};
const schema74 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"mode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["key","path"],"type":"object"};

function validate253(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.key === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "key"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.path === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "path"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.key !== undefined){
let data0 = data.key;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.mode !== undefined){
let data1 = data.mode;
const _errs2 = errors;
if(!(validate85(data1, {instancePath:instancePath+"/mode",parentData:data,parentDataProperty:"mode",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.path !== undefined){
let data2 = data.path;
const _errs3 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
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
validate253.errors = vErrors;
return errors === 0;
}


function validate252(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate253(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate253.errors : vErrors.concat(validate253.errors);
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
validate252.errors = vErrors;
return errors === 0;
}


function validate507(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.defaultMode !== undefined){
let data0 = data.defaultMode;
const _errs2 = errors;
if(!(validate85(data0, {instancePath:instancePath+"/defaultMode",parentData:data,parentDataProperty:"defaultMode",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.items !== undefined){
let data1 = data.items;
const _errs3 = errors;
if(!(validate252(data1, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate252.errors : vErrors.concat(validate252.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.name !== undefined){
let data2 = data.name;
const _errs4 = errors;
if(!(validate51(data2, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate51.errors : vErrors.concat(validate51.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.optional !== undefined){
let data3 = data.optional;
const _errs5 = errors;
if(!(validate2(data3, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate507.errors = vErrors;
return errors === 0;
}

const schema134 = {"properties":{"driver":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"nodePublishSecretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeAttributes":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"}},"required":["driver"],"type":"object","nullable":true};

function validate513(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.driver === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "driver"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.driver !== undefined){
let data0 = data.driver;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/driver",parentData:data,parentDataProperty:"driver",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fsType !== undefined){
let data1 = data.fsType;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.nodePublishSecretRef !== undefined){
let data2 = data.nodePublishSecretRef;
const _errs4 = errors;
if(!(validate244(data2, {instancePath:instancePath+"/nodePublishSecretRef",parentData:data,parentDataProperty:"nodePublishSecretRef",rootData}))){
vErrors = vErrors === null ? validate244.errors : vErrors.concat(validate244.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.readOnly !== undefined){
let data3 = data.readOnly;
const _errs5 = errors;
if(!(validate2(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.volumeAttributes !== undefined){
let data4 = data.volumeAttributes;
const _errs6 = errors;
if(!(validate15(data4, {instancePath:instancePath+"/volumeAttributes",parentData:data,parentDataProperty:"volumeAttributes",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate513.errors = vErrors;
return errors === 0;
}

const schema135 = {"properties":{"defaultMode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"items":{"$ref":"d-PZJWo9tuJUK1sF4I9c0c4HI5ec_LUrbkqbJN0kxNA"}},"type":"object","nullable":true};
const schema75 = {"items":{"$ref":"6b0JHACYCsHJ-gVlQX9UriUS8Ox9KDWSM_ixcjRqBYk"},"type":"array","nullable":true};
const schema76 = {"properties":{"fieldRef":{"$ref":"HmyyyuOupyisSiKp0rvRRJtbCek8WlFUsfGxXzcVsrg"},"mode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"resourceFieldRef":{"$ref":"lUaYvKMK_XnIMZc4H9_ERGObFXszqs8GfnzmBfc3Oh8"}},"required":["path"],"type":"object"};

function validate259(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.path === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "path"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.fieldRef !== undefined){
let data0 = data.fieldRef;
const _errs1 = errors;
if(!(validate115(data0, {instancePath:instancePath+"/fieldRef",parentData:data,parentDataProperty:"fieldRef",rootData}))){
vErrors = vErrors === null ? validate115.errors : vErrors.concat(validate115.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.mode !== undefined){
let data1 = data.mode;
const _errs2 = errors;
if(!(validate85(data1, {instancePath:instancePath+"/mode",parentData:data,parentDataProperty:"mode",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.path !== undefined){
let data2 = data.path;
const _errs3 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.resourceFieldRef !== undefined){
let data3 = data.resourceFieldRef;
const _errs4 = errors;
if(!(validate126(data3, {instancePath:instancePath+"/resourceFieldRef",parentData:data,parentDataProperty:"resourceFieldRef",rootData}))){
vErrors = vErrors === null ? validate126.errors : vErrors.concat(validate126.errors);
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
validate259.errors = vErrors;
return errors === 0;
}


function validate258(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate259(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate259.errors : vErrors.concat(validate259.errors);
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
validate258.errors = vErrors;
return errors === 0;
}


function validate520(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.defaultMode !== undefined){
let data0 = data.defaultMode;
const _errs2 = errors;
if(!(validate85(data0, {instancePath:instancePath+"/defaultMode",parentData:data,parentDataProperty:"defaultMode",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.items !== undefined){
let data1 = data.items;
const _errs3 = errors;
if(!(validate258(data1, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate258.errors : vErrors.concat(validate258.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate520.errors = vErrors;
return errors === 0;
}

const schema136 = {"properties":{"medium":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"sizeLimit":{"$ref":"3dmDSoHI3Hv64ppBBbqAEh3HzVwqzFWb_nrM2BhHTgs"}},"type":"object","nullable":true};

function validate524(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.medium !== undefined){
let data0 = data.medium;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/medium",parentData:data,parentDataProperty:"medium",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.sizeLimit !== undefined){
let data1 = data.sizeLimit;
const _errs3 = errors;
if(!(validate101(data1, {instancePath:instancePath+"/sizeLimit",parentData:data,parentDataProperty:"sizeLimit",rootData}))){
vErrors = vErrors === null ? validate101.errors : vErrors.concat(validate101.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate524.errors = vErrors;
return errors === 0;
}

const schema137 = {"properties":{"volumeClaimTemplate":{"$ref":"RWcUpIUEoEZhA_O1VJPX5UIK22JWebpZpOk0memvidY"}},"type":"object","nullable":true};
const schema138 = {"properties":{"metadata":{"$ref":"9xOEryzozGanNCCz7goq0K4hzMvoOusQQ4iDJ494Bhw"},"spec":{"$ref":"kyKOBdUty49b7DFqivVu-C5SnGwoky00zYPyDjmfQck"}},"required":["spec"],"type":"object","nullable":true};
const schema139 = {"properties":{"accessModes":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"dataSource":{"$ref":"wjNzvnq-dR_-OiqRXDrOerEJO_HAowFBQnjb_nOgEyU"},"dataSourceRef":{"$ref":"oqf7uvYqNIR47CPHDvw3sYWQXSKnigFe8zHjsXWE35M"},"resources":{"$ref":"yTxZaQI_fncHf2yr5nLz-uUGOQ9FD-geEFW-5-qjX30"},"selector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"storageClassName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumeAttributesClassName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumeMode":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumeName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object"};
const schema140 = {"properties":{"apiGroup":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"kind":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["kind","name"],"type":"object","nullable":true};

function validate533(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.name === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.apiGroup !== undefined){
let data0 = data.apiGroup;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/apiGroup",parentData:data,parentDataProperty:"apiGroup",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.kind !== undefined){
let data1 = data.kind;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.name !== undefined){
let data2 = data.name;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate533.errors = vErrors;
return errors === 0;
}

const schema141 = {"properties":{"apiGroup":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"kind":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"namespace":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["kind","name"],"type":"object","nullable":true};

function validate538(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.name === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.apiGroup !== undefined){
let data0 = data.apiGroup;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/apiGroup",parentData:data,parentDataProperty:"apiGroup",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.kind !== undefined){
let data1 = data.kind;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.name !== undefined){
let data2 = data.name;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.namespace !== undefined){
let data3 = data.namespace;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/namespace",parentData:data,parentDataProperty:"namespace",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate538.errors = vErrors;
return errors === 0;
}

const schema142 = {"properties":{"limits":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"},"requests":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"}},"type":"object","nullable":true};

function validate544(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.limits !== undefined){
let data0 = data.limits;
const _errs2 = errors;
if(!(validate104(data0, {instancePath:instancePath+"/limits",parentData:data,parentDataProperty:"limits",rootData}))){
vErrors = vErrors === null ? validate104.errors : vErrors.concat(validate104.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.requests !== undefined){
let data1 = data.requests;
const _errs3 = errors;
if(!(validate104(data1, {instancePath:instancePath+"/requests",parentData:data,parentDataProperty:"requests",rootData}))){
vErrors = vErrors === null ? validate104.errors : vErrors.concat(validate104.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate544.errors = vErrors;
return errors === 0;
}


function validate531(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.accessModes !== undefined){
let data0 = data.accessModes;
const _errs1 = errors;
if(!(validate17(data0, {instancePath:instancePath+"/accessModes",parentData:data,parentDataProperty:"accessModes",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.dataSource !== undefined){
let data1 = data.dataSource;
const _errs2 = errors;
if(!(validate533(data1, {instancePath:instancePath+"/dataSource",parentData:data,parentDataProperty:"dataSource",rootData}))){
vErrors = vErrors === null ? validate533.errors : vErrors.concat(validate533.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.dataSourceRef !== undefined){
let data2 = data.dataSourceRef;
const _errs3 = errors;
if(!(validate538(data2, {instancePath:instancePath+"/dataSourceRef",parentData:data,parentDataProperty:"dataSourceRef",rootData}))){
vErrors = vErrors === null ? validate538.errors : vErrors.concat(validate538.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.resources !== undefined){
let data3 = data.resources;
const _errs4 = errors;
if(!(validate544(data3, {instancePath:instancePath+"/resources",parentData:data,parentDataProperty:"resources",rootData}))){
vErrors = vErrors === null ? validate544.errors : vErrors.concat(validate544.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.selector !== undefined){
let data4 = data.selector;
const _errs5 = errors;
if(!(validate28(data4, {instancePath:instancePath+"/selector",parentData:data,parentDataProperty:"selector",rootData}))){
vErrors = vErrors === null ? validate28.errors : vErrors.concat(validate28.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.storageClassName !== undefined){
let data5 = data.storageClassName;
const _errs6 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/storageClassName",parentData:data,parentDataProperty:"storageClassName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.volumeAttributesClassName !== undefined){
let data6 = data.volumeAttributesClassName;
const _errs7 = errors;
if(!(validate0(data6, {instancePath:instancePath+"/volumeAttributesClassName",parentData:data,parentDataProperty:"volumeAttributesClassName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.volumeMode !== undefined){
let data7 = data.volumeMode;
const _errs8 = errors;
if(!(validate0(data7, {instancePath:instancePath+"/volumeMode",parentData:data,parentDataProperty:"volumeMode",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.volumeName !== undefined){
let data8 = data.volumeName;
const _errs9 = errors;
if(!(validate0(data8, {instancePath:instancePath+"/volumeName",parentData:data,parentDataProperty:"volumeName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate531.errors = vErrors;
return errors === 0;
}


function validate529(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.spec === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "spec"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.metadata !== undefined){
let data0 = data.metadata;
const _errs2 = errors;
if(!(validate246(data0, {instancePath:instancePath+"/metadata",parentData:data,parentDataProperty:"metadata",rootData}))){
vErrors = vErrors === null ? validate246.errors : vErrors.concat(validate246.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.spec !== undefined){
let data1 = data.spec;
const _errs3 = errors;
if(!(validate531(data1, {instancePath:instancePath+"/spec",parentData:data,parentDataProperty:"spec",rootData}))){
vErrors = vErrors === null ? validate531.errors : vErrors.concat(validate531.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate529.errors = vErrors;
return errors === 0;
}


function validate528(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.volumeClaimTemplate !== undefined){
let data0 = data.volumeClaimTemplate;
const _errs2 = errors;
if(!(validate529(data0, {instancePath:instancePath+"/volumeClaimTemplate",parentData:data,parentDataProperty:"volumeClaimTemplate",rootData}))){
vErrors = vErrors === null ? validate529.errors : vErrors.concat(validate529.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate528.errors = vErrors;
return errors === 0;
}

const schema143 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"lun":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"targetWWNs":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"wwids":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};

function validate556(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.lun !== undefined){
let data1 = data.lun;
const _errs3 = errors;
if(!(validate85(data1, {instancePath:instancePath+"/lun",parentData:data,parentDataProperty:"lun",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.readOnly !== undefined){
let data2 = data.readOnly;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.targetWWNs !== undefined){
let data3 = data.targetWWNs;
const _errs5 = errors;
if(!(validate17(data3, {instancePath:instancePath+"/targetWWNs",parentData:data,parentDataProperty:"targetWWNs",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.wwids !== undefined){
let data4 = data.wwids;
const _errs6 = errors;
if(!(validate17(data4, {instancePath:instancePath+"/wwids",parentData:data,parentDataProperty:"wwids",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate556.errors = vErrors;
return errors === 0;
}

const schema144 = {"properties":{"driver":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"options":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"}},"required":["driver"],"type":"object","nullable":true};

function validate563(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.driver === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "driver"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.driver !== undefined){
let data0 = data.driver;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/driver",parentData:data,parentDataProperty:"driver",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fsType !== undefined){
let data1 = data.fsType;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.options !== undefined){
let data2 = data.options;
const _errs4 = errors;
if(!(validate15(data2, {instancePath:instancePath+"/options",parentData:data,parentDataProperty:"options",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.readOnly !== undefined){
let data3 = data.readOnly;
const _errs5 = errors;
if(!(validate2(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.secretRef !== undefined){
let data4 = data.secretRef;
const _errs6 = errors;
if(!(validate244(data4, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate244.errors : vErrors.concat(validate244.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate563.errors = vErrors;
return errors === 0;
}

const schema145 = {"properties":{"datasetName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"datasetUUID":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate570(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.datasetName !== undefined){
let data0 = data.datasetName;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/datasetName",parentData:data,parentDataProperty:"datasetName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.datasetUUID !== undefined){
let data1 = data.datasetUUID;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/datasetUUID",parentData:data,parentDataProperty:"datasetUUID",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate570.errors = vErrors;
return errors === 0;
}

const schema146 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"partition":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"pdName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"required":["pdName"],"type":"object","nullable":true};

function validate574(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.pdName === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "pdName"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.partition !== undefined){
let data1 = data.partition;
const _errs3 = errors;
if(!(validate85(data1, {instancePath:instancePath+"/partition",parentData:data,parentDataProperty:"partition",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.pdName !== undefined){
let data2 = data.pdName;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/pdName",parentData:data,parentDataProperty:"pdName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.readOnly !== undefined){
let data3 = data.readOnly;
const _errs5 = errors;
if(!(validate2(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate574.errors = vErrors;
return errors === 0;
}

const schema147 = {"properties":{"directory":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"repository":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"revision":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["repository"],"type":"object","nullable":true};

function validate580(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.repository === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "repository"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.directory !== undefined){
let data0 = data.directory;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/directory",parentData:data,parentDataProperty:"directory",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.repository !== undefined){
let data1 = data.repository;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/repository",parentData:data,parentDataProperty:"repository",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.revision !== undefined){
let data2 = data.revision;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/revision",parentData:data,parentDataProperty:"revision",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate580.errors = vErrors;
return errors === 0;
}

const schema148 = {"properties":{"endpoints":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"required":["endpoints","path"],"type":"object","nullable":true};

function validate585(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.endpoints === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "endpoints"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.path === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "path"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.endpoints !== undefined){
let data0 = data.endpoints;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/endpoints",parentData:data,parentDataProperty:"endpoints",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.path !== undefined){
let data1 = data.path;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.readOnly !== undefined){
let data2 = data.readOnly;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate585.errors = vErrors;
return errors === 0;
}

const schema149 = {"properties":{"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["path"],"type":"object","nullable":true};

function validate590(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.path === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "path"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.path !== undefined){
let data0 = data.path;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.type !== undefined){
let data1 = data.type;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate590.errors = vErrors;
return errors === 0;
}

const schema150 = {"properties":{"pullPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"reference":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate594(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.pullPolicy !== undefined){
let data0 = data.pullPolicy;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/pullPolicy",parentData:data,parentDataProperty:"pullPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.reference !== undefined){
let data1 = data.reference;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/reference",parentData:data,parentDataProperty:"reference",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate594.errors = vErrors;
return errors === 0;
}

const schema151 = {"properties":{"chapAuthDiscovery":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"chapAuthSession":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"initiatorName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"iqn":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"iscsiInterface":{"$ref":"5cSUFXhOjtgR-1-w0YuciQg4fuFzccKB0OFYzeWCsrY"},"lun":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"portals":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"targetPortal":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["iqn","lun","targetPortal"],"type":"object","nullable":true};
const schema152 = {"default":"default","type":"string","nullable":true};

function validate604(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate604.errors = vErrors;
return errors === 0;
}


function validate598(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.iqn === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "iqn"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.lun === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "lun"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.targetPortal === undefined){
const err3 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "targetPortal"}};
if(vErrors === null){
vErrors = [err3];
}
else {
vErrors.push(err3);
}
errors++;
}
if(data.chapAuthDiscovery !== undefined){
let data0 = data.chapAuthDiscovery;
const _errs2 = errors;
if(!(validate2(data0, {instancePath:instancePath+"/chapAuthDiscovery",parentData:data,parentDataProperty:"chapAuthDiscovery",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.chapAuthSession !== undefined){
let data1 = data.chapAuthSession;
const _errs3 = errors;
if(!(validate2(data1, {instancePath:instancePath+"/chapAuthSession",parentData:data,parentDataProperty:"chapAuthSession",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.fsType !== undefined){
let data2 = data.fsType;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.initiatorName !== undefined){
let data3 = data.initiatorName;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/initiatorName",parentData:data,parentDataProperty:"initiatorName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.iqn !== undefined){
let data4 = data.iqn;
const _errs6 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/iqn",parentData:data,parentDataProperty:"iqn",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.iscsiInterface !== undefined){
let data5 = data.iscsiInterface;
const _errs7 = errors;
if(!(validate604(data5, {instancePath:instancePath+"/iscsiInterface",parentData:data,parentDataProperty:"iscsiInterface",rootData}))){
vErrors = vErrors === null ? validate604.errors : vErrors.concat(validate604.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.lun !== undefined){
let data6 = data.lun;
const _errs8 = errors;
if(!(validate31(data6, {instancePath:instancePath+"/lun",parentData:data,parentDataProperty:"lun",rootData}))){
vErrors = vErrors === null ? validate31.errors : vErrors.concat(validate31.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.portals !== undefined){
let data7 = data.portals;
const _errs9 = errors;
if(!(validate17(data7, {instancePath:instancePath+"/portals",parentData:data,parentDataProperty:"portals",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.readOnly !== undefined){
let data8 = data.readOnly;
const _errs10 = errors;
if(!(validate2(data8, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.secretRef !== undefined){
let data9 = data.secretRef;
const _errs11 = errors;
if(!(validate244(data9, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate244.errors : vErrors.concat(validate244.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.targetPortal !== undefined){
let data10 = data.targetPortal;
const _errs12 = errors;
if(!(validate1(data10, {instancePath:instancePath+"/targetPortal",parentData:data,parentDataProperty:"targetPortal",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
}
validate598.errors = vErrors;
return errors === 0;
}

const schema153 = {"properties":{"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"server":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["path","server"],"type":"object","nullable":true};

function validate613(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.path === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "path"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.server === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "server"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.path !== undefined){
let data0 = data.path;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.readOnly !== undefined){
let data1 = data.readOnly;
const _errs3 = errors;
if(!(validate2(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.server !== undefined){
let data2 = data.server;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/server",parentData:data,parentDataProperty:"server",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate613.errors = vErrors;
return errors === 0;
}

const schema154 = {"properties":{"claimName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"required":["claimName"],"type":"object","nullable":true};

function validate618(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.claimName === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "claimName"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.claimName !== undefined){
let data0 = data.claimName;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/claimName",parentData:data,parentDataProperty:"claimName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.readOnly !== undefined){
let data1 = data.readOnly;
const _errs3 = errors;
if(!(validate2(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate618.errors = vErrors;
return errors === 0;
}

const schema155 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"pdID":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["pdID"],"type":"object","nullable":true};

function validate622(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.pdID === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "pdID"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.pdID !== undefined){
let data1 = data.pdID;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/pdID",parentData:data,parentDataProperty:"pdID",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate622.errors = vErrors;
return errors === 0;
}

const schema156 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeID":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["volumeID"],"type":"object","nullable":true};

function validate626(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.volumeID === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "volumeID"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.readOnly !== undefined){
let data1 = data.readOnly;
const _errs3 = errors;
if(!(validate2(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.volumeID !== undefined){
let data2 = data.volumeID;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/volumeID",parentData:data,parentDataProperty:"volumeID",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate626.errors = vErrors;
return errors === 0;
}

const schema157 = {"properties":{"defaultMode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"sources":{"$ref":"yXKZADi5kgSwjsI2ppIyeMUpu4bJiUFINpGosidspnQ"}},"type":"object","nullable":true};
const schema158 = {"items":{"$ref":"dn4YVt3SWkPnEZSpnhO3KeVzR4VIpu7pqfcL-TcKd_8"},"type":"array","nullable":true};
const schema159 = {"properties":{"clusterTrustBundle":{"$ref":"pTiPjbxwypYoy6bApVo9pwc1crR9_v84NPbQ_C3oPbY"},"configMap":{"$ref":"_HCmSZsdcRhpFKkZM7Jl5QCi9tmpzo8UCET9LQeHx0w"},"downwardAPI":{"$ref":"lFpI7xGpYDupYMDE5JdT4IlnCI2yCKkoVlJch0sNjdg"},"podCertificate":{"$ref":"Kcg0-qwUIoLjyZwuajb1VKncuRc2pFltN23i8If-VrM"},"secret":{"$ref":"_HCmSZsdcRhpFKkZM7Jl5QCi9tmpzo8UCET9LQeHx0w"},"serviceAccountToken":{"$ref":"GWk01BlhTg6m5b0ywNZc8vdrp7TAMZkgdXFVQl52C0E"}},"type":"object"};
const schema160 = {"properties":{"labelSelector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"signerName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["path"],"type":"object","nullable":true};

function validate635(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.path === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "path"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.labelSelector !== undefined){
let data0 = data.labelSelector;
const _errs2 = errors;
if(!(validate28(data0, {instancePath:instancePath+"/labelSelector",parentData:data,parentDataProperty:"labelSelector",rootData}))){
vErrors = vErrors === null ? validate28.errors : vErrors.concat(validate28.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.name !== undefined){
let data1 = data.name;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.optional !== undefined){
let data2 = data.optional;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.path !== undefined){
let data3 = data.path;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.signerName !== undefined){
let data4 = data.signerName;
const _errs6 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/signerName",parentData:data,parentDataProperty:"signerName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate635.errors = vErrors;
return errors === 0;
}

const schema77 = {"properties":{"items":{"$ref":"gPtCVyFH2eXv-J44JI3istIuhC40N-NdI4mRYcFGaCI"},"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"type":"object","nullable":true};

function validate265(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.items !== undefined){
let data0 = data.items;
const _errs2 = errors;
if(!(validate252(data0, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate252.errors : vErrors.concat(validate252.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.name !== undefined){
let data1 = data.name;
const _errs3 = errors;
if(!(validate51(data1, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate51.errors : vErrors.concat(validate51.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.optional !== undefined){
let data2 = data.optional;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate265.errors = vErrors;
return errors === 0;
}

const schema161 = {"properties":{"items":{"$ref":"d-PZJWo9tuJUK1sF4I9c0c4HI5ec_LUrbkqbJN0kxNA"}},"type":"object","nullable":true};

function validate643(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.items !== undefined){
let data0 = data.items;
const _errs2 = errors;
if(!(validate258(data0, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate258.errors : vErrors.concat(validate258.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate643.errors = vErrors;
return errors === 0;
}

const schema162 = {"properties":{"certificateChainPath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"credentialBundlePath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"keyPath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"keyType":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"maxExpirationSeconds":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"signerName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"userAnnotations":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"}},"required":["keyType","signerName"],"type":"object","nullable":true};

function validate646(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.keyType === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "keyType"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.signerName === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "signerName"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.certificateChainPath !== undefined){
let data0 = data.certificateChainPath;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/certificateChainPath",parentData:data,parentDataProperty:"certificateChainPath",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.credentialBundlePath !== undefined){
let data1 = data.credentialBundlePath;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/credentialBundlePath",parentData:data,parentDataProperty:"credentialBundlePath",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.keyPath !== undefined){
let data2 = data.keyPath;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/keyPath",parentData:data,parentDataProperty:"keyPath",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.keyType !== undefined){
let data3 = data.keyType;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/keyType",parentData:data,parentDataProperty:"keyType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.maxExpirationSeconds !== undefined){
let data4 = data.maxExpirationSeconds;
const _errs6 = errors;
if(!(validate85(data4, {instancePath:instancePath+"/maxExpirationSeconds",parentData:data,parentDataProperty:"maxExpirationSeconds",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.signerName !== undefined){
let data5 = data.signerName;
const _errs7 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/signerName",parentData:data,parentDataProperty:"signerName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.userAnnotations !== undefined){
let data6 = data.userAnnotations;
const _errs8 = errors;
if(!(validate15(data6, {instancePath:instancePath+"/userAnnotations",parentData:data,parentDataProperty:"userAnnotations",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
}
validate646.errors = vErrors;
return errors === 0;
}

const schema163 = {"properties":{"audience":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"expirationSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["path"],"type":"object","nullable":true};

function validate656(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.path === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "path"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.audience !== undefined){
let data0 = data.audience;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/audience",parentData:data,parentDataProperty:"audience",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.expirationSeconds !== undefined){
let data1 = data.expirationSeconds;
const _errs3 = errors;
if(!(validate86(data1, {instancePath:instancePath+"/expirationSeconds",parentData:data,parentDataProperty:"expirationSeconds",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.path !== undefined){
let data2 = data.path;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate656.errors = vErrors;
return errors === 0;
}


function validate634(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.clusterTrustBundle !== undefined){
let data0 = data.clusterTrustBundle;
const _errs1 = errors;
if(!(validate635(data0, {instancePath:instancePath+"/clusterTrustBundle",parentData:data,parentDataProperty:"clusterTrustBundle",rootData}))){
vErrors = vErrors === null ? validate635.errors : vErrors.concat(validate635.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.configMap !== undefined){
let data1 = data.configMap;
const _errs2 = errors;
if(!(validate265(data1, {instancePath:instancePath+"/configMap",parentData:data,parentDataProperty:"configMap",rootData}))){
vErrors = vErrors === null ? validate265.errors : vErrors.concat(validate265.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.downwardAPI !== undefined){
let data2 = data.downwardAPI;
const _errs3 = errors;
if(!(validate643(data2, {instancePath:instancePath+"/downwardAPI",parentData:data,parentDataProperty:"downwardAPI",rootData}))){
vErrors = vErrors === null ? validate643.errors : vErrors.concat(validate643.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.podCertificate !== undefined){
let data3 = data.podCertificate;
const _errs4 = errors;
if(!(validate646(data3, {instancePath:instancePath+"/podCertificate",parentData:data,parentDataProperty:"podCertificate",rootData}))){
vErrors = vErrors === null ? validate646.errors : vErrors.concat(validate646.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.secret !== undefined){
let data4 = data.secret;
const _errs5 = errors;
if(!(validate265(data4, {instancePath:instancePath+"/secret",parentData:data,parentDataProperty:"secret",rootData}))){
vErrors = vErrors === null ? validate265.errors : vErrors.concat(validate265.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.serviceAccountToken !== undefined){
let data5 = data.serviceAccountToken;
const _errs6 = errors;
if(!(validate656(data5, {instancePath:instancePath+"/serviceAccountToken",parentData:data,parentDataProperty:"serviceAccountToken",rootData}))){
vErrors = vErrors === null ? validate656.errors : vErrors.concat(validate656.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate634.errors = vErrors;
return errors === 0;
}


function validate633(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate634(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate634.errors : vErrors.concat(validate634.errors);
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
validate633.errors = vErrors;
return errors === 0;
}


function validate631(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.defaultMode !== undefined){
let data0 = data.defaultMode;
const _errs2 = errors;
if(!(validate85(data0, {instancePath:instancePath+"/defaultMode",parentData:data,parentDataProperty:"defaultMode",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.sources !== undefined){
let data1 = data.sources;
const _errs3 = errors;
if(!(validate633(data1, {instancePath:instancePath+"/sources",parentData:data,parentDataProperty:"sources",rootData}))){
vErrors = vErrors === null ? validate633.errors : vErrors.concat(validate633.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate631.errors = vErrors;
return errors === 0;
}

const schema164 = {"properties":{"group":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"registry":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"tenant":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"user":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volume":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["registry","volume"],"type":"object","nullable":true};

function validate664(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.registry === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "registry"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.volume === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "volume"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.group !== undefined){
let data0 = data.group;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/group",parentData:data,parentDataProperty:"group",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.readOnly !== undefined){
let data1 = data.readOnly;
const _errs3 = errors;
if(!(validate2(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.registry !== undefined){
let data2 = data.registry;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/registry",parentData:data,parentDataProperty:"registry",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
if(data.user !== undefined){
let data4 = data.user;
const _errs6 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/user",parentData:data,parentDataProperty:"user",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.volume !== undefined){
let data5 = data.volume;
const _errs7 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/volume",parentData:data,parentDataProperty:"volume",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
}
validate664.errors = vErrors;
return errors === 0;
}

const schema165 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"image":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"keyring":{"$ref":"qJqT86B0L2JUpaRmtZDpYjFJgt4rjgnvcEUS3zGb1eI"},"monitors":{"$ref":"uqUtVyG9D2UAWDvqosEgVMtwK5kI3TMy2eLbDZB452I"},"pool":{"$ref":"Dh86ML4rFjEcx1-u_8ZgfsmYcv8nNBqGgZwoQ3ylFNU"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"user":{"$ref":"LvEyp1RPo8AnrMLndf_Vv4zih3rUP2kwyU22RiXstYI"}},"required":["image","monitors"],"type":"object","nullable":true};
const schema166 = {"default":"/etc/ceph/keyring","type":"string","nullable":true};

function validate675(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate675.errors = vErrors;
return errors === 0;
}

const schema167 = {"default":"rbd","type":"string","nullable":true};

function validate678(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate678.errors = vErrors;
return errors === 0;
}

const schema168 = {"default":"admin","type":"string","nullable":true};

function validate682(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate682.errors = vErrors;
return errors === 0;
}


function validate672(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.monitors === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "monitors"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.image !== undefined){
let data1 = data.image;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.keyring !== undefined){
let data2 = data.keyring;
const _errs4 = errors;
if(!(validate675(data2, {instancePath:instancePath+"/keyring",parentData:data,parentDataProperty:"keyring",rootData}))){
vErrors = vErrors === null ? validate675.errors : vErrors.concat(validate675.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.monitors !== undefined){
let data3 = data.monitors;
const _errs5 = errors;
if(!(validate6(data3, {instancePath:instancePath+"/monitors",parentData:data,parentDataProperty:"monitors",rootData}))){
vErrors = vErrors === null ? validate6.errors : vErrors.concat(validate6.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.pool !== undefined){
let data4 = data.pool;
const _errs6 = errors;
if(!(validate678(data4, {instancePath:instancePath+"/pool",parentData:data,parentDataProperty:"pool",rootData}))){
vErrors = vErrors === null ? validate678.errors : vErrors.concat(validate678.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.readOnly !== undefined){
let data5 = data.readOnly;
const _errs7 = errors;
if(!(validate2(data5, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.secretRef !== undefined){
let data6 = data.secretRef;
const _errs8 = errors;
if(!(validate244(data6, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate244.errors : vErrors.concat(validate244.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.user !== undefined){
let data7 = data.user;
const _errs9 = errors;
if(!(validate682(data7, {instancePath:instancePath+"/user",parentData:data,parentDataProperty:"user",rootData}))){
vErrors = vErrors === null ? validate682.errors : vErrors.concat(validate682.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
}
validate672.errors = vErrors;
return errors === 0;
}

const schema169 = {"properties":{"fsType":{"$ref":"Wgdy0s1CznF5U0Xdixh3dfX6xVZC-f4ZSRagme8xEZA"},"gateway":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"protectionDomain":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"CeclzNr20k5vr0FY8_Di-i5kv_hKd8rGtt9CG90tDrI"},"sslEnabled":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"storageMode":{"$ref":"nMIgymwC0Nh7u9Ub8ryXfL13c9EAIAX1eZzkO47rq5o"},"storagePool":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"system":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"volumeName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["gateway","secretRef","system"],"type":"object","nullable":true};
const schema170 = {"default":"xfs","type":"string","nullable":true};

function validate686(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate686.errors = vErrors;
return errors === 0;
}

const schema171 = {"default":"ThinProvisioned","type":"string","nullable":true};

function validate693(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate693.errors = vErrors;
return errors === 0;
}


function validate685(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.gateway === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "gateway"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.secretRef === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "secretRef"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.system === undefined){
const err3 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "system"}};
if(vErrors === null){
vErrors = [err3];
}
else {
vErrors.push(err3);
}
errors++;
}
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate686(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate686.errors : vErrors.concat(validate686.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.gateway !== undefined){
let data1 = data.gateway;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/gateway",parentData:data,parentDataProperty:"gateway",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.protectionDomain !== undefined){
let data2 = data.protectionDomain;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/protectionDomain",parentData:data,parentDataProperty:"protectionDomain",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.readOnly !== undefined){
let data3 = data.readOnly;
const _errs5 = errors;
if(!(validate2(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.secretRef !== undefined){
let data4 = data.secretRef;
const _errs6 = errors;
if(!(validate269(data4, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate269.errors : vErrors.concat(validate269.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.sslEnabled !== undefined){
let data5 = data.sslEnabled;
const _errs7 = errors;
if(!(validate2(data5, {instancePath:instancePath+"/sslEnabled",parentData:data,parentDataProperty:"sslEnabled",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.storageMode !== undefined){
let data6 = data.storageMode;
const _errs8 = errors;
if(!(validate693(data6, {instancePath:instancePath+"/storageMode",parentData:data,parentDataProperty:"storageMode",rootData}))){
vErrors = vErrors === null ? validate693.errors : vErrors.concat(validate693.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.storagePool !== undefined){
let data7 = data.storagePool;
const _errs9 = errors;
if(!(validate0(data7, {instancePath:instancePath+"/storagePool",parentData:data,parentDataProperty:"storagePool",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.system !== undefined){
let data8 = data.system;
const _errs10 = errors;
if(!(validate1(data8, {instancePath:instancePath+"/system",parentData:data,parentDataProperty:"system",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.volumeName !== undefined){
let data9 = data.volumeName;
const _errs11 = errors;
if(!(validate0(data9, {instancePath:instancePath+"/volumeName",parentData:data,parentDataProperty:"volumeName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
}
validate685.errors = vErrors;
return errors === 0;
}

const schema172 = {"properties":{"defaultMode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"items":{"$ref":"gPtCVyFH2eXv-J44JI3istIuhC40N-NdI4mRYcFGaCI"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate699(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.defaultMode !== undefined){
let data0 = data.defaultMode;
const _errs2 = errors;
if(!(validate85(data0, {instancePath:instancePath+"/defaultMode",parentData:data,parentDataProperty:"defaultMode",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.items !== undefined){
let data1 = data.items;
const _errs3 = errors;
if(!(validate252(data1, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate252.errors : vErrors.concat(validate252.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.optional !== undefined){
let data2 = data.optional;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.secretName !== undefined){
let data3 = data.secretName;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/secretName",parentData:data,parentDataProperty:"secretName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate699.errors = vErrors;
return errors === 0;
}

const schema173 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"volumeName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumeNamespace":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate705(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.readOnly !== undefined){
let data1 = data.readOnly;
const _errs3 = errors;
if(!(validate2(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.secretRef !== undefined){
let data2 = data.secretRef;
const _errs4 = errors;
if(!(validate244(data2, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate244.errors : vErrors.concat(validate244.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.volumeName !== undefined){
let data3 = data.volumeName;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/volumeName",parentData:data,parentDataProperty:"volumeName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.volumeNamespace !== undefined){
let data4 = data.volumeNamespace;
const _errs6 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/volumeNamespace",parentData:data,parentDataProperty:"volumeNamespace",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate705.errors = vErrors;
return errors === 0;
}

const schema174 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"storagePolicyID":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"storagePolicyName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumePath":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["volumePath"],"type":"object","nullable":true};

function validate712(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.volumePath === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "volumePath"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.fsType !== undefined){
let data0 = data.fsType;
const _errs2 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.storagePolicyID !== undefined){
let data1 = data.storagePolicyID;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/storagePolicyID",parentData:data,parentDataProperty:"storagePolicyID",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.storagePolicyName !== undefined){
let data2 = data.storagePolicyName;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/storagePolicyName",parentData:data,parentDataProperty:"storagePolicyName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.volumePath !== undefined){
let data3 = data.volumePath;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/volumePath",parentData:data,parentDataProperty:"volumePath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate712.errors = vErrors;
return errors === 0;
}


function validate472(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "name"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.awsElasticBlockStore !== undefined){
let data0 = data.awsElasticBlockStore;
const _errs1 = errors;
if(!(validate473(data0, {instancePath:instancePath+"/awsElasticBlockStore",parentData:data,parentDataProperty:"awsElasticBlockStore",rootData}))){
vErrors = vErrors === null ? validate473.errors : vErrors.concat(validate473.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.azureDisk !== undefined){
let data1 = data.azureDisk;
const _errs2 = errors;
if(!(validate479(data1, {instancePath:instancePath+"/azureDisk",parentData:data,parentDataProperty:"azureDisk",rootData}))){
vErrors = vErrors === null ? validate479.errors : vErrors.concat(validate479.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.azureFile !== undefined){
let data2 = data.azureFile;
const _errs3 = errors;
if(!(validate488(data2, {instancePath:instancePath+"/azureFile",parentData:data,parentDataProperty:"azureFile",rootData}))){
vErrors = vErrors === null ? validate488.errors : vErrors.concat(validate488.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.cephfs !== undefined){
let data3 = data.cephfs;
const _errs4 = errors;
if(!(validate493(data3, {instancePath:instancePath+"/cephfs",parentData:data,parentDataProperty:"cephfs",rootData}))){
vErrors = vErrors === null ? validate493.errors : vErrors.concat(validate493.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.cinder !== undefined){
let data4 = data.cinder;
const _errs5 = errors;
if(!(validate501(data4, {instancePath:instancePath+"/cinder",parentData:data,parentDataProperty:"cinder",rootData}))){
vErrors = vErrors === null ? validate501.errors : vErrors.concat(validate501.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.configMap !== undefined){
let data5 = data.configMap;
const _errs6 = errors;
if(!(validate507(data5, {instancePath:instancePath+"/configMap",parentData:data,parentDataProperty:"configMap",rootData}))){
vErrors = vErrors === null ? validate507.errors : vErrors.concat(validate507.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.csi !== undefined){
let data6 = data.csi;
const _errs7 = errors;
if(!(validate513(data6, {instancePath:instancePath+"/csi",parentData:data,parentDataProperty:"csi",rootData}))){
vErrors = vErrors === null ? validate513.errors : vErrors.concat(validate513.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.downwardAPI !== undefined){
let data7 = data.downwardAPI;
const _errs8 = errors;
if(!(validate520(data7, {instancePath:instancePath+"/downwardAPI",parentData:data,parentDataProperty:"downwardAPI",rootData}))){
vErrors = vErrors === null ? validate520.errors : vErrors.concat(validate520.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.emptyDir !== undefined){
let data8 = data.emptyDir;
const _errs9 = errors;
if(!(validate524(data8, {instancePath:instancePath+"/emptyDir",parentData:data,parentDataProperty:"emptyDir",rootData}))){
vErrors = vErrors === null ? validate524.errors : vErrors.concat(validate524.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.ephemeral !== undefined){
let data9 = data.ephemeral;
const _errs10 = errors;
if(!(validate528(data9, {instancePath:instancePath+"/ephemeral",parentData:data,parentDataProperty:"ephemeral",rootData}))){
vErrors = vErrors === null ? validate528.errors : vErrors.concat(validate528.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.fc !== undefined){
let data10 = data.fc;
const _errs11 = errors;
if(!(validate556(data10, {instancePath:instancePath+"/fc",parentData:data,parentDataProperty:"fc",rootData}))){
vErrors = vErrors === null ? validate556.errors : vErrors.concat(validate556.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.flexVolume !== undefined){
let data11 = data.flexVolume;
const _errs12 = errors;
if(!(validate563(data11, {instancePath:instancePath+"/flexVolume",parentData:data,parentDataProperty:"flexVolume",rootData}))){
vErrors = vErrors === null ? validate563.errors : vErrors.concat(validate563.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.flocker !== undefined){
let data12 = data.flocker;
const _errs13 = errors;
if(!(validate570(data12, {instancePath:instancePath+"/flocker",parentData:data,parentDataProperty:"flocker",rootData}))){
vErrors = vErrors === null ? validate570.errors : vErrors.concat(validate570.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.gcePersistentDisk !== undefined){
let data13 = data.gcePersistentDisk;
const _errs14 = errors;
if(!(validate574(data13, {instancePath:instancePath+"/gcePersistentDisk",parentData:data,parentDataProperty:"gcePersistentDisk",rootData}))){
vErrors = vErrors === null ? validate574.errors : vErrors.concat(validate574.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
if(data.gitRepo !== undefined){
let data14 = data.gitRepo;
const _errs15 = errors;
if(!(validate580(data14, {instancePath:instancePath+"/gitRepo",parentData:data,parentDataProperty:"gitRepo",rootData}))){
vErrors = vErrors === null ? validate580.errors : vErrors.concat(validate580.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs15 === errors;
}
if(data.glusterfs !== undefined){
let data15 = data.glusterfs;
const _errs16 = errors;
if(!(validate585(data15, {instancePath:instancePath+"/glusterfs",parentData:data,parentDataProperty:"glusterfs",rootData}))){
vErrors = vErrors === null ? validate585.errors : vErrors.concat(validate585.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs16 === errors;
}
if(data.hostPath !== undefined){
let data16 = data.hostPath;
const _errs17 = errors;
if(!(validate590(data16, {instancePath:instancePath+"/hostPath",parentData:data,parentDataProperty:"hostPath",rootData}))){
vErrors = vErrors === null ? validate590.errors : vErrors.concat(validate590.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs17 === errors;
}
if(data.image !== undefined){
let data17 = data.image;
const _errs18 = errors;
if(!(validate594(data17, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate594.errors : vErrors.concat(validate594.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs18 === errors;
}
if(data.iscsi !== undefined){
let data18 = data.iscsi;
const _errs19 = errors;
if(!(validate598(data18, {instancePath:instancePath+"/iscsi",parentData:data,parentDataProperty:"iscsi",rootData}))){
vErrors = vErrors === null ? validate598.errors : vErrors.concat(validate598.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs19 === errors;
}
if(data.name !== undefined){
let data19 = data.name;
const _errs20 = errors;
if(!(validate1(data19, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs20 === errors;
}
if(data.nfs !== undefined){
let data20 = data.nfs;
const _errs21 = errors;
if(!(validate613(data20, {instancePath:instancePath+"/nfs",parentData:data,parentDataProperty:"nfs",rootData}))){
vErrors = vErrors === null ? validate613.errors : vErrors.concat(validate613.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs21 === errors;
}
if(data.persistentVolumeClaim !== undefined){
let data21 = data.persistentVolumeClaim;
const _errs22 = errors;
if(!(validate618(data21, {instancePath:instancePath+"/persistentVolumeClaim",parentData:data,parentDataProperty:"persistentVolumeClaim",rootData}))){
vErrors = vErrors === null ? validate618.errors : vErrors.concat(validate618.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs22 === errors;
}
if(data.photonPersistentDisk !== undefined){
let data22 = data.photonPersistentDisk;
const _errs23 = errors;
if(!(validate622(data22, {instancePath:instancePath+"/photonPersistentDisk",parentData:data,parentDataProperty:"photonPersistentDisk",rootData}))){
vErrors = vErrors === null ? validate622.errors : vErrors.concat(validate622.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs23 === errors;
}
if(data.portworxVolume !== undefined){
let data23 = data.portworxVolume;
const _errs24 = errors;
if(!(validate626(data23, {instancePath:instancePath+"/portworxVolume",parentData:data,parentDataProperty:"portworxVolume",rootData}))){
vErrors = vErrors === null ? validate626.errors : vErrors.concat(validate626.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs24 === errors;
}
if(data.projected !== undefined){
let data24 = data.projected;
const _errs25 = errors;
if(!(validate631(data24, {instancePath:instancePath+"/projected",parentData:data,parentDataProperty:"projected",rootData}))){
vErrors = vErrors === null ? validate631.errors : vErrors.concat(validate631.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs25 === errors;
}
if(data.quobyte !== undefined){
let data25 = data.quobyte;
const _errs26 = errors;
if(!(validate664(data25, {instancePath:instancePath+"/quobyte",parentData:data,parentDataProperty:"quobyte",rootData}))){
vErrors = vErrors === null ? validate664.errors : vErrors.concat(validate664.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs26 === errors;
}
if(data.rbd !== undefined){
let data26 = data.rbd;
const _errs27 = errors;
if(!(validate672(data26, {instancePath:instancePath+"/rbd",parentData:data,parentDataProperty:"rbd",rootData}))){
vErrors = vErrors === null ? validate672.errors : vErrors.concat(validate672.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs27 === errors;
}
if(data.scaleIO !== undefined){
let data27 = data.scaleIO;
const _errs28 = errors;
if(!(validate685(data27, {instancePath:instancePath+"/scaleIO",parentData:data,parentDataProperty:"scaleIO",rootData}))){
vErrors = vErrors === null ? validate685.errors : vErrors.concat(validate685.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs28 === errors;
}
if(data.secret !== undefined){
let data28 = data.secret;
const _errs29 = errors;
if(!(validate699(data28, {instancePath:instancePath+"/secret",parentData:data,parentDataProperty:"secret",rootData}))){
vErrors = vErrors === null ? validate699.errors : vErrors.concat(validate699.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs29 === errors;
}
if(data.storageos !== undefined){
let data29 = data.storageos;
const _errs30 = errors;
if(!(validate705(data29, {instancePath:instancePath+"/storageos",parentData:data,parentDataProperty:"storageos",rootData}))){
vErrors = vErrors === null ? validate705.errors : vErrors.concat(validate705.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs30 === errors;
}
if(data.vsphereVolume !== undefined){
let data30 = data.vsphereVolume;
const _errs31 = errors;
if(!(validate712(data30, {instancePath:instancePath+"/vsphereVolume",parentData:data,parentDataProperty:"vsphereVolume",rootData}))){
vErrors = vErrors === null ? validate712.errors : vErrors.concat(validate712.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs31 === errors;
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
validate472.errors = vErrors;
return errors === 0;
}


function validate471(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate472(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate472.errors : vErrors.concat(validate472.errors);
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
validate471.errors = vErrors;
return errors === 0;
}


function validate311(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.containers === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "containers"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.activeDeadlineSeconds !== undefined){
let data0 = data.activeDeadlineSeconds;
const _errs2 = errors;
if(!(validate86(data0, {instancePath:instancePath+"/activeDeadlineSeconds",parentData:data,parentDataProperty:"activeDeadlineSeconds",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.affinity !== undefined){
let data1 = data.affinity;
const _errs3 = errors;
if(!(validate313(data1, {instancePath:instancePath+"/affinity",parentData:data,parentDataProperty:"affinity",rootData}))){
vErrors = vErrors === null ? validate313.errors : vErrors.concat(validate313.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.automountServiceAccountToken !== undefined){
let data2 = data.automountServiceAccountToken;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/automountServiceAccountToken",parentData:data,parentDataProperty:"automountServiceAccountToken",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.containers !== undefined){
let data3 = data.containers;
const _errs5 = errors;
if(!(validate331(data3, {instancePath:instancePath+"/containers",parentData:data,parentDataProperty:"containers",rootData}))){
vErrors = vErrors === null ? validate331.errors : vErrors.concat(validate331.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.dnsConfig !== undefined){
let data4 = data.dnsConfig;
const _errs6 = errors;
if(!(validate334(data4, {instancePath:instancePath+"/dnsConfig",parentData:data,parentDataProperty:"dnsConfig",rootData}))){
vErrors = vErrors === null ? validate334.errors : vErrors.concat(validate334.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.dnsPolicy !== undefined){
let data5 = data.dnsPolicy;
const _errs7 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/dnsPolicy",parentData:data,parentDataProperty:"dnsPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.enableServiceLinks !== undefined){
let data6 = data.enableServiceLinks;
const _errs8 = errors;
if(!(validate2(data6, {instancePath:instancePath+"/enableServiceLinks",parentData:data,parentDataProperty:"enableServiceLinks",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.ephemeralContainers !== undefined){
let data7 = data.ephemeralContainers;
const _errs9 = errors;
if(!(validate346(data7, {instancePath:instancePath+"/ephemeralContainers",parentData:data,parentDataProperty:"ephemeralContainers",rootData}))){
vErrors = vErrors === null ? validate346.errors : vErrors.concat(validate346.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.hostAliases !== undefined){
let data8 = data.hostAliases;
const _errs10 = errors;
if(!(validate376(data8, {instancePath:instancePath+"/hostAliases",parentData:data,parentDataProperty:"hostAliases",rootData}))){
vErrors = vErrors === null ? validate376.errors : vErrors.concat(validate376.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.hostIPC !== undefined){
let data9 = data.hostIPC;
const _errs11 = errors;
if(!(validate2(data9, {instancePath:instancePath+"/hostIPC",parentData:data,parentDataProperty:"hostIPC",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.hostNetwork !== undefined){
let data10 = data.hostNetwork;
const _errs12 = errors;
if(!(validate2(data10, {instancePath:instancePath+"/hostNetwork",parentData:data,parentDataProperty:"hostNetwork",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.hostPID !== undefined){
let data11 = data.hostPID;
const _errs13 = errors;
if(!(validate2(data11, {instancePath:instancePath+"/hostPID",parentData:data,parentDataProperty:"hostPID",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.hostUsers !== undefined){
let data12 = data.hostUsers;
const _errs14 = errors;
if(!(validate2(data12, {instancePath:instancePath+"/hostUsers",parentData:data,parentDataProperty:"hostUsers",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
if(data.hostname !== undefined){
let data13 = data.hostname;
const _errs15 = errors;
if(!(validate0(data13, {instancePath:instancePath+"/hostname",parentData:data,parentDataProperty:"hostname",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs15 === errors;
}
if(data.hostnameOverride !== undefined){
let data14 = data.hostnameOverride;
const _errs16 = errors;
if(!(validate0(data14, {instancePath:instancePath+"/hostnameOverride",parentData:data,parentDataProperty:"hostnameOverride",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs16 === errors;
}
if(data.imagePullSecrets !== undefined){
let data15 = data.imagePullSecrets;
const _errs17 = errors;
if(!(validate388(data15, {instancePath:instancePath+"/imagePullSecrets",parentData:data,parentDataProperty:"imagePullSecrets",rootData}))){
vErrors = vErrors === null ? validate388.errors : vErrors.concat(validate388.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs17 === errors;
}
if(data.initContainers !== undefined){
let data16 = data.initContainers;
const _errs18 = errors;
if(!(validate391(data16, {instancePath:instancePath+"/initContainers",parentData:data,parentDataProperty:"initContainers",rootData}))){
vErrors = vErrors === null ? validate391.errors : vErrors.concat(validate391.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs18 === errors;
}
if(data.nodeName !== undefined){
let data17 = data.nodeName;
const _errs19 = errors;
if(!(validate0(data17, {instancePath:instancePath+"/nodeName",parentData:data,parentDataProperty:"nodeName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs19 === errors;
}
if(data.nodeSelector !== undefined){
let data18 = data.nodeSelector;
const _errs20 = errors;
if(!(validate15(data18, {instancePath:instancePath+"/nodeSelector",parentData:data,parentDataProperty:"nodeSelector",rootData}))){
vErrors = vErrors === null ? validate15.errors : vErrors.concat(validate15.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs20 === errors;
}
if(data.os !== undefined){
let data19 = data.os;
const _errs21 = errors;
if(!(validate396(data19, {instancePath:instancePath+"/os",parentData:data,parentDataProperty:"os",rootData}))){
vErrors = vErrors === null ? validate396.errors : vErrors.concat(validate396.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs21 === errors;
}
if(data.overhead !== undefined){
let data20 = data.overhead;
const _errs22 = errors;
if(!(validate104(data20, {instancePath:instancePath+"/overhead",parentData:data,parentDataProperty:"overhead",rootData}))){
vErrors = vErrors === null ? validate104.errors : vErrors.concat(validate104.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs22 === errors;
}
if(data.preemptionPolicy !== undefined){
let data21 = data.preemptionPolicy;
const _errs23 = errors;
if(!(validate0(data21, {instancePath:instancePath+"/preemptionPolicy",parentData:data,parentDataProperty:"preemptionPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs23 === errors;
}
if(data.priority !== undefined){
let data22 = data.priority;
const _errs24 = errors;
if(!(validate85(data22, {instancePath:instancePath+"/priority",parentData:data,parentDataProperty:"priority",rootData}))){
vErrors = vErrors === null ? validate85.errors : vErrors.concat(validate85.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs24 === errors;
}
if(data.priorityClassName !== undefined){
let data23 = data.priorityClassName;
const _errs25 = errors;
if(!(validate0(data23, {instancePath:instancePath+"/priorityClassName",parentData:data,parentDataProperty:"priorityClassName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs25 === errors;
}
if(data.readinessGates !== undefined){
let data24 = data.readinessGates;
const _errs26 = errors;
if(!(validate403(data24, {instancePath:instancePath+"/readinessGates",parentData:data,parentDataProperty:"readinessGates",rootData}))){
vErrors = vErrors === null ? validate403.errors : vErrors.concat(validate403.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs26 === errors;
}
if(data.resourceClaims !== undefined){
let data25 = data.resourceClaims;
const _errs27 = errors;
if(!(validate408(data25, {instancePath:instancePath+"/resourceClaims",parentData:data,parentDataProperty:"resourceClaims",rootData}))){
vErrors = vErrors === null ? validate408.errors : vErrors.concat(validate408.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs27 === errors;
}
if(data.resources !== undefined){
let data26 = data.resources;
const _errs28 = errors;
if(!(validate158(data26, {instancePath:instancePath+"/resources",parentData:data,parentDataProperty:"resources",rootData}))){
vErrors = vErrors === null ? validate158.errors : vErrors.concat(validate158.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs28 === errors;
}
if(data.restartPolicy !== undefined){
let data27 = data.restartPolicy;
const _errs29 = errors;
if(!(validate0(data27, {instancePath:instancePath+"/restartPolicy",parentData:data,parentDataProperty:"restartPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs29 === errors;
}
if(data.runtimeClassName !== undefined){
let data28 = data.runtimeClassName;
const _errs30 = errors;
if(!(validate0(data28, {instancePath:instancePath+"/runtimeClassName",parentData:data,parentDataProperty:"runtimeClassName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs30 === errors;
}
if(data.schedulerName !== undefined){
let data29 = data.schedulerName;
const _errs31 = errors;
if(!(validate0(data29, {instancePath:instancePath+"/schedulerName",parentData:data,parentDataProperty:"schedulerName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs31 === errors;
}
if(data.schedulingGates !== undefined){
let data30 = data.schedulingGates;
const _errs32 = errors;
if(!(validate419(data30, {instancePath:instancePath+"/schedulingGates",parentData:data,parentDataProperty:"schedulingGates",rootData}))){
vErrors = vErrors === null ? validate419.errors : vErrors.concat(validate419.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs32 === errors;
}
if(data.schedulingGroup !== undefined){
let data31 = data.schedulingGroup;
const _errs33 = errors;
if(!(validate424(data31, {instancePath:instancePath+"/schedulingGroup",parentData:data,parentDataProperty:"schedulingGroup",rootData}))){
vErrors = vErrors === null ? validate424.errors : vErrors.concat(validate424.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs33 === errors;
}
if(data.securityContext !== undefined){
let data32 = data.securityContext;
const _errs34 = errors;
if(!(validate427(data32, {instancePath:instancePath+"/securityContext",parentData:data,parentDataProperty:"securityContext",rootData}))){
vErrors = vErrors === null ? validate427.errors : vErrors.concat(validate427.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs34 === errors;
}
if(data.serviceAccount !== undefined){
let data33 = data.serviceAccount;
const _errs35 = errors;
if(!(validate0(data33, {instancePath:instancePath+"/serviceAccount",parentData:data,parentDataProperty:"serviceAccount",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs35 === errors;
}
if(data.serviceAccountName !== undefined){
let data34 = data.serviceAccountName;
const _errs36 = errors;
if(!(validate0(data34, {instancePath:instancePath+"/serviceAccountName",parentData:data,parentDataProperty:"serviceAccountName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs36 === errors;
}
if(data.setHostnameAsFQDN !== undefined){
let data35 = data.setHostnameAsFQDN;
const _errs37 = errors;
if(!(validate2(data35, {instancePath:instancePath+"/setHostnameAsFQDN",parentData:data,parentDataProperty:"setHostnameAsFQDN",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs37 === errors;
}
if(data.shareProcessNamespace !== undefined){
let data36 = data.shareProcessNamespace;
const _errs38 = errors;
if(!(validate2(data36, {instancePath:instancePath+"/shareProcessNamespace",parentData:data,parentDataProperty:"shareProcessNamespace",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs38 === errors;
}
if(data.subdomain !== undefined){
let data37 = data.subdomain;
const _errs39 = errors;
if(!(validate0(data37, {instancePath:instancePath+"/subdomain",parentData:data,parentDataProperty:"subdomain",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs39 === errors;
}
if(data.terminationGracePeriodSeconds !== undefined){
let data38 = data.terminationGracePeriodSeconds;
const _errs40 = errors;
if(!(validate86(data38, {instancePath:instancePath+"/terminationGracePeriodSeconds",parentData:data,parentDataProperty:"terminationGracePeriodSeconds",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs40 === errors;
}
if(data.tolerations !== undefined){
let data39 = data.tolerations;
const _errs41 = errors;
if(!(validate450(data39, {instancePath:instancePath+"/tolerations",parentData:data,parentDataProperty:"tolerations",rootData}))){
vErrors = vErrors === null ? validate450.errors : vErrors.concat(validate450.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs41 === errors;
}
if(data.topologySpreadConstraints !== undefined){
let data40 = data.topologySpreadConstraints;
const _errs42 = errors;
if(!(validate459(data40, {instancePath:instancePath+"/topologySpreadConstraints",parentData:data,parentDataProperty:"topologySpreadConstraints",rootData}))){
vErrors = vErrors === null ? validate459.errors : vErrors.concat(validate459.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs42 === errors;
}
if(data.volumes !== undefined){
let data41 = data.volumes;
const _errs43 = errors;
if(!(validate471(data41, {instancePath:instancePath+"/volumes",parentData:data,parentDataProperty:"volumes",rootData}))){
vErrors = vErrors === null ? validate471.errors : vErrors.concat(validate471.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs43 === errors;
}
}
validate311.errors = vErrors;
return errors === 0;
}


function validate309(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.metadata !== undefined){
let data0 = data.metadata;
const _errs2 = errors;
if(!(validate246(data0, {instancePath:instancePath+"/metadata",parentData:data,parentDataProperty:"metadata",rootData}))){
vErrors = vErrors === null ? validate246.errors : vErrors.concat(validate246.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.spec !== undefined){
let data1 = data.spec;
const _errs3 = errors;
if(!(validate311(data1, {instancePath:instancePath+"/spec",parentData:data,parentDataProperty:"spec",rootData}))){
vErrors = vErrors === null ? validate311.errors : vErrors.concat(validate311.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate309.errors = vErrors;
return errors === 0;
}


function validate305(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.carrier === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "carrier"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.version === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "version"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.carrier !== undefined){
let data0 = data.carrier;
const _errs1 = errors;
if(!(validate10(data0, {instancePath:instancePath+"/carrier",parentData:data,parentDataProperty:"carrier",rootData}))){
vErrors = vErrors === null ? validate10.errors : vErrors.concat(validate10.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.kind !== undefined){
let data1 = data.kind;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.parameters !== undefined){
let data2 = data.parameters;
const _errs3 = errors;
if(!(validate11(data2, {instancePath:instancePath+"/parameters",parentData:data,parentDataProperty:"parameters",rootData}))){
vErrors = vErrors === null ? validate11.errors : vErrors.concat(validate11.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.template !== undefined){
let data3 = data.template;
const _errs4 = errors;
if(!(validate309(data3, {instancePath:instancePath+"/template",parentData:data,parentDataProperty:"template",rootData}))){
vErrors = vErrors === null ? validate309.errors : vErrors.concat(validate309.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.version !== undefined){
let data4 = data.version;
const _errs5 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/version",parentData:data,parentDataProperty:"version",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate305.errors = vErrors;
return errors === 0;
}


function validate288(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.placement === undefined){
const err0 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "placement"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
if(data.role === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "role"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.runtime === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "runtime"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.workload === undefined){
const err3 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "workload"}};
if(vErrors === null){
vErrors = [err3];
}
else {
vErrors.push(err3);
}
errors++;
}
if(data.downstreamName !== undefined){
let data0 = data.downstreamName;
const _errs1 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/downstreamName",parentData:data,parentDataProperty:"downstreamName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.environments !== undefined){
let data1 = data.environments;
const _errs2 = errors;
if(!(validate290(data1, {instancePath:instancePath+"/environments",parentData:data,parentDataProperty:"environments",rootData}))){
vErrors = vErrors === null ? validate290.errors : vErrors.concat(validate290.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.head !== undefined){
let data2 = data.head;
const _errs3 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/head",parentData:data,parentDataProperty:"head",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.name !== undefined){
let data3 = data.name;
const _errs4 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.placement !== undefined){
let data4 = data.placement;
const _errs5 = errors;
if(!(validate295(data4, {instancePath:instancePath+"/placement",parentData:data,parentDataProperty:"placement",rootData}))){
vErrors = vErrors === null ? validate295.errors : vErrors.concat(validate295.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.role !== undefined){
let data5 = data.role;
const _errs6 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/role",parentData:data,parentDataProperty:"role",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.runtime !== undefined){
let data6 = data.runtime;
const _errs7 = errors;
if(!(validate299(data6, {instancePath:instancePath+"/runtime",parentData:data,parentDataProperty:"runtime",rootData}))){
vErrors = vErrors === null ? validate299.errors : vErrors.concat(validate299.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.suspended !== undefined){
let data7 = data.suspended;
const _errs8 = errors;
if(!(validate2(data7, {instancePath:instancePath+"/suspended",parentData:data,parentDataProperty:"suspended",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.tags !== undefined){
let data8 = data.tags;
const _errs9 = errors;
if(!(validate3(data8, {instancePath:instancePath+"/tags",parentData:data,parentDataProperty:"tags",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.workload !== undefined){
let data9 = data.workload;
const _errs10 = errors;
if(!(validate305(data9, {instancePath:instancePath+"/workload",parentData:data,parentDataProperty:"workload",rootData}))){
vErrors = vErrors === null ? validate305.errors : vErrors.concat(validate305.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
}
else {
const err4 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err4];
}
else {
vErrors.push(err4);
}
errors++;
}
validate288.errors = vErrors;
return errors === 0;
}


function validate287(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate288(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate288.errors : vErrors.concat(validate288.errors);
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
validate287.errors = vErrors;
return errors === 0;
}


function validate282(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.sshPublicKey !== undefined){
let data1 = data.sshPublicKey;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/sshPublicKey",parentData:data,parentDataProperty:"sshPublicKey",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.suspended !== undefined){
let data2 = data.suspended;
const _errs4 = errors;
if(!(validate2(data2, {instancePath:instancePath+"/suspended",parentData:data,parentDataProperty:"suspended",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.tags !== undefined){
let data3 = data.tags;
const _errs5 = errors;
if(!(validate3(data3, {instancePath:instancePath+"/tags",parentData:data,parentDataProperty:"tags",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.tasks !== undefined){
let data4 = data.tasks;
const _errs6 = errors;
if(!(validate287(data4, {instancePath:instancePath+"/tasks",parentData:data,parentDataProperty:"tasks",rootData}))){
vErrors = vErrors === null ? validate287.errors : vErrors.concat(validate287.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate282.errors = vErrors;
return errors === 0;
}

const schema175 = {"properties":{"conditions":{"$ref":"hhAxL57cvApnLYfNWQbGuX0ymd4M4buVmG7H0q63I0w"},"endTime":{"$ref":"YL2AaRx5mZZL3drs48WsIqN7ZQe66kWzKXDNrUwOgoc"},"observedGeneration":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"phase":{"$ref":"OumCqY-lP6-t1qsO7hLYZf_ZC9lX1mEyjnwbPTz244M"},"startTime":{"$ref":"YL2AaRx5mZZL3drs48WsIqN7ZQe66kWzKXDNrUwOgoc"},"tasks":{"$ref":"Yz-_nj6jItM92Hd5eFw6zdHgDbI94J5LlmZdZvJskX4"}},"type":"object","nullable":true};
const schema176 = {"items":{"$ref":"dZ13d79yfL0AfKGv8XJr0-7aalgpv7dwgkV5EU9HxwM"},"type":"array","nullable":true};
const schema177 = {"properties":{"lastTransitionTime":{"$ref":"nWP098h5Flk6EOMa_OWYW4EnhyCrisyvgJPgk0WtC9c"},"message":{"$ref":"CG805GD3KzsipQt5EKLOQw9SjU_4PVFnnbOjDh0LzJQ"},"observedGeneration":{"$ref":"Tv0gqme9PlVaT-l831KRjUe4u9reG1m_llVaPqPtqUs"},"reason":{"$ref":"UPBfG4e9zcqG6mx3tN1LWinVzYKOFfIO2VnMgg6lAWI"},"status":{"$ref":"UcCRuQZyUlftnmEwuGDZcciwLAdzzRj4HOLofWAwyW8"},"type":{"$ref":"grctw_GwW6iDVZUzowft1rMz55_d64qiJ_wOfyk2Qoo"}},"required":["lastTransitionTime","message","reason","status","type"],"type":"object"};
const schema178 = {"format":"date-time","type":"string"};
const formats8 = formats["date-time"];

function validate730(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if((typeof data == "number") && (isFinite(data))){
}
if(typeof data === "string"){
if(!(formats8.validate(data))){
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
validate730.errors = vErrors;
return errors === 0;
}

const schema179 = {"maxLength":32768,"type":"string"};
import func0 from "@kubernetes-models/validate/runtime/ucs2length";

function validate732(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate732.errors = vErrors;
return errors === 0;
}

const schema180 = {"format":"int64","type":"integer","minimum":0,"nullable":true};

function validate734(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(formats2.validate(data))){
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
validate734.errors = vErrors;
return errors === 0;
}

const schema181 = {"maxLength":1024,"minLength":1,"pattern":"^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$","type":"string"};
const pattern1 = new RegExp("^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$", "u");

function validate736(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate736.errors = vErrors;
return errors === 0;
}

const schema182 = {"enum":["True","False","Unknown"],"type":"string"};

function validate738(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
const vSchema0 = schema182.enum;
if(!(((data === "True") || (data === "False")) || (data === "Unknown"))){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema182.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate738.errors = vErrors;
return errors === 0;
}

const schema183 = {"maxLength":316,"pattern":"^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$","type":"string"};
const pattern2 = new RegExp("^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$", "u");

function validate740(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate740.errors = vErrors;
return errors === 0;
}


function validate729(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate730(data0, {instancePath:instancePath+"/lastTransitionTime",parentData:data,parentDataProperty:"lastTransitionTime",rootData}))){
vErrors = vErrors === null ? validate730.errors : vErrors.concat(validate730.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.message !== undefined){
let data1 = data.message;
const _errs2 = errors;
if(!(validate732(data1, {instancePath:instancePath+"/message",parentData:data,parentDataProperty:"message",rootData}))){
vErrors = vErrors === null ? validate732.errors : vErrors.concat(validate732.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.observedGeneration !== undefined){
let data2 = data.observedGeneration;
const _errs3 = errors;
if(!(validate734(data2, {instancePath:instancePath+"/observedGeneration",parentData:data,parentDataProperty:"observedGeneration",rootData}))){
vErrors = vErrors === null ? validate734.errors : vErrors.concat(validate734.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.reason !== undefined){
let data3 = data.reason;
const _errs4 = errors;
if(!(validate736(data3, {instancePath:instancePath+"/reason",parentData:data,parentDataProperty:"reason",rootData}))){
vErrors = vErrors === null ? validate736.errors : vErrors.concat(validate736.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.status !== undefined){
let data4 = data.status;
const _errs5 = errors;
if(!(validate738(data4, {instancePath:instancePath+"/status",parentData:data,parentDataProperty:"status",rootData}))){
vErrors = vErrors === null ? validate738.errors : vErrors.concat(validate738.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.type !== undefined){
let data5 = data.type;
const _errs6 = errors;
if(!(validate740(data5, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate740.errors : vErrors.concat(validate740.errors);
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
validate729.errors = vErrors;
return errors === 0;
}


function validate728(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate729(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate729.errors : vErrors.concat(validate729.errors);
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
validate728.errors = vErrors;
return errors === 0;
}

const schema79 = {"format":"date-time","type":"string","nullable":true};

function validate271(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(formats8.validate(data))){
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
validate271.errors = vErrors;
return errors === 0;
}

const schema80 = {"enum":["Pending","Running","Succeeded","Failed","Stopped"],"type":"string","nullable":true};

function validate272(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
const vSchema0 = schema80.enum;
if(!(((((data === "Pending") || (data === "Running")) || (data === "Succeeded")) || (data === "Failed")) || (data === "Stopped"))){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema80.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate272.errors = vErrors;
return errors === 0;
}

const schema184 = {"items":{"$ref":"aHL9zVuE-vUZx9e8qrI2NILl4xLUs-CPenTfDmFnNo4"},"type":"array","nullable":true};
const schema185 = {"properties":{"message":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"phase":{"$ref":"OumCqY-lP6-t1qsO7hLYZf_ZC9lX1mEyjnwbPTz244M"}},"type":"object"};

function validate749(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.message !== undefined){
let data0 = data.message;
const _errs1 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/message",parentData:data,parentDataProperty:"message",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.name !== undefined){
let data1 = data.name;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.phase !== undefined){
let data2 = data.phase;
const _errs3 = errors;
if(!(validate272(data2, {instancePath:instancePath+"/phase",parentData:data,parentDataProperty:"phase",rootData}))){
vErrors = vErrors === null ? validate272.errors : vErrors.concat(validate272.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
else {
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate749.errors = vErrors;
return errors === 0;
}


function validate748(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate749(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate749.errors : vErrors.concat(validate749.errors);
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
validate748.errors = vErrors;
return errors === 0;
}


function validate727(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.conditions !== undefined){
let data0 = data.conditions;
const _errs2 = errors;
if(!(validate728(data0, {instancePath:instancePath+"/conditions",parentData:data,parentDataProperty:"conditions",rootData}))){
vErrors = vErrors === null ? validate728.errors : vErrors.concat(validate728.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.endTime !== undefined){
let data1 = data.endTime;
const _errs3 = errors;
if(!(validate271(data1, {instancePath:instancePath+"/endTime",parentData:data,parentDataProperty:"endTime",rootData}))){
vErrors = vErrors === null ? validate271.errors : vErrors.concat(validate271.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.observedGeneration !== undefined){
let data2 = data.observedGeneration;
const _errs4 = errors;
if(!(validate86(data2, {instancePath:instancePath+"/observedGeneration",parentData:data,parentDataProperty:"observedGeneration",rootData}))){
vErrors = vErrors === null ? validate86.errors : vErrors.concat(validate86.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.phase !== undefined){
let data3 = data.phase;
const _errs5 = errors;
if(!(validate272(data3, {instancePath:instancePath+"/phase",parentData:data,parentDataProperty:"phase",rootData}))){
vErrors = vErrors === null ? validate272.errors : vErrors.concat(validate272.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.startTime !== undefined){
let data4 = data.startTime;
const _errs6 = errors;
if(!(validate271(data4, {instancePath:instancePath+"/startTime",parentData:data,parentDataProperty:"startTime",rootData}))){
vErrors = vErrors === null ? validate271.errors : vErrors.concat(validate271.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.tasks !== undefined){
let data5 = data.tasks;
const _errs7 = errors;
if(!(validate748(data5, {instancePath:instancePath+"/tasks",parentData:data,parentDataProperty:"tasks",rootData}))){
vErrors = vErrors === null ? validate748.errors : vErrors.concat(validate748.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
}
validate727.errors = vErrors;
return errors === 0;
}


function validate273(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
/*# sourceURL="rlinf.io.v1alpha1.Job" */;
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
if(!(validate274(data0, {instancePath:instancePath+"/apiVersion",parentData:data,parentDataProperty:"apiVersion",rootData}))){
vErrors = vErrors === null ? validate274.errors : vErrors.concat(validate274.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.kind !== undefined){
let data1 = data.kind;
const _errs2 = errors;
if(!(validate276(data1, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate276.errors : vErrors.concat(validate276.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.metadata !== undefined){
let data2 = data.metadata;
const _errs3 = errors;
if(!(validate278(data2, {instancePath:instancePath+"/metadata",parentData:data,parentDataProperty:"metadata",rootData}))){
vErrors = vErrors === null ? validate278.errors : vErrors.concat(validate278.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.spec !== undefined){
let data3 = data.spec;
const _errs4 = errors;
if(!(validate282(data3, {instancePath:instancePath+"/spec",parentData:data,parentDataProperty:"spec",rootData}))){
vErrors = vErrors === null ? validate282.errors : vErrors.concat(validate282.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.status !== undefined){
let data4 = data.status;
const _errs5 = errors;
if(!(validate727(data4, {instancePath:instancePath+"/status",parentData:data,parentDataProperty:"status",rootData}))){
vErrors = vErrors === null ? validate727.errors : vErrors.concat(validate727.errors);
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
validate273.errors = vErrors;
return errors === 0;
}
