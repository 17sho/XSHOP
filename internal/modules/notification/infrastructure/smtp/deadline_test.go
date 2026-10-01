package smtp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/config"
	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func smtpStallFixture(t *testing.T, stage string) (string, <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	reached := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	t.Cleanup(func() { close(release); _ = ln.Close(); <-done })
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		stall := func(at string) bool {
			if stage != at {
				return false
			}
			close(reached)
			<-release
			return true
		}
		if stall("greeting") {
			return
		}
		_, _ = fmt.Fprint(conn, "220 fixture\r\n")
		reader := bufio.NewReader(conn)
		inData := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					if stall("body") {
						return
					}
					inData = false
					_, _ = fmt.Fprint(conn, "250 accepted\r\n")
				}
				continue
			}
			command := strings.ToUpper(strings.Fields(line)[0])
			if stall(strings.ToLower(command)) {
				return
			}
			switch command {
			case "EHLO":
				_, _ = fmt.Fprint(conn, "250-fixture\r\n250-AUTH PLAIN\r\n250 STARTTLS\r\n")
			case "AUTH":
				_, _ = fmt.Fprint(conn, "235 authenticated\r\n")
			case "MAIL", "RCPT":
				_, _ = fmt.Fprint(conn, "250 OK\r\n")
			case "DATA":
				inData = true
				_, _ = fmt.Fprint(conn, "354 continue\r\n")
			case "QUIT":
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				return
			case "STARTTLS":
				_, _ = fmt.Fprint(conn, "220 begin TLS\r\n")
				if stall("tls") {
					return
				}
			}
		}
	}()
	return ln.Addr().String(), reached
}

func TestSMTPAllPublicSendersHonorCallerCancellation(t *testing.T) {
	svc := New(&config.EmailConfig{Enabled: true, Host: "127.0.0.1", Port: 1, From: "from@example.test"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, send := range map[string]func() error{
		"custom": func() error { return svc.SendCustomEmailContext(ctx, "to@example.test", "subject", "body") },
		"verification": func() error {
			return svc.SendVerifyCodeContext(ctx, "to@example.test", "123456", "register", "en-US", mailbrand.Brand{})
		},
		"order": func() error {
			return svc.SendOrderStatusEmailContext(ctx, "to@example.test", notificationcontract.OrderStatusEmailInput{}, "en-US")
		},
		"order-template": func() error {
			return svc.SendOrderStatusEmailWithTemplateContext(ctx, "to@example.test", notificationcontract.OrderStatusEmailInput{}, "en-US", nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := send(); !errors.Is(err, context.Canceled) {
				t.Fatalf("caller cancellation lost: %v", err)
			}
		})
	}
}

func TestSMTPContextCancelsEverySessionPhase(t *testing.T) {
	for _, stage := range []string{"ehlo", "auth", "mail", "rcpt", "data", "body", "quit", "starttls", "tls"} {
		t.Run(stage, func(t *testing.T) {
			addr, reached := smtpStallFixture(t, stage)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			mode := "plain"
			user, pass := "", ""
			if stage == "starttls" || stage == "tls" {
				mode = "starttls"
			}
			if stage == "auth" {
				user, pass = "fixture", "fixture"
			}
			go func() {
				done <- sendMailContext(ctx, mode, addr, "127.0.0.1", "from@example.test", []string{"to@example.test"}, []byte("Subject: fixture\r\n\r\nbody\r\n"), user, pass)
			}()
			select {
			case <-reached:
			case err := <-done:
				t.Fatalf("did not reach %s: %v", stage, err)
			case <-time.After(time.Second):
				t.Fatal("fixture phase not reached")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("SMTP ignored cancellation")
			}
		})
	}
}
func TestSMTPDefaultSessionDeadlineBoundsBackgroundSender(t *testing.T) {
	addr, reached := smtpStallFixture(t, "greeting")
	host, port, _ := net.SplitHostPort(addr)
	n, _ := strconv.Atoi(port)
	svc := New(&config.EmailConfig{Enabled: true, Host: host, Port: n, From: "from@example.test"})
	done := make(chan error, 1)
	go func() { done <- svc.SendCustomEmail("to@example.test", "fixture", "body") }()
	select {
	case <-reached:
	case <-time.After(time.Second):
		t.Fatal("no SMTP connection")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stalled server succeeded")
		}
	case <-time.After(17 * time.Second):
		t.Fatal("unbounded SMTP session")
	}
}
func TestSMTPConfigSnapshotsAreIsolatedAndConcurrent(t *testing.T) {
	cfg := config.EmailConfig{Host: "first", VerifyCode: config.VerifyCodeConfig{Length: 6}}
	svc := New(&cfg)
	cfg.Host = "external mutation"
	if svc.ConfigSnapshot().Host != "first" {
		t.Fatal("caller owns live SMTP config")
	}
	snapshot := svc.ConfigSnapshot()
	snapshot.Host = "snapshot mutation"
	if svc.ConfigSnapshot().Host != "first" {
		t.Fatal("snapshot mutated live config")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				next := config.EmailConfig{Host: fmt.Sprintf("host-%d-%d", i, j), VerifyCode: config.VerifyCodeConfig{Length: 8}}
				svc.SetConfig(&next)
				_ = svc.ConfigSnapshot()
				_ = svc.SendCustomEmail("to@example.test", "fixture", "body")
			}
		}(i)
	}
	wg.Wait()
	if svc.ConfigSnapshot().VerifyCode.Length != 8 {
		t.Fatal("runtime verification policy not in snapshot")
	}
}

func TestSMTPContextCancelsStalledLoopbackGreeting(t *testing.T) {
	for _, mode := range []string{"plain", "starttls", "ssl"} {
		t.Run(mode, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				c, e := ln.Accept()
				if e == nil {
					accepted <- c
				}
			}()
			_, port, _ := net.SplitHostPort(ln.Addr().String())
			n, _ := strconv.Atoi(port)
			svc := New(&config.EmailConfig{Enabled: true, Host: "127.0.0.1", Port: n, From: "sender@example.test", UseSSL: mode == "ssl", UseTLS: mode == "starttls"})
			sender, ok := interface{}(svc).(interface {
				SendCustomEmailContext(context.Context, string, string, string) error
			})
			if !ok {
				t.Fatal("SMTP lacks caller cancellation")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- sender.SendCustomEmailContext(ctx, "to@example.test", "subject", "body") }()
			select {
			case conn := <-accepted:
				defer conn.Close()
			case <-time.After(time.Second):
				t.Fatal("no loopback connection")
			}
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("canceled send succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("SMTP ignored cancellation")
			}
		})
	}
}
