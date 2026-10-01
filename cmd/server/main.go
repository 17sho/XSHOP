package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/dujiao-next/internal/admincmd"
	"github.com/dujiao-next/internal/app"
	databasemigrations "github.com/dujiao-next/internal/bootstrap/database/migrations"
	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/logger"
	adminapplication "github.com/dujiao-next/internal/modules/identity/admin/application"
	adminstore "github.com/dujiao-next/internal/modules/identity/admin/infrastructure/gormstore"
	"github.com/dujiao-next/internal/platform/database/gormdb"

	"github.com/dujiao-next/internal/shared/passwordpolicy"
	"github.com/dujiao-next/internal/version"
	"github.com/dujiao-next/internal/web"

	"github.com/gin-gonic/gin"
)

const (
	ansiReset     = "\033[0m"
	ansiBold      = "\033[1m"
	ansiDim       = "\033[2m"
	ansiGreen     = "\033[32m"
	ansiBlue      = "\033[34m"
	ansiCyan      = "\033[36m"
	ansiBrightMag = "\033[95m"
)

func main() {
	// admin 子命令短路：./dujiao-api admin <subcommand>
	// 在 banner / web 提示 / migrate / default admin / app.Run 之前处理，
	// 避免运维操作时打印一堆无关日志。
	if len(os.Args) >= 2 && os.Args[1] == "admin" {
		runAdminSubcommand(os.Args[2:])
		return
	}

	printStartupBanner()

	// 加载配置
	cfg := config.Load()
	logger.Init(cfg.Server.Mode, cfg.Log.ToLoggerOptions())
	stdLog := logger.StdLogger()

	weakSecrets := weakRuntimeSecretNames(cfg)
	if len(weakSecrets) > 0 {
		stdLog.Fatalf("以下运行时密钥过弱、重复或仍为默认值，请配置彼此独立的强随机密钥: %s", strings.Join(weakSecrets, ", "))
	}
	defaultAdminUser, defaultAdminPass := resolveDefaultAdminCredentials(cfg)
	if unsafeBootstrapAdminPassword(cfg, defaultAdminPass) {
		if cfg.Server.Mode == "release" {
			stdLog.Fatalf("bootstrap.default_admin_password 为已知默认值或不符合密码策略；请配置强密码，或留空以跳过默认管理员初始化")
		}
		stdLog.Printf("警告: bootstrap.default_admin_password 为已知默认值或不符合密码策略")
	}

	// admin_path 的校验必须赶在数据库初始化和 AutoMigrate 之前。
	// 它原本发生在路由注册阶段，也就是迁移跑完之后 —— 那样一个配置字符串不合法
	// 就会留下「schema 已经前进、进程却起不来」的半吊子状态，回滚旧二进制还要面对新库。
	// 提前到这里，配置有问题就干净地退出，什么都没动。
	if web.Enabled() {
		if err := web.ValidateAdminPath(cfg.Web.AdminPath); err != nil {
			stdLog.Fatalf("web.admin_path 配置错误: %v", err)
		}
		fmt.Println(ansiGreen + "Embedded SPAs: admin (" + cfg.Web.AdminPath + "), user (/)" + ansiReset)
	}

	// fullstack 模式下若仍使用默认 admin 路径，提示安全风险
	if web.Enabled() && cfg.Server.Mode == "release" && cfg.Web.AdminPath == "/admin" {
		stdLog.Printf("警告: web.admin_path 仍为默认 /admin，建议修改为不易猜测的路径以降低自动化扫描风险")
	}

	// 自动创建数据目录（fullstack 二进制小白部署免去手动 mkdir）
	for _, dir := range []string{"db", "uploads", "logs"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			stdLog.Printf("警告: 创建目录 %s 失败: %v", dir, err)
		}
	}

	// 初始化数据库
	if err := gormdb.InitDB(cfg.Database.Driver, cfg.Database.DSN, gormdb.DBPoolConfig{
		MaxOpenConns:           cfg.Database.Pool.MaxOpenConns,
		MaxIdleConns:           cfg.Database.Pool.MaxIdleConns,
		ConnMaxLifetimeSeconds: cfg.Database.Pool.ConnMaxLifetimeSeconds,
		ConnMaxIdleTimeSeconds: cfg.Database.Pool.ConnMaxIdleTimeSeconds,
	}, cfg.Server.Mode); err != nil {
		stdLog.Fatalf("数据库初始化失败: %v", err)
	}

	// 自动迁移数据库表
	if err := databasemigrations.AutoMigrate(); err != nil {
		stdLog.Fatalf("数据库迁移失败: %v", err)
	}

	// 初始化默认管理员账号
	if cfg.Server.Mode == "release" && defaultAdminPass == "" {
		stdLog.Printf("警告: 未设置 DJ_DEFAULT_ADMIN_PASSWORD 且 bootstrap.default_admin_password 为空，已跳过默认管理员初始化")
	} else if err := adminapplication.InitDefaultAdmin(adminstore.New(gormdb.DB), defaultAdminUser, defaultAdminPass); err != nil {
		stdLog.Printf("警告: 初始化默认管理员失败: %v", err)
	}

	// 设置 Gin 模式
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 解析命令行参数
	var mode string
	flag.StringVar(&mode, "mode", app.ModeAll, "启动模式: all (默认), api, worker")
	flag.Parse()

	if err := app.Run(app.Options{
		Config:  cfg,
		Logger:  logger.S(),
		Signals: []os.Signal{syscall.SIGINT, syscall.SIGTERM},
		Mode:    mode,
	}); err != nil {
		stdLog.Fatalf("服务运行失败: %v", err)
	}
}

