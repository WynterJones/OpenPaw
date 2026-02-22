// Default OpenPaw widget — renders tool data as a themed key-value list.
// Customize this file to create rich visualizations for your tool's output.
//
// Available globals:
//   window.WIDGET_DATA  — the JSON data from the tool response
//   window.WIDGET_THEME — object with --op-* CSS var values (underscored keys)
//
// Available CSS vars:
//   --op-surface-0..3, --op-text-0..3, --op-border-0..1, --op-accent, etc.

(function () {
  var root = document.getElementById('widget-root');
  var data = window.WIDGET_DATA || {};

  var entries = Object.entries(data);
  if (entries.length === 0) {
    root.innerHTML = '<p style="color:var(--op-text-3);font-size:13px;">No data</p>';
    return;
  }

  var table = document.createElement('table');
  table.style.cssText = 'width:100%;border-collapse:collapse;font-size:13px;';

  entries.forEach(function (pair, i) {
    var key = pair[0];
    var val = pair[1];
    var tr = document.createElement('tr');
    tr.style.borderBottom = '1px solid var(--op-border-0, #333)';
    if (i % 2 === 0) tr.style.background = 'var(--op-surface-2, rgba(255,255,255,0.03))';

    var tdKey = document.createElement('td');
    tdKey.style.cssText = 'padding:6px 10px;font-weight:600;color:var(--op-text-2,#aaa);white-space:nowrap;vertical-align:top;';
    tdKey.textContent = key;

    var tdVal = document.createElement('td');
    tdVal.style.cssText = 'padding:6px 10px;color:var(--op-text-1,#ddd);word-break:break-word;';
    tdVal.textContent = typeof val === 'object' ? JSON.stringify(val) : String(val);

    tr.appendChild(tdKey);
    tr.appendChild(tdVal);
    table.appendChild(tr);
  });

  root.appendChild(table);
})();
