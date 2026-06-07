// Minimal typings for n8ao (ships no .d.ts). We only use N8AOPostPass with the
// pmndrs `postprocessing` EffectComposer.
declare module 'n8ao' {
  import { Pass } from 'postprocessing'
  import { Scene, Camera, Color } from 'three'

  export class N8AOPostPass extends Pass {
    constructor(scene: Scene, camera: Camera, width?: number, height?: number)
    configuration: {
      aoRadius: number
      distanceFalloff: number
      intensity: number
      color: Color
      aoSamples: number
      denoiseSamples: number
      denoiseRadius: number
      screenSpaceRadius: boolean
      halfRes: boolean
      [k: string]: unknown
    }
    setQualityMode(mode: 'Performance' | 'Low' | 'Medium' | 'High' | 'Ultra'): void
    enabled: boolean
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  export class N8AOPass extends Pass {
    constructor(...args: any[])
  }
}
