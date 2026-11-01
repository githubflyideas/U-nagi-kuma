package store

import (
	"database/sql"
	"errors"
	"github.com/githubflyideas/unagikuma/internal/engine"
	"time"
)

// Verdict commits synchronously: no authorization-bearing item waits in the
// legacy queue after revocation. Names must match the authorized stable ID.
func (r *MonitorRepository) Verdict(id int64, v engine.VerdictRecord) error {
	flushMu.Lock()
	defer flushMu.Unlock()
	return r.access(true, func(tx *sql.Tx) error {
		name, err := r.ownedMonitor(tx, id)
		if err != nil {
			return err
		}
		if name != v.Monitor {
			return ErrTenantAccess
		}
		if !r.s.batch.flushHistory([]item{{mid: id, verdict: &v}}) {
			return errors.New("history write failed")
		}
		return nil
	})
}

func (r *MonitorRepository) Rounds(id int64, from, to time.Time, limit int) (out []RoundRow, err error) {
	err = r.access(false, func(tx *sql.Tx) error {
		if _, e := r.ownedMonitor(tx, id); e != nil {
			return e
		}
		var e error
		out, e = r.s.roundsByID(id, from, to, limit)
		return e
	})
	return
}

func (r *MonitorRepository) UptimeOf(id int64, from, to time.Time) (out Uptime, err error) {
	err = r.access(false, func(tx *sql.Tx) error {
		if _, e := r.ownedMonitor(tx, id); e != nil {
			return e
		}
		var e error
		out, e = r.s.uptimeOfID(id, from, to)
		return e
	})
	return
}

func (r *MonitorRepository) Cells(id int64, from, to time.Time, n int) (out []Cell, err error) {
	err = r.access(false, func(tx *sql.Tx) error {
		if _, e := r.ownedMonitor(tx, id); e != nil {
			return e
		}
		var e error
		out, e = r.s.cellsByID(id, from, to, n)
		return e
	})
	return
}

func (r *MonitorRepository) Latency(id int64, from, to time.Time) (out []LatencyPoint, err error) {
	err = r.access(false, func(tx *sql.Tx) error {
		if _, e := r.ownedMonitor(tx, id); e != nil {
			return e
		}
		var e error
		out, e = r.s.latencyByID(id, from, to)
		return e
	})
	return
}
