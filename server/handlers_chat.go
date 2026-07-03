package server

import "net/http"

// Chat endpoints (docs/chat/PLAN.md, phase 1). Scaffold only: the full
// surface is
//
//	GET/POST      /api/chat/channels?projectId=
//	PATCH/DELETE  /api/chat/channels?projectId=&channelId=
//	POST/DELETE   /api/chat/channels/members?projectId=&channelId=[&userId=]
//	GET/POST      /api/chat/messages?projectId=&channelId=[&beforeSeq=&afterSeq=&limit=]
//	PATCH/DELETE  /api/chat/messages?projectId=&channelId=&seq=
//	GET           /api/chat/thread?projectId=&channelId=&rootSeq=
//	POST/DELETE   /api/chat/reactions?projectId=&channelId=&seq=
//
// Every handler will front-door through the chat authorizer (APS project
// role → capability, plus the private-channel ACL) before touching the
// store — no parallel permission system.

// handleChatChannels is a placeholder so the route shape is reviewable;
// phase 1 replaces it with the real store/authorizer-backed handler.
func (s *Server) handleChatChannels(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "chat is not implemented yet")
}
