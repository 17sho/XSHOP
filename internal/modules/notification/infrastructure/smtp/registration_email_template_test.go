package smtp

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/mailbrand"
)

type registrationTemplateProvider struct {
	setting settingsmessaging.RegistrationEmailTemplateSetting
	err     error
	reads   int
}

func (p *registrationTemplateProvider) GetRegistrationEmailTemplateSetting() (settingsmessaging.RegistrationEmailTemplateSetting, error) {
	p.reads++
	return p.setting, p.err
}

func TestRegistrationSendUsesTemplateThroughLoopbackSMTP(t *testing.T) {
	// Only an in-process loopback sink is used; never external SMTP.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	messages := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := textproto.NewReader(bufio.NewReader(conn))
		fmt.Fprint(conn, "220 loopback ESMTP\r\n")
		for {
			line, err := reader.ReadLine()
			if err != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"), strings.HasPrefix(line, "MAIL FROM"), strings.HasPrefix(line, "RCPT TO"):
				fmt.Fprint(conn, "250 OK\r\n")
			case line == "DATA":
				fmt.Fprint(conn, "354 continue\r\n")
				data, err := reader.ReadDotBytes()
				if err != nil {
					return
				}
				messages <- string(data)
				fmt.Fprint(conn, "250 accepted\r\n")
			case line == "QUIT":
				fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprint(conn, "500 unsupported\r\n")
			}
		}
	}()
	cfg := &config.EmailConfig{Enabled: true, Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, From: "notify@example.test"}
	cfg.VerifyCode.ExpireMinutes = 31
	p := &registrationTemplateProvider{setting: settingsmessaging.DefaultRegistrationEmailTemplateSetting()}
	p.setting.Templates.ENUS.Subject = "CONFIGURED {{site_name}}"
	p.setting.Templates.ENUS.Body = "CONFIGURED body {{code}} {{expire_minutes}}"
	svc := New(cfg, p)
	if err := svc.SendVerifyCode("buyer@example.test", "998877", "register", "en-US", mailbrand.Brand{SiteName: "Tenant", ReplyTo: "support@tenant.example"}); err != nil {
		t.Fatal(err)
	}
	select {
	case raw := <-messages:
		msg, err := mail.ReadMessage(strings.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if msg.Header.Get("Subject") != "CONFIGURED Tenant" || msg.Header.Get("Reply-To") != "support@tenant.example" || p.reads != 1 {
			t.Fatalf("template/tenant not wired: %v reads=%d", msg.Header, p.reads)
		}
		_, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
		part, err := multipart.NewReader(msg.Body, params["boundary"]).NextPart()
		if err != nil {
			t.Fatal(err)
		}
		plain, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
		if err != nil || !strings.Contains(string(plain), "CONFIGURED body 998877 31") {
			t.Fatalf("content: %s %v", plain, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no loopback message")
	}
}

func TestRegistrationBodyEditsAppearInBuiltInHTML(t *testing.T) {
	p := &registrationTemplateProvider{setting: settingsmessaging.DefaultRegistrationEmailTemplateSetting()}
	p.setting.Templates.ENUS.Body = "WELCOME <customer>\n{{code}} expires {{expire_minutes}} {{site_name}}"
	svc := New(nil, p)
	_, plain, markup := svc.buildVerificationContent("123456", "register", "en-US", mailbrand.Brand{SiteName: "Tenant"})
	if !strings.Contains(plain, "WELCOME <customer>") || !strings.Contains(markup, "WELCOME &lt;customer&gt;<br>") || !strings.Contains(markup, "123456 expires 10 Tenant") {
		t.Fatalf("body edit missing from HTML: %s", markup)
	}
	if !strings.Contains(markup, "We will never ask for your password") {
		t.Fatal("default HTML lost security footer")
	}
}

func TestVerificationRejectsHeaderInjectionBeforeSMTP(t *testing.T) {
	cfg := &config.EmailConfig{}
	svc := New(cfg)
	for _, field := range []string{"subject", "to", "from", "from_name", "reply_to", "configured_from_name"} {
		cfg.From = "notify@example.test"
		cfg.FromName = "Configured"
		to, subject := "buyer@example.test", "Subject"
		brand := mailbrand.Brand{FromName: "Tenant", ReplyTo: "support@tenant.example"}
		switch field {
		case "subject":
			subject = "Subject\r\nBcc: victim@example.test"
		case "to":
			to += "\r\nBcc: victim@example.test"
		case "from":
			cfg.From += "\r\nBcc: victim@example.test"
		case "from_name":
			brand.FromName += "\n"
		case "reply_to":
			brand.ReplyTo += "\r"
		case "configured_from_name":
			cfg.FromName += "\n"
		}
		svc.SetConfig(cfg) // Runtime changes use the synchronized snapshot API.
		if err := svc.sendVerificationEmail(to, subject, "plain", "html", brand); err != ErrEmailHeaderInvalid {
			t.Errorf("%s: got %v, want header rejection", field, err)
		}
	}
}

func TestRegistrationTemplateRuntimeFreshSettingsAndMIME(t *testing.T) {
	p := &registrationTemplateProvider{setting: settingsmessaging.DefaultRegistrationEmailTemplateSetting()}
	p.setting.Templates.ENUS.Subject = "Join {{site_name}}"
	p.setting.Templates.ENUS.Body = "Join body {{code}} expires {{expire_minutes}} {{site_name}} {{site_url}}"
	p.setting.Templates.ENUS.CustomHTML = `<table><tr><td>{{code}} / {{expire_minutes}} / {{site_name}} / {{site_url}}</td></tr></table>`
	p.setting.Templates.ENUS.CustomHTMLEnabled = true
	cfg := &config.EmailConfig{}
	cfg.VerifyCode.ExpireMinutes = 23
	svc := New(cfg, p)
	brand := mailbrand.Brand{SiteName: `<Tenant & Co>`, SiteURL: "https://tenant.example/?a=1&b=2", FromName: "Tenant", ReplyTo: "support@tenant.example"}
	subject, plain, markup := svc.buildVerificationContent("654321", constants.VerifyPurposeRegister, "en-US", brand)
	if subject != "Join <Tenant & Co>" || !strings.Contains(plain, "Join body 654321 expires 23") || !strings.Contains(plain, brand.SiteURL) {
		t.Fatalf("render: %s %s", subject, plain)
	}
	if !strings.Contains(markup, "654321 / 23 / &lt;Tenant &amp; Co&gt;") || !strings.Contains(markup, "?a=1&amp;b=2") || strings.Contains(markup, "<Tenant") {
		t.Fatalf("unsafe or incomplete HTML: %s", markup)
	}
	for _, footer := range []string{"Do not share this code with anyone.", "If you didn't request this, you can ignore this email.", "We will never ask for your password, payment details, or verification code."} {
		if !strings.Contains(plain, footer) {
			t.Errorf("plain security footer missing %q", footer)
		}
		if !strings.Contains(markup, strings.ReplaceAll(footer, "'", "&#39;")) {
			t.Errorf("HTML security footer missing %q", footer)
		}
	}
	raw := buildVerificationEmailMessage("notify@example.test", "buyer@example.test", subject, plain, markup, brand.ReplyTo)
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	media, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || media != "multipart/alternative" {
		t.Fatalf("MIME: %s %v", media, err)
	}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	for i, want := range []string{plain, markup} {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		typ, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if typ != []string{"text/plain", "text/html"}[i] {
			t.Fatalf("part order: %s", typ)
		}
		data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
		if err != nil || string(data) != want {
			t.Fatalf("MIME body mismatch: %v", err)
		}
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatal("unexpected MIME part")
	}
	p.setting.Templates.ENUS.Subject = "Updated"
	subject, _, _ = svc.buildVerificationContent("654321", constants.VerifyPurposeRegister, "en-US", brand)
	if subject != "Updated" || p.reads != 2 {
		t.Fatal("settings were cached")
	}
}

func TestRegistrationTemplateFallbackAndPurposeIsolation(t *testing.T) {
	p := &registrationTemplateProvider{setting: settingsmessaging.DefaultRegistrationEmailTemplateSetting(), err: errors.New("storage unavailable")}
	cfg := &config.EmailConfig{}
	cfg.VerifyCode.ExpireMinutes = 7
	svc := New(cfg, p)
	brand := mailbrand.Brand{SiteName: "Tenant", SiteURL: "https://tenant.example"}
	for _, locale := range []string{"zh-CN", "zh-TW", "en-US"} {
		subject, plain, markup := svc.buildVerificationContent("123456", "register", locale, brand)
		if subject == "" || !strings.Contains(plain, "123456") || !strings.Contains(plain, "7") || !strings.Contains(markup, "123456") || !strings.Contains(markup, "7") {
			t.Fatalf("lost fallback content: %s %s", plain, markup)
		}
	}
	p.err = nil
	p.setting.Templates.ENUS.CustomHTML = "<script>bad</script>"
	p.setting.Templates.ENUS.CustomHTMLEnabled = true
	_, _, markup := svc.buildVerificationContent("123456", "register", "en-US", brand)
	if strings.Contains(markup, "<script>") || !strings.Contains(markup, "123456") {
		t.Fatal("unsafe persisted HTML did not fall back")
	}
	p.setting = settingsmessaging.DefaultRegistrationEmailTemplateSetting()
	for _, purpose := range []string{constants.VerifyPurposeReset, constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew, constants.VerifyPurposeTelegramBind} {
		reads := p.reads
		subject, plain, markup := svc.buildVerificationContent("123456", purpose, "en-US", brand)
		scene := p.setting.Scenes[purpose]
		if subject != strings.ReplaceAll(scene.ENUS.Subject, "{{site_name}}", "Tenant") || !strings.Contains(plain, "123456") || !strings.Contains(plain, "7") || !strings.Contains(markup, "123456") || !strings.Contains(markup, "7") || p.reads != reads+1 {
			t.Fatalf("scene/fresh read missing: %s", purpose)
		}
		p.err = errors.New("storage unavailable")
		fallbackSubject, fallbackPlain, fallbackHTML := svc.buildVerificationContent("123456", purpose, "en-US", brand)
		if subject != fallbackSubject || plain != fallbackPlain || markup != fallbackHTML {
			t.Fatalf("fallback lost code for %s", purpose)
		}
		p.err = nil
	}
	reads := p.reads
	subject, plain, markup := svc.buildVerificationContent("123456", "future_purpose", "en-US", brand)
	wantSubject, wantPlain := buildVerifyCodeContent("123456", "future_purpose", "en-US", brand)
	if subject != wantSubject || plain != wantPlain || markup != buildVerifyCodeHTML("123456", "future_purpose", "en-US", brand, 7) || p.reads != reads {
		t.Fatal("unknown purpose changed legacy fallback")
	}

}

func TestVerificationScenesFreshProviderAndSafeFallback(t *testing.T) {
	cfg := &config.EmailConfig{}
	cfg.VerifyCode.ExpireMinutes = 17
	for _, purpose := range []string{"register", "reset", "telegram_bind", "change_email_old", "change_email_new"} {
		for _, locale := range []string{"zh-CN", "zh-TW", "en-US"} {
			t.Run(purpose+"/"+locale, func(t *testing.T) {
				p := &registrationTemplateProvider{setting: settingsmessaging.DefaultRegistrationEmailTemplateSetting()}
				svc := New(cfg, p)
				brand := mailbrand.Brand{SiteName: "Tenant", SiteURL: "https://tenant.example"}
				subject, plain, markup := svc.buildVerificationContent("123456", purpose, locale, brand)
				if p.reads != 1 || !strings.Contains(plain, "123456") || !strings.Contains(plain, "17") || !strings.Contains(markup, "123456") || !strings.Contains(markup, "17") {
					t.Fatal("default scene lost code/expiry or provider read")
				}
				for _, failure := range []string{"error", "unsafe"} {
					p.err = nil
					p.setting = settingsmessaging.DefaultRegistrationEmailTemplateSetting()
					if failure == "error" {
						p.err = errors.New("storage unavailable")
					} else {
						if purpose == "register" {
							p.setting.Templates.ENUS.Subject = "bad\r\nBcc: injected"
						} else {
							scene := p.setting.Scenes[purpose]
							scene.ENUS.CustomHTML = "<script>bad</script>"
							p.setting.Scenes[purpose] = scene
						}
					}
					s, ptext, h := svc.buildVerificationContent("123456", purpose, locale, brand)
					if s != subject || ptext != plain || h != markup {
						t.Fatalf("%s did not fall back safely", failure)
					}
				}
				p.err = nil
				p.setting = settingsmessaging.DefaultRegistrationEmailTemplateSetting()
				scene := p.setting.Templates
				if purpose != "register" {
					scene = p.setting.Scenes[purpose]
				}
				for _, lt := range []*settingsmessaging.OrderEmailLocalizedTemplate{&scene.ZHCN, &scene.ZHTW, &scene.ENUS} {
					lt.Subject = "Fresh " + purpose
				}
				if purpose == "register" {
					p.setting.Templates = scene
				} else {
					p.setting.Scenes[purpose] = scene
				}
				s, _, _ := svc.buildVerificationContent("123456", purpose, locale, brand)
				if s != "Fresh "+purpose || p.reads != 4 {
					t.Fatal("settings cached across sends")
				}
				reads := p.reads
				s, ptext, h := svc.buildVerificationContent("123456", "unknown", locale, brand)
				wantS, wantP := buildVerifyCodeContent("123456", "unknown", locale, brand)
				if s != wantS || ptext != wantP || h != buildVerifyCodeHTML("123456", "unknown", locale, brand, 17) || p.reads != reads {
					t.Fatal("unknown purpose lost original fallback")
				}
			})
		}
	}
}

func verificationLoopback(t *testing.T) (*config.EmailConfig, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	messages := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := textproto.NewReader(bufio.NewReader(conn))
		fmt.Fprint(conn, "220 loopback ESMTP\r\n")
		for {
			line, err := reader.ReadLine()
			if err != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"), strings.HasPrefix(line, "MAIL FROM"), strings.HasPrefix(line, "RCPT TO"):
				fmt.Fprint(conn, "250 OK\r\n")
			case line == "DATA":
				fmt.Fprint(conn, "354 continue\r\n")
				data, err := reader.ReadDotBytes()
				if err != nil {
					return
				}
				messages <- string(data)
				fmt.Fprint(conn, "250 accepted\r\n")
			case line == "QUIT":
				fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprint(conn, "500 unsupported\r\n")
			}
		}
	}()

	cfg := &config.EmailConfig{Enabled: true, Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, From: "notify@example.test"}
	cfg.VerifyCode.ExpireMinutes = 31
	return cfg, messages
}

