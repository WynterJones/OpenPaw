import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

// When running inside the Tauri desktop shell the macOS window uses an overlay
// title bar (no native toolbar), so the traffic-light buttons float over the
// top-left of the app. Tag <html> so the layout can offset content below them
// and turn the toolbar/sidebar into window-drag regions.
if (typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window) {
  document.documentElement.dataset.desktop = 'true'
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
