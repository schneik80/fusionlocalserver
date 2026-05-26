package server

import (
	"encoding/json"
	"net/http"

	"github.com/schneik80/FusionDataCLI/pins"
)

// Pins are stored per-hub in ~/.config/fusiondatacli/pins-<hub>.json. The
// mutate endpoints follow a Load -> mutate -> Save cycle that is not atomic on
// disk, so s.pinsMu serialises them to prevent a lost update when two clients
// pin concurrently. pins.Pin already carries JSON tags, so it doubles as the
// wire type for both responses and the POST body.

// handlePinsList -> pins.Load (query: hubId).
func (s *Server) handlePinsList(w http.ResponseWriter, r *http.Request) {
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
	s.pinsMu.Lock()
	ps, err := pins.Load(hubID)
	s.pinsMu.Unlock()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if ps == nil {
		ps = []pins.Pin{}
	}
	writeJSON(w, http.StatusOK, ps)
}

// handlePinsAdd validates and adds a pin (query: hubId; body: pin record). The
// body mirrors the TUI's pin capture — id, name, kind, project_id,
// project_alt_id, folder_path — so the bookmark stays navigable without an API
// call. The hub scope always comes from the query param, not the body.
func (s *Server) handlePinsAdd(w http.ResponseWriter, r *http.Request) {
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}

	var pin pins.Pin
	if err := json.NewDecoder(r.Body).Decode(&pin); err != nil {
		writeError(w, http.StatusBadRequest, "invalid pin body: "+err.Error())
		return
	}
	if pin.ID == "" {
		writeError(w, http.StatusBadRequest, "pin id is required")
		return
	}
	if !pins.IsPinnable(pin.Kind) {
		writeError(w, http.StatusBadRequest, "items of kind "+pin.Kind+" cannot be pinned")
		return
	}
	pin.HubID = hubID

	s.pinsMu.Lock()
	defer s.pinsMu.Unlock()

	ps, err := pins.Load(hubID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ps = pins.Add(ps, pin)
	if err := pins.Save(hubID, ps); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

// handlePinsRemove -> Load + Remove + Save (query: hubId, id).
func (s *Server) handlePinsRemove(w http.ResponseWriter, r *http.Request) {
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
	id, ok := reqParam(w, r, "id")
	if !ok {
		return
	}

	s.pinsMu.Lock()
	defer s.pinsMu.Unlock()

	ps, err := pins.Load(hubID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ps = pins.Remove(ps, id)
	if err := pins.Save(hubID, ps); err != nil {
		s.fail(w, r, err)
		return
	}
	if ps == nil {
		ps = []pins.Pin{}
	}
	writeJSON(w, http.StatusOK, ps)
}
