// Code generated from api/config/crd/bases. DO NOT EDIT.

import { formats } from "@kubernetes-models/validate";
export const validate = validate267;
const schema78 = {"type":"object","properties":{"apiVersion":{"$ref":"rgSv5V2hoWAdXQ_XErlwxUiy6phhz4ZEBCjtqeXtVec"},"kind":{"$ref":"kpIYBKaxdhMecWzUEUmlVKT-85FnmyWwTZ_mmgTihVg"},"metadata":{"$ref":"P3srTZpWadQH4s4N9KfJR0iCqydQo9YAz7xMB2qf0GQ"},"spec":{"$ref":"Gk6s26L643b2hP0jfGEx8o2OQKnqVhfmGtWU0_38eD0"},"status":{"$ref":"AB8zOlAYygIBoJ9DuWMhRQsPG82lGwEXlXtsZf6yGxg"}},"required":["apiVersion","kind"],"$id":"rlinf.io.v1alpha1.Task"};
const schema79 = {"type":"string","enum":["rlinf.io/v1alpha1"]};

function validate268(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
const vSchema0 = schema79.enum;
if(!(data === "rlinf.io/v1alpha1")){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema79.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate268.errors = vErrors;
return errors === 0;
}

const schema80 = {"type":"string","enum":["Task"]};

function validate270(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
const vSchema0 = schema80.enum;
if(!(data === "Task")){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema80.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate270.errors = vErrors;
return errors === 0;
}

const schema81 = {"nullableRef":"io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta#"};
const schema82 = {};

import { validate as validate273 } from "@kubernetes-models/apimachinery/_schemas/IoK8sApimachineryPkgApisMetaV1ObjectMeta";


function validate272(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
if(data !== null){
if(!(validate273(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate273.errors : vErrors.concat(validate273.errors);
errors = vErrors.length;
}
else {
}
}
validate272.errors = vErrors;
return errors === 0;
}

const schema83 = {"properties":{"downstreamName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"environments":{"$ref":"ER8EfLI4tSlEHvLQ8_Z43cyggmMtWivyTo46m-KzNZE"},"placement":{"$ref":"KUhfmrayW79_k19wFilmt4-fZzov5TFSde5mJCEwwZY"},"role":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"runtime":{"$ref":"GpIVJdoOsXHFetByy5WgIi3GHmwWTCpFhj_OcVo99fg"},"suspended":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"tags":{"$ref":"AZUBfZk2hZ5WZyE5h97-9eYFBx85ELOoaLL9epVNPfg"},"workload":{"$ref":"8viUqQ1oh4U-mS7-fVETX0nTT-7IJNuBfApiHkLWISg"}},"required":["placement","role","runtime","workload"],"type":"object","nullable":true};
const schema1 = {"type":"string","nullable":true};

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
validate1.errors = vErrors;
return errors === 0;
}

const schema84 = {"items":{"$ref":"EBBQtu2Gx9J6fkKNZQZN30EEJK6IhvRmiXH6HGsPPik"},"type":"array","nullable":true};
const schema2 = {"properties":{"parameters":{"$ref":"RBNvo1WzZ4oRRq0W9-hknpT7T8If536DEMBg9hyq_4o"},"type":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"version":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["type","version"],"type":"object"};
const schema3 = {};

function validate3(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
validate3.errors = null;
return true;
}

const schema0 = {"type":"string"};

function validate0(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate0.errors = vErrors;
return errors === 0;
}


function validate2(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate3(data0, {instancePath:instancePath+"/parameters",parentData:data,parentDataProperty:"parameters",rootData}))){
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
if(!(validate0(data1, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.version !== undefined){
let data2 = data.version;
const _errs3 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/version",parentData:data,parentDataProperty:"version",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate2.errors = vErrors;
return errors === 0;
}


function validate278(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate2(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
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
validate278.errors = vErrors;
return errors === 0;
}

const schema85 = {"properties":{"nodeSelector":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"}},"type":"object"};
const schema4 = {"additionalProperties":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":"object","properties":{},"nullable":true};

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
if(!(validate0(data0, {instancePath:instancePath+"/" + key0.replace(/~/g, "~0").replace(/\//g, "~1"),parentData:data,parentDataProperty:key0,rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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


function validate281(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.nodeSelector !== undefined){
let data0 = data.nodeSelector;
const _errs1 = errors;
if(!(validate7(data0, {instancePath:instancePath+"/nodeSelector",parentData:data,parentDataProperty:"nodeSelector",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
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
validate281.errors = vErrors;
return errors === 0;
}

const schema86 = {"properties":{"kind":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["kind"],"type":"object"};

function validate285(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
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
const err1 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate285.errors = vErrors;
return errors === 0;
}

const schema16 = {"type":"boolean","nullable":true};

function validate41(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate41.errors = vErrors;
return errors === 0;
}

const schema87 = {"items":{"$ref":"V89VA1AXeRuHheKt9ipXKQFE-H5_Z5APxb-xlMIJVqg"},"type":"array","nullable":true};
const schema88 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"values":{"$ref":"uqUtVyG9D2UAWDvqosEgVMtwK5kI3TMy2eLbDZB452I"}},"required":["key","values"],"type":"object"};
const schema68 = {"items":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":"array"};

function validate237(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs1 = errors;
if(!(validate0(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate237.errors = vErrors;
return errors === 0;
}


function validate291(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.values !== undefined){
let data1 = data.values;
const _errs2 = errors;
if(!(validate237(data1, {instancePath:instancePath+"/values",parentData:data,parentDataProperty:"values",rootData}))){
vErrors = vErrors === null ? validate237.errors : vErrors.concat(validate237.errors);
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
validate291.errors = vErrors;
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
if(!(validate291(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate291.errors : vErrors.concat(validate291.errors);
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

const schema89 = {"properties":{"carrier":{"$ref":"EBBQtu2Gx9J6fkKNZQZN30EEJK6IhvRmiXH6HGsPPik"},"kind":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"parameters":{"$ref":"RBNvo1WzZ4oRRq0W9-hknpT7T8If536DEMBg9hyq_4o"},"template":{"$ref":"xcL4NGh_zPZzHwtqXxR_gWzuA1WPWoXAZzpBvyzgDsw"},"version":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["carrier","version"],"type":"object"};
const schema90 = {"properties":{"metadata":{"$ref":"9xOEryzozGanNCCz7goq0K4hzMvoOusQQ4iDJ494Bhw"},"spec":{"$ref":"9ELI21-RY5S8b2WTyT-3A4ABzk4xYvXNgu9N8BXlru8"}},"type":"object","nullable":true};
const schema70 = {"properties":{"annotations":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"},"finalizers":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"labels":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"namespace":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};
const schema5 = {"items":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":"array","nullable":true};

function validate9(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate9.errors = vErrors;
return errors === 0;
}


function validate241(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate7(data0, {instancePath:instancePath+"/annotations",parentData:data,parentDataProperty:"annotations",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.finalizers !== undefined){
let data1 = data.finalizers;
const _errs3 = errors;
if(!(validate9(data1, {instancePath:instancePath+"/finalizers",parentData:data,parentDataProperty:"finalizers",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.labels !== undefined){
let data2 = data.labels;
const _errs4 = errors;
if(!(validate7(data2, {instancePath:instancePath+"/labels",parentData:data,parentDataProperty:"labels",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.name !== undefined){
let data3 = data.name;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.namespace !== undefined){
let data4 = data.namespace;
const _errs6 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/namespace",parentData:data,parentDataProperty:"namespace",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate241.errors = vErrors;
return errors === 0;
}

const schema91 = {"properties":{"activeDeadlineSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"affinity":{"$ref":"xEzn8Yn5UZwX8tOMdJxI_vy9KYtT_0JX0A88KaKocSo"},"automountServiceAccountToken":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"containers":{"$ref":"vPve3gHxxNN3Sm6pVDxHpN1aCLg_ph9lZEklsRQDPgo"},"dnsConfig":{"$ref":"FyA4xmwX0zynVwRI119QOxQbFJN1tctRkE7b2kzXuc4"},"dnsPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"enableServiceLinks":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"ephemeralContainers":{"$ref":"yKmjfLtm3wPQY2AVwwdCnmvEKUbRA-4_LyiuVKGendA"},"hostAliases":{"$ref":"tZ9rYgYSvnaUwreY3KeGfBGexu5Ycj74ilyAZUawHog"},"hostIPC":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"hostNetwork":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"hostPID":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"hostUsers":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"hostname":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"hostnameOverride":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"imagePullSecrets":{"$ref":"X1TOUYMQSutUYQ6hvqIzZbwk5omDcBNTKfnB0-fL2Z8"},"initContainers":{"$ref":"x3_xyk8cJDC-WRl-Q3Vnzp0T2UpwLUhDnhFB1cxjhUw"},"nodeName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"nodeSelector":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"},"os":{"$ref":"E97dOchMV0AJMEW2BvWE8x-plhaqw9P894BmF33ZGUY"},"overhead":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"},"preemptionPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"priority":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"priorityClassName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readinessGates":{"$ref":"tNwOOZ3NvCXgx0K8ciAkE_yKAsFotR0cS8KzXPkjiQA"},"resourceClaims":{"$ref":"0tfYwcjr1H_7z3CEXMv8E4O0eK7miDdm2dJX8axskZo"},"resources":{"$ref":"FhMRS9yiPxYjQhkDxbgv-jQ4jx9VLxRdMLNZtrOnwhI"},"restartPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"runtimeClassName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"schedulerName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"schedulingGates":{"$ref":"m4uoxKUfX1hC4hxhIVYqmLIHdW2quxepD9tixEKMfno"},"schedulingGroup":{"$ref":"_zmNSOGPBS_eId1M6G5RIekFmO_DVXdtDZe9BX5KXas"},"securityContext":{"$ref":"Q_ytIvl2Z5eadDvs5pDNIRSSXo4ZRIiK5GaG-32_Hu0"},"serviceAccount":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"serviceAccountName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"setHostnameAsFQDN":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"shareProcessNamespace":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"subdomain":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"terminationGracePeriodSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"tolerations":{"$ref":"_Va30eZa87-ZEGMNNRXBis4QY2hj9ZNqFG0WocRDLmw"},"topologySpreadConstraints":{"$ref":"nQdehtf0Mv4PatJ6VO_CgOf95dDX90Z5ka6yfaxVtLU"},"volumes":{"$ref":"6MEj9x1EdleTLWxH3GxOdDVugNWE_cdNoiAgUzIbfCY"}},"required":["containers"],"type":"object","nullable":true};
const schema31 = {"format":"int64","type":"integer","nullable":true};
const formats2 = formats.int64;

function validate79(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate79.errors = vErrors;
return errors === 0;
}

const schema92 = {"properties":{"nodeAffinity":{"$ref":"IM3S0i8cVDyz5lYunR5j9kbhD7Xy_8b7_y3KRQ-p_lg"},"podAffinity":{"$ref":"O-egicA4MB8uPievojYXDHsa_7eltZLPehir6Y4ISbM"},"podAntiAffinity":{"$ref":"O-egicA4MB8uPievojYXDHsa_7eltZLPehir6Y4ISbM"}},"type":"object","nullable":true};
const schema93 = {"properties":{"preferredDuringSchedulingIgnoredDuringExecution":{"$ref":"6e5xkVuknB576DQB84VZ02uf6mAhcmFIEb0YZ5ZeoME"},"requiredDuringSchedulingIgnoredDuringExecution":{"$ref":"dsUswhQImzOLseDnHGq7bSBqTb8SnnotFBkumLi5HQQ"}},"type":"object","nullable":true};
const schema94 = {"items":{"$ref":"0GU7bbB8-3A4umzJ_Vhsp6Y__EiDmpASVQZgVjw3dmQ"},"type":"array","nullable":true};
const schema95 = {"properties":{"preference":{"$ref":"bPs4NJDgemKZPXkEsB4N2vMyTIvZBXESTOmRdaBhgzY"},"weight":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"}},"required":["preference","weight"],"type":"object"};
const schema8 = {"properties":{"matchExpressions":{"$ref":"ayKrb4PFrKLMYqvjiutPxrbz750hbVgxft904f5rFtQ"},"matchFields":{"$ref":"ayKrb4PFrKLMYqvjiutPxrbz750hbVgxft904f5rFtQ"}},"type":"object"};
const schema6 = {"items":{"$ref":"9NBsm8QGHPFL3UnZD7QzZl0zjxatAalAe2kAVgrnyrc"},"type":"array","nullable":true};
const schema7 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"operator":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"values":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"required":["key","operator"],"type":"object"};

function validate12(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.operator !== undefined){
let data1 = data.operator;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/operator",parentData:data,parentDataProperty:"operator",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.values !== undefined){
let data2 = data.values;
const _errs3 = errors;
if(!(validate9(data2, {instancePath:instancePath+"/values",parentData:data,parentDataProperty:"values",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
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
validate12.errors = vErrors;
return errors === 0;
}


function validate11(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate12(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate12.errors : vErrors.concat(validate12.errors);
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
validate11.errors = vErrors;
return errors === 0;
}


function validate17(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.matchExpressions !== undefined){
let data0 = data.matchExpressions;
const _errs1 = errors;
if(!(validate11(data0, {instancePath:instancePath+"/matchExpressions",parentData:data,parentDataProperty:"matchExpressions",rootData}))){
vErrors = vErrors === null ? validate11.errors : vErrors.concat(validate11.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.matchFields !== undefined){
let data1 = data.matchFields;
const _errs2 = errors;
if(!(validate11(data1, {instancePath:instancePath+"/matchFields",parentData:data,parentDataProperty:"matchFields",rootData}))){
vErrors = vErrors === null ? validate11.errors : vErrors.concat(validate11.errors);
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
validate17.errors = vErrors;
return errors === 0;
}

const schema10 = {"format":"int32","type":"integer"};
const formats0 = formats.int32;

function validate23(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate23.errors = vErrors;
return errors === 0;
}


function validate307(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate17(data0, {instancePath:instancePath+"/preference",parentData:data,parentDataProperty:"preference",rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.weight !== undefined){
let data1 = data.weight;
const _errs2 = errors;
if(!(validate23(data1, {instancePath:instancePath+"/weight",parentData:data,parentDataProperty:"weight",rootData}))){
vErrors = vErrors === null ? validate23.errors : vErrors.concat(validate23.errors);
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
validate307.errors = vErrors;
return errors === 0;
}


function validate306(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate307(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate307.errors : vErrors.concat(validate307.errors);
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
validate306.errors = vErrors;
return errors === 0;
}

const schema96 = {"properties":{"nodeSelectorTerms":{"$ref":"WceES8jSl1b6JeqUZQ18OOMpo-zexwPEiCeLvTA69Xg"}},"required":["nodeSelectorTerms"],"type":"object","nullable":true};
const schema97 = {"items":{"$ref":"bPs4NJDgemKZPXkEsB4N2vMyTIvZBXESTOmRdaBhgzY"},"type":"array"};

function validate313(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(Array.isArray(data)){
let valid1 = true;
const len0 = data.length;
for(let i0=0; i0<len0; i0++){
let data0 = data[i0];
const _errs1 = errors;
if(!(validate17(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate17.errors : vErrors.concat(validate17.errors);
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
validate313.errors = vErrors;
return errors === 0;
}


function validate312(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate313(data0, {instancePath:instancePath+"/nodeSelectorTerms",parentData:data,parentDataProperty:"nodeSelectorTerms",rootData}))){
vErrors = vErrors === null ? validate313.errors : vErrors.concat(validate313.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate312.errors = vErrors;
return errors === 0;
}


function validate305(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate306(data0, {instancePath:instancePath+"/preferredDuringSchedulingIgnoredDuringExecution",parentData:data,parentDataProperty:"preferredDuringSchedulingIgnoredDuringExecution",rootData}))){
vErrors = vErrors === null ? validate306.errors : vErrors.concat(validate306.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.requiredDuringSchedulingIgnoredDuringExecution !== undefined){
let data1 = data.requiredDuringSchedulingIgnoredDuringExecution;
const _errs3 = errors;
if(!(validate312(data1, {instancePath:instancePath+"/requiredDuringSchedulingIgnoredDuringExecution",parentData:data,parentDataProperty:"requiredDuringSchedulingIgnoredDuringExecution",rootData}))){
vErrors = vErrors === null ? validate312.errors : vErrors.concat(validate312.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate305.errors = vErrors;
return errors === 0;
}

const schema12 = {"properties":{"preferredDuringSchedulingIgnoredDuringExecution":{"$ref":"Gv8Hvk-YMeAbi17zcXvGdYz35Trv28l0k6de2GFzrpY"},"requiredDuringSchedulingIgnoredDuringExecution":{"$ref":"x-4CXre8Q_6LB7j_yEJTNRttDcJly0o6xUOEkjdRAMg"}},"type":"object","nullable":true};
const schema13 = {"items":{"$ref":"9Yx6B--m2eQFYkNbnIcDLOTpZxndM94PyYZq_-kDybs"},"type":"array","nullable":true};
const schema14 = {"properties":{"podAffinityTerm":{"$ref":"8t0N05ceNTQObj1ufnNXouWS88GDJF66xuctgeD6LLg"},"weight":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"}},"required":["podAffinityTerm","weight"],"type":"object"};
const schema11 = {"properties":{"labelSelector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"matchLabelKeys":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"mismatchLabelKeys":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"namespaceSelector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"namespaces":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"topologyKey":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["topologyKey"],"type":"object"};
const schema9 = {"properties":{"matchExpressions":{"$ref":"ayKrb4PFrKLMYqvjiutPxrbz750hbVgxft904f5rFtQ"},"matchLabels":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"}},"type":"object","nullable":true};

function validate20(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate11(data0, {instancePath:instancePath+"/matchExpressions",parentData:data,parentDataProperty:"matchExpressions",rootData}))){
vErrors = vErrors === null ? validate11.errors : vErrors.concat(validate11.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.matchLabels !== undefined){
let data1 = data.matchLabels;
const _errs3 = errors;
if(!(validate7(data1, {instancePath:instancePath+"/matchLabels",parentData:data,parentDataProperty:"matchLabels",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate20.errors = vErrors;
return errors === 0;
}


function validate24(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate20(data0, {instancePath:instancePath+"/labelSelector",parentData:data,parentDataProperty:"labelSelector",rootData}))){
vErrors = vErrors === null ? validate20.errors : vErrors.concat(validate20.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.matchLabelKeys !== undefined){
let data1 = data.matchLabelKeys;
const _errs2 = errors;
if(!(validate9(data1, {instancePath:instancePath+"/matchLabelKeys",parentData:data,parentDataProperty:"matchLabelKeys",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.mismatchLabelKeys !== undefined){
let data2 = data.mismatchLabelKeys;
const _errs3 = errors;
if(!(validate9(data2, {instancePath:instancePath+"/mismatchLabelKeys",parentData:data,parentDataProperty:"mismatchLabelKeys",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.namespaceSelector !== undefined){
let data3 = data.namespaceSelector;
const _errs4 = errors;
if(!(validate20(data3, {instancePath:instancePath+"/namespaceSelector",parentData:data,parentDataProperty:"namespaceSelector",rootData}))){
vErrors = vErrors === null ? validate20.errors : vErrors.concat(validate20.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.namespaces !== undefined){
let data4 = data.namespaces;
const _errs5 = errors;
if(!(validate9(data4, {instancePath:instancePath+"/namespaces",parentData:data,parentDataProperty:"namespaces",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.topologyKey !== undefined){
let data5 = data.topologyKey;
const _errs6 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/topologyKey",parentData:data,parentDataProperty:"topologyKey",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate24.errors = vErrors;
return errors === 0;
}


function validate33(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate24(data0, {instancePath:instancePath+"/podAffinityTerm",parentData:data,parentDataProperty:"podAffinityTerm",rootData}))){
vErrors = vErrors === null ? validate24.errors : vErrors.concat(validate24.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.weight !== undefined){
let data1 = data.weight;
const _errs2 = errors;
if(!(validate23(data1, {instancePath:instancePath+"/weight",parentData:data,parentDataProperty:"weight",rootData}))){
vErrors = vErrors === null ? validate23.errors : vErrors.concat(validate23.errors);
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
validate33.errors = vErrors;
return errors === 0;
}


function validate32(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate33(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate33.errors : vErrors.concat(validate33.errors);
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
validate32.errors = vErrors;
return errors === 0;
}

const schema15 = {"items":{"$ref":"8t0N05ceNTQObj1ufnNXouWS88GDJF66xuctgeD6LLg"},"type":"array","nullable":true};

function validate38(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate24(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate24.errors : vErrors.concat(validate24.errors);
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
validate38.errors = vErrors;
return errors === 0;
}


function validate31(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate32(data0, {instancePath:instancePath+"/preferredDuringSchedulingIgnoredDuringExecution",parentData:data,parentDataProperty:"preferredDuringSchedulingIgnoredDuringExecution",rootData}))){
vErrors = vErrors === null ? validate32.errors : vErrors.concat(validate32.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.requiredDuringSchedulingIgnoredDuringExecution !== undefined){
let data1 = data.requiredDuringSchedulingIgnoredDuringExecution;
const _errs3 = errors;
if(!(validate38(data1, {instancePath:instancePath+"/requiredDuringSchedulingIgnoredDuringExecution",parentData:data,parentDataProperty:"requiredDuringSchedulingIgnoredDuringExecution",rootData}))){
vErrors = vErrors === null ? validate38.errors : vErrors.concat(validate38.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate31.errors = vErrors;
return errors === 0;
}


function validate304(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate305(data0, {instancePath:instancePath+"/nodeAffinity",parentData:data,parentDataProperty:"nodeAffinity",rootData}))){
vErrors = vErrors === null ? validate305.errors : vErrors.concat(validate305.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.podAffinity !== undefined){
let data1 = data.podAffinity;
const _errs3 = errors;
if(!(validate31(data1, {instancePath:instancePath+"/podAffinity",parentData:data,parentDataProperty:"podAffinity",rootData}))){
vErrors = vErrors === null ? validate31.errors : vErrors.concat(validate31.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.podAntiAffinity !== undefined){
let data2 = data.podAntiAffinity;
const _errs4 = errors;
if(!(validate31(data2, {instancePath:instancePath+"/podAntiAffinity",parentData:data,parentDataProperty:"podAntiAffinity",rootData}))){
vErrors = vErrors === null ? validate31.errors : vErrors.concat(validate31.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate304.errors = vErrors;
return errors === 0;
}

const schema98 = {"items":{"$ref":"chE8IbiwOYvrhp6jxf4CeNPe4xowDfDQCt1Zhaa9Prk"},"type":"array"};
const schema67 = {"properties":{"args":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"command":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"env":{"$ref":"QK6qdOVPcIj3ymOL58J6NgOdTW_Xbf1pI79iGILi8uU"},"envFrom":{"$ref":"ZRw41W3mWwhKksdKQqy6gdGI0VqZa0mht_DKnlcF-lw"},"image":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"imagePullPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"lifecycle":{"$ref":"2GolHCLsHBmPr5h5-SqeL3kwFl1yyHXgHxOurrzuSpc"},"livenessProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"ports":{"$ref":"QdA0VP-JsKHKjWu-1Hu8oyYcguMz4UHlXTFbpvKweaM"},"readinessProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"resizePolicy":{"$ref":"6FxzbMy3fQv1gheBeNadVqOl2xbXIqiFGG5KNo9kRcw"},"resources":{"$ref":"FhMRS9yiPxYjQhkDxbgv-jQ4jx9VLxRdMLNZtrOnwhI"},"restartPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"restartPolicyRules":{"$ref":"bn1AnMh-2dkJyL1LY5A_xgvSFgNvRm9Wd4RL1NkeXRU"},"securityContext":{"$ref":"ppbGhvBFiV7RuYhp7GW5xoG5M7lU4kbUlUSlBvBbdzE"},"startupProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"stdin":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"stdinOnce":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"terminationMessagePath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"terminationMessagePolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"tty":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeDevices":{"$ref":"yySLqkz1jc_2D5zmwBnHmcS-y-0ogKvDTXKQGVruH8A"},"volumeMounts":{"$ref":"K3z2jQ4aYZcOrB0gSTeLWvm5O5c50b2P06GZmb9a8OM"},"workingDir":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["name"],"type":"object"};
const schema37 = {"items":{"$ref":"oh8pSA2ZWw7M7xqiGJZa1l-wmNwDLQ-3XcRRVCMnyvA"},"type":"array","nullable":true};
const schema38 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"value":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"valueFrom":{"$ref":"3Om5CxocdGRSXm0woiJiUBiNiuG5fJ71SR9MY4QT4dM"}},"required":["name"],"type":"object"};
const schema39 = {"properties":{"configMapKeyRef":{"$ref":"Ld9L-NJLX9DB_iAQL99ICueNKI_kx3YsLhY4cC2SPx8"},"fieldRef":{"$ref":"HmyyyuOupyisSiKp0rvRRJtbCek8WlFUsfGxXzcVsrg"},"fileKeyRef":{"$ref":"n4gXUrpmTuMnrzObLAcsnh7fAb5vuSAR-bi24ZVKK7o"},"resourceFieldRef":{"$ref":"lUaYvKMK_XnIMZc4H9_ERGObFXszqs8GfnzmBfc3Oh8"},"secretKeyRef":{"$ref":"Ld9L-NJLX9DB_iAQL99ICueNKI_kx3YsLhY4cC2SPx8"}},"type":"object","nullable":true};
const schema17 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"required":["key"],"type":"object","nullable":true};
const schema18 = {"default":"","type":"string","nullable":true};

function validate44(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate44.errors = vErrors;
return errors === 0;
}


function validate42(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.name !== undefined){
let data1 = data.name;
const _errs3 = errors;
if(!(validate44(data1, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate44.errors : vErrors.concat(validate44.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.optional !== undefined){
let data2 = data.optional;
const _errs4 = errors;
if(!(validate41(data2, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate42.errors = vErrors;
return errors === 0;
}

const schema40 = {"properties":{"apiVersion":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"fieldPath":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["fieldPath"],"type":"object","nullable":true};

function validate108(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/apiVersion",parentData:data,parentDataProperty:"apiVersion",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fieldPath !== undefined){
let data1 = data.fieldPath;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/fieldPath",parentData:data,parentDataProperty:"fieldPath",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate108.errors = vErrors;
return errors === 0;
}

const schema41 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"optional":{"$ref":"PxmoJm38YOn1dCbyB5PLDDQXV2B2rqFhpzNXj0_ibe4"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"volumeName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["key","path","volumeName"],"type":"object","nullable":true};
const schema42 = {"default":false,"type":"boolean","nullable":true};

function validate114(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate114.errors = vErrors;
return errors === 0;
}


function validate112(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.optional !== undefined){
let data1 = data.optional;
const _errs3 = errors;
if(!(validate114(data1, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate114.errors : vErrors.concat(validate114.errors);
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
}
validate112.errors = vErrors;
return errors === 0;
}

const schema43 = {"properties":{"containerName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"divisor":{"$ref":"3dmDSoHI3Hv64ppBBbqAEh3HzVwqzFWb_nrM2BhHTgs"},"resource":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["resource"],"type":"object","nullable":true};
const schema34 = {"anyOf":[{"$ref":"laYO4ZujiLzTOvPomluoS9e439ISWe2RqSMRunp3ReI"},{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}],"pattern":"^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$"};
const schema20 = {"type":"integer"};

function validate50(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate50.errors = vErrors;
return errors === 0;
}

const pattern0 = new RegExp("^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$", "u");

function validate94(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
let valid0 = false;
const _errs1 = errors;
if(!(validate50(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate50.errors : vErrors.concat(validate50.errors);
errors = vErrors.length;
}
else {
}
var _valid0 = _errs1 === errors;
valid0 = valid0 || _valid0;
if(!valid0){
const _errs2 = errors;
if(!(validate0(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate94.errors = vErrors;
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
if(!(validate1(data0, {instancePath:instancePath+"/containerName",parentData:data,parentDataProperty:"containerName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.divisor !== undefined){
let data1 = data.divisor;
const _errs3 = errors;
if(!(validate94(data1, {instancePath:instancePath+"/divisor",parentData:data,parentDataProperty:"divisor",rootData}))){
vErrors = vErrors === null ? validate94.errors : vErrors.concat(validate94.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.resource !== undefined){
let data2 = data.resource;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/resource",parentData:data,parentDataProperty:"resource",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate119.errors = vErrors;
return errors === 0;
}


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
if(data.configMapKeyRef !== undefined){
let data0 = data.configMapKeyRef;
const _errs2 = errors;
if(!(validate42(data0, {instancePath:instancePath+"/configMapKeyRef",parentData:data,parentDataProperty:"configMapKeyRef",rootData}))){
vErrors = vErrors === null ? validate42.errors : vErrors.concat(validate42.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fieldRef !== undefined){
let data1 = data.fieldRef;
const _errs3 = errors;
if(!(validate108(data1, {instancePath:instancePath+"/fieldRef",parentData:data,parentDataProperty:"fieldRef",rootData}))){
vErrors = vErrors === null ? validate108.errors : vErrors.concat(validate108.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.fileKeyRef !== undefined){
let data2 = data.fileKeyRef;
const _errs4 = errors;
if(!(validate112(data2, {instancePath:instancePath+"/fileKeyRef",parentData:data,parentDataProperty:"fileKeyRef",rootData}))){
vErrors = vErrors === null ? validate112.errors : vErrors.concat(validate112.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.resourceFieldRef !== undefined){
let data3 = data.resourceFieldRef;
const _errs5 = errors;
if(!(validate119(data3, {instancePath:instancePath+"/resourceFieldRef",parentData:data,parentDataProperty:"resourceFieldRef",rootData}))){
vErrors = vErrors === null ? validate119.errors : vErrors.concat(validate119.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.secretKeyRef !== undefined){
let data4 = data.secretKeyRef;
const _errs6 = errors;
if(!(validate42(data4, {instancePath:instancePath+"/secretKeyRef",parentData:data,parentDataProperty:"secretKeyRef",rootData}))){
vErrors = vErrors === null ? validate42.errors : vErrors.concat(validate42.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate106.errors = vErrors;
return errors === 0;
}


function validate103(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data1, {instancePath:instancePath+"/value",parentData:data,parentDataProperty:"value",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.valueFrom !== undefined){
let data2 = data.valueFrom;
const _errs3 = errors;
if(!(validate106(data2, {instancePath:instancePath+"/valueFrom",parentData:data,parentDataProperty:"valueFrom",rootData}))){
vErrors = vErrors === null ? validate106.errors : vErrors.concat(validate106.errors);
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
validate103.errors = vErrors;
return errors === 0;
}


function validate102(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate103(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate103.errors : vErrors.concat(validate103.errors);
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
validate102.errors = vErrors;
return errors === 0;
}

const schema44 = {"items":{"$ref":"HHPN67ks7XM2URGVvIHxDM_VLhU9JKL3x7CLOTcCXdk"},"type":"array","nullable":true};
const schema45 = {"properties":{"configMapRef":{"$ref":"u-QkpzKhtzKCL4fqRT1udeFHaK0vv8tqmNxAND2hxII"},"prefix":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"secretRef":{"$ref":"u-QkpzKhtzKCL4fqRT1udeFHaK0vv8tqmNxAND2hxII"}},"type":"object"};
const schema19 = {"properties":{"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"type":"object","nullable":true};

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
if(data.name !== undefined){
let data0 = data.name;
const _errs2 = errors;
if(!(validate44(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate44.errors : vErrors.concat(validate44.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.optional !== undefined){
let data1 = data.optional;
const _errs3 = errors;
if(!(validate41(data1, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate47.errors = vErrors;
return errors === 0;
}


function validate128(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.configMapRef !== undefined){
let data0 = data.configMapRef;
const _errs1 = errors;
if(!(validate47(data0, {instancePath:instancePath+"/configMapRef",parentData:data,parentDataProperty:"configMapRef",rootData}))){
vErrors = vErrors === null ? validate47.errors : vErrors.concat(validate47.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.prefix !== undefined){
let data1 = data.prefix;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/prefix",parentData:data,parentDataProperty:"prefix",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.secretRef !== undefined){
let data2 = data.secretRef;
const _errs3 = errors;
if(!(validate47(data2, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate47.errors : vErrors.concat(validate47.errors);
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
validate128.errors = vErrors;
return errors === 0;
}


function validate127(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate128(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate128.errors : vErrors.concat(validate128.errors);
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
validate127.errors = vErrors;
return errors === 0;
}

const schema46 = {"properties":{"postStart":{"$ref":"bI1FYAC1lVqZU-yavNa7Tnj1QxPBg6vSykf7ECTWEso"},"preStop":{"$ref":"bI1FYAC1lVqZU-yavNa7Tnj1QxPBg6vSykf7ECTWEso"},"stopSignal":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};
const schema22 = {"properties":{"exec":{"$ref":"9C0OixFARi4hI87PTOihZwsBCgmS5p3ncDGfVlnGaWE"},"httpGet":{"$ref":"AvymdmXaS2uRanVsPLWMQgXBYDVVb7RMCKXx61FTpv8"},"sleep":{"$ref":"O9c6rU4oPqcIn0IAR6GJa06bNcsqOvXcnvmWF6hjOas"},"tcpSocket":{"$ref":"kUbDk1_JH4_cSX_6OddmnZt6rvEGMGnt00AZtEGZzIE"}},"type":"object","nullable":true};
const schema23 = {"properties":{"command":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};

function validate55(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate9(data0, {instancePath:instancePath+"/command",parentData:data,parentDataProperty:"command",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate55.errors = vErrors;
return errors === 0;
}

const schema24 = {"properties":{"host":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"httpHeaders":{"$ref":"MsS0666yf0HSUh6mG0cBqbw9ThqZk9-DowZaIzPios0"},"path":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"port":{"$ref":"ih-pWOC033UnwvHDNXL0jVD9FR7FP484ObfuKTqXkMU"},"scheme":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["port"],"type":"object","nullable":true};
const schema25 = {"items":{"$ref":"0EjQDQuCE_EGeSNjOUachoDmgPAdJKL5JCdMDcFxJHs"},"type":"array","nullable":true};
const schema26 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"value":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["name","value"],"type":"object"};

function validate61(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
const err2 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
validate61.errors = vErrors;
return errors === 0;
}


function validate60(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate61(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate61.errors : vErrors.concat(validate61.errors);
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
validate60.errors = vErrors;
return errors === 0;
}

const schema21 = {"anyOf":[{"$ref":"laYO4ZujiLzTOvPomluoS9e439ISWe2RqSMRunp3ReI"},{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}]};

function validate51(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
let valid0 = false;
const _errs1 = errors;
if(!(validate50(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate50.errors : vErrors.concat(validate50.errors);
errors = vErrors.length;
}
else {
}
var _valid0 = _errs1 === errors;
valid0 = valid0 || _valid0;
if(!valid0){
const _errs2 = errors;
if(!(validate0(data, {instancePath,parentData,parentDataProperty,rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate51.errors = vErrors;
return errors === 0;
}


function validate58(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/host",parentData:data,parentDataProperty:"host",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.httpHeaders !== undefined){
let data1 = data.httpHeaders;
const _errs3 = errors;
if(!(validate60(data1, {instancePath:instancePath+"/httpHeaders",parentData:data,parentDataProperty:"httpHeaders",rootData}))){
vErrors = vErrors === null ? validate60.errors : vErrors.concat(validate60.errors);
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
if(data.port !== undefined){
let data3 = data.port;
const _errs5 = errors;
if(!(validate51(data3, {instancePath:instancePath+"/port",parentData:data,parentDataProperty:"port",rootData}))){
vErrors = vErrors === null ? validate51.errors : vErrors.concat(validate51.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.scheme !== undefined){
let data4 = data.scheme;
const _errs6 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/scheme",parentData:data,parentDataProperty:"scheme",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate58.errors = vErrors;
return errors === 0;
}

const schema27 = {"properties":{"seconds":{"$ref":"0YQm7z5aN2fg7S8XlbxrquRU6pdIr97uCbtQYyjv3S4"}},"required":["seconds"],"type":"object","nullable":true};
const schema28 = {"format":"int64","type":"integer"};

function validate71(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate71.errors = vErrors;
return errors === 0;
}


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
if(!(validate71(data0, {instancePath:instancePath+"/seconds",parentData:data,parentDataProperty:"seconds",rootData}))){
vErrors = vErrors === null ? validate71.errors : vErrors.concat(validate71.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate70.errors = vErrors;
return errors === 0;
}

const schema29 = {"properties":{"host":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"port":{"$ref":"ih-pWOC033UnwvHDNXL0jVD9FR7FP484ObfuKTqXkMU"}},"required":["port"],"type":"object","nullable":true};

function validate74(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/host",parentData:data,parentDataProperty:"host",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.port !== undefined){
let data1 = data.port;
const _errs3 = errors;
if(!(validate51(data1, {instancePath:instancePath+"/port",parentData:data,parentDataProperty:"port",rootData}))){
vErrors = vErrors === null ? validate51.errors : vErrors.concat(validate51.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate74.errors = vErrors;
return errors === 0;
}


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
if(data.exec !== undefined){
let data0 = data.exec;
const _errs2 = errors;
if(!(validate55(data0, {instancePath:instancePath+"/exec",parentData:data,parentDataProperty:"exec",rootData}))){
vErrors = vErrors === null ? validate55.errors : vErrors.concat(validate55.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.httpGet !== undefined){
let data1 = data.httpGet;
const _errs3 = errors;
if(!(validate58(data1, {instancePath:instancePath+"/httpGet",parentData:data,parentDataProperty:"httpGet",rootData}))){
vErrors = vErrors === null ? validate58.errors : vErrors.concat(validate58.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.sleep !== undefined){
let data2 = data.sleep;
const _errs4 = errors;
if(!(validate70(data2, {instancePath:instancePath+"/sleep",parentData:data,parentDataProperty:"sleep",rootData}))){
vErrors = vErrors === null ? validate70.errors : vErrors.concat(validate70.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.tcpSocket !== undefined){
let data3 = data.tcpSocket;
const _errs5 = errors;
if(!(validate74(data3, {instancePath:instancePath+"/tcpSocket",parentData:data,parentDataProperty:"tcpSocket",rootData}))){
vErrors = vErrors === null ? validate74.errors : vErrors.concat(validate74.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate54.errors = vErrors;
return errors === 0;
}


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
if(data.postStart !== undefined){
let data0 = data.postStart;
const _errs2 = errors;
if(!(validate54(data0, {instancePath:instancePath+"/postStart",parentData:data,parentDataProperty:"postStart",rootData}))){
vErrors = vErrors === null ? validate54.errors : vErrors.concat(validate54.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.preStop !== undefined){
let data1 = data.preStop;
const _errs3 = errors;
if(!(validate54(data1, {instancePath:instancePath+"/preStop",parentData:data,parentDataProperty:"preStop",rootData}))){
vErrors = vErrors === null ? validate54.errors : vErrors.concat(validate54.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.stopSignal !== undefined){
let data2 = data.stopSignal;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/stopSignal",parentData:data,parentDataProperty:"stopSignal",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate133.errors = vErrors;
return errors === 0;
}

const schema32 = {"properties":{"exec":{"$ref":"9C0OixFARi4hI87PTOihZwsBCgmS5p3ncDGfVlnGaWE"},"failureThreshold":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"grpc":{"$ref":"TPbd1UVnDN2xj-Iuf9RvwkhMgTdHQQ3lBf7qZ98euNo"},"httpGet":{"$ref":"AvymdmXaS2uRanVsPLWMQgXBYDVVb7RMCKXx61FTpv8"},"initialDelaySeconds":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"periodSeconds":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"successThreshold":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"tcpSocket":{"$ref":"kUbDk1_JH4_cSX_6OddmnZt6rvEGMGnt00AZtEGZzIE"},"terminationGracePeriodSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"timeoutSeconds":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"}},"type":"object","nullable":true};
const schema30 = {"format":"int32","type":"integer","nullable":true};

function validate78(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate78.errors = vErrors;
return errors === 0;
}

const schema33 = {"properties":{"port":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"service":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"}},"required":["port"],"type":"object","nullable":true};

function validate83(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate23(data0, {instancePath:instancePath+"/port",parentData:data,parentDataProperty:"port",rootData}))){
vErrors = vErrors === null ? validate23.errors : vErrors.concat(validate23.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.service !== undefined){
let data1 = data.service;
const _errs3 = errors;
if(!(validate44(data1, {instancePath:instancePath+"/service",parentData:data,parentDataProperty:"service",rootData}))){
vErrors = vErrors === null ? validate44.errors : vErrors.concat(validate44.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate83.errors = vErrors;
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
if(data.exec !== undefined){
let data0 = data.exec;
const _errs2 = errors;
if(!(validate55(data0, {instancePath:instancePath+"/exec",parentData:data,parentDataProperty:"exec",rootData}))){
vErrors = vErrors === null ? validate55.errors : vErrors.concat(validate55.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.failureThreshold !== undefined){
let data1 = data.failureThreshold;
const _errs3 = errors;
if(!(validate78(data1, {instancePath:instancePath+"/failureThreshold",parentData:data,parentDataProperty:"failureThreshold",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.grpc !== undefined){
let data2 = data.grpc;
const _errs4 = errors;
if(!(validate83(data2, {instancePath:instancePath+"/grpc",parentData:data,parentDataProperty:"grpc",rootData}))){
vErrors = vErrors === null ? validate83.errors : vErrors.concat(validate83.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.httpGet !== undefined){
let data3 = data.httpGet;
const _errs5 = errors;
if(!(validate58(data3, {instancePath:instancePath+"/httpGet",parentData:data,parentDataProperty:"httpGet",rootData}))){
vErrors = vErrors === null ? validate58.errors : vErrors.concat(validate58.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.initialDelaySeconds !== undefined){
let data4 = data.initialDelaySeconds;
const _errs6 = errors;
if(!(validate78(data4, {instancePath:instancePath+"/initialDelaySeconds",parentData:data,parentDataProperty:"initialDelaySeconds",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.periodSeconds !== undefined){
let data5 = data.periodSeconds;
const _errs7 = errors;
if(!(validate78(data5, {instancePath:instancePath+"/periodSeconds",parentData:data,parentDataProperty:"periodSeconds",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.successThreshold !== undefined){
let data6 = data.successThreshold;
const _errs8 = errors;
if(!(validate78(data6, {instancePath:instancePath+"/successThreshold",parentData:data,parentDataProperty:"successThreshold",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.tcpSocket !== undefined){
let data7 = data.tcpSocket;
const _errs9 = errors;
if(!(validate74(data7, {instancePath:instancePath+"/tcpSocket",parentData:data,parentDataProperty:"tcpSocket",rootData}))){
vErrors = vErrors === null ? validate74.errors : vErrors.concat(validate74.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.terminationGracePeriodSeconds !== undefined){
let data8 = data.terminationGracePeriodSeconds;
const _errs10 = errors;
if(!(validate79(data8, {instancePath:instancePath+"/terminationGracePeriodSeconds",parentData:data,parentDataProperty:"terminationGracePeriodSeconds",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.timeoutSeconds !== undefined){
let data9 = data.timeoutSeconds;
const _errs11 = errors;
if(!(validate78(data9, {instancePath:instancePath+"/timeoutSeconds",parentData:data,parentDataProperty:"timeoutSeconds",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
}
validate80.errors = vErrors;
return errors === 0;
}

const schema47 = {"items":{"$ref":"W4Z_n5BnB66USjWh05DixBIRyKBkgB0p3XT4-b20o7U"},"type":"array","nullable":true};
const schema48 = {"properties":{"containerPort":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"hostIP":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"hostPort":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"protocol":{"$ref":"clxWPOfeflcI7GT0mKf9XOAew5S3Ye83ruoYctXlNVY"}},"required":["containerPort"],"type":"object"};
const schema49 = {"default":"TCP","type":"string","nullable":true};

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
validate143.errors = vErrors;
return errors === 0;
}


function validate138(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate23(data0, {instancePath:instancePath+"/containerPort",parentData:data,parentDataProperty:"containerPort",rootData}))){
vErrors = vErrors === null ? validate23.errors : vErrors.concat(validate23.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.hostIP !== undefined){
let data1 = data.hostIP;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/hostIP",parentData:data,parentDataProperty:"hostIP",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.hostPort !== undefined){
let data2 = data.hostPort;
const _errs3 = errors;
if(!(validate78(data2, {instancePath:instancePath+"/hostPort",parentData:data,parentDataProperty:"hostPort",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.name !== undefined){
let data3 = data.name;
const _errs4 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.protocol !== undefined){
let data4 = data.protocol;
const _errs5 = errors;
if(!(validate143(data4, {instancePath:instancePath+"/protocol",parentData:data,parentDataProperty:"protocol",rootData}))){
vErrors = vErrors === null ? validate143.errors : vErrors.concat(validate143.errors);
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
validate138.errors = vErrors;
return errors === 0;
}


function validate137(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate138(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate138.errors : vErrors.concat(validate138.errors);
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
validate137.errors = vErrors;
return errors === 0;
}

const schema50 = {"items":{"$ref":"7TSxkO796UTLbFFdnMD77Gk_wJkH64SQBmtdtNBI1DI"},"type":"array","nullable":true};
const schema51 = {"properties":{"resourceName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"restartPolicy":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["resourceName","restartPolicy"],"type":"object"};

function validate147(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/resourceName",parentData:data,parentDataProperty:"resourceName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.restartPolicy !== undefined){
let data1 = data.restartPolicy;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/restartPolicy",parentData:data,parentDataProperty:"restartPolicy",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate147.errors = vErrors;
return errors === 0;
}


function validate146(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate147(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate147.errors : vErrors.concat(validate147.errors);
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
validate146.errors = vErrors;
return errors === 0;
}

const schema52 = {"properties":{"claims":{"$ref":"jARlz_jRCwuBtBAPv6O31_X-vye3IHpN8UfU2yAeEBc"},"limits":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"},"requests":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"}},"type":"object","nullable":true};
const schema53 = {"items":{"$ref":"fBVfIKFFliLviCbz7ba4Y3cixNwtkbdwlBfhQb5Wskg"},"type":"array","nullable":true};
const schema54 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"request":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["name"],"type":"object"};

function validate153(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.request !== undefined){
let data1 = data.request;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/request",parentData:data,parentDataProperty:"request",rootData}))){
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
validate153.errors = vErrors;
return errors === 0;
}


function validate152(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate153(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate153.errors : vErrors.concat(validate153.errors);
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
validate152.errors = vErrors;
return errors === 0;
}

const schema35 = {"additionalProperties":{"$ref":"3dmDSoHI3Hv64ppBBbqAEh3HzVwqzFWb_nrM2BhHTgs"},"type":"object","properties":{},"nullable":true};

function validate97(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate94(data0, {instancePath:instancePath+"/" + key0.replace(/~/g, "~0").replace(/\//g, "~1"),parentData:data,parentDataProperty:key0,rootData}))){
vErrors = vErrors === null ? validate94.errors : vErrors.concat(validate94.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate97.errors = vErrors;
return errors === 0;
}


function validate151(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate152(data0, {instancePath:instancePath+"/claims",parentData:data,parentDataProperty:"claims",rootData}))){
vErrors = vErrors === null ? validate152.errors : vErrors.concat(validate152.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.limits !== undefined){
let data1 = data.limits;
const _errs3 = errors;
if(!(validate97(data1, {instancePath:instancePath+"/limits",parentData:data,parentDataProperty:"limits",rootData}))){
vErrors = vErrors === null ? validate97.errors : vErrors.concat(validate97.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.requests !== undefined){
let data2 = data.requests;
const _errs4 = errors;
if(!(validate97(data2, {instancePath:instancePath+"/requests",parentData:data,parentDataProperty:"requests",rootData}))){
vErrors = vErrors === null ? validate97.errors : vErrors.concat(validate97.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate151.errors = vErrors;
return errors === 0;
}

const schema55 = {"items":{"$ref":"tySkinukrNUkRdT5RTlRnJ6Q5BCp7PH4cvcjnXQi7qk"},"type":"array","nullable":true};
const schema56 = {"properties":{"action":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"exitCodes":{"$ref":"kC8sbNbSbDgzM2lKWJrZR2A3aIX9Qs7YPcegJUJ89mw"}},"required":["action"],"type":"object"};
const schema57 = {"properties":{"operator":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"values":{"$ref":"vFZBaVxDYEw8WDgeqavILxL3SULZ27GRcVk6BpSH3Rc"}},"required":["operator"],"type":"object","nullable":true};
const schema58 = {"items":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"type":"array","nullable":true};

function validate165(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate23(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate23.errors : vErrors.concat(validate23.errors);
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
validate165.errors = vErrors;
return errors === 0;
}


function validate163(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/operator",parentData:data,parentDataProperty:"operator",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.values !== undefined){
let data1 = data.values;
const _errs3 = errors;
if(!(validate165(data1, {instancePath:instancePath+"/values",parentData:data,parentDataProperty:"values",rootData}))){
vErrors = vErrors === null ? validate165.errors : vErrors.concat(validate165.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate163.errors = vErrors;
return errors === 0;
}


function validate161(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/action",parentData:data,parentDataProperty:"action",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.exitCodes !== undefined){
let data1 = data.exitCodes;
const _errs2 = errors;
if(!(validate163(data1, {instancePath:instancePath+"/exitCodes",parentData:data,parentDataProperty:"exitCodes",rootData}))){
vErrors = vErrors === null ? validate163.errors : vErrors.concat(validate163.errors);
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
validate161.errors = vErrors;
return errors === 0;
}


function validate160(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate161(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate161.errors : vErrors.concat(validate161.errors);
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
validate160.errors = vErrors;
return errors === 0;
}

const schema59 = {"properties":{"allowPrivilegeEscalation":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"appArmorProfile":{"$ref":"aMnE7Im_y76-Nz9q0hhh6yja57sQFej02mLc00OwhtE"},"capabilities":{"$ref":"ihLpIC0AJcbzOXQ910bylE0NrJtM_usuKt6HWO2C5R8"},"privileged":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"procMount":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnlyRootFilesystem":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"runAsGroup":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"runAsNonRoot":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"runAsUser":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"seLinuxOptions":{"$ref":"RAvVUd1y4DBTzL23WWML0HifcsQtvUbbJLKVUvzXXO4"},"seccompProfile":{"$ref":"aMnE7Im_y76-Nz9q0hhh6yja57sQFej02mLc00OwhtE"},"windowsOptions":{"$ref":"6z1i1A1Aq99luryppL8NHWpsKmtBfItfBpL24p91f_I"}},"type":"object","nullable":true};
const schema36 = {"properties":{"localhostProfile":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"type":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["type"],"type":"object","nullable":true};

function validate99(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/localhostProfile",parentData:data,parentDataProperty:"localhostProfile",rootData}))){
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
validate99.errors = vErrors;
return errors === 0;
}

const schema60 = {"properties":{"add":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"drop":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};

function validate173(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate9(data0, {instancePath:instancePath+"/add",parentData:data,parentDataProperty:"add",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.drop !== undefined){
let data1 = data.drop;
const _errs3 = errors;
if(!(validate9(data1, {instancePath:instancePath+"/drop",parentData:data,parentDataProperty:"drop",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate173.errors = vErrors;
return errors === 0;
}

const schema61 = {"properties":{"level":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"role":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"type":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"user":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate183(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/level",parentData:data,parentDataProperty:"level",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.role !== undefined){
let data1 = data.role;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/role",parentData:data,parentDataProperty:"role",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.type !== undefined){
let data2 = data.type;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.user !== undefined){
let data3 = data.user;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/user",parentData:data,parentDataProperty:"user",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate183.errors = vErrors;
return errors === 0;
}

const schema62 = {"properties":{"gmsaCredentialSpec":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"gmsaCredentialSpecName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"hostProcess":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"runAsUserName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

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
if(data.gmsaCredentialSpec !== undefined){
let data0 = data.gmsaCredentialSpec;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/gmsaCredentialSpec",parentData:data,parentDataProperty:"gmsaCredentialSpec",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.gmsaCredentialSpecName !== undefined){
let data1 = data.gmsaCredentialSpecName;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/gmsaCredentialSpecName",parentData:data,parentDataProperty:"gmsaCredentialSpecName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.hostProcess !== undefined){
let data2 = data.hostProcess;
const _errs4 = errors;
if(!(validate41(data2, {instancePath:instancePath+"/hostProcess",parentData:data,parentDataProperty:"hostProcess",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.runAsUserName !== undefined){
let data3 = data.runAsUserName;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/runAsUserName",parentData:data,parentDataProperty:"runAsUserName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
if(data.allowPrivilegeEscalation !== undefined){
let data0 = data.allowPrivilegeEscalation;
const _errs2 = errors;
if(!(validate41(data0, {instancePath:instancePath+"/allowPrivilegeEscalation",parentData:data,parentDataProperty:"allowPrivilegeEscalation",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.appArmorProfile !== undefined){
let data1 = data.appArmorProfile;
const _errs3 = errors;
if(!(validate99(data1, {instancePath:instancePath+"/appArmorProfile",parentData:data,parentDataProperty:"appArmorProfile",rootData}))){
vErrors = vErrors === null ? validate99.errors : vErrors.concat(validate99.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.capabilities !== undefined){
let data2 = data.capabilities;
const _errs4 = errors;
if(!(validate173(data2, {instancePath:instancePath+"/capabilities",parentData:data,parentDataProperty:"capabilities",rootData}))){
vErrors = vErrors === null ? validate173.errors : vErrors.concat(validate173.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.privileged !== undefined){
let data3 = data.privileged;
const _errs5 = errors;
if(!(validate41(data3, {instancePath:instancePath+"/privileged",parentData:data,parentDataProperty:"privileged",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.procMount !== undefined){
let data4 = data.procMount;
const _errs6 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/procMount",parentData:data,parentDataProperty:"procMount",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.readOnlyRootFilesystem !== undefined){
let data5 = data.readOnlyRootFilesystem;
const _errs7 = errors;
if(!(validate41(data5, {instancePath:instancePath+"/readOnlyRootFilesystem",parentData:data,parentDataProperty:"readOnlyRootFilesystem",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.runAsGroup !== undefined){
let data6 = data.runAsGroup;
const _errs8 = errors;
if(!(validate79(data6, {instancePath:instancePath+"/runAsGroup",parentData:data,parentDataProperty:"runAsGroup",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.runAsNonRoot !== undefined){
let data7 = data.runAsNonRoot;
const _errs9 = errors;
if(!(validate41(data7, {instancePath:instancePath+"/runAsNonRoot",parentData:data,parentDataProperty:"runAsNonRoot",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.runAsUser !== undefined){
let data8 = data.runAsUser;
const _errs10 = errors;
if(!(validate79(data8, {instancePath:instancePath+"/runAsUser",parentData:data,parentDataProperty:"runAsUser",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.seLinuxOptions !== undefined){
let data9 = data.seLinuxOptions;
const _errs11 = errors;
if(!(validate183(data9, {instancePath:instancePath+"/seLinuxOptions",parentData:data,parentDataProperty:"seLinuxOptions",rootData}))){
vErrors = vErrors === null ? validate183.errors : vErrors.concat(validate183.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.seccompProfile !== undefined){
let data10 = data.seccompProfile;
const _errs12 = errors;
if(!(validate99(data10, {instancePath:instancePath+"/seccompProfile",parentData:data,parentDataProperty:"seccompProfile",rootData}))){
vErrors = vErrors === null ? validate99.errors : vErrors.concat(validate99.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.windowsOptions !== undefined){
let data11 = data.windowsOptions;
const _errs13 = errors;
if(!(validate190(data11, {instancePath:instancePath+"/windowsOptions",parentData:data,parentDataProperty:"windowsOptions",rootData}))){
vErrors = vErrors === null ? validate190.errors : vErrors.concat(validate190.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
}
validate170.errors = vErrors;
return errors === 0;
}

const schema63 = {"items":{"$ref":"OVr1UXw7GhHO0fgY9cIvIVSoMiw0xn85m1qUa_o9V8U"},"type":"array","nullable":true};
const schema64 = {"properties":{"devicePath":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["devicePath","name"],"type":"object"};

function validate197(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/devicePath",parentData:data,parentDataProperty:"devicePath",rootData}))){
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
validate197.errors = vErrors;
return errors === 0;
}


function validate196(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate197(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate197.errors : vErrors.concat(validate197.errors);
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
validate196.errors = vErrors;
return errors === 0;
}

const schema65 = {"items":{"$ref":"77fVc4wpT49BE5gRmrKBt_Stn6VnY9fBPevvzJEor0U"},"type":"array","nullable":true};
const schema66 = {"properties":{"mountPath":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"mountPropagation":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"recursiveReadOnly":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"subPath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"subPathExpr":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["mountPath","name"],"type":"object"};

function validate202(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/mountPath",parentData:data,parentDataProperty:"mountPath",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.mountPropagation !== undefined){
let data1 = data.mountPropagation;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/mountPropagation",parentData:data,parentDataProperty:"mountPropagation",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.name !== undefined){
let data2 = data.name;
const _errs3 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.readOnly !== undefined){
let data3 = data.readOnly;
const _errs4 = errors;
if(!(validate41(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.recursiveReadOnly !== undefined){
let data4 = data.recursiveReadOnly;
const _errs5 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/recursiveReadOnly",parentData:data,parentDataProperty:"recursiveReadOnly",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.subPath !== undefined){
let data5 = data.subPath;
const _errs6 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/subPath",parentData:data,parentDataProperty:"subPath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.subPathExpr !== undefined){
let data6 = data.subPathExpr;
const _errs7 = errors;
if(!(validate1(data6, {instancePath:instancePath+"/subPathExpr",parentData:data,parentDataProperty:"subPathExpr",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate202.errors = vErrors;
return errors === 0;
}


function validate201(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate202(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate202.errors : vErrors.concat(validate202.errors);
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
validate201.errors = vErrors;
return errors === 0;
}


function validate211(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate9(data0, {instancePath:instancePath+"/args",parentData:data,parentDataProperty:"args",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.command !== undefined){
let data1 = data.command;
const _errs2 = errors;
if(!(validate9(data1, {instancePath:instancePath+"/command",parentData:data,parentDataProperty:"command",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.env !== undefined){
let data2 = data.env;
const _errs3 = errors;
if(!(validate102(data2, {instancePath:instancePath+"/env",parentData:data,parentDataProperty:"env",rootData}))){
vErrors = vErrors === null ? validate102.errors : vErrors.concat(validate102.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.envFrom !== undefined){
let data3 = data.envFrom;
const _errs4 = errors;
if(!(validate127(data3, {instancePath:instancePath+"/envFrom",parentData:data,parentDataProperty:"envFrom",rootData}))){
vErrors = vErrors === null ? validate127.errors : vErrors.concat(validate127.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.image !== undefined){
let data4 = data.image;
const _errs5 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.imagePullPolicy !== undefined){
let data5 = data.imagePullPolicy;
const _errs6 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/imagePullPolicy",parentData:data,parentDataProperty:"imagePullPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.lifecycle !== undefined){
let data6 = data.lifecycle;
const _errs7 = errors;
if(!(validate133(data6, {instancePath:instancePath+"/lifecycle",parentData:data,parentDataProperty:"lifecycle",rootData}))){
vErrors = vErrors === null ? validate133.errors : vErrors.concat(validate133.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.livenessProbe !== undefined){
let data7 = data.livenessProbe;
const _errs8 = errors;
if(!(validate80(data7, {instancePath:instancePath+"/livenessProbe",parentData:data,parentDataProperty:"livenessProbe",rootData}))){
vErrors = vErrors === null ? validate80.errors : vErrors.concat(validate80.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.name !== undefined){
let data8 = data.name;
const _errs9 = errors;
if(!(validate0(data8, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.ports !== undefined){
let data9 = data.ports;
const _errs10 = errors;
if(!(validate137(data9, {instancePath:instancePath+"/ports",parentData:data,parentDataProperty:"ports",rootData}))){
vErrors = vErrors === null ? validate137.errors : vErrors.concat(validate137.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.readinessProbe !== undefined){
let data10 = data.readinessProbe;
const _errs11 = errors;
if(!(validate80(data10, {instancePath:instancePath+"/readinessProbe",parentData:data,parentDataProperty:"readinessProbe",rootData}))){
vErrors = vErrors === null ? validate80.errors : vErrors.concat(validate80.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.resizePolicy !== undefined){
let data11 = data.resizePolicy;
const _errs12 = errors;
if(!(validate146(data11, {instancePath:instancePath+"/resizePolicy",parentData:data,parentDataProperty:"resizePolicy",rootData}))){
vErrors = vErrors === null ? validate146.errors : vErrors.concat(validate146.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.resources !== undefined){
let data12 = data.resources;
const _errs13 = errors;
if(!(validate151(data12, {instancePath:instancePath+"/resources",parentData:data,parentDataProperty:"resources",rootData}))){
vErrors = vErrors === null ? validate151.errors : vErrors.concat(validate151.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.restartPolicy !== undefined){
let data13 = data.restartPolicy;
const _errs14 = errors;
if(!(validate1(data13, {instancePath:instancePath+"/restartPolicy",parentData:data,parentDataProperty:"restartPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
if(data.restartPolicyRules !== undefined){
let data14 = data.restartPolicyRules;
const _errs15 = errors;
if(!(validate160(data14, {instancePath:instancePath+"/restartPolicyRules",parentData:data,parentDataProperty:"restartPolicyRules",rootData}))){
vErrors = vErrors === null ? validate160.errors : vErrors.concat(validate160.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs15 === errors;
}
if(data.securityContext !== undefined){
let data15 = data.securityContext;
const _errs16 = errors;
if(!(validate170(data15, {instancePath:instancePath+"/securityContext",parentData:data,parentDataProperty:"securityContext",rootData}))){
vErrors = vErrors === null ? validate170.errors : vErrors.concat(validate170.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs16 === errors;
}
if(data.startupProbe !== undefined){
let data16 = data.startupProbe;
const _errs17 = errors;
if(!(validate80(data16, {instancePath:instancePath+"/startupProbe",parentData:data,parentDataProperty:"startupProbe",rootData}))){
vErrors = vErrors === null ? validate80.errors : vErrors.concat(validate80.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs17 === errors;
}
if(data.stdin !== undefined){
let data17 = data.stdin;
const _errs18 = errors;
if(!(validate41(data17, {instancePath:instancePath+"/stdin",parentData:data,parentDataProperty:"stdin",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs18 === errors;
}
if(data.stdinOnce !== undefined){
let data18 = data.stdinOnce;
const _errs19 = errors;
if(!(validate41(data18, {instancePath:instancePath+"/stdinOnce",parentData:data,parentDataProperty:"stdinOnce",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs19 === errors;
}
if(data.terminationMessagePath !== undefined){
let data19 = data.terminationMessagePath;
const _errs20 = errors;
if(!(validate1(data19, {instancePath:instancePath+"/terminationMessagePath",parentData:data,parentDataProperty:"terminationMessagePath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs20 === errors;
}
if(data.terminationMessagePolicy !== undefined){
let data20 = data.terminationMessagePolicy;
const _errs21 = errors;
if(!(validate1(data20, {instancePath:instancePath+"/terminationMessagePolicy",parentData:data,parentDataProperty:"terminationMessagePolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs21 === errors;
}
if(data.tty !== undefined){
let data21 = data.tty;
const _errs22 = errors;
if(!(validate41(data21, {instancePath:instancePath+"/tty",parentData:data,parentDataProperty:"tty",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs22 === errors;
}
if(data.volumeDevices !== undefined){
let data22 = data.volumeDevices;
const _errs23 = errors;
if(!(validate196(data22, {instancePath:instancePath+"/volumeDevices",parentData:data,parentDataProperty:"volumeDevices",rootData}))){
vErrors = vErrors === null ? validate196.errors : vErrors.concat(validate196.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs23 === errors;
}
if(data.volumeMounts !== undefined){
let data23 = data.volumeMounts;
const _errs24 = errors;
if(!(validate201(data23, {instancePath:instancePath+"/volumeMounts",parentData:data,parentDataProperty:"volumeMounts",rootData}))){
vErrors = vErrors === null ? validate201.errors : vErrors.concat(validate201.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs24 === errors;
}
if(data.workingDir !== undefined){
let data24 = data.workingDir;
const _errs25 = errors;
if(!(validate1(data24, {instancePath:instancePath+"/workingDir",parentData:data,parentDataProperty:"workingDir",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate211.errors = vErrors;
return errors === 0;
}


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
if(!(validate211(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate211.errors : vErrors.concat(validate211.errors);
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

const schema99 = {"properties":{"nameservers":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"options":{"$ref":"3GiESGFQ1bt10pSKgDXOz2E8WZV5mfZ_lzY5h2ZooyI"},"searches":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};
const schema100 = {"items":{"$ref":"F4Xy5Fz36W_URgJ39USLrSRkrK9xMUDLknD7E0nmTrw"},"type":"array","nullable":true};
const schema101 = {"properties":{"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"value":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object"};

function validate328(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
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
const err0 = {instancePath,schemaPath:"#/type",keyword:"type",params:{type: "object"}};
if(vErrors === null){
vErrors = [err0];
}
else {
vErrors.push(err0);
}
errors++;
}
validate328.errors = vErrors;
return errors === 0;
}


function validate327(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate328(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate328.errors : vErrors.concat(validate328.errors);
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
validate327.errors = vErrors;
return errors === 0;
}


function validate325(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate9(data0, {instancePath:instancePath+"/nameservers",parentData:data,parentDataProperty:"nameservers",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.options !== undefined){
let data1 = data.options;
const _errs3 = errors;
if(!(validate327(data1, {instancePath:instancePath+"/options",parentData:data,parentDataProperty:"options",rootData}))){
vErrors = vErrors === null ? validate327.errors : vErrors.concat(validate327.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.searches !== undefined){
let data2 = data.searches;
const _errs4 = errors;
if(!(validate9(data2, {instancePath:instancePath+"/searches",parentData:data,parentDataProperty:"searches",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate325.errors = vErrors;
return errors === 0;
}

const schema102 = {"items":{"$ref":"C-ceGOizjWksqCzap3EC-S8JeZSosW0aT9tkSJlW-hw"},"type":"array","nullable":true};
const schema103 = {"properties":{"args":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"command":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"env":{"$ref":"QK6qdOVPcIj3ymOL58J6NgOdTW_Xbf1pI79iGILi8uU"},"envFrom":{"$ref":"ZRw41W3mWwhKksdKQqy6gdGI0VqZa0mht_DKnlcF-lw"},"image":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"imagePullPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"lifecycle":{"$ref":"2GolHCLsHBmPr5h5-SqeL3kwFl1yyHXgHxOurrzuSpc"},"livenessProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"ports":{"$ref":"QdA0VP-JsKHKjWu-1Hu8oyYcguMz4UHlXTFbpvKweaM"},"readinessProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"resizePolicy":{"$ref":"6FxzbMy3fQv1gheBeNadVqOl2xbXIqiFGG5KNo9kRcw"},"resources":{"$ref":"FhMRS9yiPxYjQhkDxbgv-jQ4jx9VLxRdMLNZtrOnwhI"},"restartPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"restartPolicyRules":{"$ref":"bn1AnMh-2dkJyL1LY5A_xgvSFgNvRm9Wd4RL1NkeXRU"},"securityContext":{"$ref":"ppbGhvBFiV7RuYhp7GW5xoG5M7lU4kbUlUSlBvBbdzE"},"startupProbe":{"$ref":"5AW0Q6kBN4321Hs5WncP7jZ-5npxooKpHn9Lz5S4Ppc"},"stdin":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"stdinOnce":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"targetContainerName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"terminationMessagePath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"terminationMessagePolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"tty":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeDevices":{"$ref":"yySLqkz1jc_2D5zmwBnHmcS-y-0ogKvDTXKQGVruH8A"},"volumeMounts":{"$ref":"K3z2jQ4aYZcOrB0gSTeLWvm5O5c50b2P06GZmb9a8OM"},"workingDir":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["name"],"type":"object"};

function validate338(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate9(data0, {instancePath:instancePath+"/args",parentData:data,parentDataProperty:"args",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.command !== undefined){
let data1 = data.command;
const _errs2 = errors;
if(!(validate9(data1, {instancePath:instancePath+"/command",parentData:data,parentDataProperty:"command",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.env !== undefined){
let data2 = data.env;
const _errs3 = errors;
if(!(validate102(data2, {instancePath:instancePath+"/env",parentData:data,parentDataProperty:"env",rootData}))){
vErrors = vErrors === null ? validate102.errors : vErrors.concat(validate102.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.envFrom !== undefined){
let data3 = data.envFrom;
const _errs4 = errors;
if(!(validate127(data3, {instancePath:instancePath+"/envFrom",parentData:data,parentDataProperty:"envFrom",rootData}))){
vErrors = vErrors === null ? validate127.errors : vErrors.concat(validate127.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.image !== undefined){
let data4 = data.image;
const _errs5 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.imagePullPolicy !== undefined){
let data5 = data.imagePullPolicy;
const _errs6 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/imagePullPolicy",parentData:data,parentDataProperty:"imagePullPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.lifecycle !== undefined){
let data6 = data.lifecycle;
const _errs7 = errors;
if(!(validate133(data6, {instancePath:instancePath+"/lifecycle",parentData:data,parentDataProperty:"lifecycle",rootData}))){
vErrors = vErrors === null ? validate133.errors : vErrors.concat(validate133.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.livenessProbe !== undefined){
let data7 = data.livenessProbe;
const _errs8 = errors;
if(!(validate80(data7, {instancePath:instancePath+"/livenessProbe",parentData:data,parentDataProperty:"livenessProbe",rootData}))){
vErrors = vErrors === null ? validate80.errors : vErrors.concat(validate80.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.name !== undefined){
let data8 = data.name;
const _errs9 = errors;
if(!(validate0(data8, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.ports !== undefined){
let data9 = data.ports;
const _errs10 = errors;
if(!(validate137(data9, {instancePath:instancePath+"/ports",parentData:data,parentDataProperty:"ports",rootData}))){
vErrors = vErrors === null ? validate137.errors : vErrors.concat(validate137.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.readinessProbe !== undefined){
let data10 = data.readinessProbe;
const _errs11 = errors;
if(!(validate80(data10, {instancePath:instancePath+"/readinessProbe",parentData:data,parentDataProperty:"readinessProbe",rootData}))){
vErrors = vErrors === null ? validate80.errors : vErrors.concat(validate80.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.resizePolicy !== undefined){
let data11 = data.resizePolicy;
const _errs12 = errors;
if(!(validate146(data11, {instancePath:instancePath+"/resizePolicy",parentData:data,parentDataProperty:"resizePolicy",rootData}))){
vErrors = vErrors === null ? validate146.errors : vErrors.concat(validate146.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.resources !== undefined){
let data12 = data.resources;
const _errs13 = errors;
if(!(validate151(data12, {instancePath:instancePath+"/resources",parentData:data,parentDataProperty:"resources",rootData}))){
vErrors = vErrors === null ? validate151.errors : vErrors.concat(validate151.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.restartPolicy !== undefined){
let data13 = data.restartPolicy;
const _errs14 = errors;
if(!(validate1(data13, {instancePath:instancePath+"/restartPolicy",parentData:data,parentDataProperty:"restartPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
if(data.restartPolicyRules !== undefined){
let data14 = data.restartPolicyRules;
const _errs15 = errors;
if(!(validate160(data14, {instancePath:instancePath+"/restartPolicyRules",parentData:data,parentDataProperty:"restartPolicyRules",rootData}))){
vErrors = vErrors === null ? validate160.errors : vErrors.concat(validate160.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs15 === errors;
}
if(data.securityContext !== undefined){
let data15 = data.securityContext;
const _errs16 = errors;
if(!(validate170(data15, {instancePath:instancePath+"/securityContext",parentData:data,parentDataProperty:"securityContext",rootData}))){
vErrors = vErrors === null ? validate170.errors : vErrors.concat(validate170.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs16 === errors;
}
if(data.startupProbe !== undefined){
let data16 = data.startupProbe;
const _errs17 = errors;
if(!(validate80(data16, {instancePath:instancePath+"/startupProbe",parentData:data,parentDataProperty:"startupProbe",rootData}))){
vErrors = vErrors === null ? validate80.errors : vErrors.concat(validate80.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs17 === errors;
}
if(data.stdin !== undefined){
let data17 = data.stdin;
const _errs18 = errors;
if(!(validate41(data17, {instancePath:instancePath+"/stdin",parentData:data,parentDataProperty:"stdin",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs18 === errors;
}
if(data.stdinOnce !== undefined){
let data18 = data.stdinOnce;
const _errs19 = errors;
if(!(validate41(data18, {instancePath:instancePath+"/stdinOnce",parentData:data,parentDataProperty:"stdinOnce",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs19 === errors;
}
if(data.targetContainerName !== undefined){
let data19 = data.targetContainerName;
const _errs20 = errors;
if(!(validate1(data19, {instancePath:instancePath+"/targetContainerName",parentData:data,parentDataProperty:"targetContainerName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs20 === errors;
}
if(data.terminationMessagePath !== undefined){
let data20 = data.terminationMessagePath;
const _errs21 = errors;
if(!(validate1(data20, {instancePath:instancePath+"/terminationMessagePath",parentData:data,parentDataProperty:"terminationMessagePath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs21 === errors;
}
if(data.terminationMessagePolicy !== undefined){
let data21 = data.terminationMessagePolicy;
const _errs22 = errors;
if(!(validate1(data21, {instancePath:instancePath+"/terminationMessagePolicy",parentData:data,parentDataProperty:"terminationMessagePolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs22 === errors;
}
if(data.tty !== undefined){
let data22 = data.tty;
const _errs23 = errors;
if(!(validate41(data22, {instancePath:instancePath+"/tty",parentData:data,parentDataProperty:"tty",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs23 === errors;
}
if(data.volumeDevices !== undefined){
let data23 = data.volumeDevices;
const _errs24 = errors;
if(!(validate196(data23, {instancePath:instancePath+"/volumeDevices",parentData:data,parentDataProperty:"volumeDevices",rootData}))){
vErrors = vErrors === null ? validate196.errors : vErrors.concat(validate196.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs24 === errors;
}
if(data.volumeMounts !== undefined){
let data24 = data.volumeMounts;
const _errs25 = errors;
if(!(validate201(data24, {instancePath:instancePath+"/volumeMounts",parentData:data,parentDataProperty:"volumeMounts",rootData}))){
vErrors = vErrors === null ? validate201.errors : vErrors.concat(validate201.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs25 === errors;
}
if(data.workingDir !== undefined){
let data25 = data.workingDir;
const _errs26 = errors;
if(!(validate1(data25, {instancePath:instancePath+"/workingDir",parentData:data,parentDataProperty:"workingDir",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate338.errors = vErrors;
return errors === 0;
}


function validate337(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate338(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate338.errors : vErrors.concat(validate338.errors);
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
validate337.errors = vErrors;
return errors === 0;
}

const schema104 = {"items":{"$ref":"1HespOXKrotYoqvobtjdYpFt-3Uj4rr3rOO-NdY0MJw"},"type":"array","nullable":true};
const schema105 = {"properties":{"hostnames":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"ip":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["ip"],"type":"object"};

function validate368(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate9(data0, {instancePath:instancePath+"/hostnames",parentData:data,parentDataProperty:"hostnames",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.ip !== undefined){
let data1 = data.ip;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/ip",parentData:data,parentDataProperty:"ip",rootData}))){
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
validate368.errors = vErrors;
return errors === 0;
}


function validate367(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate368(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate368.errors : vErrors.concat(validate368.errors);
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
validate367.errors = vErrors;
return errors === 0;
}

const schema106 = {"items":{"$ref":"CeclzNr20k5vr0FY8_Di-i5kv_hKd8rGtt9CG90tDrI"},"type":"array","nullable":true};
const schema76 = {"properties":{"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"}},"type":"object"};

function validate264(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.name !== undefined){
let data0 = data.name;
const _errs1 = errors;
if(!(validate44(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate44.errors : vErrors.concat(validate44.errors);
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
validate264.errors = vErrors;
return errors === 0;
}


function validate379(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate264(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate264.errors : vErrors.concat(validate264.errors);
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
validate379.errors = vErrors;
return errors === 0;
}

const schema107 = {"items":{"$ref":"chE8IbiwOYvrhp6jxf4CeNPe4xowDfDQCt1Zhaa9Prk"},"type":"array","nullable":true};

function validate382(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate211(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate211.errors : vErrors.concat(validate211.errors);
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
validate382.errors = vErrors;
return errors === 0;
}

const schema108 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["name"],"type":"object","nullable":true};

function validate387(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate387.errors = vErrors;
return errors === 0;
}

const schema109 = {"items":{"$ref":"7SJ8IHNZpr4K9bVQcc8BXyxEB7fvxKGrCHMOn4uuNjk"},"type":"array","nullable":true};
const schema110 = {"properties":{"conditionType":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["conditionType"],"type":"object"};

function validate395(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/conditionType",parentData:data,parentDataProperty:"conditionType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate395.errors = vErrors;
return errors === 0;
}


function validate394(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate395(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate395.errors : vErrors.concat(validate395.errors);
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
validate394.errors = vErrors;
return errors === 0;
}

const schema111 = {"items":{"$ref":"8oMSVTgbQLfKCbtW7rBmcM71xueDXJZxnN_-a3YWUoE"},"type":"array","nullable":true};
const schema112 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"resourceClaimName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"resourceClaimTemplateName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["name"],"type":"object"};

function validate400(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.resourceClaimName !== undefined){
let data1 = data.resourceClaimName;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/resourceClaimName",parentData:data,parentDataProperty:"resourceClaimName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.resourceClaimTemplateName !== undefined){
let data2 = data.resourceClaimTemplateName;
const _errs3 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/resourceClaimTemplateName",parentData:data,parentDataProperty:"resourceClaimTemplateName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate400.errors = vErrors;
return errors === 0;
}


function validate399(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate400(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate400.errors : vErrors.concat(validate400.errors);
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
validate399.errors = vErrors;
return errors === 0;
}

const schema113 = {"items":{"$ref":"Tv4xxEM4-rkWx429stngdyg1y-NXOJv9XnOY7jdo7LY"},"type":"array","nullable":true};
const schema114 = {"properties":{"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["name"],"type":"object"};

function validate411(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate411.errors = vErrors;
return errors === 0;
}


function validate410(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate411(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate411.errors : vErrors.concat(validate411.errors);
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
validate410.errors = vErrors;
return errors === 0;
}

const schema115 = {"properties":{"podGroupName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate415(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/podGroupName",parentData:data,parentDataProperty:"podGroupName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate415.errors = vErrors;
return errors === 0;
}

const schema116 = {"properties":{"appArmorProfile":{"$ref":"aMnE7Im_y76-Nz9q0hhh6yja57sQFej02mLc00OwhtE"},"fsGroup":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"fsGroupChangePolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"runAsGroup":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"runAsNonRoot":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"runAsUser":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"seLinuxChangePolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"seLinuxOptions":{"$ref":"RAvVUd1y4DBTzL23WWML0HifcsQtvUbbJLKVUvzXXO4"},"seccompProfile":{"$ref":"aMnE7Im_y76-Nz9q0hhh6yja57sQFej02mLc00OwhtE"},"supplementalGroups":{"$ref":"61CuU5PR0DTXgf6CSVY2YGQyMC0xseKwOl9CPTT5SMw"},"supplementalGroupsPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"sysctls":{"$ref":"MsS0666yf0HSUh6mG0cBqbw9ThqZk9-DowZaIzPios0"},"windowsOptions":{"$ref":"6z1i1A1Aq99luryppL8NHWpsKmtBfItfBpL24p91f_I"}},"type":"object","nullable":true};
const schema117 = {"items":{"$ref":"0YQm7z5aN2fg7S8XlbxrquRU6pdIr97uCbtQYyjv3S4"},"type":"array","nullable":true};

function validate428(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate71(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate71.errors : vErrors.concat(validate71.errors);
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
validate428.errors = vErrors;
return errors === 0;
}


function validate418(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate99(data0, {instancePath:instancePath+"/appArmorProfile",parentData:data,parentDataProperty:"appArmorProfile",rootData}))){
vErrors = vErrors === null ? validate99.errors : vErrors.concat(validate99.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fsGroup !== undefined){
let data1 = data.fsGroup;
const _errs3 = errors;
if(!(validate79(data1, {instancePath:instancePath+"/fsGroup",parentData:data,parentDataProperty:"fsGroup",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.fsGroupChangePolicy !== undefined){
let data2 = data.fsGroupChangePolicy;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/fsGroupChangePolicy",parentData:data,parentDataProperty:"fsGroupChangePolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.runAsGroup !== undefined){
let data3 = data.runAsGroup;
const _errs5 = errors;
if(!(validate79(data3, {instancePath:instancePath+"/runAsGroup",parentData:data,parentDataProperty:"runAsGroup",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.runAsNonRoot !== undefined){
let data4 = data.runAsNonRoot;
const _errs6 = errors;
if(!(validate41(data4, {instancePath:instancePath+"/runAsNonRoot",parentData:data,parentDataProperty:"runAsNonRoot",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.runAsUser !== undefined){
let data5 = data.runAsUser;
const _errs7 = errors;
if(!(validate79(data5, {instancePath:instancePath+"/runAsUser",parentData:data,parentDataProperty:"runAsUser",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.seLinuxChangePolicy !== undefined){
let data6 = data.seLinuxChangePolicy;
const _errs8 = errors;
if(!(validate1(data6, {instancePath:instancePath+"/seLinuxChangePolicy",parentData:data,parentDataProperty:"seLinuxChangePolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.seLinuxOptions !== undefined){
let data7 = data.seLinuxOptions;
const _errs9 = errors;
if(!(validate183(data7, {instancePath:instancePath+"/seLinuxOptions",parentData:data,parentDataProperty:"seLinuxOptions",rootData}))){
vErrors = vErrors === null ? validate183.errors : vErrors.concat(validate183.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.seccompProfile !== undefined){
let data8 = data.seccompProfile;
const _errs10 = errors;
if(!(validate99(data8, {instancePath:instancePath+"/seccompProfile",parentData:data,parentDataProperty:"seccompProfile",rootData}))){
vErrors = vErrors === null ? validate99.errors : vErrors.concat(validate99.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.supplementalGroups !== undefined){
let data9 = data.supplementalGroups;
const _errs11 = errors;
if(!(validate428(data9, {instancePath:instancePath+"/supplementalGroups",parentData:data,parentDataProperty:"supplementalGroups",rootData}))){
vErrors = vErrors === null ? validate428.errors : vErrors.concat(validate428.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.supplementalGroupsPolicy !== undefined){
let data10 = data.supplementalGroupsPolicy;
const _errs12 = errors;
if(!(validate1(data10, {instancePath:instancePath+"/supplementalGroupsPolicy",parentData:data,parentDataProperty:"supplementalGroupsPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.sysctls !== undefined){
let data11 = data.sysctls;
const _errs13 = errors;
if(!(validate60(data11, {instancePath:instancePath+"/sysctls",parentData:data,parentDataProperty:"sysctls",rootData}))){
vErrors = vErrors === null ? validate60.errors : vErrors.concat(validate60.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.windowsOptions !== undefined){
let data12 = data.windowsOptions;
const _errs14 = errors;
if(!(validate190(data12, {instancePath:instancePath+"/windowsOptions",parentData:data,parentDataProperty:"windowsOptions",rootData}))){
vErrors = vErrors === null ? validate190.errors : vErrors.concat(validate190.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
}
validate418.errors = vErrors;
return errors === 0;
}

const schema118 = {"items":{"$ref":"0ByfSGW7wvmVW7bPgwyiWBjeufbPURNaUIcLKKK35ZA"},"type":"array","nullable":true};
const schema119 = {"properties":{"effect":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"key":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"operator":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"tolerationSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"value":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object"};

function validate442(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.effect !== undefined){
let data0 = data.effect;
const _errs1 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/effect",parentData:data,parentDataProperty:"effect",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.key !== undefined){
let data1 = data.key;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.operator !== undefined){
let data2 = data.operator;
const _errs3 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/operator",parentData:data,parentDataProperty:"operator",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.tolerationSeconds !== undefined){
let data3 = data.tolerationSeconds;
const _errs4 = errors;
if(!(validate79(data3, {instancePath:instancePath+"/tolerationSeconds",parentData:data,parentDataProperty:"tolerationSeconds",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.value !== undefined){
let data4 = data.value;
const _errs5 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/value",parentData:data,parentDataProperty:"value",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate442.errors = vErrors;
return errors === 0;
}


function validate441(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate442(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate442.errors : vErrors.concat(validate442.errors);
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
validate441.errors = vErrors;
return errors === 0;
}

const schema120 = {"items":{"$ref":"StMxQItS9v0SfOwJaZyLYLt5kRuP512kSHqU4NYTwI0"},"type":"array","nullable":true};
const schema121 = {"properties":{"labelSelector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"matchLabelKeys":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"maxSkew":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"minDomains":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"nodeAffinityPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"nodeTaintsPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"topologyKey":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"whenUnsatisfiable":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["maxSkew","topologyKey","whenUnsatisfiable"],"type":"object"};

function validate451(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate20(data0, {instancePath:instancePath+"/labelSelector",parentData:data,parentDataProperty:"labelSelector",rootData}))){
vErrors = vErrors === null ? validate20.errors : vErrors.concat(validate20.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.matchLabelKeys !== undefined){
let data1 = data.matchLabelKeys;
const _errs2 = errors;
if(!(validate9(data1, {instancePath:instancePath+"/matchLabelKeys",parentData:data,parentDataProperty:"matchLabelKeys",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.maxSkew !== undefined){
let data2 = data.maxSkew;
const _errs3 = errors;
if(!(validate23(data2, {instancePath:instancePath+"/maxSkew",parentData:data,parentDataProperty:"maxSkew",rootData}))){
vErrors = vErrors === null ? validate23.errors : vErrors.concat(validate23.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.minDomains !== undefined){
let data3 = data.minDomains;
const _errs4 = errors;
if(!(validate78(data3, {instancePath:instancePath+"/minDomains",parentData:data,parentDataProperty:"minDomains",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.nodeAffinityPolicy !== undefined){
let data4 = data.nodeAffinityPolicy;
const _errs5 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/nodeAffinityPolicy",parentData:data,parentDataProperty:"nodeAffinityPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.nodeTaintsPolicy !== undefined){
let data5 = data.nodeTaintsPolicy;
const _errs6 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/nodeTaintsPolicy",parentData:data,parentDataProperty:"nodeTaintsPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.topologyKey !== undefined){
let data6 = data.topologyKey;
const _errs7 = errors;
if(!(validate0(data6, {instancePath:instancePath+"/topologyKey",parentData:data,parentDataProperty:"topologyKey",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.whenUnsatisfiable !== undefined){
let data7 = data.whenUnsatisfiable;
const _errs8 = errors;
if(!(validate0(data7, {instancePath:instancePath+"/whenUnsatisfiable",parentData:data,parentDataProperty:"whenUnsatisfiable",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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

const schema122 = {"items":{"$ref":"Hj6zWhh70IB7X0q3eRZDjlHhz7sqbnQspRAima0qQSE"},"type":"array","nullable":true};
const schema123 = {"properties":{"awsElasticBlockStore":{"$ref":"vPA0bm54uQADvg8Ep2156ezWb9pvkjUSgi79O6AEzUs"},"azureDisk":{"$ref":"wFqLNNJgW_o5ObauitiCOyT0NGvkjQH59OgBfR3nx78"},"azureFile":{"$ref":"ra-qRI4a9EDFDlPkSamRu2Q2QFXgcgrTkMEaIAfIFlQ"},"cephfs":{"$ref":"hF2IjS1nzlRhEgH0aYEOkSyicJRz2WA5TydzEGoRLKg"},"cinder":{"$ref":"fNCUZPQc9iEaWmz8ai20p-XbVRg8tBJIhSF3TXq-YMs"},"configMap":{"$ref":"aviBc3qnN2f55E2vimjJLIzsV9DrIo0yQQb5hwqxUOc"},"csi":{"$ref":"oQ5udLLiWaZv5Xde630bmkROSPyJ-qElIPSHcGuPOY4"},"downwardAPI":{"$ref":"GMd9Y-VcblyvoFTlA1OqTa5xw10fjnz_tJ5HDF6AEOU"},"emptyDir":{"$ref":"dD_f54ZfU9CI7NKO29jcEDlRdCPgexeuapdopjIU01E"},"ephemeral":{"$ref":"eNrnIy2CikSgPB0aVTsii2hzgtDrTs6oGNyfcyCWEMs"},"fc":{"$ref":"4YMYCoeKb62-XCvxNQKZZg8ljPa2bImErUwuV6lfzMc"},"flexVolume":{"$ref":"87oLwN-NCLaqdDpK_Xpdr0U9qiabmcRmprmqZV3W2OM"},"flocker":{"$ref":"aKg6ZgUktk8XvWlSQnzXgMGusTLk-Y4v0E66MJqnxKM"},"gcePersistentDisk":{"$ref":"Nc8CY9PKVBuFiqw-t7h5cOVQLWFbxUZcHQ_nHz9xtm0"},"gitRepo":{"$ref":"rS29wFkNVu1ceZUQHqPBmyB8-Ms8_nnLBrD35UT73dI"},"glusterfs":{"$ref":"TpFF3OzeCU-chmDfffTsAmyFGMVfPLAPDphy4r3XKjg"},"hostPath":{"$ref":"EXTQAV0X282Zg3GxvBwRnhWqJJDwIVAFTHzBb8v3nlg"},"image":{"$ref":"5c0MzG5RcFc2MWsW2mHabMHz4ImtJyTyHppNKfKeOdQ"},"iscsi":{"$ref":"8HbKIUlglpSfrcdpgZeJFdBlpWGsIvExGUK1n48WUr0"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"nfs":{"$ref":"nwSeAMDcw-4i412pD0rNHf229BX9fN7BBcjt-3DdCKI"},"persistentVolumeClaim":{"$ref":"Nt1_4vgMqd6c7fTj2yEYei6As7a9E2OyyNV5h8I-T-w"},"photonPersistentDisk":{"$ref":"Ed4AAIR_qYXTjtKQpuCQkveQduEVxyKSFYufAbsWSJQ"},"portworxVolume":{"$ref":"_qkJOFjT4iVO-EThDJ3tJ8R8OjXW5ytgOhPUrts7VCw"},"projected":{"$ref":"7xCtd90hb1cpRlMmHzI_H6sbzbpkjpI3GAlSHRbg64E"},"quobyte":{"$ref":"ZXGgqPX-VDcSDf8w43imCiVzHKzcgfpUOl-yALusknA"},"rbd":{"$ref":"j-DO-_nrU_kl5foZpuWFww_njUnDUr4E1u8oCeqXJE4"},"scaleIO":{"$ref":"Pz5rdKKPt5xwuDA5BqivksbvxoLDreFxoGght9-tDko"},"secret":{"$ref":"p2IOGKuqym0QlcKZpielo4SFxm41yjJyrMQ9Pmr5A2A"},"storageos":{"$ref":"JLIMsOAl8xZaat8jT6oTbEVgrK7d76K8GSAAjC8omEk"},"vsphereVolume":{"$ref":"Yl_IkrhrBmYK2ezUSAmmTuS5zATmqlK0Gh1MlcOGsT8"}},"required":["name"],"type":"object"};
const schema124 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"partition":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeID":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["volumeID"],"type":"object","nullable":true};

function validate464(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.partition !== undefined){
let data1 = data.partition;
const _errs3 = errors;
if(!(validate78(data1, {instancePath:instancePath+"/partition",parentData:data,parentDataProperty:"partition",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.readOnly !== undefined){
let data2 = data.readOnly;
const _errs4 = errors;
if(!(validate41(data2, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.volumeID !== undefined){
let data3 = data.volumeID;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/volumeID",parentData:data,parentDataProperty:"volumeID",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate464.errors = vErrors;
return errors === 0;
}

const schema125 = {"properties":{"cachingMode":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"diskName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"diskURI":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"fsType":{"$ref":"b7rsf0DzZXS6T136mheaM3HHbss23WkJGWNWmAb7obA"},"kind":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"PxmoJm38YOn1dCbyB5PLDDQXV2B2rqFhpzNXj0_ibe4"}},"required":["diskName","diskURI"],"type":"object","nullable":true};
const schema126 = {"default":"ext4","type":"string","nullable":true};

function validate474(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate474.errors = vErrors;
return errors === 0;
}


function validate470(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/cachingMode",parentData:data,parentDataProperty:"cachingMode",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.diskName !== undefined){
let data1 = data.diskName;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/diskName",parentData:data,parentDataProperty:"diskName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.diskURI !== undefined){
let data2 = data.diskURI;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/diskURI",parentData:data,parentDataProperty:"diskURI",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.fsType !== undefined){
let data3 = data.fsType;
const _errs5 = errors;
if(!(validate474(data3, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate474.errors : vErrors.concat(validate474.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.kind !== undefined){
let data4 = data.kind;
const _errs6 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.readOnly !== undefined){
let data5 = data.readOnly;
const _errs7 = errors;
if(!(validate114(data5, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate114.errors : vErrors.concat(validate114.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
}
validate470.errors = vErrors;
return errors === 0;
}

const schema127 = {"properties":{"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"shareName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["secretName","shareName"],"type":"object","nullable":true};

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
if(!(validate41(data0, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.secretName !== undefined){
let data1 = data.secretName;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/secretName",parentData:data,parentDataProperty:"secretName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.shareName !== undefined){
let data2 = data.shareName;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/shareName",parentData:data,parentDataProperty:"shareName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate479.errors = vErrors;
return errors === 0;
}

const schema128 = {"properties":{"monitors":{"$ref":"uqUtVyG9D2UAWDvqosEgVMtwK5kI3TMy2eLbDZB452I"},"path":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretFile":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"user":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["monitors"],"type":"object","nullable":true};
const schema69 = {"properties":{"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"}},"type":"object","nullable":true};

function validate239(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate44(data0, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate44.errors : vErrors.concat(validate44.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate239.errors = vErrors;
return errors === 0;
}


function validate484(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate237(data0, {instancePath:instancePath+"/monitors",parentData:data,parentDataProperty:"monitors",rootData}))){
vErrors = vErrors === null ? validate237.errors : vErrors.concat(validate237.errors);
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
if(!(validate41(data2, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.secretFile !== undefined){
let data3 = data.secretFile;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/secretFile",parentData:data,parentDataProperty:"secretFile",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.secretRef !== undefined){
let data4 = data.secretRef;
const _errs6 = errors;
if(!(validate239(data4, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate239.errors : vErrors.concat(validate239.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.user !== undefined){
let data5 = data.user;
const _errs7 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/user",parentData:data,parentDataProperty:"user",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
}
validate484.errors = vErrors;
return errors === 0;
}

const schema129 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"volumeID":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["volumeID"],"type":"object","nullable":true};

function validate492(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
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
if(!(validate41(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.secretRef !== undefined){
let data2 = data.secretRef;
const _errs4 = errors;
if(!(validate239(data2, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate239.errors : vErrors.concat(validate239.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.volumeID !== undefined){
let data3 = data.volumeID;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/volumeID",parentData:data,parentDataProperty:"volumeID",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate492.errors = vErrors;
return errors === 0;
}

const schema130 = {"properties":{"defaultMode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"items":{"$ref":"gPtCVyFH2eXv-J44JI3istIuhC40N-NdI4mRYcFGaCI"},"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"type":"object","nullable":true};
const schema71 = {"items":{"$ref":"Mw85TCQKW0HVKrKU83sDokxzwBc5-5PPrcGiCJqci0s"},"type":"array","nullable":true};
const schema72 = {"properties":{"key":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"mode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["key","path"],"type":"object"};

function validate248(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/key",parentData:data,parentDataProperty:"key",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.mode !== undefined){
let data1 = data.mode;
const _errs2 = errors;
if(!(validate78(data1, {instancePath:instancePath+"/mode",parentData:data,parentDataProperty:"mode",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.path !== undefined){
let data2 = data.path;
const _errs3 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate248.errors = vErrors;
return errors === 0;
}


function validate247(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate248(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate248.errors : vErrors.concat(validate248.errors);
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
validate247.errors = vErrors;
return errors === 0;
}


function validate498(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate78(data0, {instancePath:instancePath+"/defaultMode",parentData:data,parentDataProperty:"defaultMode",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.items !== undefined){
let data1 = data.items;
const _errs3 = errors;
if(!(validate247(data1, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate247.errors : vErrors.concat(validate247.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.name !== undefined){
let data2 = data.name;
const _errs4 = errors;
if(!(validate44(data2, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate44.errors : vErrors.concat(validate44.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.optional !== undefined){
let data3 = data.optional;
const _errs5 = errors;
if(!(validate41(data3, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate498.errors = vErrors;
return errors === 0;
}

const schema131 = {"properties":{"driver":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"nodePublishSecretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeAttributes":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"}},"required":["driver"],"type":"object","nullable":true};

function validate504(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/driver",parentData:data,parentDataProperty:"driver",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fsType !== undefined){
let data1 = data.fsType;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.nodePublishSecretRef !== undefined){
let data2 = data.nodePublishSecretRef;
const _errs4 = errors;
if(!(validate239(data2, {instancePath:instancePath+"/nodePublishSecretRef",parentData:data,parentDataProperty:"nodePublishSecretRef",rootData}))){
vErrors = vErrors === null ? validate239.errors : vErrors.concat(validate239.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.readOnly !== undefined){
let data3 = data.readOnly;
const _errs5 = errors;
if(!(validate41(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.volumeAttributes !== undefined){
let data4 = data.volumeAttributes;
const _errs6 = errors;
if(!(validate7(data4, {instancePath:instancePath+"/volumeAttributes",parentData:data,parentDataProperty:"volumeAttributes",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate504.errors = vErrors;
return errors === 0;
}

const schema132 = {"properties":{"defaultMode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"items":{"$ref":"d-PZJWo9tuJUK1sF4I9c0c4HI5ec_LUrbkqbJN0kxNA"}},"type":"object","nullable":true};
const schema73 = {"items":{"$ref":"6b0JHACYCsHJ-gVlQX9UriUS8Ox9KDWSM_ixcjRqBYk"},"type":"array","nullable":true};
const schema74 = {"properties":{"fieldRef":{"$ref":"HmyyyuOupyisSiKp0rvRRJtbCek8WlFUsfGxXzcVsrg"},"mode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"resourceFieldRef":{"$ref":"lUaYvKMK_XnIMZc4H9_ERGObFXszqs8GfnzmBfc3Oh8"}},"required":["path"],"type":"object"};

function validate254(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate108(data0, {instancePath:instancePath+"/fieldRef",parentData:data,parentDataProperty:"fieldRef",rootData}))){
vErrors = vErrors === null ? validate108.errors : vErrors.concat(validate108.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.mode !== undefined){
let data1 = data.mode;
const _errs2 = errors;
if(!(validate78(data1, {instancePath:instancePath+"/mode",parentData:data,parentDataProperty:"mode",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.path !== undefined){
let data2 = data.path;
const _errs3 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.resourceFieldRef !== undefined){
let data3 = data.resourceFieldRef;
const _errs4 = errors;
if(!(validate119(data3, {instancePath:instancePath+"/resourceFieldRef",parentData:data,parentDataProperty:"resourceFieldRef",rootData}))){
vErrors = vErrors === null ? validate119.errors : vErrors.concat(validate119.errors);
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
validate254.errors = vErrors;
return errors === 0;
}


function validate253(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate254(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate254.errors : vErrors.concat(validate254.errors);
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
validate253.errors = vErrors;
return errors === 0;
}


function validate511(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate78(data0, {instancePath:instancePath+"/defaultMode",parentData:data,parentDataProperty:"defaultMode",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.items !== undefined){
let data1 = data.items;
const _errs3 = errors;
if(!(validate253(data1, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate253.errors : vErrors.concat(validate253.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate511.errors = vErrors;
return errors === 0;
}

const schema133 = {"properties":{"medium":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"sizeLimit":{"$ref":"3dmDSoHI3Hv64ppBBbqAEh3HzVwqzFWb_nrM2BhHTgs"}},"type":"object","nullable":true};

function validate515(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/medium",parentData:data,parentDataProperty:"medium",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.sizeLimit !== undefined){
let data1 = data.sizeLimit;
const _errs3 = errors;
if(!(validate94(data1, {instancePath:instancePath+"/sizeLimit",parentData:data,parentDataProperty:"sizeLimit",rootData}))){
vErrors = vErrors === null ? validate94.errors : vErrors.concat(validate94.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate515.errors = vErrors;
return errors === 0;
}

const schema134 = {"properties":{"volumeClaimTemplate":{"$ref":"RWcUpIUEoEZhA_O1VJPX5UIK22JWebpZpOk0memvidY"}},"type":"object","nullable":true};
const schema135 = {"properties":{"metadata":{"$ref":"9xOEryzozGanNCCz7goq0K4hzMvoOusQQ4iDJ494Bhw"},"spec":{"$ref":"kyKOBdUty49b7DFqivVu-C5SnGwoky00zYPyDjmfQck"}},"required":["spec"],"type":"object","nullable":true};
const schema136 = {"properties":{"accessModes":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"dataSource":{"$ref":"wjNzvnq-dR_-OiqRXDrOerEJO_HAowFBQnjb_nOgEyU"},"dataSourceRef":{"$ref":"oqf7uvYqNIR47CPHDvw3sYWQXSKnigFe8zHjsXWE35M"},"resources":{"$ref":"yTxZaQI_fncHf2yr5nLz-uUGOQ9FD-geEFW-5-qjX30"},"selector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"storageClassName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumeAttributesClassName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumeMode":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumeName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object"};
const schema137 = {"properties":{"apiGroup":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"kind":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["kind","name"],"type":"object","nullable":true};

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
if(!(validate1(data0, {instancePath:instancePath+"/apiGroup",parentData:data,parentDataProperty:"apiGroup",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.kind !== undefined){
let data1 = data.kind;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.name !== undefined){
let data2 = data.name;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate524.errors = vErrors;
return errors === 0;
}

const schema138 = {"properties":{"apiGroup":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"kind":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"name":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"namespace":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["kind","name"],"type":"object","nullable":true};

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
if(!(validate1(data0, {instancePath:instancePath+"/apiGroup",parentData:data,parentDataProperty:"apiGroup",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.kind !== undefined){
let data1 = data.kind;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.name !== undefined){
let data2 = data.name;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.namespace !== undefined){
let data3 = data.namespace;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/namespace",parentData:data,parentDataProperty:"namespace",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate529.errors = vErrors;
return errors === 0;
}

const schema139 = {"properties":{"limits":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"},"requests":{"$ref":"RCKC7guMhfSVV-Y94GrPgvOuvS1Phl11ypc2ADk-g5Q"}},"type":"object","nullable":true};

function validate535(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate97(data0, {instancePath:instancePath+"/limits",parentData:data,parentDataProperty:"limits",rootData}))){
vErrors = vErrors === null ? validate97.errors : vErrors.concat(validate97.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.requests !== undefined){
let data1 = data.requests;
const _errs3 = errors;
if(!(validate97(data1, {instancePath:instancePath+"/requests",parentData:data,parentDataProperty:"requests",rootData}))){
vErrors = vErrors === null ? validate97.errors : vErrors.concat(validate97.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate535.errors = vErrors;
return errors === 0;
}


function validate522(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.accessModes !== undefined){
let data0 = data.accessModes;
const _errs1 = errors;
if(!(validate9(data0, {instancePath:instancePath+"/accessModes",parentData:data,parentDataProperty:"accessModes",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.dataSource !== undefined){
let data1 = data.dataSource;
const _errs2 = errors;
if(!(validate524(data1, {instancePath:instancePath+"/dataSource",parentData:data,parentDataProperty:"dataSource",rootData}))){
vErrors = vErrors === null ? validate524.errors : vErrors.concat(validate524.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.dataSourceRef !== undefined){
let data2 = data.dataSourceRef;
const _errs3 = errors;
if(!(validate529(data2, {instancePath:instancePath+"/dataSourceRef",parentData:data,parentDataProperty:"dataSourceRef",rootData}))){
vErrors = vErrors === null ? validate529.errors : vErrors.concat(validate529.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.resources !== undefined){
let data3 = data.resources;
const _errs4 = errors;
if(!(validate535(data3, {instancePath:instancePath+"/resources",parentData:data,parentDataProperty:"resources",rootData}))){
vErrors = vErrors === null ? validate535.errors : vErrors.concat(validate535.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.selector !== undefined){
let data4 = data.selector;
const _errs5 = errors;
if(!(validate20(data4, {instancePath:instancePath+"/selector",parentData:data,parentDataProperty:"selector",rootData}))){
vErrors = vErrors === null ? validate20.errors : vErrors.concat(validate20.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.storageClassName !== undefined){
let data5 = data.storageClassName;
const _errs6 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/storageClassName",parentData:data,parentDataProperty:"storageClassName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.volumeAttributesClassName !== undefined){
let data6 = data.volumeAttributesClassName;
const _errs7 = errors;
if(!(validate1(data6, {instancePath:instancePath+"/volumeAttributesClassName",parentData:data,parentDataProperty:"volumeAttributesClassName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.volumeMode !== undefined){
let data7 = data.volumeMode;
const _errs8 = errors;
if(!(validate1(data7, {instancePath:instancePath+"/volumeMode",parentData:data,parentDataProperty:"volumeMode",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.volumeName !== undefined){
let data8 = data.volumeName;
const _errs9 = errors;
if(!(validate1(data8, {instancePath:instancePath+"/volumeName",parentData:data,parentDataProperty:"volumeName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
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
validate522.errors = vErrors;
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
if(!(validate241(data0, {instancePath:instancePath+"/metadata",parentData:data,parentDataProperty:"metadata",rootData}))){
vErrors = vErrors === null ? validate241.errors : vErrors.concat(validate241.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.spec !== undefined){
let data1 = data.spec;
const _errs3 = errors;
if(!(validate522(data1, {instancePath:instancePath+"/spec",parentData:data,parentDataProperty:"spec",rootData}))){
vErrors = vErrors === null ? validate522.errors : vErrors.concat(validate522.errors);
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


function validate519(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate520(data0, {instancePath:instancePath+"/volumeClaimTemplate",parentData:data,parentDataProperty:"volumeClaimTemplate",rootData}))){
vErrors = vErrors === null ? validate520.errors : vErrors.concat(validate520.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate519.errors = vErrors;
return errors === 0;
}

const schema140 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"lun":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"targetWWNs":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"wwids":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"}},"type":"object","nullable":true};

function validate547(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.lun !== undefined){
let data1 = data.lun;
const _errs3 = errors;
if(!(validate78(data1, {instancePath:instancePath+"/lun",parentData:data,parentDataProperty:"lun",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.readOnly !== undefined){
let data2 = data.readOnly;
const _errs4 = errors;
if(!(validate41(data2, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.targetWWNs !== undefined){
let data3 = data.targetWWNs;
const _errs5 = errors;
if(!(validate9(data3, {instancePath:instancePath+"/targetWWNs",parentData:data,parentDataProperty:"targetWWNs",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.wwids !== undefined){
let data4 = data.wwids;
const _errs6 = errors;
if(!(validate9(data4, {instancePath:instancePath+"/wwids",parentData:data,parentDataProperty:"wwids",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate547.errors = vErrors;
return errors === 0;
}

const schema141 = {"properties":{"driver":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"options":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"}},"required":["driver"],"type":"object","nullable":true};

function validate554(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/driver",parentData:data,parentDataProperty:"driver",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.fsType !== undefined){
let data1 = data.fsType;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.options !== undefined){
let data2 = data.options;
const _errs4 = errors;
if(!(validate7(data2, {instancePath:instancePath+"/options",parentData:data,parentDataProperty:"options",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.readOnly !== undefined){
let data3 = data.readOnly;
const _errs5 = errors;
if(!(validate41(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.secretRef !== undefined){
let data4 = data.secretRef;
const _errs6 = errors;
if(!(validate239(data4, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate239.errors : vErrors.concat(validate239.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate554.errors = vErrors;
return errors === 0;
}

const schema142 = {"properties":{"datasetName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"datasetUUID":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate561(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/datasetName",parentData:data,parentDataProperty:"datasetName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.datasetUUID !== undefined){
let data1 = data.datasetUUID;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/datasetUUID",parentData:data,parentDataProperty:"datasetUUID",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate561.errors = vErrors;
return errors === 0;
}

const schema143 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"partition":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"pdName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"required":["pdName"],"type":"object","nullable":true};

function validate565(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.partition !== undefined){
let data1 = data.partition;
const _errs3 = errors;
if(!(validate78(data1, {instancePath:instancePath+"/partition",parentData:data,parentDataProperty:"partition",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.pdName !== undefined){
let data2 = data.pdName;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/pdName",parentData:data,parentDataProperty:"pdName",rootData}))){
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
if(!(validate41(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate565.errors = vErrors;
return errors === 0;
}

const schema144 = {"properties":{"directory":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"repository":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"revision":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["repository"],"type":"object","nullable":true};

function validate571(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/directory",parentData:data,parentDataProperty:"directory",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.repository !== undefined){
let data1 = data.repository;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/repository",parentData:data,parentDataProperty:"repository",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.revision !== undefined){
let data2 = data.revision;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/revision",parentData:data,parentDataProperty:"revision",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate571.errors = vErrors;
return errors === 0;
}

const schema145 = {"properties":{"endpoints":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"required":["endpoints","path"],"type":"object","nullable":true};

function validate576(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/endpoints",parentData:data,parentDataProperty:"endpoints",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
if(!(validate41(data2, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate576.errors = vErrors;
return errors === 0;
}

const schema146 = {"properties":{"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"type":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["path"],"type":"object","nullable":true};

function validate581(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
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
validate581.errors = vErrors;
return errors === 0;
}

const schema147 = {"properties":{"pullPolicy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"reference":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

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
if(data.pullPolicy !== undefined){
let data0 = data.pullPolicy;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/pullPolicy",parentData:data,parentDataProperty:"pullPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.reference !== undefined){
let data1 = data.reference;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/reference",parentData:data,parentDataProperty:"reference",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate585.errors = vErrors;
return errors === 0;
}

const schema148 = {"properties":{"chapAuthDiscovery":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"chapAuthSession":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"initiatorName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"iqn":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"iscsiInterface":{"$ref":"5cSUFXhOjtgR-1-w0YuciQg4fuFzccKB0OFYzeWCsrY"},"lun":{"$ref":"6VbGrbfqnPJ4dMmNewHbRn4M59VZOmu0uhV1edBN8qE"},"portals":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"targetPortal":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["iqn","lun","targetPortal"],"type":"object","nullable":true};
const schema149 = {"default":"default","type":"string","nullable":true};

function validate595(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate595.errors = vErrors;
return errors === 0;
}


function validate589(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate41(data0, {instancePath:instancePath+"/chapAuthDiscovery",parentData:data,parentDataProperty:"chapAuthDiscovery",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.chapAuthSession !== undefined){
let data1 = data.chapAuthSession;
const _errs3 = errors;
if(!(validate41(data1, {instancePath:instancePath+"/chapAuthSession",parentData:data,parentDataProperty:"chapAuthSession",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.fsType !== undefined){
let data2 = data.fsType;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.initiatorName !== undefined){
let data3 = data.initiatorName;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/initiatorName",parentData:data,parentDataProperty:"initiatorName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.iqn !== undefined){
let data4 = data.iqn;
const _errs6 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/iqn",parentData:data,parentDataProperty:"iqn",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.iscsiInterface !== undefined){
let data5 = data.iscsiInterface;
const _errs7 = errors;
if(!(validate595(data5, {instancePath:instancePath+"/iscsiInterface",parentData:data,parentDataProperty:"iscsiInterface",rootData}))){
vErrors = vErrors === null ? validate595.errors : vErrors.concat(validate595.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.lun !== undefined){
let data6 = data.lun;
const _errs8 = errors;
if(!(validate23(data6, {instancePath:instancePath+"/lun",parentData:data,parentDataProperty:"lun",rootData}))){
vErrors = vErrors === null ? validate23.errors : vErrors.concat(validate23.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.portals !== undefined){
let data7 = data.portals;
const _errs9 = errors;
if(!(validate9(data7, {instancePath:instancePath+"/portals",parentData:data,parentDataProperty:"portals",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.readOnly !== undefined){
let data8 = data.readOnly;
const _errs10 = errors;
if(!(validate41(data8, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.secretRef !== undefined){
let data9 = data.secretRef;
const _errs11 = errors;
if(!(validate239(data9, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate239.errors : vErrors.concat(validate239.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.targetPortal !== undefined){
let data10 = data.targetPortal;
const _errs12 = errors;
if(!(validate0(data10, {instancePath:instancePath+"/targetPortal",parentData:data,parentDataProperty:"targetPortal",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
}
validate589.errors = vErrors;
return errors === 0;
}

const schema150 = {"properties":{"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"server":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["path","server"],"type":"object","nullable":true};

function validate604(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
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
if(!(validate41(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.server !== undefined){
let data2 = data.server;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/server",parentData:data,parentDataProperty:"server",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate604.errors = vErrors;
return errors === 0;
}

const schema151 = {"properties":{"claimName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"required":["claimName"],"type":"object","nullable":true};

function validate609(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate0(data0, {instancePath:instancePath+"/claimName",parentData:data,parentDataProperty:"claimName",rootData}))){
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
if(!(validate41(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate609.errors = vErrors;
return errors === 0;
}

const schema152 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"pdID":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["pdID"],"type":"object","nullable":true};

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
if(!(validate1(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.pdID !== undefined){
let data1 = data.pdID;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/pdID",parentData:data,parentDataProperty:"pdID",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate613.errors = vErrors;
return errors === 0;
}

const schema153 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"volumeID":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["volumeID"],"type":"object","nullable":true};

function validate617(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
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
if(!(validate41(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.volumeID !== undefined){
let data2 = data.volumeID;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/volumeID",parentData:data,parentDataProperty:"volumeID",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate617.errors = vErrors;
return errors === 0;
}

const schema154 = {"properties":{"defaultMode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"sources":{"$ref":"yXKZADi5kgSwjsI2ppIyeMUpu4bJiUFINpGosidspnQ"}},"type":"object","nullable":true};
const schema155 = {"items":{"$ref":"dn4YVt3SWkPnEZSpnhO3KeVzR4VIpu7pqfcL-TcKd_8"},"type":"array","nullable":true};
const schema156 = {"properties":{"clusterTrustBundle":{"$ref":"pTiPjbxwypYoy6bApVo9pwc1crR9_v84NPbQ_C3oPbY"},"configMap":{"$ref":"_HCmSZsdcRhpFKkZM7Jl5QCi9tmpzo8UCET9LQeHx0w"},"downwardAPI":{"$ref":"lFpI7xGpYDupYMDE5JdT4IlnCI2yCKkoVlJch0sNjdg"},"podCertificate":{"$ref":"Kcg0-qwUIoLjyZwuajb1VKncuRc2pFltN23i8If-VrM"},"secret":{"$ref":"_HCmSZsdcRhpFKkZM7Jl5QCi9tmpzo8UCET9LQeHx0w"},"serviceAccountToken":{"$ref":"GWk01BlhTg6m5b0ywNZc8vdrp7TAMZkgdXFVQl52C0E"}},"type":"object"};
const schema157 = {"properties":{"labelSelector":{"$ref":"VVCKgJOBvzPMFDWky4rRQ5NkqJqB9eXDLOrP2lnPweI"},"name":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"signerName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["path"],"type":"object","nullable":true};

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
if(!(validate20(data0, {instancePath:instancePath+"/labelSelector",parentData:data,parentDataProperty:"labelSelector",rootData}))){
vErrors = vErrors === null ? validate20.errors : vErrors.concat(validate20.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.name !== undefined){
let data1 = data.name;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.optional !== undefined){
let data2 = data.optional;
const _errs4 = errors;
if(!(validate41(data2, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.path !== undefined){
let data3 = data.path;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/path",parentData:data,parentDataProperty:"path",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.signerName !== undefined){
let data4 = data.signerName;
const _errs6 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/signerName",parentData:data,parentDataProperty:"signerName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate626.errors = vErrors;
return errors === 0;
}

const schema75 = {"properties":{"items":{"$ref":"gPtCVyFH2eXv-J44JI3istIuhC40N-NdI4mRYcFGaCI"},"name":{"$ref":"i2HI34rLdQVXbQqblUmatkJQE-Oau9qTO7E67Oi233U"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"}},"type":"object","nullable":true};

function validate260(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate247(data0, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate247.errors : vErrors.concat(validate247.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.name !== undefined){
let data1 = data.name;
const _errs3 = errors;
if(!(validate44(data1, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate44.errors : vErrors.concat(validate44.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.optional !== undefined){
let data2 = data.optional;
const _errs4 = errors;
if(!(validate41(data2, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
}
validate260.errors = vErrors;
return errors === 0;
}

const schema158 = {"properties":{"items":{"$ref":"d-PZJWo9tuJUK1sF4I9c0c4HI5ec_LUrbkqbJN0kxNA"}},"type":"object","nullable":true};

function validate634(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate253(data0, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate253.errors : vErrors.concat(validate253.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
}
validate634.errors = vErrors;
return errors === 0;
}

const schema159 = {"properties":{"certificateChainPath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"credentialBundlePath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"keyPath":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"keyType":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"maxExpirationSeconds":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"signerName":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"userAnnotations":{"$ref":"HJJzIHbiJa8GGL2u2CTOVy-Hry2MGWIvSIk30RN75wI"}},"required":["keyType","signerName"],"type":"object","nullable":true};

function validate637(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/certificateChainPath",parentData:data,parentDataProperty:"certificateChainPath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.credentialBundlePath !== undefined){
let data1 = data.credentialBundlePath;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/credentialBundlePath",parentData:data,parentDataProperty:"credentialBundlePath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.keyPath !== undefined){
let data2 = data.keyPath;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/keyPath",parentData:data,parentDataProperty:"keyPath",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.keyType !== undefined){
let data3 = data.keyType;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/keyType",parentData:data,parentDataProperty:"keyType",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.maxExpirationSeconds !== undefined){
let data4 = data.maxExpirationSeconds;
const _errs6 = errors;
if(!(validate78(data4, {instancePath:instancePath+"/maxExpirationSeconds",parentData:data,parentDataProperty:"maxExpirationSeconds",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.signerName !== undefined){
let data5 = data.signerName;
const _errs7 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/signerName",parentData:data,parentDataProperty:"signerName",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.userAnnotations !== undefined){
let data6 = data.userAnnotations;
const _errs8 = errors;
if(!(validate7(data6, {instancePath:instancePath+"/userAnnotations",parentData:data,parentDataProperty:"userAnnotations",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
}
validate637.errors = vErrors;
return errors === 0;
}

const schema160 = {"properties":{"audience":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"expirationSeconds":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"path":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["path"],"type":"object","nullable":true};

function validate647(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/audience",parentData:data,parentDataProperty:"audience",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.expirationSeconds !== undefined){
let data1 = data.expirationSeconds;
const _errs3 = errors;
if(!(validate79(data1, {instancePath:instancePath+"/expirationSeconds",parentData:data,parentDataProperty:"expirationSeconds",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
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
}
validate647.errors = vErrors;
return errors === 0;
}


function validate625(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
let vErrors = null;
let errors = 0;
const _errs0 = errors;
if(data && typeof data == "object" && !Array.isArray(data)){
if(data.clusterTrustBundle !== undefined){
let data0 = data.clusterTrustBundle;
const _errs1 = errors;
if(!(validate626(data0, {instancePath:instancePath+"/clusterTrustBundle",parentData:data,parentDataProperty:"clusterTrustBundle",rootData}))){
vErrors = vErrors === null ? validate626.errors : vErrors.concat(validate626.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.configMap !== undefined){
let data1 = data.configMap;
const _errs2 = errors;
if(!(validate260(data1, {instancePath:instancePath+"/configMap",parentData:data,parentDataProperty:"configMap",rootData}))){
vErrors = vErrors === null ? validate260.errors : vErrors.concat(validate260.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.downwardAPI !== undefined){
let data2 = data.downwardAPI;
const _errs3 = errors;
if(!(validate634(data2, {instancePath:instancePath+"/downwardAPI",parentData:data,parentDataProperty:"downwardAPI",rootData}))){
vErrors = vErrors === null ? validate634.errors : vErrors.concat(validate634.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.podCertificate !== undefined){
let data3 = data.podCertificate;
const _errs4 = errors;
if(!(validate637(data3, {instancePath:instancePath+"/podCertificate",parentData:data,parentDataProperty:"podCertificate",rootData}))){
vErrors = vErrors === null ? validate637.errors : vErrors.concat(validate637.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.secret !== undefined){
let data4 = data.secret;
const _errs5 = errors;
if(!(validate260(data4, {instancePath:instancePath+"/secret",parentData:data,parentDataProperty:"secret",rootData}))){
vErrors = vErrors === null ? validate260.errors : vErrors.concat(validate260.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.serviceAccountToken !== undefined){
let data5 = data.serviceAccountToken;
const _errs6 = errors;
if(!(validate647(data5, {instancePath:instancePath+"/serviceAccountToken",parentData:data,parentDataProperty:"serviceAccountToken",rootData}))){
vErrors = vErrors === null ? validate647.errors : vErrors.concat(validate647.errors);
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
validate625.errors = vErrors;
return errors === 0;
}


function validate624(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate625(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate625.errors : vErrors.concat(validate625.errors);
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
validate624.errors = vErrors;
return errors === 0;
}


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
if(data.defaultMode !== undefined){
let data0 = data.defaultMode;
const _errs2 = errors;
if(!(validate78(data0, {instancePath:instancePath+"/defaultMode",parentData:data,parentDataProperty:"defaultMode",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.sources !== undefined){
let data1 = data.sources;
const _errs3 = errors;
if(!(validate624(data1, {instancePath:instancePath+"/sources",parentData:data,parentDataProperty:"sources",rootData}))){
vErrors = vErrors === null ? validate624.errors : vErrors.concat(validate624.errors);
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

const schema161 = {"properties":{"group":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"registry":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"tenant":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"user":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volume":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["registry","volume"],"type":"object","nullable":true};

function validate655(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/group",parentData:data,parentDataProperty:"group",rootData}))){
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
if(!(validate41(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.registry !== undefined){
let data2 = data.registry;
const _errs4 = errors;
if(!(validate0(data2, {instancePath:instancePath+"/registry",parentData:data,parentDataProperty:"registry",rootData}))){
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
if(!(validate1(data3, {instancePath:instancePath+"/tenant",parentData:data,parentDataProperty:"tenant",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.user !== undefined){
let data4 = data.user;
const _errs6 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/user",parentData:data,parentDataProperty:"user",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.volume !== undefined){
let data5 = data.volume;
const _errs7 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/volume",parentData:data,parentDataProperty:"volume",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
}
validate655.errors = vErrors;
return errors === 0;
}

const schema162 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"image":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"keyring":{"$ref":"qJqT86B0L2JUpaRmtZDpYjFJgt4rjgnvcEUS3zGb1eI"},"monitors":{"$ref":"uqUtVyG9D2UAWDvqosEgVMtwK5kI3TMy2eLbDZB452I"},"pool":{"$ref":"Dh86ML4rFjEcx1-u_8ZgfsmYcv8nNBqGgZwoQ3ylFNU"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"user":{"$ref":"LvEyp1RPo8AnrMLndf_Vv4zih3rUP2kwyU22RiXstYI"}},"required":["image","monitors"],"type":"object","nullable":true};
const schema163 = {"default":"/etc/ceph/keyring","type":"string","nullable":true};

function validate666(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate666.errors = vErrors;
return errors === 0;
}

const schema164 = {"default":"rbd","type":"string","nullable":true};

function validate669(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate669.errors = vErrors;
return errors === 0;
}

const schema165 = {"default":"admin","type":"string","nullable":true};

function validate673(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate673.errors = vErrors;
return errors === 0;
}


function validate663(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.image !== undefined){
let data1 = data.image;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.keyring !== undefined){
let data2 = data.keyring;
const _errs4 = errors;
if(!(validate666(data2, {instancePath:instancePath+"/keyring",parentData:data,parentDataProperty:"keyring",rootData}))){
vErrors = vErrors === null ? validate666.errors : vErrors.concat(validate666.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.monitors !== undefined){
let data3 = data.monitors;
const _errs5 = errors;
if(!(validate237(data3, {instancePath:instancePath+"/monitors",parentData:data,parentDataProperty:"monitors",rootData}))){
vErrors = vErrors === null ? validate237.errors : vErrors.concat(validate237.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.pool !== undefined){
let data4 = data.pool;
const _errs6 = errors;
if(!(validate669(data4, {instancePath:instancePath+"/pool",parentData:data,parentDataProperty:"pool",rootData}))){
vErrors = vErrors === null ? validate669.errors : vErrors.concat(validate669.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.readOnly !== undefined){
let data5 = data.readOnly;
const _errs7 = errors;
if(!(validate41(data5, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.secretRef !== undefined){
let data6 = data.secretRef;
const _errs8 = errors;
if(!(validate239(data6, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate239.errors : vErrors.concat(validate239.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.user !== undefined){
let data7 = data.user;
const _errs9 = errors;
if(!(validate673(data7, {instancePath:instancePath+"/user",parentData:data,parentDataProperty:"user",rootData}))){
vErrors = vErrors === null ? validate673.errors : vErrors.concat(validate673.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
}
validate663.errors = vErrors;
return errors === 0;
}

const schema166 = {"properties":{"fsType":{"$ref":"Wgdy0s1CznF5U0Xdixh3dfX6xVZC-f4ZSRagme8xEZA"},"gateway":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"protectionDomain":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"CeclzNr20k5vr0FY8_Di-i5kv_hKd8rGtt9CG90tDrI"},"sslEnabled":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"storageMode":{"$ref":"nMIgymwC0Nh7u9Ub8ryXfL13c9EAIAX1eZzkO47rq5o"},"storagePool":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"system":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"volumeName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"required":["gateway","secretRef","system"],"type":"object","nullable":true};
const schema167 = {"default":"xfs","type":"string","nullable":true};

function validate677(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate677.errors = vErrors;
return errors === 0;
}

const schema168 = {"default":"ThinProvisioned","type":"string","nullable":true};

function validate684(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate684.errors = vErrors;
return errors === 0;
}


function validate676(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate677(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate677.errors : vErrors.concat(validate677.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.gateway !== undefined){
let data1 = data.gateway;
const _errs3 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/gateway",parentData:data,parentDataProperty:"gateway",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.protectionDomain !== undefined){
let data2 = data.protectionDomain;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/protectionDomain",parentData:data,parentDataProperty:"protectionDomain",rootData}))){
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
if(!(validate41(data3, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.secretRef !== undefined){
let data4 = data.secretRef;
const _errs6 = errors;
if(!(validate264(data4, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate264.errors : vErrors.concat(validate264.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.sslEnabled !== undefined){
let data5 = data.sslEnabled;
const _errs7 = errors;
if(!(validate41(data5, {instancePath:instancePath+"/sslEnabled",parentData:data,parentDataProperty:"sslEnabled",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.storageMode !== undefined){
let data6 = data.storageMode;
const _errs8 = errors;
if(!(validate684(data6, {instancePath:instancePath+"/storageMode",parentData:data,parentDataProperty:"storageMode",rootData}))){
vErrors = vErrors === null ? validate684.errors : vErrors.concat(validate684.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.storagePool !== undefined){
let data7 = data.storagePool;
const _errs9 = errors;
if(!(validate1(data7, {instancePath:instancePath+"/storagePool",parentData:data,parentDataProperty:"storagePool",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.system !== undefined){
let data8 = data.system;
const _errs10 = errors;
if(!(validate0(data8, {instancePath:instancePath+"/system",parentData:data,parentDataProperty:"system",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.volumeName !== undefined){
let data9 = data.volumeName;
const _errs11 = errors;
if(!(validate1(data9, {instancePath:instancePath+"/volumeName",parentData:data,parentDataProperty:"volumeName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
}
validate676.errors = vErrors;
return errors === 0;
}

const schema169 = {"properties":{"defaultMode":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"items":{"$ref":"gPtCVyFH2eXv-J44JI3istIuhC40N-NdI4mRYcFGaCI"},"optional":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate690(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate78(data0, {instancePath:instancePath+"/defaultMode",parentData:data,parentDataProperty:"defaultMode",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.items !== undefined){
let data1 = data.items;
const _errs3 = errors;
if(!(validate247(data1, {instancePath:instancePath+"/items",parentData:data,parentDataProperty:"items",rootData}))){
vErrors = vErrors === null ? validate247.errors : vErrors.concat(validate247.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.optional !== undefined){
let data2 = data.optional;
const _errs4 = errors;
if(!(validate41(data2, {instancePath:instancePath+"/optional",parentData:data,parentDataProperty:"optional",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.secretName !== undefined){
let data3 = data.secretName;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/secretName",parentData:data,parentDataProperty:"secretName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate690.errors = vErrors;
return errors === 0;
}

const schema170 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"readOnly":{"$ref":"CTX4WtafS9K1Gv7k2X1H0J3M9LziUVVRBMOR0rDDzTk"},"secretRef":{"$ref":"HGGQOMQgT1vNp8FYFYYzJJVb9JUwnCv6j6jyprlvEho"},"volumeName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumeNamespace":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};

function validate696(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
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
if(!(validate41(data1, {instancePath:instancePath+"/readOnly",parentData:data,parentDataProperty:"readOnly",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.secretRef !== undefined){
let data2 = data.secretRef;
const _errs4 = errors;
if(!(validate239(data2, {instancePath:instancePath+"/secretRef",parentData:data,parentDataProperty:"secretRef",rootData}))){
vErrors = vErrors === null ? validate239.errors : vErrors.concat(validate239.errors);
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
if(data.volumeNamespace !== undefined){
let data4 = data.volumeNamespace;
const _errs6 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/volumeNamespace",parentData:data,parentDataProperty:"volumeNamespace",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
}
validate696.errors = vErrors;
return errors === 0;
}

const schema171 = {"properties":{"fsType":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"storagePolicyID":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"storagePolicyName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"volumePath":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["volumePath"],"type":"object","nullable":true};

function validate703(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate1(data0, {instancePath:instancePath+"/fsType",parentData:data,parentDataProperty:"fsType",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.storagePolicyID !== undefined){
let data1 = data.storagePolicyID;
const _errs3 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/storagePolicyID",parentData:data,parentDataProperty:"storagePolicyID",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.storagePolicyName !== undefined){
let data2 = data.storagePolicyName;
const _errs4 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/storagePolicyName",parentData:data,parentDataProperty:"storagePolicyName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.volumePath !== undefined){
let data3 = data.volumePath;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/volumePath",parentData:data,parentDataProperty:"volumePath",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
}
validate703.errors = vErrors;
return errors === 0;
}


function validate463(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate464(data0, {instancePath:instancePath+"/awsElasticBlockStore",parentData:data,parentDataProperty:"awsElasticBlockStore",rootData}))){
vErrors = vErrors === null ? validate464.errors : vErrors.concat(validate464.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.azureDisk !== undefined){
let data1 = data.azureDisk;
const _errs2 = errors;
if(!(validate470(data1, {instancePath:instancePath+"/azureDisk",parentData:data,parentDataProperty:"azureDisk",rootData}))){
vErrors = vErrors === null ? validate470.errors : vErrors.concat(validate470.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.azureFile !== undefined){
let data2 = data.azureFile;
const _errs3 = errors;
if(!(validate479(data2, {instancePath:instancePath+"/azureFile",parentData:data,parentDataProperty:"azureFile",rootData}))){
vErrors = vErrors === null ? validate479.errors : vErrors.concat(validate479.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.cephfs !== undefined){
let data3 = data.cephfs;
const _errs4 = errors;
if(!(validate484(data3, {instancePath:instancePath+"/cephfs",parentData:data,parentDataProperty:"cephfs",rootData}))){
vErrors = vErrors === null ? validate484.errors : vErrors.concat(validate484.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.cinder !== undefined){
let data4 = data.cinder;
const _errs5 = errors;
if(!(validate492(data4, {instancePath:instancePath+"/cinder",parentData:data,parentDataProperty:"cinder",rootData}))){
vErrors = vErrors === null ? validate492.errors : vErrors.concat(validate492.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.configMap !== undefined){
let data5 = data.configMap;
const _errs6 = errors;
if(!(validate498(data5, {instancePath:instancePath+"/configMap",parentData:data,parentDataProperty:"configMap",rootData}))){
vErrors = vErrors === null ? validate498.errors : vErrors.concat(validate498.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.csi !== undefined){
let data6 = data.csi;
const _errs7 = errors;
if(!(validate504(data6, {instancePath:instancePath+"/csi",parentData:data,parentDataProperty:"csi",rootData}))){
vErrors = vErrors === null ? validate504.errors : vErrors.concat(validate504.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.downwardAPI !== undefined){
let data7 = data.downwardAPI;
const _errs8 = errors;
if(!(validate511(data7, {instancePath:instancePath+"/downwardAPI",parentData:data,parentDataProperty:"downwardAPI",rootData}))){
vErrors = vErrors === null ? validate511.errors : vErrors.concat(validate511.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.emptyDir !== undefined){
let data8 = data.emptyDir;
const _errs9 = errors;
if(!(validate515(data8, {instancePath:instancePath+"/emptyDir",parentData:data,parentDataProperty:"emptyDir",rootData}))){
vErrors = vErrors === null ? validate515.errors : vErrors.concat(validate515.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.ephemeral !== undefined){
let data9 = data.ephemeral;
const _errs10 = errors;
if(!(validate519(data9, {instancePath:instancePath+"/ephemeral",parentData:data,parentDataProperty:"ephemeral",rootData}))){
vErrors = vErrors === null ? validate519.errors : vErrors.concat(validate519.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.fc !== undefined){
let data10 = data.fc;
const _errs11 = errors;
if(!(validate547(data10, {instancePath:instancePath+"/fc",parentData:data,parentDataProperty:"fc",rootData}))){
vErrors = vErrors === null ? validate547.errors : vErrors.concat(validate547.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.flexVolume !== undefined){
let data11 = data.flexVolume;
const _errs12 = errors;
if(!(validate554(data11, {instancePath:instancePath+"/flexVolume",parentData:data,parentDataProperty:"flexVolume",rootData}))){
vErrors = vErrors === null ? validate554.errors : vErrors.concat(validate554.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.flocker !== undefined){
let data12 = data.flocker;
const _errs13 = errors;
if(!(validate561(data12, {instancePath:instancePath+"/flocker",parentData:data,parentDataProperty:"flocker",rootData}))){
vErrors = vErrors === null ? validate561.errors : vErrors.concat(validate561.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.gcePersistentDisk !== undefined){
let data13 = data.gcePersistentDisk;
const _errs14 = errors;
if(!(validate565(data13, {instancePath:instancePath+"/gcePersistentDisk",parentData:data,parentDataProperty:"gcePersistentDisk",rootData}))){
vErrors = vErrors === null ? validate565.errors : vErrors.concat(validate565.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
if(data.gitRepo !== undefined){
let data14 = data.gitRepo;
const _errs15 = errors;
if(!(validate571(data14, {instancePath:instancePath+"/gitRepo",parentData:data,parentDataProperty:"gitRepo",rootData}))){
vErrors = vErrors === null ? validate571.errors : vErrors.concat(validate571.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs15 === errors;
}
if(data.glusterfs !== undefined){
let data15 = data.glusterfs;
const _errs16 = errors;
if(!(validate576(data15, {instancePath:instancePath+"/glusterfs",parentData:data,parentDataProperty:"glusterfs",rootData}))){
vErrors = vErrors === null ? validate576.errors : vErrors.concat(validate576.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs16 === errors;
}
if(data.hostPath !== undefined){
let data16 = data.hostPath;
const _errs17 = errors;
if(!(validate581(data16, {instancePath:instancePath+"/hostPath",parentData:data,parentDataProperty:"hostPath",rootData}))){
vErrors = vErrors === null ? validate581.errors : vErrors.concat(validate581.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs17 === errors;
}
if(data.image !== undefined){
let data17 = data.image;
const _errs18 = errors;
if(!(validate585(data17, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate585.errors : vErrors.concat(validate585.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs18 === errors;
}
if(data.iscsi !== undefined){
let data18 = data.iscsi;
const _errs19 = errors;
if(!(validate589(data18, {instancePath:instancePath+"/iscsi",parentData:data,parentDataProperty:"iscsi",rootData}))){
vErrors = vErrors === null ? validate589.errors : vErrors.concat(validate589.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs19 === errors;
}
if(data.name !== undefined){
let data19 = data.name;
const _errs20 = errors;
if(!(validate0(data19, {instancePath:instancePath+"/name",parentData:data,parentDataProperty:"name",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs20 === errors;
}
if(data.nfs !== undefined){
let data20 = data.nfs;
const _errs21 = errors;
if(!(validate604(data20, {instancePath:instancePath+"/nfs",parentData:data,parentDataProperty:"nfs",rootData}))){
vErrors = vErrors === null ? validate604.errors : vErrors.concat(validate604.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs21 === errors;
}
if(data.persistentVolumeClaim !== undefined){
let data21 = data.persistentVolumeClaim;
const _errs22 = errors;
if(!(validate609(data21, {instancePath:instancePath+"/persistentVolumeClaim",parentData:data,parentDataProperty:"persistentVolumeClaim",rootData}))){
vErrors = vErrors === null ? validate609.errors : vErrors.concat(validate609.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs22 === errors;
}
if(data.photonPersistentDisk !== undefined){
let data22 = data.photonPersistentDisk;
const _errs23 = errors;
if(!(validate613(data22, {instancePath:instancePath+"/photonPersistentDisk",parentData:data,parentDataProperty:"photonPersistentDisk",rootData}))){
vErrors = vErrors === null ? validate613.errors : vErrors.concat(validate613.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs23 === errors;
}
if(data.portworxVolume !== undefined){
let data23 = data.portworxVolume;
const _errs24 = errors;
if(!(validate617(data23, {instancePath:instancePath+"/portworxVolume",parentData:data,parentDataProperty:"portworxVolume",rootData}))){
vErrors = vErrors === null ? validate617.errors : vErrors.concat(validate617.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs24 === errors;
}
if(data.projected !== undefined){
let data24 = data.projected;
const _errs25 = errors;
if(!(validate622(data24, {instancePath:instancePath+"/projected",parentData:data,parentDataProperty:"projected",rootData}))){
vErrors = vErrors === null ? validate622.errors : vErrors.concat(validate622.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs25 === errors;
}
if(data.quobyte !== undefined){
let data25 = data.quobyte;
const _errs26 = errors;
if(!(validate655(data25, {instancePath:instancePath+"/quobyte",parentData:data,parentDataProperty:"quobyte",rootData}))){
vErrors = vErrors === null ? validate655.errors : vErrors.concat(validate655.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs26 === errors;
}
if(data.rbd !== undefined){
let data26 = data.rbd;
const _errs27 = errors;
if(!(validate663(data26, {instancePath:instancePath+"/rbd",parentData:data,parentDataProperty:"rbd",rootData}))){
vErrors = vErrors === null ? validate663.errors : vErrors.concat(validate663.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs27 === errors;
}
if(data.scaleIO !== undefined){
let data27 = data.scaleIO;
const _errs28 = errors;
if(!(validate676(data27, {instancePath:instancePath+"/scaleIO",parentData:data,parentDataProperty:"scaleIO",rootData}))){
vErrors = vErrors === null ? validate676.errors : vErrors.concat(validate676.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs28 === errors;
}
if(data.secret !== undefined){
let data28 = data.secret;
const _errs29 = errors;
if(!(validate690(data28, {instancePath:instancePath+"/secret",parentData:data,parentDataProperty:"secret",rootData}))){
vErrors = vErrors === null ? validate690.errors : vErrors.concat(validate690.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs29 === errors;
}
if(data.storageos !== undefined){
let data29 = data.storageos;
const _errs30 = errors;
if(!(validate696(data29, {instancePath:instancePath+"/storageos",parentData:data,parentDataProperty:"storageos",rootData}))){
vErrors = vErrors === null ? validate696.errors : vErrors.concat(validate696.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs30 === errors;
}
if(data.vsphereVolume !== undefined){
let data30 = data.vsphereVolume;
const _errs31 = errors;
if(!(validate703(data30, {instancePath:instancePath+"/vsphereVolume",parentData:data,parentDataProperty:"vsphereVolume",rootData}))){
vErrors = vErrors === null ? validate703.errors : vErrors.concat(validate703.errors);
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
validate463.errors = vErrors;
return errors === 0;
}


function validate462(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate463(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate463.errors : vErrors.concat(validate463.errors);
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
validate462.errors = vErrors;
return errors === 0;
}


function validate302(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate79(data0, {instancePath:instancePath+"/activeDeadlineSeconds",parentData:data,parentDataProperty:"activeDeadlineSeconds",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.affinity !== undefined){
let data1 = data.affinity;
const _errs3 = errors;
if(!(validate304(data1, {instancePath:instancePath+"/affinity",parentData:data,parentDataProperty:"affinity",rootData}))){
vErrors = vErrors === null ? validate304.errors : vErrors.concat(validate304.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.automountServiceAccountToken !== undefined){
let data2 = data.automountServiceAccountToken;
const _errs4 = errors;
if(!(validate41(data2, {instancePath:instancePath+"/automountServiceAccountToken",parentData:data,parentDataProperty:"automountServiceAccountToken",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.containers !== undefined){
let data3 = data.containers;
const _errs5 = errors;
if(!(validate322(data3, {instancePath:instancePath+"/containers",parentData:data,parentDataProperty:"containers",rootData}))){
vErrors = vErrors === null ? validate322.errors : vErrors.concat(validate322.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.dnsConfig !== undefined){
let data4 = data.dnsConfig;
const _errs6 = errors;
if(!(validate325(data4, {instancePath:instancePath+"/dnsConfig",parentData:data,parentDataProperty:"dnsConfig",rootData}))){
vErrors = vErrors === null ? validate325.errors : vErrors.concat(validate325.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.dnsPolicy !== undefined){
let data5 = data.dnsPolicy;
const _errs7 = errors;
if(!(validate1(data5, {instancePath:instancePath+"/dnsPolicy",parentData:data,parentDataProperty:"dnsPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.enableServiceLinks !== undefined){
let data6 = data.enableServiceLinks;
const _errs8 = errors;
if(!(validate41(data6, {instancePath:instancePath+"/enableServiceLinks",parentData:data,parentDataProperty:"enableServiceLinks",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.ephemeralContainers !== undefined){
let data7 = data.ephemeralContainers;
const _errs9 = errors;
if(!(validate337(data7, {instancePath:instancePath+"/ephemeralContainers",parentData:data,parentDataProperty:"ephemeralContainers",rootData}))){
vErrors = vErrors === null ? validate337.errors : vErrors.concat(validate337.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.hostAliases !== undefined){
let data8 = data.hostAliases;
const _errs10 = errors;
if(!(validate367(data8, {instancePath:instancePath+"/hostAliases",parentData:data,parentDataProperty:"hostAliases",rootData}))){
vErrors = vErrors === null ? validate367.errors : vErrors.concat(validate367.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.hostIPC !== undefined){
let data9 = data.hostIPC;
const _errs11 = errors;
if(!(validate41(data9, {instancePath:instancePath+"/hostIPC",parentData:data,parentDataProperty:"hostIPC",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.hostNetwork !== undefined){
let data10 = data.hostNetwork;
const _errs12 = errors;
if(!(validate41(data10, {instancePath:instancePath+"/hostNetwork",parentData:data,parentDataProperty:"hostNetwork",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
if(data.hostPID !== undefined){
let data11 = data.hostPID;
const _errs13 = errors;
if(!(validate41(data11, {instancePath:instancePath+"/hostPID",parentData:data,parentDataProperty:"hostPID",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs13 === errors;
}
if(data.hostUsers !== undefined){
let data12 = data.hostUsers;
const _errs14 = errors;
if(!(validate41(data12, {instancePath:instancePath+"/hostUsers",parentData:data,parentDataProperty:"hostUsers",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs14 === errors;
}
if(data.hostname !== undefined){
let data13 = data.hostname;
const _errs15 = errors;
if(!(validate1(data13, {instancePath:instancePath+"/hostname",parentData:data,parentDataProperty:"hostname",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs15 === errors;
}
if(data.hostnameOverride !== undefined){
let data14 = data.hostnameOverride;
const _errs16 = errors;
if(!(validate1(data14, {instancePath:instancePath+"/hostnameOverride",parentData:data,parentDataProperty:"hostnameOverride",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs16 === errors;
}
if(data.imagePullSecrets !== undefined){
let data15 = data.imagePullSecrets;
const _errs17 = errors;
if(!(validate379(data15, {instancePath:instancePath+"/imagePullSecrets",parentData:data,parentDataProperty:"imagePullSecrets",rootData}))){
vErrors = vErrors === null ? validate379.errors : vErrors.concat(validate379.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs17 === errors;
}
if(data.initContainers !== undefined){
let data16 = data.initContainers;
const _errs18 = errors;
if(!(validate382(data16, {instancePath:instancePath+"/initContainers",parentData:data,parentDataProperty:"initContainers",rootData}))){
vErrors = vErrors === null ? validate382.errors : vErrors.concat(validate382.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs18 === errors;
}
if(data.nodeName !== undefined){
let data17 = data.nodeName;
const _errs19 = errors;
if(!(validate1(data17, {instancePath:instancePath+"/nodeName",parentData:data,parentDataProperty:"nodeName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs19 === errors;
}
if(data.nodeSelector !== undefined){
let data18 = data.nodeSelector;
const _errs20 = errors;
if(!(validate7(data18, {instancePath:instancePath+"/nodeSelector",parentData:data,parentDataProperty:"nodeSelector",rootData}))){
vErrors = vErrors === null ? validate7.errors : vErrors.concat(validate7.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs20 === errors;
}
if(data.os !== undefined){
let data19 = data.os;
const _errs21 = errors;
if(!(validate387(data19, {instancePath:instancePath+"/os",parentData:data,parentDataProperty:"os",rootData}))){
vErrors = vErrors === null ? validate387.errors : vErrors.concat(validate387.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs21 === errors;
}
if(data.overhead !== undefined){
let data20 = data.overhead;
const _errs22 = errors;
if(!(validate97(data20, {instancePath:instancePath+"/overhead",parentData:data,parentDataProperty:"overhead",rootData}))){
vErrors = vErrors === null ? validate97.errors : vErrors.concat(validate97.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs22 === errors;
}
if(data.preemptionPolicy !== undefined){
let data21 = data.preemptionPolicy;
const _errs23 = errors;
if(!(validate1(data21, {instancePath:instancePath+"/preemptionPolicy",parentData:data,parentDataProperty:"preemptionPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs23 === errors;
}
if(data.priority !== undefined){
let data22 = data.priority;
const _errs24 = errors;
if(!(validate78(data22, {instancePath:instancePath+"/priority",parentData:data,parentDataProperty:"priority",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs24 === errors;
}
if(data.priorityClassName !== undefined){
let data23 = data.priorityClassName;
const _errs25 = errors;
if(!(validate1(data23, {instancePath:instancePath+"/priorityClassName",parentData:data,parentDataProperty:"priorityClassName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs25 === errors;
}
if(data.readinessGates !== undefined){
let data24 = data.readinessGates;
const _errs26 = errors;
if(!(validate394(data24, {instancePath:instancePath+"/readinessGates",parentData:data,parentDataProperty:"readinessGates",rootData}))){
vErrors = vErrors === null ? validate394.errors : vErrors.concat(validate394.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs26 === errors;
}
if(data.resourceClaims !== undefined){
let data25 = data.resourceClaims;
const _errs27 = errors;
if(!(validate399(data25, {instancePath:instancePath+"/resourceClaims",parentData:data,parentDataProperty:"resourceClaims",rootData}))){
vErrors = vErrors === null ? validate399.errors : vErrors.concat(validate399.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs27 === errors;
}
if(data.resources !== undefined){
let data26 = data.resources;
const _errs28 = errors;
if(!(validate151(data26, {instancePath:instancePath+"/resources",parentData:data,parentDataProperty:"resources",rootData}))){
vErrors = vErrors === null ? validate151.errors : vErrors.concat(validate151.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs28 === errors;
}
if(data.restartPolicy !== undefined){
let data27 = data.restartPolicy;
const _errs29 = errors;
if(!(validate1(data27, {instancePath:instancePath+"/restartPolicy",parentData:data,parentDataProperty:"restartPolicy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs29 === errors;
}
if(data.runtimeClassName !== undefined){
let data28 = data.runtimeClassName;
const _errs30 = errors;
if(!(validate1(data28, {instancePath:instancePath+"/runtimeClassName",parentData:data,parentDataProperty:"runtimeClassName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs30 === errors;
}
if(data.schedulerName !== undefined){
let data29 = data.schedulerName;
const _errs31 = errors;
if(!(validate1(data29, {instancePath:instancePath+"/schedulerName",parentData:data,parentDataProperty:"schedulerName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs31 === errors;
}
if(data.schedulingGates !== undefined){
let data30 = data.schedulingGates;
const _errs32 = errors;
if(!(validate410(data30, {instancePath:instancePath+"/schedulingGates",parentData:data,parentDataProperty:"schedulingGates",rootData}))){
vErrors = vErrors === null ? validate410.errors : vErrors.concat(validate410.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs32 === errors;
}
if(data.schedulingGroup !== undefined){
let data31 = data.schedulingGroup;
const _errs33 = errors;
if(!(validate415(data31, {instancePath:instancePath+"/schedulingGroup",parentData:data,parentDataProperty:"schedulingGroup",rootData}))){
vErrors = vErrors === null ? validate415.errors : vErrors.concat(validate415.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs33 === errors;
}
if(data.securityContext !== undefined){
let data32 = data.securityContext;
const _errs34 = errors;
if(!(validate418(data32, {instancePath:instancePath+"/securityContext",parentData:data,parentDataProperty:"securityContext",rootData}))){
vErrors = vErrors === null ? validate418.errors : vErrors.concat(validate418.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs34 === errors;
}
if(data.serviceAccount !== undefined){
let data33 = data.serviceAccount;
const _errs35 = errors;
if(!(validate1(data33, {instancePath:instancePath+"/serviceAccount",parentData:data,parentDataProperty:"serviceAccount",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs35 === errors;
}
if(data.serviceAccountName !== undefined){
let data34 = data.serviceAccountName;
const _errs36 = errors;
if(!(validate1(data34, {instancePath:instancePath+"/serviceAccountName",parentData:data,parentDataProperty:"serviceAccountName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs36 === errors;
}
if(data.setHostnameAsFQDN !== undefined){
let data35 = data.setHostnameAsFQDN;
const _errs37 = errors;
if(!(validate41(data35, {instancePath:instancePath+"/setHostnameAsFQDN",parentData:data,parentDataProperty:"setHostnameAsFQDN",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs37 === errors;
}
if(data.shareProcessNamespace !== undefined){
let data36 = data.shareProcessNamespace;
const _errs38 = errors;
if(!(validate41(data36, {instancePath:instancePath+"/shareProcessNamespace",parentData:data,parentDataProperty:"shareProcessNamespace",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs38 === errors;
}
if(data.subdomain !== undefined){
let data37 = data.subdomain;
const _errs39 = errors;
if(!(validate1(data37, {instancePath:instancePath+"/subdomain",parentData:data,parentDataProperty:"subdomain",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs39 === errors;
}
if(data.terminationGracePeriodSeconds !== undefined){
let data38 = data.terminationGracePeriodSeconds;
const _errs40 = errors;
if(!(validate79(data38, {instancePath:instancePath+"/terminationGracePeriodSeconds",parentData:data,parentDataProperty:"terminationGracePeriodSeconds",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs40 === errors;
}
if(data.tolerations !== undefined){
let data39 = data.tolerations;
const _errs41 = errors;
if(!(validate441(data39, {instancePath:instancePath+"/tolerations",parentData:data,parentDataProperty:"tolerations",rootData}))){
vErrors = vErrors === null ? validate441.errors : vErrors.concat(validate441.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs41 === errors;
}
if(data.topologySpreadConstraints !== undefined){
let data40 = data.topologySpreadConstraints;
const _errs42 = errors;
if(!(validate450(data40, {instancePath:instancePath+"/topologySpreadConstraints",parentData:data,parentDataProperty:"topologySpreadConstraints",rootData}))){
vErrors = vErrors === null ? validate450.errors : vErrors.concat(validate450.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs42 === errors;
}
if(data.volumes !== undefined){
let data41 = data.volumes;
const _errs43 = errors;
if(!(validate462(data41, {instancePath:instancePath+"/volumes",parentData:data,parentDataProperty:"volumes",rootData}))){
vErrors = vErrors === null ? validate462.errors : vErrors.concat(validate462.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs43 === errors;
}
}
validate302.errors = vErrors;
return errors === 0;
}


function validate300(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate241(data0, {instancePath:instancePath+"/metadata",parentData:data,parentDataProperty:"metadata",rootData}))){
vErrors = vErrors === null ? validate241.errors : vErrors.concat(validate241.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.spec !== undefined){
let data1 = data.spec;
const _errs3 = errors;
if(!(validate302(data1, {instancePath:instancePath+"/spec",parentData:data,parentDataProperty:"spec",rootData}))){
vErrors = vErrors === null ? validate302.errors : vErrors.concat(validate302.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
}
validate300.errors = vErrors;
return errors === 0;
}


function validate296(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate2(data0, {instancePath:instancePath+"/carrier",parentData:data,parentDataProperty:"carrier",rootData}))){
vErrors = vErrors === null ? validate2.errors : vErrors.concat(validate2.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.kind !== undefined){
let data1 = data.kind;
const _errs2 = errors;
if(!(validate1(data1, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.parameters !== undefined){
let data2 = data.parameters;
const _errs3 = errors;
if(!(validate3(data2, {instancePath:instancePath+"/parameters",parentData:data,parentDataProperty:"parameters",rootData}))){
vErrors = vErrors === null ? validate3.errors : vErrors.concat(validate3.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.template !== undefined){
let data3 = data.template;
const _errs4 = errors;
if(!(validate300(data3, {instancePath:instancePath+"/template",parentData:data,parentDataProperty:"template",rootData}))){
vErrors = vErrors === null ? validate300.errors : vErrors.concat(validate300.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.version !== undefined){
let data4 = data.version;
const _errs5 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/version",parentData:data,parentDataProperty:"version",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate296.errors = vErrors;
return errors === 0;
}


function validate276(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.placement === undefined){
const err1 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "placement"}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
if(data.role === undefined){
const err2 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "role"}};
if(vErrors === null){
vErrors = [err2];
}
else {
vErrors.push(err2);
}
errors++;
}
if(data.runtime === undefined){
const err3 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "runtime"}};
if(vErrors === null){
vErrors = [err3];
}
else {
vErrors.push(err3);
}
errors++;
}
if(data.workload === undefined){
const err4 = {instancePath,schemaPath:"#/required",keyword:"required",params:{missingProperty: "workload"}};
if(vErrors === null){
vErrors = [err4];
}
else {
vErrors.push(err4);
}
errors++;
}
if(data.downstreamName !== undefined){
let data0 = data.downstreamName;
const _errs2 = errors;
if(!(validate1(data0, {instancePath:instancePath+"/downstreamName",parentData:data,parentDataProperty:"downstreamName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.environments !== undefined){
let data1 = data.environments;
const _errs3 = errors;
if(!(validate278(data1, {instancePath:instancePath+"/environments",parentData:data,parentDataProperty:"environments",rootData}))){
vErrors = vErrors === null ? validate278.errors : vErrors.concat(validate278.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.placement !== undefined){
let data2 = data.placement;
const _errs4 = errors;
if(!(validate281(data2, {instancePath:instancePath+"/placement",parentData:data,parentDataProperty:"placement",rootData}))){
vErrors = vErrors === null ? validate281.errors : vErrors.concat(validate281.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.role !== undefined){
let data3 = data.role;
const _errs5 = errors;
if(!(validate0(data3, {instancePath:instancePath+"/role",parentData:data,parentDataProperty:"role",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.runtime !== undefined){
let data4 = data.runtime;
const _errs6 = errors;
if(!(validate285(data4, {instancePath:instancePath+"/runtime",parentData:data,parentDataProperty:"runtime",rootData}))){
vErrors = vErrors === null ? validate285.errors : vErrors.concat(validate285.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.suspended !== undefined){
let data5 = data.suspended;
const _errs7 = errors;
if(!(validate41(data5, {instancePath:instancePath+"/suspended",parentData:data,parentDataProperty:"suspended",rootData}))){
vErrors = vErrors === null ? validate41.errors : vErrors.concat(validate41.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.tags !== undefined){
let data6 = data.tags;
const _errs8 = errors;
if(!(validate290(data6, {instancePath:instancePath+"/tags",parentData:data,parentDataProperty:"tags",rootData}))){
vErrors = vErrors === null ? validate290.errors : vErrors.concat(validate290.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.workload !== undefined){
let data7 = data.workload;
const _errs9 = errors;
if(!(validate296(data7, {instancePath:instancePath+"/workload",parentData:data,parentDataProperty:"workload",rootData}))){
vErrors = vErrors === null ? validate296.errors : vErrors.concat(validate296.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
}
validate276.errors = vErrors;
return errors === 0;
}

const schema172 = {"properties":{"completionTime":{"$ref":"YL2AaRx5mZZL3drs48WsIqN7ZQe66kWzKXDNrUwOgoc"},"conditions":{"$ref":"hhAxL57cvApnLYfNWQbGuX0ymd4M4buVmG7H0q63I0w"},"events":{"$ref":"Ju-wMSQJ-nWv9bz8kP8PuWhur4odJBfn2n0aWeAvRzo"},"message":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"observedGeneration":{"$ref":"otlW-2SioSJxw2ZRa8bQrFi14CUkm_yS2tfdShK98Vk"},"observedNodes":{"$ref":"uqi-NWpwJAtCtKg340P2GU85hOwdccNKh8Eansj5IGE"},"phase":{"$ref":"OumCqY-lP6-t1qsO7hLYZf_ZC9lX1mEyjnwbPTz244M"},"pullProgress":{"$ref":"gDMAFrrisSGbwGplQDJI2kIWbFoO35CEb_0uFAdTsQ8"},"retryCount":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"startTime":{"$ref":"YL2AaRx5mZZL3drs48WsIqN7ZQe66kWzKXDNrUwOgoc"},"tensorBoardProxy":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"}},"type":"object","nullable":true};
const schema77 = {"format":"date-time","type":"string","nullable":true};
const formats8 = formats["date-time"];

function validate266(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate266.errors = vErrors;
return errors === 0;
}

const schema173 = {"items":{"$ref":"dZ13d79yfL0AfKGv8XJr0-7aalgpv7dwgkV5EU9HxwM"},"type":"array","nullable":true};
const schema174 = {"properties":{"lastTransitionTime":{"$ref":"nWP098h5Flk6EOMa_OWYW4EnhyCrisyvgJPgk0WtC9c"},"message":{"$ref":"CG805GD3KzsipQt5EKLOQw9SjU_4PVFnnbOjDh0LzJQ"},"observedGeneration":{"$ref":"Tv0gqme9PlVaT-l831KRjUe4u9reG1m_llVaPqPtqUs"},"reason":{"$ref":"UPBfG4e9zcqG6mx3tN1LWinVzYKOFfIO2VnMgg6lAWI"},"status":{"$ref":"UcCRuQZyUlftnmEwuGDZcciwLAdzzRj4HOLofWAwyW8"},"type":{"$ref":"grctw_GwW6iDVZUzowft1rMz55_d64qiJ_wOfyk2Qoo"}},"required":["lastTransitionTime","message","reason","status","type"],"type":"object"};
const schema175 = {"format":"date-time","type":"string"};

function validate720(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate720.errors = vErrors;
return errors === 0;
}

const schema176 = {"maxLength":32768,"type":"string"};
import func0 from "@kubernetes-models/validate/runtime/ucs2length";

function validate722(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate722.errors = vErrors;
return errors === 0;
}

const schema177 = {"format":"int64","type":"integer","minimum":0,"nullable":true};

function validate724(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate724.errors = vErrors;
return errors === 0;
}

const schema178 = {"maxLength":1024,"minLength":1,"pattern":"^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$","type":"string"};
const pattern1 = new RegExp("^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$", "u");

function validate726(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate726.errors = vErrors;
return errors === 0;
}

const schema179 = {"enum":["True","False","Unknown"],"type":"string"};

function validate728(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
const vSchema0 = schema179.enum;
if(!(((data === "True") || (data === "False")) || (data === "Unknown"))){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema179.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate728.errors = vErrors;
return errors === 0;
}

const schema180 = {"maxLength":316,"pattern":"^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$","type":"string"};
const pattern2 = new RegExp("^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$", "u");

function validate730(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate730.errors = vErrors;
return errors === 0;
}


function validate719(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate720(data0, {instancePath:instancePath+"/lastTransitionTime",parentData:data,parentDataProperty:"lastTransitionTime",rootData}))){
vErrors = vErrors === null ? validate720.errors : vErrors.concat(validate720.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.message !== undefined){
let data1 = data.message;
const _errs2 = errors;
if(!(validate722(data1, {instancePath:instancePath+"/message",parentData:data,parentDataProperty:"message",rootData}))){
vErrors = vErrors === null ? validate722.errors : vErrors.concat(validate722.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.observedGeneration !== undefined){
let data2 = data.observedGeneration;
const _errs3 = errors;
if(!(validate724(data2, {instancePath:instancePath+"/observedGeneration",parentData:data,parentDataProperty:"observedGeneration",rootData}))){
vErrors = vErrors === null ? validate724.errors : vErrors.concat(validate724.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.reason !== undefined){
let data3 = data.reason;
const _errs4 = errors;
if(!(validate726(data3, {instancePath:instancePath+"/reason",parentData:data,parentDataProperty:"reason",rootData}))){
vErrors = vErrors === null ? validate726.errors : vErrors.concat(validate726.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.status !== undefined){
let data4 = data.status;
const _errs5 = errors;
if(!(validate728(data4, {instancePath:instancePath+"/status",parentData:data,parentDataProperty:"status",rootData}))){
vErrors = vErrors === null ? validate728.errors : vErrors.concat(validate728.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.type !== undefined){
let data5 = data.type;
const _errs6 = errors;
if(!(validate730(data5, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate730.errors : vErrors.concat(validate730.errors);
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
validate719.errors = vErrors;
return errors === 0;
}


function validate718(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate719(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate719.errors : vErrors.concat(validate719.errors);
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
validate718.errors = vErrors;
return errors === 0;
}

const schema181 = {"items":{"$ref":"psMXK1PdmbXnnoGH_RHVnCc5FjNkMdWz1aoeVYVaq1s"},"type":"array","nullable":true};
const schema182 = {"properties":{"count":{"$ref":"j4SBnKFzNWrJQU4F8rp8qSB0IqehV1dh50NKzJhlTjs"},"lastTime":{"$ref":"YL2AaRx5mZZL3drs48WsIqN7ZQe66kWzKXDNrUwOgoc"},"message":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"objectKind":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"objectName":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"reason":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"source":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"type":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"}},"required":["reason","type"],"type":"object"};

function validate735(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate78(data0, {instancePath:instancePath+"/count",parentData:data,parentDataProperty:"count",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.lastTime !== undefined){
let data1 = data.lastTime;
const _errs2 = errors;
if(!(validate266(data1, {instancePath:instancePath+"/lastTime",parentData:data,parentDataProperty:"lastTime",rootData}))){
vErrors = vErrors === null ? validate266.errors : vErrors.concat(validate266.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.message !== undefined){
let data2 = data.message;
const _errs3 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/message",parentData:data,parentDataProperty:"message",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.objectKind !== undefined){
let data3 = data.objectKind;
const _errs4 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/objectKind",parentData:data,parentDataProperty:"objectKind",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.objectName !== undefined){
let data4 = data.objectName;
const _errs5 = errors;
if(!(validate1(data4, {instancePath:instancePath+"/objectName",parentData:data,parentDataProperty:"objectName",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.reason !== undefined){
let data5 = data.reason;
const _errs6 = errors;
if(!(validate0(data5, {instancePath:instancePath+"/reason",parentData:data,parentDataProperty:"reason",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.source !== undefined){
let data6 = data.source;
const _errs7 = errors;
if(!(validate1(data6, {instancePath:instancePath+"/source",parentData:data,parentDataProperty:"source",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.type !== undefined){
let data7 = data.type;
const _errs8 = errors;
if(!(validate0(data7, {instancePath:instancePath+"/type",parentData:data,parentDataProperty:"type",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
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
validate735.errors = vErrors;
return errors === 0;
}


function validate734(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate735(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate735.errors : vErrors.concat(validate735.errors);
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
validate734.errors = vErrors;
return errors === 0;
}

const schema183 = {"enum":["Pending","Running","Succeeded","Failed","Stopped"],"type":"string","nullable":true};

function validate749(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
const vSchema0 = schema183.enum;
if(!(((((data === "Pending") || (data === "Running")) || (data === "Succeeded")) || (data === "Failed")) || (data === "Stopped"))){
const err1 = {instancePath,schemaPath:"#/enum",keyword:"enum",params:{allowedValues: schema183.enum}};
if(vErrors === null){
vErrors = [err1];
}
else {
vErrors.push(err1);
}
errors++;
}
validate749.errors = vErrors;
return errors === 0;
}

const schema184 = {"items":{"$ref":"eXqsosMmVlqiYyBADb2y8LOSfkVbCL3AlfIBXQ87hmE"},"type":"array","nullable":true};
const schema185 = {"properties":{"downloaded":{"$ref":"0YQm7z5aN2fg7S8XlbxrquRU6pdIr97uCbtQYyjv3S4"},"image":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"message":{"$ref":"ldoCEsh0ARg8F2aeji6S38NFl7aLPSuz1hGlgX2tjuQ"},"speed":{"$ref":"0hFN2lc8EZNwdRPFUdVm0IS7gM8AwqK_YhN0yw_gZLU"},"status":{"$ref":"sLlNkWhANXZ15lvmcujppy1mXxqn6f-GaeQpWRYm_dw"},"total":{"$ref":"0YQm7z5aN2fg7S8XlbxrquRU6pdIr97uCbtQYyjv3S4"}},"required":["downloaded","image","speed","status","total"],"type":"object"};
const schema186 = {"type":"number"};

function validate756(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
validate756.errors = vErrors;
return errors === 0;
}


function validate752(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate71(data0, {instancePath:instancePath+"/downloaded",parentData:data,parentDataProperty:"downloaded",rootData}))){
vErrors = vErrors === null ? validate71.errors : vErrors.concat(validate71.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.image !== undefined){
let data1 = data.image;
const _errs2 = errors;
if(!(validate0(data1, {instancePath:instancePath+"/image",parentData:data,parentDataProperty:"image",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.message !== undefined){
let data2 = data.message;
const _errs3 = errors;
if(!(validate1(data2, {instancePath:instancePath+"/message",parentData:data,parentDataProperty:"message",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.speed !== undefined){
let data3 = data.speed;
const _errs4 = errors;
if(!(validate756(data3, {instancePath:instancePath+"/speed",parentData:data,parentDataProperty:"speed",rootData}))){
vErrors = vErrors === null ? validate756.errors : vErrors.concat(validate756.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.status !== undefined){
let data4 = data.status;
const _errs5 = errors;
if(!(validate0(data4, {instancePath:instancePath+"/status",parentData:data,parentDataProperty:"status",rootData}))){
vErrors = vErrors === null ? validate0.errors : vErrors.concat(validate0.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.total !== undefined){
let data5 = data.total;
const _errs6 = errors;
if(!(validate71(data5, {instancePath:instancePath+"/total",parentData:data,parentDataProperty:"total",rootData}))){
vErrors = vErrors === null ? validate71.errors : vErrors.concat(validate71.errors);
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
validate752.errors = vErrors;
return errors === 0;
}


function validate751(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(!(validate752(data0, {instancePath:instancePath+"/" + i0,parentData:data,parentDataProperty:i0,rootData}))){
vErrors = vErrors === null ? validate752.errors : vErrors.concat(validate752.errors);
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
validate751.errors = vErrors;
return errors === 0;
}


function validate716(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
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
if(data.completionTime !== undefined){
let data0 = data.completionTime;
const _errs2 = errors;
if(!(validate266(data0, {instancePath:instancePath+"/completionTime",parentData:data,parentDataProperty:"completionTime",rootData}))){
vErrors = vErrors === null ? validate266.errors : vErrors.concat(validate266.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.conditions !== undefined){
let data1 = data.conditions;
const _errs3 = errors;
if(!(validate718(data1, {instancePath:instancePath+"/conditions",parentData:data,parentDataProperty:"conditions",rootData}))){
vErrors = vErrors === null ? validate718.errors : vErrors.concat(validate718.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.events !== undefined){
let data2 = data.events;
const _errs4 = errors;
if(!(validate734(data2, {instancePath:instancePath+"/events",parentData:data,parentDataProperty:"events",rootData}))){
vErrors = vErrors === null ? validate734.errors : vErrors.concat(validate734.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.message !== undefined){
let data3 = data.message;
const _errs5 = errors;
if(!(validate1(data3, {instancePath:instancePath+"/message",parentData:data,parentDataProperty:"message",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs5 === errors;
}
if(data.observedGeneration !== undefined){
let data4 = data.observedGeneration;
const _errs6 = errors;
if(!(validate79(data4, {instancePath:instancePath+"/observedGeneration",parentData:data,parentDataProperty:"observedGeneration",rootData}))){
vErrors = vErrors === null ? validate79.errors : vErrors.concat(validate79.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs6 === errors;
}
if(data.observedNodes !== undefined){
let data5 = data.observedNodes;
const _errs7 = errors;
if(!(validate9(data5, {instancePath:instancePath+"/observedNodes",parentData:data,parentDataProperty:"observedNodes",rootData}))){
vErrors = vErrors === null ? validate9.errors : vErrors.concat(validate9.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs7 === errors;
}
if(data.phase !== undefined){
let data6 = data.phase;
const _errs8 = errors;
if(!(validate749(data6, {instancePath:instancePath+"/phase",parentData:data,parentDataProperty:"phase",rootData}))){
vErrors = vErrors === null ? validate749.errors : vErrors.concat(validate749.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs8 === errors;
}
if(data.pullProgress !== undefined){
let data7 = data.pullProgress;
const _errs9 = errors;
if(!(validate751(data7, {instancePath:instancePath+"/pullProgress",parentData:data,parentDataProperty:"pullProgress",rootData}))){
vErrors = vErrors === null ? validate751.errors : vErrors.concat(validate751.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs9 === errors;
}
if(data.retryCount !== undefined){
let data8 = data.retryCount;
const _errs10 = errors;
if(!(validate78(data8, {instancePath:instancePath+"/retryCount",parentData:data,parentDataProperty:"retryCount",rootData}))){
vErrors = vErrors === null ? validate78.errors : vErrors.concat(validate78.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs10 === errors;
}
if(data.startTime !== undefined){
let data9 = data.startTime;
const _errs11 = errors;
if(!(validate266(data9, {instancePath:instancePath+"/startTime",parentData:data,parentDataProperty:"startTime",rootData}))){
vErrors = vErrors === null ? validate266.errors : vErrors.concat(validate266.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs11 === errors;
}
if(data.tensorBoardProxy !== undefined){
let data10 = data.tensorBoardProxy;
const _errs12 = errors;
if(!(validate1(data10, {instancePath:instancePath+"/tensorBoardProxy",parentData:data,parentDataProperty:"tensorBoardProxy",rootData}))){
vErrors = vErrors === null ? validate1.errors : vErrors.concat(validate1.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs12 === errors;
}
}
validate716.errors = vErrors;
return errors === 0;
}


function validate267(data, {instancePath="", parentData, parentDataProperty, rootData=data}={}){
/*# sourceURL="rlinf.io.v1alpha1.Task" */;
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
if(!(validate268(data0, {instancePath:instancePath+"/apiVersion",parentData:data,parentDataProperty:"apiVersion",rootData}))){
vErrors = vErrors === null ? validate268.errors : vErrors.concat(validate268.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs1 === errors;
}
if(data.kind !== undefined){
let data1 = data.kind;
const _errs2 = errors;
if(!(validate270(data1, {instancePath:instancePath+"/kind",parentData:data,parentDataProperty:"kind",rootData}))){
vErrors = vErrors === null ? validate270.errors : vErrors.concat(validate270.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs2 === errors;
}
if(data.metadata !== undefined){
let data2 = data.metadata;
const _errs3 = errors;
if(!(validate272(data2, {instancePath:instancePath+"/metadata",parentData:data,parentDataProperty:"metadata",rootData}))){
vErrors = vErrors === null ? validate272.errors : vErrors.concat(validate272.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs3 === errors;
}
if(data.spec !== undefined){
let data3 = data.spec;
const _errs4 = errors;
if(!(validate276(data3, {instancePath:instancePath+"/spec",parentData:data,parentDataProperty:"spec",rootData}))){
vErrors = vErrors === null ? validate276.errors : vErrors.concat(validate276.errors);
errors = vErrors.length;
}
else {
}
var valid0 = _errs4 === errors;
}
if(data.status !== undefined){
let data4 = data.status;
const _errs5 = errors;
if(!(validate716(data4, {instancePath:instancePath+"/status",parentData:data,parentDataProperty:"status",rootData}))){
vErrors = vErrors === null ? validate716.errors : vErrors.concat(validate716.errors);
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
validate267.errors = vErrors;
return errors === 0;
}
