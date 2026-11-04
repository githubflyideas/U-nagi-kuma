package app

import (
	"bufio"
	"context"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/githubflyideas/unagikuma/internal/store"
)

// smtpRecorder accepts every message and records its recipients and
// subject.
type smtpRecorder struct {
	mu   sync.Mutex
	msgs []smtpMsg
}

type smtpMsg struct {
	to      []string
	subject string
}

func (s *smtpRecorder) listen(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return l.Addr().String()
}

func (s *smtpRecorder) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := func(l string) { io.WriteString(c, l+"\r\n") }
	w("220 fake ESMTP")
	var cur smtpMsg
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if inData {
			if line == ".\r\n" {
				inData = false
				s.mu.Lock()
				s.msgs = append(s.msgs, cur)
				s.mu.Unlock()
				cur = smtpMsg{}
				w("250 queued")
			} else if h, ok := strings.CutPrefix(line, "Subject: "); ok && cur.subject == "" {
				cur.subject = strings.TrimSpace(h)
			}
			continue
		}
		cmd := strings.TrimSpace(line)
		switch up := strings.ToUpper(cmd); {
		case strings.HasPrefix(up, "RCPT TO:"):
			cur.to = append(cur.to, strings.ToLower(strings.Trim(cmd[8:], "<> ")))
			w("250 ok")
		case strings.HasPrefix(up, "DATA"):
			inData = true
			w("354 go ahead")
		case strings.HasPrefix(up, "QUIT"):
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}

// recipients lists who received any message so far.
func (s *smtpRecorder) recipients() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	set := map[string]bool{}
	for _, m := range s.msgs {
		for _, r := range m.to {
			set[r] = true
		}
	}
	var out []string
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// When the shared infrastructure fails, the managers of the teams it
// affects are told, each at their own address through the channel their
// contact entry names: the teams with monitors that depend on it, or every
// team when none does. Team monitors keep to their teams.
func TestManagersHearOfSharedFailures(t *testing.T) {
	rec := &smtpRecorder{}
	host, port, _ := net.SplitHostPort(rec.listen(t))
	a, fp, _, _ := newApp(t, `$FILE revision=1 interval=1s retry=500ms timeout=300ms attempts=1
$SET engine.first-round-delay=0s suspend.recheck-spread=0s notify.backstop-max=50
mail    IN CHANNEL email host=`+host+` port=`+port+` tls=none from=mon@example.com to=ops@example.com
default IN POLICY chain=mail
web     IN TEAM managers=li operators=zhang
db      IN TEAM managers=wang
gw      IN TCP 10.0.0.1:443
dns     IN TCP 10.0.0.53:53
shop    IN TCP 10.0.0.2:443 team=web parent=gw
pg      IN TCP 10.0.0.3:5432 team=db
`)
	for _, p := range []store.Person{
		{Handle: "li", Name: "Li", Username: "li", Emails: []string{"li@example.com"}, Notify: []string{"mail"}},
		{Handle: "wang", Name: "Wang", Username: "wang", Emails: []string{"wang@example.com"}, Notify: []string{"mail"}},
		{Handle: "zhang", Name: "Zhang", Username: "zhang", Emails: []string{"zhang@example.com"}, Notify: []string{"mail"}},
	} {
		if _, err := a.Store().SavePerson(p); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	defer func() { cancel(); <-done }()
	a.DirectoryChanged()

	wait := func(what string, want ...string) {
		t.Helper()
		sort.Strings(want)
		deadline := time.Now().Add(15 * time.Second)
		for {
			got := rec.recipients()
			if strings.Join(got, ",") == strings.Join(want, ",") {
				return
			}
			if time.Now().After(deadline) {
				for id := int64(1); id < 6; id++ {
					ds, _ := a.Store().Decisions(id, "", time.Time{}, time.Now())
					for _, d := range ds {
						t.Logf("%d %s %s %s %s %v %s", id, d.IntentID, d.Monitor, d.Kind, d.Step, d.Recipients, d.Note)
					}
				}
				t.Fatalf("%s: recipients %v, want %v", what, got, want)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	// gw: web's shop depends on it, so web's manager hears of it, not db's.
	fp.set("gw", true)
	wait("gw down", "ops@example.com", "li@example.com")
	// dns: nothing depends on it, so every team's manager hears of it.
	fp.set("dns", true)
	wait("dns down", "ops@example.com", "li@example.com", "wang@example.com")
	// A team monitor goes through the usual policy only.
	rec.mu.Lock()
	rec.msgs = nil
	rec.mu.Unlock()
	fp.set("pg", true)
	wait("pg down", "ops@example.com")
	time.Sleep(2 * time.Second)
	if got := rec.recipients(); strings.Join(got, ",") != "ops@example.com" {
		t.Fatalf("a team monitor's failure reached %v", got)
	}
	// The managers who were told of a failure are told of its recovery.
	rec.mu.Lock()
	rec.msgs = nil
	rec.mu.Unlock()
	fp.set("gw", false)
	wait("gw recovered", "ops@example.com", "li@example.com")
}
