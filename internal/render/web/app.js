/*
 * intent-review 前端：链/意图切换只改显隐，不重建 DOM。
 * 路由：<chain-id>/<intent-id>/<change-index(1 基)>
 */
(function () {
  'use strict';

  var KEEP_CONTEXT = 3;   // 首尾各保留的 context 行数
  var FOLD_THRESHOLD = 6; // 超过该长度的 context 连续段才折叠

  function qsa(sel, root) {
    return Array.prototype.slice.call((root || document).querySelectorAll(sel));
  }

  var sections = qsa('[data-chain-section]');
  var chainButtons = qsa('.chain-item');
  var state = { chain: null, intent: null };

  function sectionOf(id) {
    for (var i = 0; i < sections.length; i++) {
      if (sections[i].getAttribute('data-chain-section') === id) return sections[i];
    }
    return null;
  }

  function nodesOf(sec) { return qsa('.timeline-node', sec); }

  function panelOf(sec, intentId) {
    var panels = qsa('[data-panel]', sec);
    for (var i = 0; i < panels.length; i++) {
      if (panels[i].getAttribute('data-intent') === intentId) return panels[i];
    }
    return null;
  }

  function navOf(sec) { return sec.querySelector('.intent-timeline'); }

  function setChain(id) {
    sections.forEach(function (sec) {
      sec.hidden = sec.getAttribute('data-chain-section') !== id;
    });
    chainButtons.forEach(function (btn) {
      var on = btn.getAttribute('data-chain') === id;
      btn.classList.toggle('is-active', on);
      if (on) btn.setAttribute('aria-current', 'true');
      else btn.removeAttribute('aria-current');
    });
    state.chain = id;
    document.body.classList.remove('sidebar-open');
  }

  function setIntent(intentId, opts) {
    var sec = sectionOf(state.chain);
    if (!sec) return null;
    var panels = qsa('[data-panel]', sec);
    var active = null;
    panels.forEach(function (panel) {
      var on = panel.getAttribute('data-intent') === intentId;
      panel.hidden = !on;
      if (on) active = panel;
    });
    if (!active) return null;
    state.intent = intentId;

    var nodes = nodesOf(sec);
    var activeIndex = -1;
    nodes.forEach(function (node, i) {
      if (node.getAttribute('data-intent') === intentId) activeIndex = i;
    });
    nodes.forEach(function (node, i) {
      var isActive = i === activeIndex;
      node.classList.toggle('is-active', isActive);
      node.classList.toggle('is-passed', i < activeIndex);
      var btn = node.querySelector('.node-btn');
      if (!btn) return;
      if (isActive) btn.setAttribute('aria-current', 'step');
      else btn.removeAttribute('aria-current');
    });

    var details = active.querySelector('details.panel-details');
    if (details) details.open = true;

    scrollNodeIntoView(sec, activeIndex);
    if (opts && opts.scroll) scrollPanelTop(active, sec);
    return active;
  }

  function scrollNodeIntoView(sec, index) {
    var nav = navOf(sec);
    if (!nav) return;
    var scroller = nav.querySelector('.timeline-scroll');
    if (!scroller) return;
    var node = nodesOf(sec)[index];
    if (!node) return;
    var left = node.offsetLeft;
    var right = left + node.offsetWidth;
    var pad = 16;
    if (left < scroller.scrollLeft + pad) {
      scroller.scrollLeft = Math.max(0, left - pad);
    } else if (right > scroller.scrollLeft + scroller.clientWidth - pad) {
      scroller.scrollLeft = right - scroller.clientWidth + pad;
    }
  }

  function stickyOffset(sec) {
    var nav = navOf(sec);
    return nav ? nav.getBoundingClientRect().height : 0;
  }

  function scrollPanelTop(panel, sec) {
    var top = panel.getBoundingClientRect().top + window.pageYOffset - stickyOffset(sec) - 8;
    window.scrollTo(0, Math.max(0, top));
  }

  function scrollToChange(panel, changeNumber, sec) {
    var el = panel.querySelector('[data-change-index="' + changeNumber + '"]');
    if (!el) return;
    var top = el.getBoundingClientRect().top + window.pageYOffset - stickyOffset(sec) - 8;
    window.scrollTo(0, Math.max(0, top));
  }

  function defaultRoute() {
    var sec = sections[0];
    if (!sec) return null;
    var nodes = nodesOf(sec);
    return {
      chain: sec.getAttribute('data-chain-section'),
      intent: nodes.length ? nodes[0].getAttribute('data-intent') : null,
      change: null
    };
  }

  function encodeHash(route) {
    var parts = [route.chain];
    if (route.intent) parts.push(route.intent);
    if (route.change) parts.push(String(route.change));
    return '#' + parts.map(function (p) {
      try { return encodeURIComponent(p); } catch (e) { return p; }
    }).join('/');
  }

  function decode(s) {
    try { return decodeURIComponent(s); } catch (e) { return s; }
  }

  function parseHash() {
    var raw = window.location.hash.replace(/^#/, '');
    if (!raw) return null;
    var parts = raw.split('/');
    var change = parts[2] ? parseInt(parts[2], 10) : null;
    return {
      chain: decode(parts[0] || ''),
      intent: parts[1] ? decode(parts[1]) : null,
      change: change && change > 0 ? change : null
    };
  }

  function apply(route, opts) {
    opts = opts || {};
    if (!route || !sectionOf(route.chain)) route = defaultRoute();
    if (!route) return;

    setChain(route.chain);
    var sec = sectionOf(route.chain);
    var intentId = route.intent;
    if (!intentId || !panelOf(sec, intentId)) {
      var nodes = nodesOf(sec);
      intentId = nodes.length ? nodes[0].getAttribute('data-intent') : null;
    }
    var active = intentId ? setIntent(intentId, { scroll: !!opts.scroll }) : null;
    if (active && route.change) scrollToChange(active, route.change, sec);

    if (opts.push) {
      var target = { chain: route.chain, intent: state.intent, change: route.change || null };
      try {
        history.pushState(target, '', encodeHash(target));
      } catch (e) {
        window.location.hash = encodeHash(target);
      }
    }
  }

  function activate(route, opts) {
    apply(route, opts || {});
  }

  /* ---------- 上下文折叠 ---------- */

  var foldSeq = 0;

  function setupFolds() {
    qsa('table.diff').forEach(function (table) {
      var tbody = table.tBodies[0];
      if (!tbody) return;
      var rows = Array.prototype.slice.call(tbody.rows);
      var runs = [];
      var run = [];
      function flush() {
        if (run.length) runs.push(run);
        run = [];
      }
      rows.forEach(function (row) {
        if (!row.classList.contains('row-ctx') || row.hidden) {
          flush();
          return;
        }
        run.push(row);
      });
      flush();
      runs.forEach(function (rowsInRun, idx) {
        if (rowsInRun.length <= FOLD_THRESHOLD) return;
        var leading = idx === 0;
        var trailing = idx === runs.length - 1;
        var hideStart, hideEnd;
        if (leading && trailing) {
          // 整张表都是 context：首尾各留 KEEP_CONTEXT 行。
          hideStart = KEEP_CONTEXT;
          hideEnd = rowsInRun.length - KEEP_CONTEXT;
        } else if (leading) {
          // 前置 context：保留紧邻变更的末尾 KEEP_CONTEXT 行。
          hideStart = 0;
          hideEnd = rowsInRun.length - KEEP_CONTEXT;
        } else if (trailing) {
          // 后置 context：保留紧邻变更的开头 KEEP_CONTEXT 行。
          hideStart = KEEP_CONTEXT;
          hideEnd = rowsInRun.length;
        } else {
          // 两段变更之间：两侧各留 KEEP_CONTEXT 行。
          hideStart = KEEP_CONTEXT;
          hideEnd = rowsInRun.length - KEEP_CONTEXT;
        }
        if (hideEnd <= hideStart) return;
        var group = 'g' + (++foldSeq);
        for (var i = hideStart; i < hideEnd; i++) {
          rowsInRun[i].hidden = true;
          rowsInRun[i].setAttribute('data-fold', group);
        }
        var count = hideEnd - hideStart;
        var tr = document.createElement('tr');
        tr.className = 'fold-row';
        var td = document.createElement('td');
        td.colSpan = 4;
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'fold-toggle';
        btn.setAttribute('data-fold', group);
        btn.setAttribute('data-count', String(count));
        btn.setAttribute('aria-expanded', 'false');
        btn.textContent = '展开 ' + count + ' 行';
        td.appendChild(btn);
        tr.appendChild(td);
        tbody.insertBefore(tr, rowsInRun[hideStart]);
      });
    });
  }

  function toggleFold(btn) {
    var group = btn.getAttribute('data-fold');
    var tbody = btn.closest('tbody');
    if (!tbody) return;
    var expanded = btn.getAttribute('aria-expanded') === 'true';
    var rows = tbody.querySelectorAll('[data-fold="' + group + '"]');
    Array.prototype.forEach.call(rows, function (row) {
      if (row.tagName === 'TR') row.hidden = expanded;
    });
    btn.setAttribute('aria-expanded', expanded ? 'false' : 'true');
    var count = btn.getAttribute('data-count');
    btn.textContent = expanded ? '展开 ' + count + ' 行' : '收起 ' + count + ' 行';
  }

  /* ---------- 事件 ---------- */

  document.addEventListener('click', function (ev) {
    var target = ev.target;

    var toggle = target.closest && target.closest('.nav-toggle');
    if (toggle) {
      document.body.classList.toggle('sidebar-open');
      return;
    }

    var foldBtn = target.closest && target.closest('.fold-toggle');
    if (foldBtn) {
      toggleFold(foldBtn);
      return;
    }

    var chainBtn = target.closest && target.closest('.chain-item');
    if (chainBtn) {
      var chainId = chainBtn.getAttribute('data-chain');
      activate({ chain: chainId, intent: null, change: null }, { push: true, scroll: false });
      return;
    }

    var nodeBtn = target.closest && target.closest('.node-btn');
    if (nodeBtn) {
      var li = nodeBtn.closest('.timeline-node');
      var sec = nodeBtn.closest('[data-chain-section]');
      if (!li || !sec) return;
      activate({
        chain: sec.getAttribute('data-chain-section'),
        intent: li.getAttribute('data-intent'),
        change: null
      }, { push: true, scroll: true });
    }
  });

  document.addEventListener('keydown', function (ev) {
    if (ev.defaultPrevented || ev.altKey || ev.ctrlKey || ev.metaKey) return;
    if (ev.key !== 'ArrowLeft' && ev.key !== 'ArrowRight') return;
    var t = ev.target;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable)) return;
    var sec = sectionOf(state.chain);
    if (!sec) return;
    var nodes = nodesOf(sec);
    var idx = -1;
    nodes.forEach(function (n, i) {
      if (n.getAttribute('data-intent') === state.intent) idx = i;
    });
    var next = ev.key === 'ArrowRight' ? idx + 1 : idx - 1;
    if (next < 0 || next >= nodes.length) return;
    ev.preventDefault();
    activate({
      chain: state.chain,
      intent: nodes[next].getAttribute('data-intent'),
      change: null
    }, { push: true, scroll: true });
  });

  window.addEventListener('popstate', function () {
    activate(parseHash(), { scroll: true });
  });

  window.addEventListener('hashchange', function () {
    activate(parseHash(), { scroll: true });
  });

  /* ---------- 初始化 ---------- */

  setupFolds();
  activate(parseHash() || defaultRoute(), { scroll: false });

  // 暴露最小状态供 e2e 断言（只读，不提供写操作）。
  window.__intentReview = {
    state: state,
    chains: sections.map(function (sec) { return sec.getAttribute('data-chain-section'); })
  };
})();
