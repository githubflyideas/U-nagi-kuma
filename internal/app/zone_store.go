package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/githubflyideas/unagikuma/internal/config"
)

const MaxZoneBytes = 1 << 20

var ErrZoneNameConflict = errors.New("zone name already exists; choose another name or explicitly update its ID")

// ZoneBinding is local administrator-authorized metadata, never imported.
// Fingerprint is a public SHA-256 identity fingerprint, not a private key.
type ZoneBinding struct {
	SourceNode       string `json:"source_node"`
	Fingerprint      string `json:"fingerprint,omitempty"`
	SourceZoneID     string `json:"source_zone_id,omitempty"`
	SourceGeneration uint64 `json:"source_generation,omitempty"`
}
type ZoneRecord struct {
	Version int                `json:"version"`
	Header  config.ZoneHeader  `json:"header"`
	Binding ZoneBinding        `json:"binding"`
	Origin  *config.ZoneHeader `json:"origin,omitempty"`
	// Synced is an authenticated remote checkpoint, never populated by import.
	Synced *config.ZoneHeader `json:"synced,omitempty"`
	Body   string             `json:"body"`
}

func (z ZoneRecord) Role(localNode string) string {
	if z.Header.SourceNode == "" {
		return "local"
	}
	if z.Header.SourceNode == localNode {
		return "primary"
	}
	return "secondary"
}
func (z ZoneRecord) Revision() string { b, _ := json.Marshal(z); return config.ZoneHash(b) }

type ZoneImportOptions struct {
	Name             string
	TargetID         string
	ExpectedRevision string
}

// ZoneStore has one serialized writer per canonical directory within this process.
// Host-local administrators must protect the data directory; it is not a
// multi-process database. Never point two server processes at the same data dir.
type ZoneStore struct {
	dir, local string
	opt        config.Options
	mu         *sync.Mutex
}

var zoneStoreLocks sync.Map

func OpenZoneStore(dataDir, localNode string, opt config.Options) (*ZoneStore, error) {
	if !config.ValidName(localNode) {
		return nil, fmt.Errorf("invalid local node identity")
	}
	dir, err := filepath.Abs(filepath.Join(dataDir, "zones"))
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	for p := dir; ; p = filepath.Dir(p) {
		fi, e := os.Lstat(p)
		if e != nil {
			return nil, e
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("zone directory must not use symlinks")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	dir = real
	lockKey := strings.ToLower(dir)
	m, _ := zoneStoreLocks.LoadOrStore(lockKey, &sync.Mutex{})
	return &ZoneStore{dir: dir, local: localNode, opt: opt, mu: m.(*sync.Mutex)}, nil
}
func (s *ZoneStore) path(id string) (string, error) {
	if !config.ValidZoneID(id) {
		return "", fmt.Errorf("invalid zone ID")
	}
	p := filepath.Join(s.dir, id+".json")
	if fi, err := os.Lstat(p); err == nil {
		if !fi.Mode().IsRegular() {
			return "", fmt.Errorf("zone file must be regular")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return p, nil
}
func (s *ZoneStore) Get(id string) (ZoneRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(id)
}
func (s *ZoneStore) get(id string) (ZoneRecord, error) {
	var z ZoneRecord
	p, err := s.path(id)
	if err != nil {
		return z, err
	}
	f, err := os.Open(p)
	if err != nil {
		return z, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4*MaxZoneBytes+1))
	if err != nil {
		return z, err
	}
	if len(b) > 4*MaxZoneBytes {
		return z, fmt.Errorf("zone storage too large")
	}
	if err = json.Unmarshal(b, &z); err != nil {
		return ZoneRecord{}, fmt.Errorf("invalid zone storage")
	}
	if z.Header.ZoneID != id || z.Version != 1 || len(z.Body) > MaxZoneBytes || !config.ValidName(z.Header.Name) || z.Header.Generation == 0 || z.Header.ContentHash != config.ZoneHash([]byte(z.Body)) || z.Binding.SourceNode != z.Header.SourceNode {
		return ZoneRecord{}, fmt.Errorf("zone storage integrity failure")
	}
	if err := s.validateBinding(z.Binding); err != nil {
		return ZoneRecord{}, fmt.Errorf("invalid stored zone binding")
	}
	_, cfg, _, err := config.ParseZone(p, []byte(z.Body), s.validationOptions())
	if err != nil || cfg.Serial != z.Header.Serial {
		return ZoneRecord{}, fmt.Errorf("invalid stored zone body")
	}
	return z, nil
}
func (s *ZoneStore) validationOptions() config.Options { o := s.opt; o.SecretKnown = nil; return o }
func (s *ZoneStore) nameAvailable(name, except string) error {
	es, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, e := range es {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if id == except {
			continue
		}
		z, err := s.get(id)
		if err != nil {
			return err
		}
		if z.Header.Name == name {
			return ErrZoneNameConflict
		}
	}
	return nil
}
func newZoneID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
func (s *ZoneStore) Import(raw []byte, o ZoneImportOptions) (ZoneRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var z ZoneRecord
	if len(raw) > MaxZoneBytes {
		return z, fmt.Errorf("zone import too large")
	}
	origin, cfg, body, err := config.ParseZone("import.bind", raw, s.validationOptions())
	if err != nil {
		return z, err
	}
	name := o.Name
	if name == "" && origin != nil {
		name = origin.Name
	}
	if !config.ValidName(name) {
		return z, fmt.Errorf("zone name required")
	}
	if o.TargetID != "" {
		old, e := s.get(o.TargetID)
		if e != nil {
			return z, e
		}
		if o.ExpectedRevision == "" || old.Revision() != o.ExpectedRevision {
			return z, ErrConflict
		}
		if old.Role(s.local) == "secondary" {
			return z, ErrReadonly
		}
		if err = config.CheckSerial(cfg.Serial, old.Header.Serial); err != nil || cfg.Serial == old.Header.Serial {
			return z, fmt.Errorf("updated SOA serial must advance")
		}
		if err = s.nameAvailable(name, o.TargetID); err != nil {
			return z, err
		}
		old.Header.Name = name
		old.Header.Serial = cfg.Serial
		old.Header.ContentHash = config.ZoneHash(body)
		old.Body = string(body)
		if err = s.persist(old); err != nil {
			return z, err
		}
		return old, nil
	}
	if err = s.nameAvailable(name, ""); err != nil {
		return z, err
	}
	id, err := newZoneID()
	if err != nil {
		return z, err
	}
	z = ZoneRecord{Version: 1, Header: config.ZoneHeader{ZoneID: id, Name: name, Generation: 1, Serial: cfg.Serial, ContentHash: config.ZoneHash(body)}, Origin: origin, Body: string(body)}
	if err = s.persist(z); err != nil {
		return ZoneRecord{}, err
	}
	return z, nil
}
func (s *ZoneStore) persist(z ZoneRecord) error {
	p, err := s.path(z.Header.ZoneID)
	if err != nil {
		return err
	}
	b, err := json.Marshal(z)
	if err != nil {
		return err
	}
	if len(b) > 4*MaxZoneBytes {
		return fmt.Errorf("zone storage too large")
	}
	tmp, err := os.CreateTemp(s.dir, ".zone-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Recheck against accidental symbolic-link replacement before publication.
	if _, err = s.path(z.Header.ZoneID); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
