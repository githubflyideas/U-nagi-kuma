package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/githubflyideas/unagikuma/internal/engine"
	"github.com/githubflyideas/unagikuma/internal/notify"
)

// DispatchWorkspaceNotifications must run inside BeginExecution. Its sender must
// honor ctx and a timeout shorter than the 35s lease. An acknowledged intent is
// never resent. A crash after external acceptance but before the SQLite ACK can
// resend after lease expiry; stable DeliveryID enables receiver deduplication.
// No exactly-once external delivery is claimed.
func (w *WorkspaceRuntimeWriter) DispatchWorkspaceNotifications(ctx context.Context, send func(context.Context, string, *notify.Message) bool) (pending bool, err error) {
	type intent struct {
		id, mid, iid, lease int64
		channel, raw        string
	}
	var list []intent
	err = w.access(func(tx *sql.Tx) error {
		r := w.reader
		admitted := false
		for lease := range r.s.workspaceExecutions {
			if lease.reader == r && lease.ctx == ctx {
				admitted = true
				break
			}
		}
		if !admitted {
			return ErrTenantAccess
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if _, e := tx.Exec(`UPDATE workspace_notification_intents SET state='cancelled',updated_at=? WHERE tenant_id=? AND workspace_id=? AND state='pending' AND (generation!=? OR NOT EXISTS(SELECT 1 FROM monitors m WHERE m.id=monitor_id AND m.retired_at IS NULL))`, ms(r.s.now()), r.tenant, r.workspace, r.generation); e != nil {
			return e
		}
		rows, e := tx.Query(`SELECT id,monitor_id,incident_id,channel,message FROM workspace_notification_intents WHERE tenant_id=? AND workspace_id=? AND generation=? AND state='pending' AND lease_until<=? ORDER BY id LIMIT 64`, r.tenant, r.workspace, r.generation, ms(r.s.now()))
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v intent
			if e = rows.Scan(&v.id, &v.mid, &v.iid, &v.channel, &v.raw); e != nil {
				return e
			}
			list = append(list, v)
		}
		return rows.Err()
	})
	if err != nil {
		return false, err
	}
	for _, v := range list {
		if err = ctx.Err(); err != nil {
			return true, err
		}
		claimed := false
		err = w.access(func(tx *sql.Tx) error {
			r := w.reader
			now := ms(r.s.now())
			v.lease = ms(r.s.now().Add(35 * time.Second))
			res, e := tx.Exec(`UPDATE workspace_notification_intents SET lease_until=? WHERE id=? AND tenant_id=? AND workspace_id=? AND generation=? AND state='pending' AND lease_until<=? AND EXISTS(SELECT 1 FROM monitors m WHERE m.id=monitor_id AND m.tenant_id=? AND m.workspace_id=? AND m.retired_at IS NULL)`, v.lease, v.id, r.tenant, r.workspace, r.generation, now, r.tenant, r.workspace)
			if e != nil {
				return e
			}
			n, e := res.RowsAffected()
			claimed = n == 1
			return e
		})
		if err != nil {
			return true, err
		}
		if !claimed {
			continue
		}
		var message notify.Message
		if err = json.Unmarshal([]byte(v.raw), &message); err != nil {
			return true, err
		}
		message.DeliveryID = fmt.Sprintf("workspace:%s:%s:%d", w.reader.tenant, w.reader.workspace, v.id)
		if err = ctx.Err(); err != nil {
			return true, err
		}
		// Resolve credentials in send, then perform the final admission there too.
		ok := send(ctx, v.channel, &message)
		err = w.access(func(tx *sql.Tx) error {
			r := w.reader
			state := "pending"
			if ok {
				state = "delivered"
			}
			res, e := tx.Exec(`UPDATE workspace_notification_intents SET state=?,lease_until=0,updated_at=? WHERE id=? AND tenant_id=? AND workspace_id=? AND generation=? AND state='pending' AND lease_until=?`, state, ms(r.s.now()), v.id, r.tenant, r.workspace, r.generation, v.lease)
			if e != nil {
				return e
			}
			n, e := res.RowsAffected()
			if e != nil {
				return e
			}
			if n != 1 {
				return errors.New("workspace delivery lease lost")
			}
			if !ok {
				return nil
			}
			var raw string
			if e = tx.QueryRow(`SELECT state FROM workspace_incidents WHERE id=? AND tenant_id=? AND workspace_id=? AND monitor_id=?`, v.iid, r.tenant, r.workspace, v.mid).Scan(&raw); e != nil {
				return e
			}
			var incident engine.IncidentState
			if e = json.Unmarshal([]byte(raw), &incident); e != nil {
				return e
			}
			if message.Kind != string(engine.NoticeRecovery) {
				incident.Delivered()
			}
			b, e := json.Marshal(incident)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(`UPDATE workspace_incidents SET state=?,updated_at=? WHERE id=?`, string(b), ms(r.s.now()), v.iid); e != nil {
				return e
			}
			_, e = tx.Exec(`INSERT INTO workspace_events(tenant_id,workspace_id,monitor_id,incident_id,kind,at) VALUES(?,?,?,?,?,?)`, r.tenant, r.workspace, v.mid, v.iid, "delivered_"+message.Kind, ms(r.s.now()))
			return e
		})
		if err != nil {
			return true, err
		}
	}
	err = w.access(func(tx *sql.Tx) error {
		var n int
		r := w.reader
		e := tx.QueryRow(`SELECT count(*) FROM workspace_notification_intents WHERE tenant_id=? AND workspace_id=? AND generation=? AND state='pending'`, r.tenant, r.workspace, r.generation).Scan(&n)
		pending = n > 0
		return e
	})
	return
}

// CheckNotificationAuthority is a final pre-network live generation gate. The
// execution context cancels already admitted requests on revoke/replacement.
func (w *WorkspaceRuntimeWriter) CheckNotificationAuthority() error {
	return w.access(func(*sql.Tx) error { return nil })
}
