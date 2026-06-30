// The Fusion brand mark used in the app header. Fixed brand colors (orange /
// brown / white), so it's an inline SVG rather than a themed/currentColor glyph.
export function FusionLogo({ size = 24 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden
      style={{ display: 'block', flexShrink: 0 }}
    >
      <path
        d="M3.87416 1.01074L3.87985 1.00111L22.1067 5.02901H24V22.886H3.40727C3.0643 22.886 2.73537 22.7497 2.49284 22.5072C2.25032 22.2647 2.11407 21.9357 2.11407 21.5928V19.843L3.87416 1.01074Z"
        fill="#933C00"
      />
      <path
        d="M22.2342 18.9067H3.87415V1.01077L3.87983 1H20.9409C21.2839 1 21.6129 1.13625 21.8554 1.37877C22.0979 1.62129 22.2341 1.95022 22.2341 2.2932L22.2342 18.9067Z"
        fill="#FF6B00"
      />
      <path d="M0 21.2444L3.87988 18.9067V1L0 3.33771V21.2444Z" fill="#FF9548" />
      <path
        d="M12.859 6.79282V9.16038H16.1197V10.9184H12.859V14.9305H10.3777V5.00645H16.7151V6.79282H12.859Z"
        fill="white"
      />
    </svg>
  )
}
