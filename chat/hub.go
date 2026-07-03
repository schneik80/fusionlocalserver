package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Hub is the in-process SSE fan-out (docs/chat/PLAN.md phase 2): one
// subscriber per open browser tab per project, one short ring buffer per
// project for reconnect replay. The store, not the hub, is the source of
// truth — anything the ring can't replay is answered with a reset frame and
// the client refetches over REST, so losing hub state can never lose data.
//
// Event ids are "<epoch>-<seq>". seq increments per published event; epoch
// comes from the store and changes every process run, which is how a
// reconnecting client's Last-Event-ID from before a restart is detected as
// unusable (parseable, wrong epoch → reset).
type Hub struct {
	authz *Authorizer
	epoch func(projectID string) (int64, error)

	mu       sync.Mutex
	projects map[string]*hubProject
}

const (
	ringCap = 512
	ringTTL = 10 * time.Minute

	// subBuf is a subscriber's channel depth. A subscriber too slow to
	// drain it is closed rather than allowed to stall publishes — its
	// EventSource reconnects and resyncs through the ring (or a reset).
	subBuf = 64
)

// Event is the wire envelope for every chat SSE event (design doc §3).
type Event struct {
	Type string `json:"type"`
	V    int    `json:"v"`
	Data any    `json:"data"`
}

// Vis controls which of a project's subscribers may see an event. The zero
// value is public (every project subscriber). Private events carry a
// snapshot of the channel whose ACL governs them; ExtraUserIDs are always
// delivered regardless of the ACL (e.g. a just-removed member learning of
// their removal). UserOnly events reach ONLY the ExtraUserIDs — no channel
// or role fallback — which is how per-user events (read.updated syncing a
// user's own tabs) stay invisible to everyone else.
type Vis struct {
	Private      bool
	Channel      Channel
	ExtraUserIDs []string
	UserOnly     bool
}

// Frame is one rendered SSE event: the id to send, the JSON payload, and
// the visibility the writer must enforce (via Hub.Entitled) before writing
// it to a particular subscriber.
type Frame struct {
	ID   string
	Data []byte
	vis  Vis
}

// Subscriber is one open event stream. Events arrive on Events(); Closed()
// fires when the hub disconnects the subscriber (shutdown, overflow).
type Subscriber struct {
	ch   chan Frame
	quit chan struct{}
	once sync.Once
}

func (s *Subscriber) Events() <-chan Frame    { return s.ch }
func (s *Subscriber) Closed() <-chan struct{} { return s.quit }

func (s *Subscriber) close() {
	s.once.Do(func() { close(s.quit) })
}

type ringEntry struct {
	seq  int64
	at   time.Time
	data []byte
	vis  Vis
}

type hubProject struct {
	epoch int64
	seq   int64
	ring  []ringEntry
	subs  map[*Subscriber]struct{}
}

// NewHub wires a Hub to the authorizer (for per-subscriber visibility
// checks) and an epoch source (Store.EventEpoch).
func NewHub(authz *Authorizer, epoch func(projectID string) (int64, error)) *Hub {
	return &Hub{authz: authz, epoch: epoch, projects: make(map[string]*hubProject)}
}

// Publish renders ev once, appends it to the project's ring, and fans it
// out. Slow subscribers are disconnected rather than waited on. Publish
// never blocks on entitlement checks — those happen in each subscriber's
// writer via Entitled.
func (h *Hub) Publish(projectID string, ev Event, vis Vis) error {
	return h.publish(projectID, ev, vis, false)
}

// PublishEphemeral fans ev out WITHOUT touching the ring or the id
// sequence: the frame carries no id, so it never advances a client's
// Last-Event-ID and is never replayed after a reconnect. Typing indicators
// only (docs/chat/PLAN.md phase 4) — anything durable goes via Publish.
func (h *Hub) PublishEphemeral(projectID string, ev Event, vis Vis) error {
	return h.publish(projectID, ev, vis, true)
}

func (h *Hub) publish(projectID string, ev Event, vis Vis, ephemeral bool) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	hp, err := h.projectLocked(projectID)
	if err != nil {
		return err
	}
	f := Frame{Data: data, vis: vis}
	if !ephemeral {
		hp.seq++
		now := time.Now()
		hp.ring = append(hp.ring, ringEntry{seq: hp.seq, at: now, data: data, vis: vis})
		for len(hp.ring) > 0 && (len(hp.ring) > ringCap || now.Sub(hp.ring[0].at) > ringTTL) {
			hp.ring = hp.ring[1:]
		}
		f.ID = eventID(hp.epoch, hp.seq)
	}
	for sub := range hp.subs {
		select {
		case sub.ch <- f:
		default:
			delete(hp.subs, sub)
			sub.close()
		}
	}
	return nil
}

