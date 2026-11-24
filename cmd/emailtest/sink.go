package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/mail"
	"strings"
	"sync"
	"time"
)

type Mail struct {
	EnvelopeFrom string
	Rcpts        []string
	Raw          string
	Header       mail.Header
	Body         string
	consumed     bool
}

func (m *Mail) Subject() string { return m.Header.Get("Subject") }

// Sink is a minimal SMTP server that accepts everything and keeps the messages,
// speaking AUTH LOGIN like the servers the app talks to.
type Sink struct {
	ln    net.Listener
	mu    sync.Mutex
	mails []*Mail
	Port  int
}

func startSink() (*Sink, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Sink{ln: ln, Port: ln.Addr().(*net.TCPAddr).Port}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handle(c)
		}
	}()
	return s, nil
}

func (s *Sink) Close() { _ = s.ln.Close() }

func address(line string) string {
	if i := strings.Index(line, "<"); i >= 0 {
		if j := strings.Index(line[i:], ">"); j > 0 {
			return line[i+1 : i+j]
		}
	}
	return strings.TrimSpace(line[strings.Index(line, ":")+1:])
}

func (s *Sink) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	say := func(format string, a ...any) { fmt.Fprintf(c, format+"\r\n", a...) }
	readLine := func() (string, bool) {
		_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
		l, err := r.ReadString('\n')
		return strings.TrimRight(l, "\r\n"), err == nil
	}

	say("220 sink ESMTP ready")
	cur := &Mail{}
	for {
		line, ok := readLine()
		if !ok {
			return
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			say("250-sink")
			say("250 AUTH LOGIN PLAIN")
		case strings.HasPrefix(upper, "HELO"):
			say("250 sink")
		case strings.HasPrefix(upper, "AUTH LOGIN"):
			say("334 VXNlcm5hbWU6")
			if _, ok := readLine(); !ok {
				return
			}
			say("334 UGFzc3dvcmQ6")
			if _, ok := readLine(); !ok {
				return
			}
			say("235 2.7.0 authenticated")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			cur = &Mail{EnvelopeFrom: address(line)}
			say("250 ok")
		case strings.HasPrefix(upper, "RCPT TO:"):
			cur.Rcpts = append(cur.Rcpts, address(line))
			say("250 ok")
		case upper == "DATA":
			say("354 end with <CRLF>.<CRLF>")
			var buf []string
			for {
				l, ok := readLine()
				if !ok {
					return
				}
				if l == "." {
					break
				}
				buf = append(buf, strings.TrimPrefix(l, "."))
			}
			cur.Raw = strings.Join(buf, "\r\n")
			if msg, err := mail.ReadMessage(bytes.NewReader([]byte(cur.Raw))); err == nil {
				cur.Header = msg.Header
				body, _ := io.ReadAll(msg.Body)
				cur.Body = string(body)
			} else {
				cur.Header = mail.Header{}
			}
			s.mu.Lock()
			s.mails = append(s.mails, cur)
			s.mu.Unlock()
			cur = &Mail{}
			say("250 ok queued")
		case upper == "RSET":
			cur = &Mail{}
			say("250 ok")
		case upper == "NOOP":
			say("250 ok")
		case upper == "QUIT":
			say("221 bye")
			return
		default:
			say("502 not implemented")
		}
	}
}

// Wait returns the first unclaimed message matching, claiming it so it is
// only matched once, or nil after d.
func (s *Sink) Wait(d time.Duration, match func(*Mail) bool) *Mail {
	deadline := time.Now().Add(d)
	for {
		s.mu.Lock()
		for _, m := range s.mails {
			if !m.consumed && match(m) {
				m.consumed = true
				s.mu.Unlock()
				return m
			}
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Sink) Unclaimed() []*Mail {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Mail
	for _, m := range s.mails {
		if !m.consumed {
			out = append(out, m)
		}
	}
	return out
}

func (s *Sink) Total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.mails)
}

func to(addr string) func(*Mail) bool {
	return func(m *Mail) bool {
		for _, r := range m.Rcpts {
			if strings.EqualFold(r, addr) {
				return true
			}
		}
		return false
	}
}

func toWithSubject(addr, subjectContains string) func(*Mail) bool {
	return func(m *Mail) bool {
		return to(addr)(m) && strings.Contains(m.Subject(), subjectContains)
	}
}
