package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ZoneHeader is public provenance, never a trust grant. Serial is the SOA
// serial; ContentHash covers the exact body with the $ZONE line removed.
type ZoneHeader struct {
	ZoneID      string `json:"zone_id"`
	Name        string `json:"name"`
	SourceNode  string `json:"source_node,omitempty"`
	Generation  uint64 `json:"generation"`
	Serial      uint32 `json:"serial"`
	ContentHash string `json:"content_hash"`
}

var zoneIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ValidZoneID(id string) bool  { return zoneIDPattern.MatchString(id) }
func ZoneHash(body []byte) string { h := sha256.Sum256(body); return hex.EncodeToString(h[:]) }

// ParseZone is the opt-in zone parser. Legacy Load/Parse remain unchanged.
// Exactly one single-line $ZONE may follow SOA, before any ordinary records.
// Role and trust belong to local storage, not portable configuration.
func ParseZone(path string, src []byte, opt Options) (*ZoneHeader, *Config, []byte, error) {
	f, ds := Parse(path, src)
	if HasErrors(ds) {
		return nil, nil, nil, fmt.Errorf("invalid zone syntax: %s", ds[0].Code)
	}
	var header *ZoneHeader
	lines := strings.Split(string(src), "\n")
	soa, records := false, false
	for i := range f.Stmts {
		s := &f.Stmts[i]
		if len(s.Tokens) == 0 {
			continue
		}
		if s.isDirective() && s.directive().Name == "ZONE" {
			if header != nil || !soa || records || s.StartLine != s.EndLine {
				return nil, nil, nil, fmt.Errorf("$ZONE must occur once after SOA and before records")
			}
			h := &ZoneHeader{}
			seen := map[string]bool{}
			for _, t := range s.directive().Args {
				k, v, ok := t.option()
				if !ok || seen[k] {
					return nil, nil, nil, fmt.Errorf("invalid or duplicate zone field")
				}
				seen[k] = true
				switch k {
				case "zone_id":
					h.ZoneID = v
				case "name":
					h.Name = v
				case "source_node":
					h.SourceNode = v
				case "generation":
					n, e := strconv.ParseUint(v, 10, 64)
					if e != nil {
						return nil, nil, nil, fmt.Errorf("invalid generation")
					}
					h.Generation = n
				default:
					return nil, nil, nil, fmt.Errorf("unknown zone field %s", k)
				}
			}
			if !ValidZoneID(h.ZoneID) || !ValidName(h.Name) || h.Generation == 0 || (h.SourceNode != "" && !ValidName(h.SourceNode)) {
				return nil, nil, nil, fmt.Errorf("invalid zone identity")
			}
			header = h
			lines[s.StartLine-1] = "" // remove below, not a hash-sensitive blank
			lines = append(lines[:s.StartLine-1], lines[s.StartLine:]...)
		} else if r, ok := StmtRecord(s); ok {
			if r.Type == "SOA" {
				soa = true
			} else {
				records = true
			}
		}
	}
	body := []byte(strings.Join(lines, "\n"))
	_, cfg, ds := Load(path, body, opt)
	if HasErrors(ds) {
		for _, d := range ds {
			if d.Severity == SevError {
				return nil, nil, nil, fmt.Errorf("invalid zone configuration (%s)", d.Code)
			}
		}
	}
	if header != nil {
		header.Serial = cfg.Serial
		header.ContentHash = ZoneHash(body)
	}
	return header, cfg, body, nil
}
