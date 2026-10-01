package smtp

import (
	"context"
	"crypto/tls"
	"net"
	"net/smtp"
	"time"
)

const smtpSessionTimeout = 15 * time.Second

// One deadline bounds dial, TLS, greeting, AUTH, DATA and QUIT together.
func sendMailContext(parent context.Context, mode, addr, host, from string, to []string, msg []byte, username, password string) (err error) {
	ctx, cancel := context.WithTimeout(parent, smtpSessionTimeout)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: smtpSessionTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	var wire net.Conn = conn
	if mode == "ssl" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return err
		}
		wire = tlsConn
	}
	client, err := smtp.NewClient(wire, host)
	if err != nil {
		return err
	}
	defer closeSMTPClientOnError(client, &err, host, addr)
	if mode == "starttls" {
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if err := authenticateSMTPClient(client, host, username, password); err != nil {
		return err
	}
	return sendSMTPData(client, host, addr, from, to, msg)
}
