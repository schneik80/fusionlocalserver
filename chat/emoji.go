package chat

// AllowedEmoji is the server-side reaction allowlist (docs/chat/PLAN.md
// phase 4): reactions are picked from a fixed palette, not typed, so the
// server refuses anything else on ADD. Removal is deliberately not
// allowlist-checked — a reaction that got in under an older palette must
// stay removable.
var AllowedEmoji = []string{
	"👍", "👎", "❤️", "😄", "🎉", "🚀", "👀", "✅", "❓", "😕",
}

var allowedEmojiSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(AllowedEmoji))
	for _, e := range AllowedEmoji {
		m[e] = struct{}{}
	}
	return m
}()

// IsAllowedEmoji reports whether e is in the reaction palette.
func IsAllowedEmoji(e string) bool {
	_, ok := allowedEmojiSet[e]
	return ok
}
