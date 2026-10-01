# XSHOP 测试通道在线升级（候选实现，部署前必须独立复核）

本实现仅支持 `dujiao-preview.service`。私有源码和下载仓库固定为 `17sho/XSHOP`；不创建公开仓库、不启用 Actions、不使用官方更新包。正式站不在授权范围。

## 边界与协议

- 应用已有 JWT（含数据库令牌撤销校验）和 RBAC 先执行。另加 `admin_id` + `admin_is_super == true` 检查；即使普通管理员获得通配权限，也不允许升级。
- 后台中文页面：`xshop-upgrade`，仅超级管理员显示；确认后再次检查权限和更新摘要。
- 应用接口：`GET /api/v1/admin/xshop-upgrade/status`，空正文 `POST .../check`，`POST .../install` 正文必须严格为 `{"digest":"64位小写十六进制"}`。不接受服务名、URL、命令、文件路径、重启或降级参数。
- 根权限 helper 仅监听 `/run/xshop-preview-upgrader/control.sock`（root:dujiao，0660）。SO_PEERCRED 必须同时匹配 `dujiao` UID 和精确 cgroup `/system.slice/dujiao-preview.service`；同 UID 的生产服务不能调用。
- helper 使用 root 已有 gh 凭据读取固定私有仓库；凭据不进入源码、应用、前端、升级包或响应。无需给应用开放外网，也无需新增更宽权限。现有 gh 账户凭据范围较宽，应在运维条件允许时改用仅此仓库只读下载凭据；本候选未创建/扩权任何令牌。
- 最新正式发布（GitHub latest API）的 tag 必须以 `xshop-preview-` 开头，非 draft。预发布不会进入 latest，应将测试 B 发布为此私有仓库内的普通 release；这是下载通道选择，不是生产部署。
- manifest 使用 Ed25519 对精确 JSON 字节签名。根权限策略文件固定公钥、schema 指纹、preview/embedded-preview/linux/amd64/updater=1。Product 严格为 `XSHOP`。从 A 的确切二进制 SHA-256 到 B、sequence 1→2；源码和二进制包分别校验尺寸和 SHA-256。
- 本地额外限额：manifest 1MiB，signature 64 bytes，压缩包128MiB，源码64MiB，程序256MiB；单 gzip 流、单 USTAR 正常文件 `dujiao-next`、0755、精确 padding 和两个结束块，无 PAX/GNU/额外成员/尾随数据/链接。
- 每个私有下载子进程有超时和 stdout 限额；操作总超时10分钟。应用仅提交任务，helper 异步工作，应用重启不终止升级。
- 状态目录0700，journal0600，全局 flock 和进程内互斥。先保留旧程序并持久记录 sequence fence + pending journal，再停测试服务、确认 PID/cgroup 清空、备份配置及 SQLite、同文件系统原子替换、启动并校验 `/proc/<PID>/exe` 哈希和 JSON `/health`。失败/重启恢复仅切回已校验旧程序，不倒灌数据库，不覆盖上传文件。
- 失败 sequence 也被消费，不能重复安装。恢复失败会保持 pending 并拒绝继续更新，必须人工处理。只有 root 明确运行 `--initialize` 可以首次创建 sequence=1；常规启动缺失 journal 时失败关闭，不能自动重置防回退状态。

## 不变的旧安全合同

`versionUpdateRemoval.test.ts` 仍保留全部旧更新器/重启/降级入口、客户端、翻译、依赖的缺席断言。测试标题明确指向“旧官方更新器”；唯一读取路径改为已去激活的 `source-workflows/ci.yml`（内容未改变）。新增 `xshopUpgradeContract.test.ts` 和 Go/DOM 测试验证独立的新合同，未恢复旧机制。

复制的 sibling verifier 原始字节必须在外部 evidence 保存 SHA-256。隔离副本仅集成 XSHOP Product 和固定仓库 URL 的身份变更及相应 fixture/期望值；原仓库和 sibling 原文件不变。

## 可重现构建（先提交、再构建）

在无 node_modules/dist 的干净 checkout，使用仓库锁定的 Node24.21.x / pnpm10.34.6：