func printStartupBanner() {
	writeStartupBanner(os.Stdout)
}

func writeStartupBanner(w io.Writer) {
	fmt.Fprintln(w, ansiBrightMag+"╔══════════════════════════════════════════════════════════════════════╗"+ansiReset)
	fmt.Fprintln(w, ansiBrightMag+"║                      🚀 Dujiao-Next 启动中                  	      ║"+ansiReset)
	fmt.Fprintln(w, ansiBrightMag+"╚══════════════════════════════════════════════════════════════════════╝"+ansiReset)
	fmt.Fprintln(w, ansiCyan+"██████╗ ██╗   ██╗     ██╗ █████╗  ██████╗      ███╗   ██╗███████╗██╗  ██╗████████╗"+ansiReset)
	fmt.Fprintln(w, ansiCyan+"██╔══██╗██║   ██║     ██║██╔══██╗██╔═══██╗     ████╗  ██║██╔════╝╚██╗██╔╝╚══██╔══╝"+ansiReset)
	fmt.Fprintln(w, ansiCyan+"██║  ██║██║   ██║     ██║███████║██║   ██║     ██╔██╗ ██║█████╗   ╚███╔╝    ██║   "+ansiReset)
	fmt.Fprintln(w, ansiCyan+"██║  ██║██║   ██║██   ██║██╔══██║██║   ██║     ██║╚██╗██║██╔══╝   ██╔██╗    ██║   "+ansiReset)
	fmt.Fprintln(w, ansiCyan+"██████╔╝╚██████╔╝╚█████╔╝██║  ██║╚██████╔╝     ██║ ╚████║███████╗██╔╝ ██╗   ██║   "+ansiReset)
	fmt.Fprintln(w, ansiCyan+"╚═════╝  ╚═════╝  ╚════╝ ╚═╝  ╚═╝ ╚═════╝      ╚═╝  ╚═══╝╚══════╝╚═╝  ╚═╝   ╚═╝   "+ansiReset)
	fmt.Fprintln(w, ansiGreen+ansiBold+"Open Source Repositories"+ansiReset)
	fmt.Fprintln(w, ansiBlue+"• Organization:  https://github.com/dujiao-next"+ansiReset)
	fmt.Fprintln(w, ansiBlue+"• Main:    		 https://github.com/dujiao-next/dujiao-next"+ansiReset)
	fmt.Fprintln(w, ansiBlue+"• Official:		 https://dujiao-next.com"+ansiReset)
	fmt.Fprintln(w, ansiBlue+"• Discussion Group: https://t.me/dujiaonext_official"+ansiReset)
	fmt.Fprintln(w, ansiGreen+"Version: "+version.Version+ansiReset)
	fmt.Fprintln(w, ansiDim+"--------------------------------------------------------------"+ansiReset)
}

