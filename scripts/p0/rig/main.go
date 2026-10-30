// Command rig is the test network's far side for scripts/p0/harness.sh:
// HTTP targets that can be told to fail, a webhook receiver that keeps
// every message, and a tiny DNS server. Not part of Unagikuma.
//
//	rig target ADDR LOG   /ok?i=N answers 200 unless /ctl?i=N&code=C (or /ctl?all=C);
//	                      every check is logged (time, i) to LOG for the herd check
//	rig hook ADDR LOG     every POST is appended to LOG as {"at":..,"body":..}
//	rig dns ADDR IP       answers every A question with IP
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatal("usage: rig target|hook ADDR LOG | rig dns ADDR IP")
	}
	switch os.Args[1] {
	case "target":
		target(os.Args[2], os.Args[3])
	case "hook":
		hook(os.Args[2], os.Args[3])
	case "dns":
		dns(os.Args[2], os.Args[3])
	default:
		log.Fatal("unknown role")
	}
}

func appender(path string) (func([]byte), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	return func(b []byte) {
		mu.Lock()
		f.Write(append(b, '\n'))
		mu.Unlock()
	}, nil
}

func target(addr, logPath string) {
	logLine, err := appender(logPath)
	if err != nil {
		log.Fatal(err)
	}
	var mu sync.Mutex
	codes := map[string]int{}
	all := 0
	m := http.NewServeMux()
	m.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		i := r.URL.Query().Get("i")
		logLine([]byte(time.Now().Format(time.RFC3339Nano) + " " + i))
		mu.Lock()
		c := codes[i]
		if all != 0 {
			c = all
		}
		mu.Unlock()
		if c == 0 {
			c = 200
		}
		w.WriteHeader(c)
		fmt.Fprintf(w, "code %d\n", c)
	})
	m.HandleFunc("/ctl", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		defer mu.Unlock()
		if a := q.Get("all"); a != "" {
			all, _ = strconv.Atoi(a)
		}
		if i := q.Get("i"); i != "" {
			codes[i], _ = strconv.Atoi(q.Get("code"))
		}
		if q.Get("reset") != "" {
			codes, all = map[string]int{}, 0
		}
		fmt.Fprintf(w, "ok all=%d n=%d\n", all, len(codes))
	})
	log.Fatal(http.ListenAndServe(addr, m))
}

func hook(addr, logPath string) {
	logLine, err := appender(logPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body any
		if json.Unmarshal(b, &body) != nil {
			body = string(b)
		}
		line, _ := json.Marshal(map[string]any{"at": time.Now().Format(time.RFC3339Nano), "path": r.URL.Path, "body": body})
		logLine(line)
		w.WriteHeader(http.StatusOK)
	})))
}

// dns answers A questions with one address (AAAA and the rest: no data).
func dns(addr, ip string) {
	a, err := netip.ParseAddr(ip)
	if err != nil || !a.Is4() {
		log.Fatal("dns needs an IPv4 address")
	}
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, 1500)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil || n < 12 {
			continue
		}
		q := buf[:n]
		// Find the end of the question name.
		p := 12
		for p < n && q[p] != 0 {
			p += int(q[p]) + 1
		}
		if p+5 > n {
			continue
		}
		qtype := binary.BigEndian.Uint16(q[p+1:])
		end := p + 5
		out := append([]byte(nil), q[:end]...)
		out[2] = 0x81 | (q[2] & 0x01) // response, recursion desired echoed
		out[3] = 0x80                 // recursion available, no error
		binary.BigEndian.PutUint16(out[6:], 0)
		binary.BigEndian.PutUint16(out[8:], 0)
		binary.BigEndian.PutUint16(out[10:], 0)
		if qtype == 1 {
			binary.BigEndian.PutUint16(out[6:], 1)
			b4 := a.As4()
			out = append(out, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 30, 0, 4, b4[0], b4[1], b4[2], b4[3])
		}
		pc.WriteTo(out, from)
	}
}
