import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'
import AppErrorBoundary from './componentjs/AppErrorBoundary.jsx'

const container = document.getElementById('root')

const root = createRoot(container)

function ensureFrontendCrashOverlay(message) {
    const text = String(message ?? '').trim()
    if (!text) return
    let el = document.getElementById('frontend-crash-overlay')
    if (!el) {
        el = document.createElement('div')
        el.id = 'frontend-crash-overlay'
        el.style.position = 'fixed'
        el.style.inset = '0'
        el.style.zIndex = '2147483647'
        el.style.padding = '24px'
        el.style.overflow = 'auto'
        el.style.whiteSpace = 'pre-wrap'
        el.style.wordBreak = 'break-word'
        el.style.background = '#fff4f4'
        el.style.color = '#6b1f1f'
        el.style.fontFamily = 'Nunito, sans-serif'
        document.body.appendChild(el)
    }
    el.textContent = `前端发生未捕获异常：\n\n${text}`
}

window.addEventListener('error', (event) => {
    const error = event?.error
    const message = error?.stack || error?.message || event?.message || 'unknown error'
    console.error('window.error:', error || event)
    ensureFrontendCrashOverlay(message)
})

window.addEventListener('unhandledrejection', (event) => {
    const reason = event?.reason
    const message =
        reason?.stack || reason?.message || (typeof reason === 'string' ? reason : JSON.stringify(reason, null, 2))
    console.error('window.unhandledrejection:', reason)
    ensureFrontendCrashOverlay(message)
})

root.render(
    <React.StrictMode>
        <AppErrorBoundary>
            <App/>
        </AppErrorBoundary>
    </React.StrictMode>
)
