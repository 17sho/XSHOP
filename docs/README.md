# XSHOP 文档导航

[← 返回项目首页](../README.md) · [正式发布](https://github.com/17sho/XSHOP/releases/latest)

## 使用与开发

| 文档 | 内容与适用范围 |
| --- | --- |
| [开发、构建与部署参考](DEVELOPMENT.md) | 默认分支的工具链、目录、架构、运行模式与本地构建；不是最新正式版源码承诺 |
| [正式升级与数据边界](UPGRADE.md) | 1.1.9 发布入口、已接入生产通道的后台升级步骤、备份与数据保留 |
| [源码隐私说明](../XSHOP_SOURCE_PRIVACY.md) | 去身份化源码快照范围；其中 fresh installation 描述面向该快照，不否定另行发布的正式升级包 |
| [用户商城技术说明](../frontend/user/README.md) | 用户前端开发参考 |
| [管理后台技术说明](../frontend/admin/README.md) | 管理前端开发参考 |

## 安全与协议

这些文档记录特定协议或修复边界，不构成“已消除所有漏洞”的保证。部署前仍需核对所用版本与实际配置。

| 文档 | 主题 |
| --- | --- |
| [依赖安全](../DEPENDENCY_SECURITY.md) | 依赖约束与安全检查 |
| [渠道签名协议](../CHANNEL_SIGNATURE_PROTOCOL.md) | 渠道请求签名与协议边界 |
| [交付安全](../FULFILLMENT_RELEASE_SECURITY.md) | 交付与敏感内容释放规则 |
| [上传与缓存安全](../UPLOAD_CACHE_SECURITY.md) | 上传内容与敏感响应缓存 |
| [支付回调安全](../internal/modules/payment/CALLBACK_SECURITY.md) | 支付回调的校验与处理边界 |

## 历史升级候选参考

- [测试通道在线升级候选（中文）](../XSHOP_ONLINE_UPGRADE.md)
- [Preview upgrade candidate（英文）](../XSHOP_PREVIEW_UPGRADE.md)

上述文件保留早期候选与审查背景。涉及私有仓库、preview 服务、候选 sequence 或操作时序的内容，不应直接用于当前公开仓库的正式生产升级；当前正式使用入口见 [正式升级](UPGRADE.md) 和对应 Release 说明。本次仅整理展示，不迁移或重写升级机制。

## 许可证与上游

- [GNU GPLv3 许可证](../LICENSE)
- [Dujiao-Next 上游项目](https://github.com/dujiao-next/dujiao-next)

XSHOP 是定制版本，不代表上游项目或其赞助商。再分发时保留许可证、版权说明及对应源码义务；上游镜像与安装器不是 XSHOP 安装方式。
