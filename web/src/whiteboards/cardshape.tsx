import { useLayoutEffect, useRef } from 'react'
import { HTMLContainer, Rectangle2d, ShapeUtil, type Editor, type TLShape } from 'tldraw'
import { RefCard } from '../components/RefCard'

// The whiteboard's own shape: a live app card pinned to the canvas. Its only
// state is an fls: token — the same compact reference chat, the wiki and task
// bodies use — so a card placed here is not a screenshot of a task or a batch
// but the real thing, re-hydrated by RefCard on every render and always current.
//
// That reuse is the whole point: any card scheme the app gains later
// (fls:doc / fls:task / fls:job / fls:batch today) works on the whiteboard for
// free, because RefCard is the single place that maps a token to its renderer.

export const FLS_CARD_TYPE = 'fls-card'

declare module 'tldraw' {
	export interface TLGlobalShapePropsMap {
		[FLS_CARD_TYPE]: { w: number; h: number; token: string }
	}
}

export type FlsCardShape = TLShape<typeof FLS_CARD_TYPE>

// The size a card is CREATED at, and the cap it may grow to. The real card
// (RefCard) sizes to its content — a short name renders a narrow card — so after
// mount the shape measures the rendered card and shrinks its geometry to fit
// (see CardBody). CARD_W is the ceiling; CARD_H the initial guess before the
// first measure. Keeping geometry == paint is what makes bound arrows point at
// the card rather than at the empty right edge of a fixed 320 box.
export const CARD_W = 320
export const CARD_H = 96

export class FlsCardShapeUtil extends ShapeUtil<FlsCardShape> {
	static override type = FLS_CARD_TYPE

	override getDefaultProps(): FlsCardShape['props'] {
		return { w: CARD_W, h: CARD_H, token: '' }
	}

	// Cards are content, not geometry: let them move but not distort, so a task
	// card never ends up squashed into an unreadable sliver.
	override canResize() {
		return false
	}

	override getGeometry(shape: FlsCardShape) {
		return new Rectangle2d({
			width: shape.props.w,
			height: shape.props.h,
			isFilled: true,
		})
	}

	override component(shape: FlsCardShape) {
		return <CardBody editor={this.editor} shape={shape} />
	}

	override getIndicatorPath(shape: FlsCardShape) {
		const path = new Path2D()
		path.rect(0, 0, shape.props.w, shape.props.h)
		return path
	}
}

// CardBody renders the card and keeps the SHAPE's geometry equal to the card's
// actual painted size. RefCard sizes to content, so a fixed 320x96 shape left a
// short card floating in an oversized box — the selection border and (worse) a
// bound arrow's anchor sat at the box's centre, out in empty space. Measuring
// the rendered card and syncing w/h back onto the shape makes the box hug the
// card, so arrows land on it.
function CardBody({ editor, shape }: { editor: Editor; shape: FlsCardShape }) {
	const ref = useRef<HTMLDivElement>(null)
	// Pointer events are off until the shape is the only selection. Without that,
	// the card's own click targets would swallow the drag that moves it; with it,
	// one click selects and the next interacts (opening a dialog / navigating).
	const interactive = editor.getOnlySelectedShapeId() === shape.id

	useLayoutEffect(() => {
		const el = ref.current
		if (!el) return
		const sync = () => {
			const w = Math.min(CARD_W, Math.ceil(el.offsetWidth))
			const h = Math.ceil(el.offsetHeight)
			if (!w || !h) return
			// Within a pixel is a match — don't churn the store on rounding.
			if (Math.abs(w - shape.props.w) <= 1 && Math.abs(h - shape.props.h) <= 1) return
			// Shrink from the centre, not the top-left, so a card stays put over
			// its intended spot — otherwise a whole placed tree drifts up-left as
			// its cards measure smaller than the 320x96 they were created at.
			const x = shape.x - (w - shape.props.w) / 2
			const y = shape.y - (h - shape.props.h) / 2
			// history:'ignore' — an auto-measure is not a user edit, so it must
			// not land on the undo stack. It still persists via autosave, so a
			// reloaded board opens already the right size and re-measures to a
			// no-op (the guard above), leaving no churn.
			editor.run(
				() =>
					editor.updateShape<FlsCardShape>({
						id: shape.id,
						type: FLS_CARD_TYPE,
						x,
						y,
						props: { w, h },
					}),
				{ history: 'ignore' },
			)
		}
		sync()
		// The measured node is `max-content`, so its size tracks the card's
		// content, not the shape box — resizing the shape can't feed back into it.
		const ro = new ResizeObserver(sync)
		ro.observe(el)
		return () => ro.disconnect()
	}, [editor, shape.id, shape.props.w, shape.props.h, shape.props.token])

	return (
		<HTMLContainer
			style={{
				width: shape.props.w,
				height: shape.props.h,
				pointerEvents: interactive ? 'all' : 'none',
				display: 'flex',
				alignItems: 'center',
				overflow: 'visible',
			}}
		>
			<div ref={ref} style={{ width: 'max-content', maxWidth: CARD_W, display: 'flex', alignItems: 'center' }}>
				<RefCard token={shape.props.token} />
			</div>
		</HTMLContainer>
	)
}
