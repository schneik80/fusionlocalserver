import { HTMLContainer, Rectangle2d, ShapeUtil, type TLShape } from 'tldraw'
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

// Cards render at a fixed, readable size; the underlying card components are
// designed for a ~420px column, not arbitrary scaling.
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
		// Pointer events are off until the shape is the only selection. Without
		// that, the card's own click targets would swallow the drag that moves
		// it; with it, one click selects and the next interacts (opening the
		// task/job dialog or jumping to the document).
		const interactive = this.editor.getOnlySelectedShapeId() === shape.id
		return (
			<HTMLContainer
				style={{
					width: shape.props.w,
					height: shape.props.h,
					pointerEvents: interactive ? 'all' : 'none',
					display: 'flex',
					alignItems: 'center',
					overflow: 'hidden',
				}}
			>
				<RefCard token={shape.props.token} />
			</HTMLContainer>
		)
	}

	override getIndicatorPath(shape: FlsCardShape) {
		const path = new Path2D()
		path.rect(0, 0, shape.props.w, shape.props.h)
		return path
	}
}
