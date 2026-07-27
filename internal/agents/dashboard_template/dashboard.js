// Custom Dashboard
// Use OpenPaw.callTool(), OpenPaw.getTools(), OpenPaw.refresh() to build your dashboard.
// Import any library via esm.sh: import { Chart } from 'https://esm.sh/chart.js@4/auto'
//
// To save anything the user enters, use OpenPaw.storage — localStorage does NOT
// work in this sandboxed frame and loses the data on reload:
//   await OpenPaw.storage.set('products', list)
//   const list = await OpenPaw.storage.get('products', [])

const app = document.getElementById('app');
app.innerHTML = '<p style="color: var(--op-text-2); padding: 2rem;">Dashboard loading...</p>';
