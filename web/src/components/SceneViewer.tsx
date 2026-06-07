import { useEffect, useRef, useState } from 'react'
import { Box, Checkbox, CircularProgress, FormControlLabel, Slider, Typography } from '@mui/material'
import * as THREE from 'three'
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js'
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js'
import { RGBELoader } from 'three/examples/jsm/loaders/RGBELoader.js'
import { LineSegments2 } from 'three/examples/jsm/lines/LineSegments2.js'
import { LineSegmentsGeometry } from 'three/examples/jsm/lines/LineSegmentsGeometry.js'
import { LineMaterial } from 'three/examples/jsm/lines/LineMaterial.js'
import { EffectComposer, EffectPass, RenderPass, ToneMappingEffect, ToneMappingMode } from 'postprocessing'
import { N8AOPostPass } from 'n8ao'

// SceneViewer renders a design's exported GLB in a self-contained three.js
// viewer: OrbitControls, HDRI image-based lighting, N8AO screen-space ambient
// occlusion (real crease/contact shadows over the whole frame), a fat-line CAD
// edge overlay, and a scene-graph tree with per-node visibility. We use three.js
// directly (not <model-viewer>) so we own the three.js version and the
// post-processing pipeline.

const HDR_URL = '/environments/Generic.hdr'
const EDGE_THRESHOLD_DEG = 30 // hard-edge angle for the CAD outline overlay
const EDGE_WIDTH_PX = 2

interface TreeNode {
  uuid: string
  name: string
  depth: number
}

