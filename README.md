# SigNoz 分支变更说明

本文说明当前分支相对 `main` 的两个主要变更：

1. 补齐 OIDC 登录、配置、SSO 快捷入口和登出上下文能力。
2. 加固 Docker 镜像构建，降低基础镜像和 APK 依赖安装的不稳定性。

## 1. OIDC 登录能力补齐

### OpenSpec 对应

- `openspec/specs/oidc-callback-authentication/spec.md`
- `openspec/specs/oidc-domain-configuration/spec.md`

这两个规格覆盖 OIDC callback 登录、配置字段、前端管理表单、登录页 SSO shortcut、
logout context，以及相关 OpenAPI/DTO 契约。

### 改了什么

- 新增 OIDC callback auth provider 注册，使 `oidc` auth domain 可以生成登录 URL、处理
  `/api/v1/complete/oidc` 回调，并提供 provider logout URL。
- OIDC 登录 URL 会指向 provider authorization endpoint，带上：
  - `redirect_uri=<当前域名>/api/v1/complete/oidc`
  - versioned signed `state`
  - `prompt=select_account`
  - 有效 scopes，且始终包含 `openid`
- OIDC callback state 使用 HMAC 签名校验，避免 state 被篡改。
- callback 会交换 `code`、校验 `id_token`，并在需要时从 UserInfo 补充 claims。
- 支持从 claims 中解析 email、name、groups、role，供后续用户创建和角色映射使用。
- 新增/补齐 OIDC 配置字段：
  - `scopes`
  - `emailVerifiedPolicy`
  - `enforceEmailDomain`
  - `allowJit`
- `emailVerifiedPolicy` 支持：
  - `strict`：未验证邮箱直接拒绝
  - `warn`：允许登录但记录 warning
  - `ignore`：跳过邮箱验证 claim 检查
- `allowJit=false` 时，不再自动创建用户，只允许已有用户通过 OIDC 登录。
- 新增登录页 SSO shortcut context：
  - API：`/api/v2/sessions/sso_context`
  - 前端登录页会展示可点击的 SSO 登录按钮
- 新增 OIDC logout context：
  - API：`/api/v2/sessions/logout_context`
  - 如果 provider metadata 提供 `end_session_endpoint`，返回 provider logout URL
  - logout URL 会包含 `post_logout_redirect_uri=<当前域名>/login` 和 `client_id`
- OIDC 管理表单新增可复制的集成 URL：
  - `OIDC Callback URL`
  - `Post Logout Redirect URI`

### 效果怎么样

- OIDC 可以作为一等 callback 登录方式使用，而不是只停留在配置层。
- 登录流程有签名 state 保护，回调安全性更清楚。
- 对 email 验证、邮箱域名匹配、JIT 自动建用户有更细的运营控制。
- OIDC provider 只返回 thin ID token 时，可以通过 UserInfo 补齐 email 等 claims。
- 登录页可以直接展示 SSO shortcut，用户不用先输入邮箱才能进入 OIDC 登录。
- OIDC 登出可以跳到 provider 的 end-session endpoint，减少只清本地 session 后 provider
  仍保持登录态的问题。
- OpenAPI、后端类型、前端 DTO、前端表单和 OpenSpec 的描述更加一致。

### 主要影响文件

- `pkg/authn/callbackauthn/oidccallbackauthn/authn.go`
- `pkg/signoz/authn.go`
- `pkg/modules/session/implsession/module.go`
- `pkg/modules/session/implsession/handler.go`
- `pkg/apiserver/signozapiserver/session.go`
- `pkg/types/authtypes/oidc.go`
- `pkg/types/authtypes/session.go`
- `docs/api/openapi.yml`
- `frontend/src/container/Login/index.tsx`
- `frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/Providers/AuthnOIDC.tsx`
- `frontend/src/api/v2/sessions/sso_context/get.ts`
- `frontend/src/api/v2/sessions/logout/get.ts`

## 2. Docker 镜像构建加固

### OpenSpec 对应

- `openspec/specs/community-autobuild-action-runner/spec.md`
- 需求：`Community Dockerfile SHALL use digest-pinned base image`

该规格要求社区镜像构建使用 `alpine@sha256:<digest>` 作为基础镜像，并由 workflow 将解析出的
Alpine digest 通过 `ALPINE_SHA` build arg 传给 Dockerfile。

### 改了什么

- 社区 Dockerfile 使用 digest-pinned Alpine base image：

```dockerfile
FROM alpine@sha256:${ALPINE_SHA}
```

- 部分 Dockerfile 将 `ALPINE_SHA` 默认值从占位符改成真实 digest：

```dockerfile
ARG ALPINE_SHA="1e42bbe2508154c9126d48c2b8a75420c3544343bf86fd041fb7527e017a4b4a"
```

- 四个 Dockerfile 的 CA 证书安装从 `apk update && apk add ... && rm -rf /var/cache/apk/*`
  改为：

```dockerfile
apk add --no-cache ca-certificates-bundle
```

- 给 APK 安装增加最多 3 次重试：
  - 第 1 次失败后等待 5 秒
  - 第 2 次失败后等待 10 秒
  - 第 3 次仍失败则退出构建

### 效果怎么样

- 社区镜像基础层更确定，减少 mutable tag 带来的漂移和供应链歧义。
- 本地或非 workflow 构建时，不会因为 `ALPINE_SHA` 默认值还是占位符而直接失败。
- APK 仓库或网络短暂抖动时，构建会自动重试，不会立刻失败。
- `apk add --no-cache` 避免把 APK index 写进镜像层，镜像层更干净。
- 运行时镜像仍包含 HTTPS 访问需要的 CA certificate bundle。

### 主要影响文件

- `cmd/community/Dockerfile`
- `cmd/community/Dockerfile.multi-arch`
- `cmd/enterprise/Dockerfile`
- `cmd/enterprise/Dockerfile.multi-arch`

说明：`cmd/enterprise/Dockerfile` 本次只加固 CA 证书安装过程，仍保留 `FROM alpine:3.20.3`；
digest-pinned base image 主要对应社区镜像和 multi-arch Dockerfile。

## 验证情况

已执行：

```bash
git diff --check -- cmd/community/Dockerfile cmd/community/Dockerfile.multi-arch cmd/enterprise/Dockerfile cmd/enterprise/Dockerfile.multi-arch
git diff --cached --check
openspec validate --all --json
```

结果：

- Dockerfile 空白/格式检查通过。
- README 和 OpenSpec 文档提交前 diff 检查通过。
- 3 个 OpenSpec 主规格均 valid。

未执行完整 Docker build，因为这些 Dockerfile 依赖 build context 中已经存在的目标二进制，例如：

- `target/linux-${TARGETARCH}/signoz-community`
- `target/linux-${TARGETARCH}/signoz`
- `target/linux-${ARCH}/signoz-community`
- `target/linux-${ARCH}/signoz`

## 影响范围

- OIDC 变更影响登录、SSO shortcut、OIDC auth-domain 管理、session logout context 和 API contract。
- Docker 变更只影响镜像构建过程，不改变 SigNoz 应用业务逻辑。