func isWeakSecret(secret string) bool {
	if len(secret) < 32 {
		return true
	}
	normalized := strings.ToLower(secret)
	if strings.Contains(normalized, "change-me") ||
		strings.Contains(normalized, "change-in-production") ||
		strings.Contains(normalized, "your-secret-key") {
		return true
	}
	return false
}

func weakRuntimeSecretNames(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	candidates := []struct {
		name   string
		secret string
	}{
		{name: "app.secret_key", secret: cfg.App.SecretKey},
		{name: "jwt.secret", secret: cfg.JWT.SecretKey},
		{name: "user_jwt.secret", secret: cfg.UserJWT.SecretKey},
	}

	weak := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if isWeakSecret(candidate.secret) {
			weak = append(weak, candidate.name)
		}
	}
	for idx := range candidates {
		secret := strings.TrimSpace(candidates[idx].secret)
		if secret == "" {
			continue
		}
		for other := 0; other < idx; other++ {
			if secret != strings.TrimSpace(candidates[other].secret) {
				continue
			}
			if !containsString(weak, candidates[other].name) {
				weak = append(weak, candidates[other].name)
			}
			if !containsString(weak, candidates[idx].name) {
				weak = append(weak, candidates[idx].name)
			}
		}
	}
	return weak
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func unsafeBootstrapAdminPassword(cfg *config.Config, password string) bool {
	password = strings.TrimSpace(password)
	if password == "" {
		return false
	}
	switch strings.ToLower(password) {
	case "admin", "admin123", "password", "password123", "change-me", "changeme":
		return true
	}
	return cfg == nil || passwordpolicy.Validate(cfg.Security.PasswordPolicy.ValidationPolicy(), password) != nil
}

// runAdminSubcommand 处理 ./dujiao-api admin <subcommand>，仅初始化 DB
// 后委托给 internal/admincmd 包，不启动 HTTP / worker / web 等服务。
func runAdminSubcommand(args []string) {
	cfg := config.Load()
	if err := gormdb.InitDB(cfg.Database.Driver, cfg.Database.DSN, gormdb.DBPoolConfig{
		MaxOpenConns:           cfg.Database.Pool.MaxOpenConns,
		MaxIdleConns:           cfg.Database.Pool.MaxIdleConns,
		ConnMaxLifetimeSeconds: cfg.Database.Pool.ConnMaxLifetimeSeconds,
		ConnMaxIdleTimeSeconds: cfg.Database.Pool.ConnMaxIdleTimeSeconds,
	}, cfg.Server.Mode); err != nil {
		fmt.Fprintf(os.Stderr, "init db: %v\n", err)
		os.Exit(1)
	}
	admincmd.Run(args)
}

// resolveDefaultAdminCredentials 解析默认管理员初始化凭据（环境变量优先，其次 config.yml）
func resolveDefaultAdminCredentials(cfg *config.Config) (string, string) {
	user := strings.TrimSpace(os.Getenv("DJ_DEFAULT_ADMIN_USERNAME"))
	pass := strings.TrimSpace(os.Getenv("DJ_DEFAULT_ADMIN_PASSWORD"))
	if cfg == nil {
		return user, pass
	}
	if user == "" {
		user = strings.TrimSpace(cfg.Bootstrap.DefaultAdminUsername)
	}
	if pass == "" {
		pass = strings.TrimSpace(cfg.Bootstrap.DefaultAdminPassword)
	}
	return user, pass
}
