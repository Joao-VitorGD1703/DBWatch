import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { FeatureFlags } from '@carbon/react'
import './index.scss'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <FeatureFlags flags={{ 'enable-v12-release': true }}>
        <App />
      </FeatureFlags>
    </BrowserRouter>
  </StrictMode>,
)
