package main

import (
	"net/http"
	"strings"
)

// widget.js — drop-in for demos:
//
//	<div id="sl-account"></div>
//	<script src="https://auth.demo.sedlab.dev/widget.js" data-demo="voice" data-resource="openai" defer></script>
//
// Renders "Войти и попробовать вживую" or "Имя · живой режим: осталось 2/3", and dispatches
// window event "sedlab:account" {authenticated, name, resources[]} so the demo can show its mock/live banner.
// The demo server still decides live vs mock (module accounts → consume); the widget is display only.
const widgetSrc = `(function(){
var s=document.currentScript||document.querySelector('script[src*="/widget.js"]');if(!s)return;
var AUTH=%AUTH%,demo=s.getAttribute('data-demo')||'',res=s.getAttribute('data-resource')||'';
var lang=(s.getAttribute('data-lang')||document.documentElement.lang||navigator.language||'ru').slice(0,2)==='en'?'en':'ru';
var T={ru:{in:'Войти и попробовать вживую',hint:'Живой режим отправляет ваш запрос провайдеру: {p}. Не вводите чужие персональные данные.',lim:'Живой режим: осталось {n}/{m} сегодня',out:'Живой лимит на сегодня исчерпан: показываем демо-режим',acc:'Мой аккаунт'},
en:{in:'Sign in to try live',hint:"Live mode sends your request to: {p}. Don't enter other people's personal data.",lim:'Live mode: {n}/{m} left today',out:"Today's live limit is used up. Showing demo mode",acc:'My account'}}[lang];
var plist=s.getAttribute('data-providers')||'';
function el(t,a,c){var e=document.createElement(t);for(var k in a)e.setAttribute(k,a[k]);if(c)e.textContent=c;return e}
var box=document.getElementById('sl-account');if(!box){box=el('div',{id:'sl-account'});box.style.cssText='position:fixed;right:12px;bottom:12px;z-index:9999';document.body.appendChild(box)}
var css='font:14px/1.3 system-ui,sans-serif;display:inline-flex;gap:8px;align-items:center;padding:8px 12px;border-radius:999px;background:#fff;color:#1c1c1e;border:1px solid #e4e2dc;box-shadow:0 2px 10px rgba(0,0,0,.08);text-decoration:none';
function emit(d){try{window.dispatchEvent(new CustomEvent('sedlab:account',{detail:d}))}catch(e){}}
function q(p){return fetch(AUTH+p,{credentials:'include'}).then(function(r){return r.json()})}
q('/api/v1/me').then(function(me){
 if(!me.authenticated){var a=el('a',{href:AUTH+'/login?return='+encodeURIComponent(location.href)+'&lang='+lang},T.in);a.style.cssText=css+';background:#2f5bea;color:#fff;border-color:#2f5bea;font-weight:600';
  var w=el('div');w.style.cssText='display:flex;flex-direction:column;align-items:flex-end;gap:4px;max-width:320px';w.appendChild(a);
  if(plist){var h=el('div',{},T.hint.replace('{p}',plist));h.style.cssText='font:12px/1.35 system-ui,sans-serif;color:#6b6b70;text-align:right';w.appendChild(h)}
  box.replaceChildren(w);emit({authenticated:false,resources:[]});return}
 q('/api/v1/me/limits?demo='+encodeURIComponent(demo)).then(function(l){
  var rs=l.resources||[],r=null;for(var i=0;i<rs.length;i++){if(!res||rs[i].resource===res){r=rs[i];break}}
  var a=el('a',{href:AUTH+'/account?lang='+lang,title:T.acc});a.style.cssText=css;
  a.appendChild(el('b',{},me.name||T.acc));
  if(r){var txt=r.live_available?T.lim.replace('{n}',r.user_left<0?'∞':r.user_left).replace('{m}',r.user_limit<0?'∞':r.user_limit):T.out;a.appendChild(el('span',{},'· '+txt))}
  box.replaceChildren(a);emit({authenticated:true,name:me.name,resources:rs});
 })}).catch(function(){emit({authenticated:false,resources:[],error:true})});
})();`

func (a *App) widgetJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	_, _ = w.Write([]byte(strings.Replace(widgetSrc, "%AUTH%", `"`+a.cfg.PublicURL+`"`, 1)))
}