// Subscribe registers a stream for the project. lastEventID (may be empty)
// is the client's SSE cursor: when it parses to the current epoch and the
// ring still covers everything after it, the missed frames are returned for
// replay; otherwise reset=true tells the caller to instruct a full REST
// resync. Replay frames still need per-frame Entitled filtering.
func (h *Hub) Subscribe(projectID, lastEventID string) (sub *Subscriber, replay []Frame, reset bool, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hp, err := h.projectLocked(projectID)
	if err != nil {
		return nil, nil, false, err
	}
	sub = &Subscriber{ch: make(chan Frame, subBuf), quit: make(chan struct{})}
	hp.subs[sub] = struct{}{}

	if lastEventID == "" {
		return sub, nil, false, nil
	}
	epoch, seq, ok := parseEventID(lastEventID)
	switch {
	case !ok, epoch != hp.epoch, seq > hp.seq:
		return sub, nil, true, nil
	case seq == hp.seq:
		return sub, nil, false, nil
	case len(hp.ring) == 0 || hp.ring[0].seq > seq+1:
		// The events after seq are (partly) gone from the ring.
		return sub, nil, true, nil
	}
	for _, e := range hp.ring {
		if e.seq > seq {
			replay = append(replay, Frame{ID: eventID(hp.epoch, e.seq), Data: e.data, vis: e.vis})
		}
	}
	return sub, replay, false, nil
}

// Unsubscribe drops the subscriber (idempotent; also safe after CloseAll).
func (h *Hub) Unsubscribe(projectID string, sub *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if hp, ok := h.projects[projectID]; ok {
		delete(hp.subs, sub)
	}
	sub.close()
}

// Entitled reports whether a subscriber identified by (token, id) may see
// the frame: public frames always, private frames per the channel's
// two-layer rule, ExtraUserIDs unconditionally (matched on user id, or on
// email for sessions predating the Sub claim), UserOnly frames exclusively
// so. May fetch the roster when the authorizer's cache has lapsed — call
// it from the subscriber's own writer goroutine, never from Publish.
func (h *Hub) Entitled(ctx context.Context, token string, id Identity, projectID string, f Frame) (bool, error) {
	for _, u := range f.vis.ExtraUserIDs {
		if u == "" {
			continue
		}
		if u == id.UserID || (id.Email != "" && strings.EqualFold(u, id.Email)) {
			return true, nil
		}
	}
	if f.vis.UserOnly {
		return false, nil
	}
	if !f.vis.Private {
		return true, nil
	}
	return h.authz.CanAccessChannel(ctx, token, id, projectID, f.vis.Channel)
}

// CloseAll disconnects every subscriber (server drain/rebind). The hub
// stays usable — a rebind serves new subscriptions immediately after.
func (h *Hub) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, hp := range h.projects {
		for sub := range hp.subs {
			sub.close()
		}
		hp.subs = make(map[*Subscriber]struct{})
	}
}

// projectLocked resolves per-project hub state, pulling the epoch from the
// store on first touch. Called under h.mu.
func (h *Hub) projectLocked(projectID string) (*hubProject, error) {
	if hp, ok := h.projects[projectID]; ok {
		return hp, nil
	}
	epoch, err := h.epoch(projectID)
	if err != nil {
		return nil, err
	}
	hp := &hubProject{epoch: epoch, subs: make(map[*Subscriber]struct{})}
	h.projects[projectID] = hp
	return hp, nil
}

func eventID(epoch, seq int64) string {
	return fmt.Sprintf("%d-%d", epoch, seq)
}

func parseEventID(id string) (epoch, seq int64, ok bool) {
	dash := strings.IndexByte(id, '-')
	if dash <= 0 || dash == len(id)-1 {
		return 0, 0, false
	}
	epoch, err1 := strconv.ParseInt(id[:dash], 10, 64)
	seq, err2 := strconv.ParseInt(id[dash+1:], 10, 64)
	return epoch, seq, err1 == nil && err2 == nil
}