func TestUnifiedVerificationScenesSendThroughLoopbackSMTP(t *testing.T) {
	purposes := []string{"register", "reset", "telegram_bind", "change_email_old", "change_email_new"}
	for _, purpose := range purposes {
		for _, locale := range []string{"zh-CN", "zh-TW", "en-US"} {
			for _, advanced := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/advanced=%v", purpose, locale, advanced), func(t *testing.T) {
					cfg, messages := verificationLoopback(t)
					p := &registrationTemplateProvider{setting: settingsmessaging.DefaultRegistrationEmailTemplateSetting()}
					for _, key := range purposes {
						scene := p.setting.Templates
						if key != "register" {
							scene = p.setting.Scenes[key]
						}
						for language, lt := range map[string]*settingsmessaging.OrderEmailLocalizedTemplate{"zh-CN": &scene.ZHCN, "zh-TW": &scene.ZHTW, "en-US": &scene.ENUS} {
							lt.Subject = key + "/" + language + " {{site_name}}"
							lt.Body = key + "/" + language + " plain {{code}} expires {{expire_minutes}} {{site_name}} {{site_url}}"
							lt.CustomHTML = "<table><tr><td>" + key + "/" + language + " advanced {{code}} expires {{expire_minutes}} {{site_name}} {{site_url}}</td></tr></table>"
							lt.CustomHTMLEnabled = advanced
						}
						if key == "register" {
							p.setting.Templates = scene
						} else {
							p.setting.Scenes[key] = scene
						}
					}
					svc := New(cfg, p)
					brand := mailbrand.Brand{SiteName: "<Tenant & Co>", SiteURL: "https://tenant.example/?a=1&b=2", ReplyTo: "support@tenant.example"}
					if err := svc.SendVerifyCode("buyer@example.test", "CODE<&>", purpose, locale, brand); err != nil {
						t.Fatal(err)
					}
					var raw string
					select {
					case raw = <-messages:
					case <-time.After(5 * time.Second):
						t.Fatal("no loopback message")
					}
					msg, err := mail.ReadMessage(strings.NewReader(raw))
					if err != nil {
						t.Fatal(err)
					}
					subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
					if err != nil {
						t.Fatal(err)
					}
					if subject != purpose+"/"+locale+" "+brand.SiteName || msg.Header.Get("Reply-To") != brand.ReplyTo || p.reads != 1 {
						t.Fatalf("routing/branding/fresh provider: %s reads=%d", subject, p.reads)
					}
					media, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
					if err != nil || media != "multipart/alternative" {
						t.Fatal("not multipart alternative")
					}
					reader := multipart.NewReader(msg.Body, params["boundary"])
					for i, typ := range []string{"text/plain", "text/html"} {
						part, err := reader.NextPart()
						if err != nil {
							t.Fatal(err)
						}
						actual, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
						if actual != typ {
							t.Fatalf("MIME order: %s", actual)
						}
						data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
						if err != nil {
							t.Fatal(err)
						}
						text := string(data)
						if i == 0 {
							if !strings.Contains(text, purpose+"/"+locale+" plain CODE<&> expires 31") || !strings.Contains(text, registrationSecurityFooter(locale)) {
								t.Fatalf("plain routing/code/expiry/footer: %s", text)
							}
						} else {
							marker := " plain "
							if advanced {
								marker = " advanced "
							}
							if !strings.Contains(text, purpose+"/"+locale+marker+"CODE&lt;&amp;&gt; expires 31") || !strings.Contains(text, "&lt;Tenant &amp; Co&gt;") || !strings.Contains(text, "?a=1&amp;b=2") || strings.Contains(text, "<Tenant") {
								t.Fatalf("HTML routing/escape/code/expiry: %s", text)
							}
							for _, line := range strings.Split(registrationSecurityFooter(locale), "\n") {
								if !strings.Contains(text, html.EscapeString(line)) {
									t.Fatalf("HTML footer missing: %s", line)
								}
							}
						}
						for _, other := range purposes {
							if other != purpose && strings.Contains(text, other+"/") {
								t.Fatalf("purpose leak %s into %s", other, purpose)
							}
						}
					}
					if _, err := reader.NextPart(); err != io.EOF {
						t.Fatal("extra MIME part")
					}
				})
			}
		}
	}
}
