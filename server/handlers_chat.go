package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/schneik80/fusionlocalserver/chat"
)

// Chat endpoints (docs/chat/PLAN.md, phase 1). Every handler front-doors
// through the chat authorizer — the caller's APS project role mapped to
// capabilities, plus the private-channel ACL — before touching the store;
// there is no parallel permission system. Reads and writes are REST; the
// SPA polls for updates until the SSE stream lands in phase 2.
//
// Because private channels must not leak their existence, a channel the
// caller cannot access answers 404 (not 403) everywhere.

// chatMaxBody caps every chat request body (same 64 KiB cap as pins).
const chatMaxBody = 64 << 10

// chatCtx is what every chat handler resolves first: the caller's token,
// identity, session id (the rate-limit key), and the project in question.
type chatCtx struct {
	projectID string
	token     string
	id        chat.Identity
	sessID    string
}

// chatReq gates a chat request: store available, session + token present,
// projectId given. Writes the error response itself when not ok.
func (s *Server) chatReq(w http.ResponseWriter, r *http.Request) (chatCtx, bool) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat storage is unavailable on this server")
		return chatCtx{}, false
	}
	tok, ok := s.token(r.Context(), w, r)
	if !ok {
		return chatCtx{}, false
	}
	sess, ok := sessionFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return chatCtx{}, false
	}
	projectID, ok := reqParam(w, r, "projectId")
	if !ok {
		return chatCtx{}, false
	}
	return chatCtx{
		projectID: projectID,
		token:     tok,
		id:        chat.Identity{UserID: sess.Profile.Sub, Email: sess.Profile.Email},
		sessID:    sess.ID,
	}, true
}

// chatCan enforces a capability, writing 403 (or the fetch failure) itself.
func (s *Server) chatCan(ctx context.Context, w http.ResponseWriter, r *http.Request, c chatCtx, cap chat.Capability) bool {
	ok, err := s.chatAuthz.Can(ctx, c.token, c.id, c.projectID, cap)
	if err != nil {
		s.fail(w, r, err)
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, safeErrorMessage(http.StatusForbidden))
		return false
	}
	return true
}

// chatChannel resolves a channel the caller may access. Inaccessible
// private channels 404 like unknown ones, hiding their existence.
func (s *Server) chatChannel(ctx context.Context, w http.ResponseWriter, r *http.Request, c chatCtx, channelID string) (chat.Channel, bool) {
	ch, err := s.chat.GetChannel(c.projectID, channelID)
	if err != nil {
		s.chatError(w, r, err)
		return chat.Channel{}, false
	}
	ok, err := s.chatAuthz.CanAccessChannel(ctx, c.token, c.id, c.projectID, ch)
	if err != nil {
		s.fail(w, r, err)
		return chat.Channel{}, false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no such channel")
		return chat.Channel{}, false
	}
	return ch, true
}

// chatModOrCreator reports whether the caller moderates the project or
// created the channel — the bar for rename/archive/member management.
func (s *Server) chatModOrCreator(ctx context.Context, w http.ResponseWriter, r *http.Request, c chatCtx, ch chat.Channel) (bool, bool) {
	mod, err := s.chatAuthz.Can(ctx, c.token, c.id, c.projectID, chat.CapModerate)
	if err != nil {
		s.fail(w, r, err)
		return false, false
	}
	return mod || (ch.CreatedBy != "" && ch.CreatedBy == c.id.UserID), true
}

// chatError maps store errors onto the uniform envelope. Store sentinel
// texts are our own and safe to echo; raw I/O errors are not (they carry
// filesystem paths), so those log fully and answer generically.
func (s *Server) chatError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, chat.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, chat.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, chat.ErrFutureVersion):
		s.logger.Error("chat: refusing data from a newer version", "err", err)
		writeError(w, http.StatusServiceUnavailable, "chat data on this server was written by a newer version")
	default:
		s.logger.Error("chat: storage error", "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "chat storage error")
	}
}

func decodeChatBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, chatMaxBody)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// reqSeq reads a required int64 query parameter (message seq).
func reqSeq(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	raw, ok := reqParam(w, r, name)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return n, true
}

// ---- channels ----

// handleChatChannels lists the channels the caller can see: all public
// ones, plus private ones where they're a member (or project moderator).
func (s *Server) handleChatChannels(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	if !s.chatCan(ctx, w, r, c, chat.CapRead) {
		return
	}
	chans, err := s.chat.Channels(c.projectID)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	out := []ChannelDTO{}
	for _, ch := range chans {
		if ch.IsPrivate {
			visible, aerr := s.chatAuthz.CanAccessChannel(ctx, c.token, c.id, c.projectID, ch)
			if aerr != nil {
				s.fail(w, r, aerr)
				return
			}
			if !visible {
				continue
			}
		}
		out = append(out, channelDTO(ch))
	}
	caps := ChatCapsDTO{}
	for _, probe := range []struct {
		cap chat.Capability
		dst *bool
	}{
		{chat.CapPost, &caps.Post},
		{chat.CapCreateChannel, &caps.CreateChannel},
		{chat.CapModerate, &caps.Moderate},
	} {
		v, cerr := s.chatAuthz.Can(ctx, c.token, c.id, c.projectID, probe.cap)
		if cerr != nil {
			s.fail(w, r, cerr)
			return
		}
		*probe.dst = v
	}
	writeJSON(w, http.StatusOK, ChannelListDTO{Channels: out, Capabilities: caps})
}

