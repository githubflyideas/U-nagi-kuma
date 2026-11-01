package server

import (
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/githubflyideas/unagikuma/internal/app"
)

// Never serialize ZoneRecord: Body can contain plaintext credentials.
func (s *Server) zoneView(z app.ZoneRecord, applied string) map[string]any {
	return map[string]any{"id": z.Header.ZoneID, "name": z.Header.Name, "serial": z.Header.Serial,
		"generation": z.Header.Generation, "revision": `"` + z.Revision() + `"`, "applied_revision": applied,
		"role": z.Role(s.app.Config().Config.MName), "source_node": z.Binding.SourceNode,
		"fingerprint": z.Binding.Fingerprint, "sync_status": "not_connected"}
}

var zoneMissing = regexp.MustCompile(`^(?:missing|unavailable) zone dependency ((?:secret|env):[A-Za-z0-9_.-]+)$`)

func zoneError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, app.ErrConflict):
		return Errorf(412, CodeChanged, "zone revision changed; refresh and review before retrying")
	case errors.Is(err, app.ErrZoneNameConflict):
		return Errorf(409, CodeConflict, "zone name already exists; choose another name or explicitly update its ID")
	case errors.Is(err, app.ErrReadonly), errors.Is(err, app.ErrSuperseded):
		return Errorf(409, CodeConfigReadonly, "zone cannot be changed in its current role")
	case errors.Is(err, os.ErrNotExist):
		return notFound("zone")
	}
	if m := zoneMissing.FindStringSubmatch(err.Error()); m != nil {
		return &APIError{Status: 400, Code: CodeInvalid, Message: "zone dependency unavailable; provision it locally before applying", Extra: map[string]any{"missing": []string{m[1]}}}
	}
	// Parser and filesystem errors may contain user input, URLs or local paths.
	return Errorf(400, CodeInvalid, "zone operation rejected; check configuration, dependencies and source binding")
}
func zoneRevision(r *http.Request) (string, error) {
	v := r.Header.Get("If-Match")
	if v == "" {
		return "", Errorf(428, CodeChanged, "If-Match zone revision required")
	}
	if len(v) != 66 || v[0] != '"' || v[65] != '"' {
		return "", Errorf(412, CodeChanged, "invalid zone revision; refresh before retrying")
	}
	for _, c := range v[1:65] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", Errorf(412, CodeChanged, "invalid zone revision")
		}
	}
	return v[1:65], nil
}
func (s *Server) zoneDecode(r *http.Request, v any) error {
	if err := decodeJSON(r, min(s.maxBody(), int64(2*app.MaxZoneBytes)), v); err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.Status == 413 {
			return err
		}
		return Errorf(400, CodeInvalid, "invalid zone request")
	}
	return nil
}
func (s *Server) zonesList(w http.ResponseWriter, r *http.Request) (any, error) {
	zs, err := s.app.ListZones()
	if err != nil {
		return nil, zoneError(err)
	}
	out := make([]map[string]any, 0, len(zs))
	for _, z := range zs {
		applied := ""
		if z.AppliedRevision != "" {
			applied = `"` + z.AppliedRevision + `"`
		}
		out = append(out, s.zoneView(z.Record, applied))
	}
	return map[string]any{"items": out}, nil
}
func (s *Server) zoneImportRequest(r *http.Request) ([]byte, app.ZoneImportOptions, error) {
	var req struct {
		Text     string `json:"text"`
		Name     string `json:"name"`
		TargetID string `json:"target_id"`
	}
	if err := s.zoneDecode(r, &req); err != nil {
		return nil, app.ZoneImportOptions{}, err
	}
	if len(req.Text) > app.MaxZoneBytes {
		return nil, app.ZoneImportOptions{}, Errorf(413, CodeTooLarge, "zone text too large")
	}
	o := app.ZoneImportOptions{Name: req.Name, TargetID: req.TargetID}
	if o.TargetID != "" {
		var err error
		o.ExpectedRevision, err = zoneRevision(r)
		if err != nil {
			return nil, o, err
		}
	}
	return []byte(req.Text), o, nil
}
func (s *Server) zonesPreview(w http.ResponseWriter, r *http.Request) (any, error) {
	raw, o, err := s.zoneImportRequest(r)
	if err != nil {
		return nil, err
	}
	v, err := s.app.PreviewZone(raw, o)
	return v, zoneError(err)
}
func (s *Server) zonesImport(w http.ResponseWriter, r *http.Request) (any, error) {
	raw, o, err := s.zoneImportRequest(r)
	if err != nil {
		return nil, err
	}
	z, err := s.app.ImportZone(raw, o)
	if err != nil {
		return nil, zoneError(err)
	}
	w.Header().Set("ETag", `"`+z.Revision()+`"`)
	return s.zoneView(z, ""), nil
}
func (s *Server) zonesApply(w http.ResponseWriter, r *http.Request) (any, error) {
	rev, err := zoneRevision(r)
	if err != nil {
		return nil, err
	}
	var req struct{}
	if err = s.zoneDecode(r, &req); err != nil {
		return nil, err
	}
	z, err := s.app.ApplyZone(r.PathValue("id"), rev)
	if err != nil {
		return nil, zoneError(err)
	}
	w.Header().Set("ETag", `"`+z.Revision()+`"`)
	return s.zoneView(z, `"`+z.Revision()+`"`), nil
}
func (s *Server) zonesExport(w http.ResponseWriter, r *http.Request) (any, error) {
	v, err := s.app.ExportZone(r.PathValue("id"))
	return v, zoneError(err)
}
func (s *Server) zonesRebind(w http.ResponseWriter, r *http.Request) (any, error) {
	rev, err := zoneRevision(r)
	if err != nil {
		return nil, err
	}
	var req struct {
		SourceNode  string `json:"source_node"`
		Fingerprint string `json:"fingerprint"`
		Confirm     bool   `json:"confirm"`
	}
	if err = s.zoneDecode(r, &req); err != nil {
		return nil, err
	}
	if !req.Confirm {
		return nil, Errorf(400, CodeInvalid, "explicit source and fingerprint confirmation required")
	}
	z, err := s.app.RebindZone(r.PathValue("id"), rev, app.ZoneBinding{SourceNode: req.SourceNode, Fingerprint: req.Fingerprint})
	if err != nil {
		return nil, zoneError(err)
	}
	w.Header().Set("ETag", `"`+z.Revision()+`"`)
	return s.zoneView(z, ""), nil
}
