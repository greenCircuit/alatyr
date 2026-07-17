import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './style/colors.css'
import './style/utilities.css'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
