package xshopupgrade

// Identity is fixed at compilation, never selected by policy, environment or callers.
const PreviewService = "dujiao-next.service"
const PreviewRoot = "/opt/dujiao-next"
const StateRoot = "/var/lib/xshop-production-upgrader"
const RuntimeRoot = "/run/xshop-production-upgrader"
const SocketPath = RuntimeRoot + "/control.sock"
const PolicyPath = "/etc/xshop-production-upgrader/policy.json"
const TagPrefix = "xshop-production-"
const Channel = "stable"
const Profile = "embedded-production"
const Production = true
