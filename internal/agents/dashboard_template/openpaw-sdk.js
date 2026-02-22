(function () {
  'use strict';

  var msgId = 0;
  var pending = {};

  window.addEventListener('message', function (e) {
    var data = e.data;
    if (!data) return;

    if (data.type === 'openpaw_response' && pending[data.id]) {
      var p = pending[data.id];
      delete pending[data.id];
      if (data.error) p.reject(new Error(data.error));
      else p.resolve(data.result);
    }

    if (data.type === 'openpaw_theme') {
      var root = document.documentElement;
      var vars = data.vars || {};
      for (var key in vars) {
        if (vars.hasOwnProperty(key)) {
          root.style.setProperty(key, vars[key]);
        }
      }
    }
  });

  function request(action, data) {
    return new Promise(function (resolve, reject) {
      var id = ++msgId;
      pending[id] = { resolve: resolve, reject: reject };
      var msg = { type: 'openpaw_request', id: id, action: action };
      if (data) {
        for (var k in data) {
          if (data.hasOwnProperty(k)) msg[k] = data[k];
        }
      }
      window.parent.postMessage(msg, '*');
      setTimeout(function () {
        if (pending[id]) {
          delete pending[id];
          reject(new Error('Request timeout'));
        }
      }, 30000);
    });
  }

  function injectTheme() {
    window.parent.postMessage({ type: 'openpaw_theme_request' }, '*');
  }

  window.OpenPaw = {
    async callTool(toolId, endpoint, payload) {
      return request('callTool', { toolId: toolId, endpoint: endpoint, payload: payload });
    },

    async getTools() {
      return request('getTools');
    },

    refresh(callback, intervalMs) {
      if (typeof callback !== 'function') return;
      callback();
      var id = setInterval(callback, intervalMs || 30000);
      return function () { clearInterval(id); };
    },

    injectTheme: injectTheme,

    get theme() {
      var style = getComputedStyle(document.documentElement);
      return {
        surface0: style.getPropertyValue('--op-surface-0').trim(),
        surface1: style.getPropertyValue('--op-surface-1').trim(),
        surface2: style.getPropertyValue('--op-surface-2').trim(),
        surface3: style.getPropertyValue('--op-surface-3').trim(),
        border0: style.getPropertyValue('--op-border-0').trim(),
        border1: style.getPropertyValue('--op-border-1').trim(),
        text0: style.getPropertyValue('--op-text-0').trim(),
        text1: style.getPropertyValue('--op-text-1').trim(),
        text2: style.getPropertyValue('--op-text-2').trim(),
        text3: style.getPropertyValue('--op-text-3').trim(),
        accent: style.getPropertyValue('--op-accent').trim(),
        accentHover: style.getPropertyValue('--op-accent-hover').trim(),
        accentMuted: style.getPropertyValue('--op-accent-muted').trim(),
        accentText: style.getPropertyValue('--op-accent-text').trim(),
        danger: style.getPropertyValue('--op-danger').trim(),
        dangerHover: style.getPropertyValue('--op-danger-hover').trim(),
      };
    },
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', injectTheme);
  } else {
    injectTheme();
  }
})();
