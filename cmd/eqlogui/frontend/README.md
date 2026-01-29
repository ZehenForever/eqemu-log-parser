# eqlogui frontend

## Tailwind content globs

If you add TS/TSX components (for example `src/components/Modal.tsx`), make sure `tailwind.config.js` includes `ts`/`tsx` in the `content` globs. Otherwise Tailwind may purge those classes and components can appear unstyled or invisible.
