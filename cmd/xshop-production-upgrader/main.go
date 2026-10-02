// Linux-only privileged daemon. Install only after independent review.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/xshopupgrade"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func trusted(path string, dir bool) error {
	for p := path; ; p = filepath.Dir(p) {
		s, err := os.Lstat(p)
		if err != nil {
			return err
		}
		st, ok := s.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || s.Mode()&0022 != 0 || s.Mode()&os.ModeSymlink != 0 {
			return errors.New("root-owned non-writable path required")
		}
		if p == path && dir != s.IsDir() {
			return errors.New("path type")
		}
		if p == "/" {
			return nil
		}
	}
}
func bootstrap(engine *xshopupgrade.Engine, args []string) (bool, error) {
	if len(args) == 1 && args[0] == "--initialize" {
		return true, engine.Initialize(1)
	}
	if len(args) != 0 {
		return false, errors.New("unsupported helper CLI")
	}
	_, err := engine.ReadState()
	return false, err
}

// A failed recovery must not cause systemd to repeatedly stop preview writers.
// Exit 78 is paired with RestartPreventExitStatus in the reviewed unit.
var errRecoveryBlocked = errors.New("manual recovery required")

func exitCode(err error) int {
	if errors.Is(err, errRecoveryBlocked) {
		return 78
	}
	return 1
}

func run() error {
	if !xshopupgrade.Production {
		return errors.New("production build tag required")
	}
	if os.Geteuid() != 0 {
		return errors.New("root helper required")
	}
	const policyPath = xshopupgrade.PolicyPath
	if err := trusted(policyPath, false); err != nil {
		return err
	}
	var policy xshopupgrade.OperatorPolicy
	raw, err := os.ReadFile(policyPath)
	if err != nil {
		return err
	}
	if len(raw) > 4096 {
		return errors.New("policy too large")
	}
	if err = json.Unmarshal(raw, &policy); err != nil {
		return err
	}
	if err = xshopupgrade.ValidateOperatorPolicy(policy); err != nil {
		return err
	}
	for _, p := range []string{xshopupgrade.StateRoot, xshopupgrade.RuntimeRoot, xshopupgrade.PreviewRoot} {
		if err = trusted(p, true); err != nil {
			return err
		}
	}
	lock, err := os.OpenFile(filepath.Join(xshopupgrade.StateRoot, "helper.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("helper already running")
	}
	target := filepath.Join(xshopupgrade.PreviewRoot, "dujiao-next")
	if err = trusted(target, false); err != nil {
		return err
	} // Bootstrap must make executable root-owned; preserve runtime user's data rights.
	key, _ := xshopupgrade.ParseKey(policy.PublicKey)
	ctl := xshopupgrade.Systemd{HealthURL: xshopupgrade.PolicyHealthURL(policy)}
	engine := &xshopupgrade.Engine{Target: target, StateDir: xshopupgrade.StateRoot, Control: ctl, Backup: xshopupgrade.PreviewBackup(policy.Database)}
	if done, initErr := bootstrap(engine, os.Args[1:]); initErr != nil {
		return initErr
	} else if done {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), xshopupgrade.RecoveryTimeout)
	defer cancel()
	if err = engine.Recover(ctx); err != nil {
		return errors.Join(errRecoveryBlocked, err)
	}
	account, err := user.Lookup("dujiao")
	if err != nil {
		return err
	}
	uid, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return err
	}
	// Require systemd-managed empty runtime directory on startup; never unlink another listener.
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: xshopupgrade.SocketPath, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = os.Chown(xshopupgrade.SocketPath, 0, gid); err != nil {
		return err
	}
	if err = os.Chmod(xshopupgrade.SocketPath, 0660); err != nil {
		return err
	}
	h := &xshopupgrade.Helper{Engine: engine, Source: xshopupgrade.GitHub{}, PublicKey: key, Schema: policy.Schema}
	type peerKey struct{}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 4096, ConnContext: func(ctx context.Context, c net.Conn) context.Context {
		allowed := false
		if conn, ok := c.(*net.UnixConn); ok {
			raw, err := conn.SyscallConn()
			if err == nil {
				raw.Control(func(fd uintptr) {
					cred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
					if err == nil {
						group, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", cred.Pid))
						allowed = err == nil && xshopupgrade.PeerAllowed(cred.Uid, uint32(uid), string(group))
					}
				})
			}
		}
		return context.WithValue(ctx, peerKey{}, allowed)
	}, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowed, _ := r.Context().Value(peerKey{}).(bool); !allowed {
			http.Error(w, "peer denied", 403)
			return
		}
		h.ServeHTTP(w, r)
	})}
	return server.Serve(listener)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "XSHOP production helper stopped:", err)
		os.Exit(exitCode(err))
	}
}
