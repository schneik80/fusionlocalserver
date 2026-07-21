// Shared timings for the app's tab motion, so the project tabs and a document's
// detail tabs can't drift apart.
//
// Below MUI's 225/195 default: switching a tab is a far more frequent gesture
// than drilling down, and it should read as a nudge rather than an animation.
// Still well above the 100-120 ms `sx` transitions reserved for hover and other
// small state changes.
export const TAB_SLIDE_TIMEOUT = { enter: 180, exit: 150 }
