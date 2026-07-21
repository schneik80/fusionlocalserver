/// <reference types="vite/client" />

// Typed build-time config. `VITE_`-prefixed vars are the only ones Vite exposes
// to client code, and they are INLINED INTO THE BUNDLE at build time — put
// nothing here that must stay secret from someone holding the binary.
interface ImportMetaEnv {
  /**
   * tldraw SDK licence key (see docs/whiteboards/STATUS.md). Optional: without
   * it tldraw treats a non-loopback HTTPS page as an unlicensed production
   * deployment and hides the editor five seconds after it mounts.
   */
  readonly VITE_TLDRAW_LICENSE_KEY?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