func (s *Server) handleChatChannelCreate(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	if !s.chatCan(ctx, w, r, c, chat.CapCreateChannel) {
		return
	}
	if !s.chatOpLim.Allow(c.sessID) {
		writeError(w, http.StatusTooManyRequests, safeErrorMessage(http.StatusTooManyRequests))
		return
	}
	var in struct {
		Name      string   `json:"name"`
		Topic     string   `json:"topic"`
		IsPrivate bool     `json:"isPrivate"`
		MemberIDs []string `json:"memberIds"`
	}
	if !decodeChatBody(w, r, &in) {
		return
	}
	if in.IsPrivate && c.id.UserID == "" {
		// Without a stable user id there is nothing to key the ACL on.
		writeError(w, http.StatusBadRequest, "your session has no user id; sign out and back in to create private channels")
		return
	}
	ch, err := s.chat.CreateChannel(c.projectID, in.Name, in.Topic, c.id.UserID, in.IsPrivate, in.MemberIDs)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, channelDTO(ch))
}

func (s *Server) handleChatChannelUpdate(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}
	allowed, ok := s.chatModOrCreator(ctx, w, r, c, ch)
	if !ok {
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, safeErrorMessage(http.StatusForbidden))
		return
	}
	if !s.chatOpLim.Allow(c.sessID) {
		writeError(w, http.StatusTooManyRequests, safeErrorMessage(http.StatusTooManyRequests))
		return
	}
	var in struct {
		Name  *string `json:"name"`
		Topic *string `json:"topic"`
	}
	if !decodeChatBody(w, r, &in) {
		return
	}
	updated, err := s.chat.UpdateChannel(c.projectID, ch.ID, in.Name, in.Topic)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, channelDTO(updated))
}

// handleChatChannelArchive is DELETE on a channel: soft-archive (the root
// channel refuses, per the store invariant).
func (s *Server) handleChatChannelArchive(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}
	allowed, ok := s.chatModOrCreator(ctx, w, r, c, ch)
	if !ok {
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, safeErrorMessage(http.StatusForbidden))
		return
	}
	archived, err := s.chat.ArchiveChannel(c.projectID, ch.ID)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, channelDTO(archived))
}

func (s *Server) handleChatMemberAdd(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}
	allowed, ok := s.chatModOrCreator(ctx, w, r, c, ch)
	if !ok {
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, safeErrorMessage(http.StatusForbidden))
		return
	}
	var in struct {
		UserID string `json:"userId"`
	}
	if !decodeChatBody(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.UserID) == "" {
		writeError(w, http.StatusBadRequest, "userId is required")
		return
	}
	// The target must be an active member of the project (design doc §REST).
	target := chat.Identity{UserID: in.UserID}
	isMember, err := s.chatAuthz.Can(ctx, c.token, target, c.projectID, chat.CapRead)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !isMember {
		writeError(w, http.StatusBadRequest, "user is not an active member of this project")
		return
	}
	updated, err := s.chat.AddChannelMember(c.projectID, ch.ID, in.UserID, c.id.UserID)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, channelDTO(updated))
}

func (s *Server) handleChatMemberRemove(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	userID, ok := reqParam(w, r, "userId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}
	if userID != c.id.UserID { // anyone may leave; removing others takes moderator/creator
		allowed, ok := s.chatModOrCreator(ctx, w, r, c, ch)
		if !ok {
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, safeErrorMessage(http.StatusForbidden))
			return
		}
	}
	updated, err := s.chat.RemoveChannelMember(c.projectID, ch.ID, userID)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, channelDTO(updated))
}

// ---- messages ----

// handleChatMessages reads a channel. Without cursors it returns the newest
// page of the top-level timeline; beforeSeq pages history backward;
// afterSeq returns the polling delta (every message, replies included,
// newer than the cursor).
func (s *Server) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}

	q := r.URL.Query()
	var msgs []chat.Message
	var err error
	if raw := q.Get("afterSeq"); raw != "" {
		after, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || after < 0 {
			writeError(w, http.StatusBadRequest, "invalid afterSeq")
			return
		}
		msgs, err = s.chat.ListMessagesAfter(c.projectID, ch.ID, after)
	} else {
		var before int64
		if raw := q.Get("beforeSeq"); raw != "" {
			before, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || before <= 0 {
				writeError(w, http.StatusBadRequest, "invalid beforeSeq")
				return
			}
		}
		limit := 0
		if raw := q.Get("limit"); raw != "" {
			limit, err = strconv.Atoi(raw)
			if err != nil || limit <= 0 {
				writeError(w, http.StatusBadRequest, "invalid limit")
				return
			}
		}
		msgs, err = s.chat.ListMessages(c.projectID, ch.ID, before, limit)
	}
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	latest, err := s.chat.LatestSeq(c.projectID, ch.ID)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, messageListDTO(msgs, latest))
}

