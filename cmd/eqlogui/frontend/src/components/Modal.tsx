import React, { useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'

type ModalProps = {
  open: boolean
  title: string
  onClose: () => void
  children: React.ReactNode
}

export default function Modal({ open, title, onClose, children }: ModalProps) {
  const closeBtnRef = useRef<HTMLButtonElement | null>(null)

  useEffect(() => {
    if (!open) return

    const onKeyDown = (ev: KeyboardEvent) => {
      if (ev.key === 'Escape') onClose()
    }

    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [open, onClose])

  useEffect(() => {
    if (!open) return

    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'

    // Focus close button (simple focus management)
    // Using rAF ensures the button exists in the DOM.
    requestAnimationFrame(() => {
      const el = closeBtnRef.current
      if (!el) return
      try {
        // preventScroll is supported in modern browsers; fall back gracefully.
        el.focus({ preventScroll: true })
      } catch {
        el.focus()
      }
    })

    return () => {
      document.body.style.overflow = prev
    }
  }, [open])

  if (!open) return null

  return createPortal(
    <div className="fixed inset-0 z-50">
      <div className="absolute inset-0 bg-black/70" onClick={onClose} />

      <div className="absolute inset-0 flex items-center justify-center p-4">
        <div
          className="flex w-[90vw] max-w-6xl max-h-[85vh] flex-col overflow-hidden rounded-lg border border-slate-800 bg-slate-950 shadow-xl"
          onClick={(ev: React.MouseEvent<HTMLDivElement>) => ev.stopPropagation()}
          role="dialog"
          aria-modal="true"
        >
          <div className="flex items-center justify-between border-b border-slate-800 px-4 py-3">
            <div className="font-semibold text-slate-100">{title}</div>
            <button
              ref={closeBtnRef}
              className="rounded-md border border-slate-800 bg-slate-950 px-2 py-1 text-xs text-slate-200 hover:bg-slate-900"
              onClick={onClose}
              type="button"
            >
              Close
            </button>
          </div>

          <div className="flex-1 overflow-auto p-4">{children}</div>
        </div>
      </div>
    </div>,
    document.body,
  )
}