export function SceneViewer({ glbUrl }: { glbUrl: string }) {
  const mountRef = useRef<HTMLDivElement | null>(null)

  const gl = useRef<{
    renderer?: THREE.WebGLRenderer
    scene?: THREE.Scene
    camera?: THREE.PerspectiveCamera
    controls?: OrbitControls
    composer?: EffectComposer
    n8ao?: N8AOPostPass
    objByUuid: Map<string, THREE.Object3D>
    edgeLines: LineSegments2[]
    edgeMaterials: LineMaterial[]
    raf: number
    pending: boolean
    disposed: boolean
  }>({ objByUuid: new Map(), edgeLines: [], edgeMaterials: [], raf: 0, pending: false, disposed: false })

  const [tree, setTree] = useState<TreeNode[]>([])
  const [, forceTick] = useState(0)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [brightness, setBrightness] = useState(1.0)
  const [ao, setAo] = useState(true)
  const [edges, setEdges] = useState(true)

  const requestRender = useRef(() => {})

  useEffect(() => {
    const mount = mountRef.current
    if (!mount) return
    const g = gl.current
    g.disposed = false
    setLoaded(false)
    setError(null)
    setTree([])

    const width = mount.clientWidth || 1
    const height = mount.clientHeight || 1

    const renderer = new THREE.WebGLRenderer({ antialias: false, alpha: true, powerPreference: 'high-performance' })
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
    renderer.setSize(width, height)
    // Tone mapping is handled by a ToneMappingEffect in the composer, not the
    // renderer (the composer bypasses renderer tone mapping). Brightness is
    // driven by scene.environmentIntensity below.
    renderer.toneMapping = THREE.NoToneMapping
    renderer.setClearColor(0x000000, 0)
    mount.appendChild(renderer.domElement)

    const scene = new THREE.Scene()
    scene.environmentIntensity = brightness
    const camera = new THREE.PerspectiveCamera(45, width / height, 0.1, 1000)
    const controls = new OrbitControls(camera, renderer.domElement)
    controls.enableDamping = false

    // MSAA on the composer handles anti-aliasing (simpler than an SMAA pass,
    // and SMAAEffect can't be merged with other effects in one EffectPass).
    const composer = new EffectComposer(renderer, { multisampling: 4 })
    composer.addPass(new RenderPass(scene, camera))
    const n8ao = new N8AOPostPass(scene, camera, width, height)
    n8ao.configuration.intensity = 3
    n8ao.configuration.aoSamples = 16
    n8ao.configuration.denoiseSamples = 8
    n8ao.setQualityMode('Medium')
    composer.addPass(n8ao)
    composer.addPass(new EffectPass(camera, new ToneMappingEffect({ mode: ToneMappingMode.NEUTRAL })))

    Object.assign(g, { renderer, scene, camera, controls, composer, n8ao })
    g.objByUuid = new Map()
    g.edgeLines = []
    g.edgeMaterials = []

    const render = () => {
      if (g.disposed) return
      g.pending = false
      composer.render()
    }
    requestRender.current = () => {
      if (g.pending || g.disposed) return
      g.pending = true
      g.raf = requestAnimationFrame(render)
    }
    controls.addEventListener('change', requestRender.current)

    let envMap: THREE.Texture | undefined
    const pmrem = new THREE.PMREMGenerator(renderer)
    new RGBELoader().load(
      HDR_URL,
      (tex) => {
        if (g.disposed) {
          tex.dispose()
          return
        }
        tex.mapping = THREE.EquirectangularReflectionMapping
        envMap = pmrem.fromEquirectangular(tex).texture
        scene.environment = envMap
        tex.dispose()
        pmrem.dispose()
        requestRender.current()
      },
      undefined,
      () => {
        /* lighting falls back to none on HDR load failure; geometry still shows */
      },
    )

    new GLTFLoader().load(
      glbUrl,
      (gltf) => {
        if (g.disposed) return
        scene.add(gltf.scene)

        gltf.scene.traverse((obj) => {
          if ((obj.userData as { ogsHidden?: boolean })?.ogsHidden === true) obj.visible = false
        })

        // Frame the camera, and — critically — set SANE near/far planes. A huge
        // near:far ratio wrecks depth precision (back faces bleed through, and
        // N8AO's depth-based occlusion produces nothing).
        const box = new THREE.Box3().setFromObject(gltf.scene)
        const size = box.getSize(new THREE.Vector3())
        const center = box.getCenter(new THREE.Vector3())
        const maxDim = Math.max(size.x, size.y, size.z) || 1
        const dist = (maxDim / 2 / Math.tan((camera.fov * Math.PI) / 360)) * 1.6
        camera.near = maxDim * 0.05
        camera.far = maxDim * 50
        camera.position.set(center.x + dist * 0.6, center.y + dist * 0.4, center.z + dist)
        camera.updateProjectionMatrix()
        controls.target.copy(center)
        controls.update()

        // CAD edge overlay using fat lines (LineMaterial honours linewidth in px;
        // LineBasicMaterial is always 1px). Collect the meshes FIRST: adding the
        // edge children during traverse() would make traverse visit each new
        // LineSegments2 (itself a Mesh subclass) and recurse forever.
        const meshes: THREE.Mesh[] = []
        gltf.scene.traverse((obj) => {
          const mesh = obj as THREE.Mesh
          if (mesh.isMesh && mesh.geometry && !(obj instanceof LineSegments2)) meshes.push(mesh)
        })
        for (const mesh of meshes) {
          const eg = new THREE.EdgesGeometry(mesh.geometry, EDGE_THRESHOLD_DEG)
          const lsg = new LineSegmentsGeometry().fromEdgesGeometry(eg)
          eg.dispose()
          const lm = new LineMaterial({ color: 0x202020, linewidth: EDGE_WIDTH_PX })
          lm.resolution.set(width, height)
          const seg = new LineSegments2(lsg, lm)
          seg.visible = edges
          seg.renderOrder = 1
          mesh.add(seg)
          g.edgeLines.push(seg)
          g.edgeMaterials.push(lm)
        }

        n8ao.configuration.aoRadius = maxDim * 0.15
        n8ao.configuration.distanceFalloff = n8ao.configuration.aoRadius

        const order: TreeNode[] = []
        const objByUuid = new Map<string, THREE.Object3D>()
        const walk = (obj: THREE.Object3D, depth: number) => {
          if (depth >= 0 && obj.name && !(obj instanceof LineSegments2)) {
            order.push({ uuid: obj.uuid, name: obj.name, depth })
            objByUuid.set(obj.uuid, obj)
          }
          obj.children.forEach((c) => walk(c, depth + 1))
        }
        walk(gltf.scene, -1)
        g.objByUuid = objByUuid
        setTree(order)
        setLoaded(true)
        requestRender.current()
      },
      undefined,
      (err) => {
        if (!g.disposed) setError(err instanceof Error ? err.message : 'failed to load model')
      },
    )

    const ro = new ResizeObserver(() => {
      const w = mount.clientWidth || 1
      const h = mount.clientHeight || 1
      renderer.setSize(w, h)
      composer.setSize(w, h)
      camera.aspect = w / h
      camera.updateProjectionMatrix()
      g.edgeMaterials.forEach((m) => m.resolution.set(w, h))
      requestRender.current()
    })
    ro.observe(mount)

    return () => {
      g.disposed = true
      cancelAnimationFrame(g.raf)
      ro.disconnect()
      controls.dispose()
      composer.dispose()
      envMap?.dispose()
      scene.traverse((obj) => {
        const mesh = obj as THREE.Mesh
        if (mesh.geometry) mesh.geometry.dispose()
        const mat = (mesh as THREE.Mesh).material
        if (Array.isArray(mat)) mat.forEach((m) => m.dispose())
        else if (mat) (mat as THREE.Material).dispose()
      })
      renderer.dispose()
      if (renderer.domElement.parentNode === mount) mount.removeChild(renderer.domElement)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [glbUrl])

  // Brightness scales the HDRI lighting directly (tone mapping is fixed in the
  // composer, so toneMappingExposure has no effect here).
  useEffect(() => {
    const g = gl.current
    if (g.scene) {
      g.scene.environmentIntensity = brightness
      requestRender.current()
    }
  }, [brightness])

  useEffect(() => {
    const g = gl.current
    if (g.n8ao) {
      g.n8ao.enabled = ao
      requestRender.current()
    }
  }, [ao])

  useEffect(() => {
    const g = gl.current
    g.edgeLines.forEach((l) => (l.visible = edges))
    requestRender.current()
  }, [edges, tree])

  const toggleNode = (uuid: string) => {
    const obj = gl.current.objByUuid.get(uuid)
    if (!obj) return
    obj.visible = !obj.visible
    forceTick((n) => n + 1)
    requestRender.current()
  }

  return (
    <Box sx={{ display: 'flex', flex: 1, minWidth: 0, height: '100%', minHeight: 320 }}>
      {tree.length > 0 && (
        <Box sx={{ width: 240, flexShrink: 0, overflowY: 'auto', borderRight: 1, borderColor: 'divider', py: 0.5 }}>
          <Typography variant="overline" sx={{ display: 'block', px: 1, color: 'text.secondary', lineHeight: 2 }}>
            Components
          </Typography>
          {tree.map((n) => {
            const obj = gl.current.objByUuid.get(n.uuid)
            return (
              <Box key={n.uuid} sx={{ display: 'flex', alignItems: 'center', pl: n.depth * 2 }}>
                <Checkbox size="small" checked={obj ? obj.visible : true} onChange={() => toggleNode(n.uuid)} sx={{ p: 0.25 }} />
                <Typography variant="caption" noWrap title={n.name} sx={{ minWidth: 0 }}>
                  {n.name}
                </Typography>
              </Box>
            )
          })}
        </Box>
      )}

      <Box sx={{ position: 'relative', flex: 1, minWidth: 0, bgcolor: 'action.hover' }}>
        <Box ref={mountRef} sx={{ position: 'absolute', inset: 0, '& canvas': { display: 'block' } }} />

        {!loaded && !error && (
          <Box sx={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <CircularProgress size={28} />
          </Box>
        )}
        {error && (
          <Box sx={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', p: 2 }}>
            <Typography variant="body2" color="text.secondary" align="center">
              No 3D geometry to display.
              <br />
              This design may have been saved without cached graphics.
            </Typography>
          </Box>
        )}

        {loaded && (
          <Box
            sx={{
              position: 'absolute',
              top: 8,
              right: 8,
              display: 'flex',
              alignItems: 'center',
              gap: 1.5,
              px: 1.5,
              py: 0.5,
              borderRadius: 1,
              bgcolor: 'background.paper',
              opacity: 0.9,
              boxShadow: 1,
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Typography variant="caption" color="text.secondary">
                Brightness
              </Typography>
              <Slider
                size="small"
                min={0.2}
                max={2}
                step={0.05}
                value={brightness}
                onChange={(_, v) => setBrightness(v as number)}
                sx={{ width: 90 }}
              />
            </Box>
            <FormControlLabel
              control={<Checkbox size="small" checked={ao} onChange={(e) => setAo(e.target.checked)} sx={{ p: 0.25 }} />}
              label={<Typography variant="caption">AO</Typography>}
              sx={{ m: 0 }}
            />
            <FormControlLabel
              control={<Checkbox size="small" checked={edges} onChange={(e) => setEdges(e.target.checked)} sx={{ p: 0.25 }} />}
              label={<Typography variant="caption">Edges</Typography>}
              sx={{ m: 0 }}
            />
          </Box>
        )}
      </Box>
    </Box>
  )
}