// handleChatMessageCreate posts a message (or thread reply). clientMsgId
// dedupes retries: a replay answers 200 with the original message where a
// fresh create answers 201.
func (s *Server) handleChatMessageCreate(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	if !s.chatCan(ctx, w, r, c, chat.CapPost) {
		return
	}
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}
	if !s.chatMsgLim.Allow(c.sessID) {
		writeError(w, http.StatusTooManyRequests, safeErrorMessage(http.StatusTooManyRequests))
		return
	}
	var in struct {
		Body          string `json:"body"`
		ClientMsgID   string `json:"clientMsgId"`
		ThreadRootSeq int64  `json:"threadRootSeq"`
	}
	if !decodeChatBody(w, r, &in) {
		return
	}
	authorName := c.id.Email
	if sess, ok := sessionFromCtx(r.Context()); ok && sess.Profile.Name != "" {
		authorName = sess.Profile.Name
	}
	msg, created, err := s.chat.CreateMessage(c.projectID, ch.ID, c.id.UserID, authorName, in.ClientMsgID, in.Body, in.ThreadRootSeq)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, messageDTO(msg))
}

// handleChatMessageEdit edits the caller's own message (moderators delete,
// they don't rewrite others' words — per the design doc's capability table).
func (s *Server) handleChatMessageEdit(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	seq, ok := reqSeq(w, r, "seq")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	if !s.chatCan(ctx, w, r, c, chat.CapEditOwn) {
		return
	}
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}
	msg, err := s.chat.GetMessage(c.projectID, ch.ID, seq)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	if msg.AuthorID == "" || msg.AuthorID != c.id.UserID {
		writeError(w, http.StatusForbidden, "you can only edit your own messages")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decodeChatBody(w, r, &in) {
		return
	}
	updated, err := s.chat.EditMessage(c.projectID, ch.ID, seq, in.Body)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, messageDTO(updated))
}

// handleChatMessageDelete soft-deletes: the author may delete their own
// message; moderators may delete anyone's.
func (s *Server) handleChatMessageDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	seq, ok := reqSeq(w, r, "seq")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}
	msg, err := s.chat.GetMessage(c.projectID, ch.ID, seq)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	own := msg.AuthorID != "" && msg.AuthorID == c.id.UserID
	if own {
		if !s.chatCan(ctx, w, r, c, chat.CapEditOwn) {
			return
		}
	} else if !s.chatCan(ctx, w, r, c, chat.CapModerate) {
		return
	}
	deleted, err := s.chat.DeleteMessage(c.projectID, ch.ID, seq)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, messageDTO(deleted))
}

// handleChatThread returns a thread: root message first, replies ascending.
func (s *Server) handleChatThread(w http.ResponseWriter, r *http.Request) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	rootSeq, ok := reqSeq(w, r, "rootSeq")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}
	msgs, err := s.chat.ListThread(c.projectID, ch.ID, rootSeq)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	latest, err := s.chat.LatestSeq(c.projectID, ch.ID)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, messageListDTO(msgs, latest))
}

// ---- reactions ----

func (s *Server) handleChatReactionAdd(w http.ResponseWriter, r *http.Request) {
	s.handleChatReaction(w, r, s.chat.AddReaction)
}

func (s *Server) handleChatReactionRemove(w http.ResponseWriter, r *http.Request) {
	s.handleChatReaction(w, r, s.chat.RemoveReaction)
}

func (s *Server) handleChatReaction(w http.ResponseWriter, r *http.Request, apply func(projectID, channelID string, seq int64, userID, emoji string) (chat.Message, error)) {
	c, ok := s.chatReq(w, r)
	if !ok {
		return
	}
	channelID, ok := reqParam(w, r, "channelId")
	if !ok {
		return
	}
	seq, ok := reqSeq(w, r, "seq")
	if !ok {
		return
	}
	emoji, ok := reqParam(w, r, "emoji")
	if !ok {
		return
	}
	if len(emoji) > 64 {
		writeError(w, http.StatusBadRequest, "invalid emoji")
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	if !s.chatCan(ctx, w, r, c, chat.CapReact) {
		return
	}
	ch, ok := s.chatChannel(ctx, w, r, c, channelID)
	if !ok {
		return
	}
	if c.id.UserID == "" {
		writeError(w, http.StatusBadRequest, "your session has no user id; sign out and back in to react")
		return
	}
	msg, err := apply(c.projectID, ch.ID, seq, c.id.UserID, emoji)
	if err != nil {
		s.chatError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, messageDTO(msg))
}