```sh
(cd frontend/admin && pnpm install --offline --frozen-lockfile)
(cd frontend/user && pnpm install --offline --frozen-lockfile)
node scripts/check_frontend_dependency_safety.cjs admin
node scripts/check_frontend_dependency_safety.cjs user
(cd frontend/admin && pnpm build:fullstack)
(cd frontend/user && pnpm build)
(cd frontend/admin && pnpm exec vitest run && node --experimental-strip-types --test --test-concurrency=1 tests/*.test.ts)
(cd frontend/user && node --experimental-strip-types --test --test-concurrency=1 tests/*.test.ts)
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go vet ./...
CGO_ENABLED=1 go test -race ./internal/customupgrade ./internal/xshopupgrade ./internal/xshophttp
python3 scripts/check_frontend_assets.py frontend/user/dist frontend/admin/dist
mkdir -p internal/web/dist
cp -a frontend/admin/dist internal/web/dist/admin
cp -a frontend/user/dist internal/web/dist/user
# OUT 必须位于 checkout 外。A/B 仅注入不同版本，源码提交和数据库结构相同。
CGO_ENABLED=0 go build -trimpath -tags release,fullstack -ldflags '-s -w -X github.com/dujiao-next/internal/version.Version=xshop-preview-a1' -o "$OUT/A/dujiao-next" ./cmd/server
CGO_ENABLED=0 go build -trimpath -tags release,fullstack -ldflags '-s -w -X github.com/dujiao-next/internal/version.Version=xshop-preview-b1' -o "$OUT/B/dujiao-next" ./cmd/server
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o "$OUT/xshop-preview-upgrader" ./cmd/xshop-preview-upgrader
go version -m "$OUT/A/dujiao-next"
go version -m "$OUT/B/dujiao-next"
# 私钥0600、保留在 checkout 和包外；切勿上传。
python3 scripts/package_xshop_candidates.py . "$OUT" "$PRIVATE_KEY"
XSHOP_REHEARSAL_DIR="$OUT" go test -count=1 -v ./internal/xshopupgrade -run TestActualCandidateChain
```

候选包生成器不发布、下载或部署。源码包是干净提交的 git archive，包含 LICENSE、对应源码、配置示例、依赖锁和构建输入，没有 Git 历史、运行配置、数据库、上传目录或私钥。单程序升级包与对应源码包/manifest/signature一起分发；保留源码包中的 GPLv3 license/版权与上游归属。

## 独立复核后的运维接线（不是自动执行脚本）

1. 复核候选提交、所有新 helper/installer 文件、测试日志、二进制 vcs.revision/vcs.modified=false、最终包 SHA-256、签名公钥和绑定的 schema 指纹。先在新的临时目录执行上述实际 A→B 与健康失败→A 演练，再核对保留的正式/测试站控制基线。
2. 在私有 XSHOP push 去标识化源码提交，再创建 `xshop-preview-b1` release，仅发布 B 的升级包、对应源码包、manifest.json、manifest.sig 和 LICENSE。保持 private 和 Actions disabled，并通过 GitHub API读回精确 tag/asset bytes 和本地 SHA-256。若 source gate/独立审查有阻塞，不发布。
3. 首次 bootstrap 只停已有 preview Writer；在站点之外的0600/0700受限位置保存旧程序、配置、SQLite一致性备份及上传文件清单。保留数据、配置/密钥、上传文件和独立禁用外网规则。将已审核 A 同文件系统原子替换为 root:root 0755；不要覆盖 DB/config/uploads、修改生产服务或共享代理。
4. 新增 root-owned helper 二进制于 `/usr/local/libexec/xshop-preview-upgrader`。`/etc/xshop-preview-upgrader/policy.json` 必须 root:root0600，填入候选 `public-key.txt`、`schema-fingerprint.txt`、实际 preview SQLite路径、loopback端口、`bootstrap_sequence:1`；路径限制为 `/opt/dujiao-preview/db/<basename>`。创建 root-owned0700 state目录和root-owned0755 runtime目录；一次性 root执行 helper `--initialize`。部署经过复核的 service 模板，再启动helper；不为应用开放网络/写二进制/执行systemctl权限。
5. 启动 preview A，用实际 JSON health + `/proc` 运行哈希验证。注意现有站点若独立静态服务后台，需同步首次 A 的 admin assets 和保留的私有 base；A/B frontend bytes相同，后续单程序升级不要求另行替换静态资源。验证实际中文入口、超级管理员有效令牌、普通管理员/匿名/撤销令牌拒绝以及同UID生产进程无法访问helper。
6. 逐表按原列检查旧数据值而非仅count，检查config/uploads与正式站baseline不变。只把 A 留在preview；不要点击安装 B。用户自行从后台检查和安装，之后重新校验B运行哈希/版本/保留数据。

## 明确限制

实现方演练采用本地受限 fixture source 和临时独立实际应用进程，不声称已经上传、部署特权helper或在用户preview上完成操作。未发布前，私有 release asset 实网下载不能被当作通过；独立审查者发布后必须真实读回并验证。内核 peer正向接线、systemd模板运行权限、实际后台静态入口、原preview数据兼容性需要部署者在独立review后验收。普通 kill/服务重启恢复依赖完整journal和旧程序；不保证磁盘损坏、恶意root或手工删journal时自动恢复，不对数据库迁移做自动回退。
