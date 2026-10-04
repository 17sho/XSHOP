<div align="center">

# XSHOP

### 数字商品商城 · 自托管 · Go + Vue

从商品管理到订单交付，连接用户商城与管理后台。

[![正式发布](https://img.shields.io/github/v/release/17sho/XSHOP?label=release&color=2563eb)](https://github.com/17sho/XSHOP/releases/latest)
[![许可证](https://img.shields.io/github/license/17sho/XSHOP?color=64748b)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26.8-00ADD8?logo=go&logoColor=white)](go.mod)
[![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)](frontend/user/package.json)

**[获取正式版](https://github.com/17sho/XSHOP/releases/latest) · [文档导航](docs/README.md) · [源码构建](docs/DEVELOPMENT.md) · [正式升级](docs/UPGRADE.md)**

</div>

---

## 项目简介

XSHOP 是基于 [Dujiao-Next](https://github.com/dujiao-next/dujiao-next) 定制的数字商品商城，包含 Go 后端、Vue 用户商城与管理后台。采用模块化单体架构，支持内嵌前后台资源的全栈构建。

本仓库提供去身份化的源码与技术文档，不包含真实站点配置、客户资料、业务数据库、订单或部署凭据。默认品牌为 XSHOP，示例配置需要自行填写。

> **版本入口**：当前正式发布为 [1.1.9](https://github.com/17sho/XSHOP/releases/tag/xshop-production-v1.1.9)，提供 Linux amd64 升级包、对应源码包、签名清单及许可证。默认分支保留源码快照，**不等同于最新正式包的源码**；复现某一正式版请使用该 Release 附带的源码包，而不是直接用 `main` 替换现有程序。

## 核心功能

| 能力 | 说明 | 源码入口 |
| --- | --- | --- |
| 商品与库存 | 分类、商品、SKU、价格与卡密库存管理 | [商品目录](internal/modules/catalog/) · [卡密](internal/modules/cardsecret/) |
| 订单与交付 | 用户 / 游客订单、自动与人工交付、订单查询 | [订单](internal/modules/order/) · [交付](internal/modules/fulfillment/) |
| 支付与钱包 | 支付渠道适配、回调处理、余额与充值 | [支付](internal/modules/payment/) · [钱包](internal/modules/wallet/) |
| 商城与内容 | 用户商城、模板切换、公告、文章与媒体管理 | [商城模板](frontend/user/src/templates/) · [内容](internal/modules/content/) |
| 账户与权限 | 用户 / 管理员认证、RBAC、TOTP 双因素认证 | [权限模型](internal/authz/) · [账户认证](internal/modules/identity/) |
| 对接与分销 | 上游商品与订单对接、渠道 API、分销站点管理 | [上游](internal/modules/upstreamapi/) · [渠道](internal/modules/channelapi/) · [分销](internal/modules/reseller/) |

功能可用性取决于配置、权限与外部服务；支付、邮件及第三方身份验证需要运营者自行配置和验收。这不是无漏洞或外部服务可用性的保证。

## 快速开始

### 已有 XSHOP 正式站：使用后台升级

已接入正式签名升级通道的站点，由**超级管理员**在后台检查更新、下载并确认应用，再按提示重启。先阅读 [正式升级与数据边界](docs/UPGRADE.md)，不要使用上游安装脚本覆盖定制程序。

### 新部署 / 开发：从本仓库源码构建

仓库没有 XSHOP 专用一键安装器。新安装需准备 Go **1.26.8**、Node **24.21.x**、pnpm **10.34.6**，并自行配置服务、反向代理、TLS 与持久化目录。

```sh
git clone https://github.com/17sho/XSHOP.git
cd XSHOP
cp config.yml.example config.yml
# 编辑 config.yml：设置独立 JWT 密钥、数据库及后台路径；勿提交真实配置。
corepack enable
(cd frontend/admin && pnpm install --frozen-lockfile && pnpm run build:fullstack)
(cd frontend/user && pnpm install --frozen-lockfile && pnpm run build)
mkdir -p internal/web/dist
cp -R frontend/admin/dist internal/web/dist/admin
cp -R frontend/user/dist internal/web/dist/user
go build -tags release,fullstack -o dujiao-next ./cmd/server
./dujiao-next
```

上述命令面向全新 checkout，构建的是默认分支快照，不会生成正式签名升级包。管理后台必须使用 `build:fullstack`，不能用普通 `build` 代替。依赖安装需要网络；SQLite、上传文件与日志目录需要持久化。

详见 [开发、构建与部署参考](docs/DEVELOPMENT.md)。仓库 [Dockerfile](Dockerfile) 也提供本地全栈镜像构建路径；上游镜像、安装器和更新包**不是 XSHOP 定制版本**。

## 正式升级与数据保留

- 正式包仅交付应用代码及内嵌前后台资源，不携带测试站数据库、配置、Redis、worker/service、会员规则、商品库存或业务记录。
- 保留正式站自己的用户、订单、余额、卡密、上传文件、密钥及运营设置；测试站的验证码设置不会复制到正式站。
- 发布到 GitHub 不等于已升级站点；必须由管理员主动确认。升级前保留程序、配置、数据库和上传文件备份，并检查版本兼容性。
- 保持生产 / 测试通道隔离、签名校验与单调递增 sequence；不要清空 journal 或重置 high-water。程序回退不恢复业务数据库，也不降低 high-water。

**[阅读完整升级说明 →](docs/UPGRADE.md)**

## 文档导航

| 你想做什么 | 从这里开始 |
| --- | --- |
| 找到全部文档与历史说明 | [文档索引](docs/README.md) |
| 本地开发、全栈构建、了解目录结构 | [开发与构建](docs/DEVELOPMENT.md) |
| 获取正式版、确认升级步骤与数据边界 | [正式升级](docs/UPGRADE.md) · [Release 记录](https://github.com/17sho/XSHOP/releases) |
| 了解源码去身份化范围 | [源码隐私说明](XSHOP_SOURCE_PRIVACY.md) |
| 查看签名协议、依赖与交付安全说明 | [安全文档入口](docs/README.md#安全与协议) |

## 技术栈与目录

**后端**：Go · Gin · GORM · SQLite / PostgreSQL · Casbin · Redis / asynq（异步任务）

**前端**：Vue 3 · TypeScript · Vite · Tailwind CSS · pnpm

**交付**：两个独立 SPA，全栈构建后通过 `go:embed` 内嵌到同一程序。

```text
cmd/                 应用与工具入口
internal/modules/    业务模块：领域、应用、契约与适配器
internal/bootstrap/  模块接线
internal/authz/      RBAC 权限模型
internal/web/        前后台内嵌资源与路由
internal/architecture/ 架构约束测试
frontend/admin/      管理后台
frontend/user/       用户商城
deploy/              部署参考模板
scripts/             构建、安全检查与候选包工具
source-workflows/    未启用的工作流参考源码
docs/                文档导航与使用说明
```

## 来源、许可证与责任边界

本项目基于 [Dujiao-Next](https://github.com/dujiao-next/dujiao-next)，保留原项目版权声明与 **GNU GPLv3** 许可证，具体条款以 [LICENSE](LICENSE) 为准。XSHOP 是定制版本，不代表上游项目或其赞助商。

发布、再分发与修改时请遵守许可证及对应源码义务。上游文档可作为技术参考，但不代表 XSHOP 的部署或升级合同。运营者应自行核实商品与交易的合法性，配置访问权限、支付与邮件服务，做好备份和安全维护。

`source-workflows/` 仅保留工作流源码作为参考，未作为 GitHub Actions 启用；不要未经审核启用自动发布或部署。
