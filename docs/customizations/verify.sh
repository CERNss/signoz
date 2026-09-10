#!/usr/bin/env bash
# 客制化存活校验:rebase / 同步上游后运行,确认 docs/customizations/README.md
# 中列出的所有客制化锚点仍然存在。
#
# 用法:
#   bash docs/customizations/verify.sh          # 静态锚点校验
#   bash docs/customizations/verify.sh --full   # 追加 go test / 前端测试 / openspec validate
#
# 基线:v0.140.0(kind/spec envelope)。锚点说明见 docs/customizations/README.md。
set -u

cd "$(git rev-parse --show-toplevel)" || exit 1

PASS=0
FAIL=0
FAILED_ITEMS=()

check_file() { # check_file <path> <desc>
  if [ -e "$1" ]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    FAILED_ITEMS+=("[缺文件] $1 ($2)")
  fi
}

check_grep() { # check_grep <pattern> <path> <desc>
  if [ -e "$2" ] && grep -q -- "$1" "$2"; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    FAILED_ITEMS+=("[缺锚点] $2 :: $1 ($3)")
  fi
}

echo "== P0-1 OIDC callback 登录(后端) =="
check_file pkg/authn/callbackauthn/oidccallbackauthn/authn.go "OIDC callback provider"
check_file pkg/authn/callbackauthn/oidccallbackauthn/authn_test.go "OIDC callback tests"
check_grep "oidcProviderAndOAuth2Config" pkg/authn/callbackauthn/oidccallbackauthn/authn.go "provider/oauth2 config"
check_grep "authn.LogoutURLProvider" pkg/authn/callbackauthn/oidccallbackauthn/authn.go "logout URL provider impl"
check_grep "claimsFromUserInfo" pkg/authn/callbackauthn/oidccallbackauthn/authn.go "UserInfo claims 兜底"
# v0.140 kind/spec envelope:配置只能经 Config().OIDCConfig() 取
check_grep "Config().OIDCConfig()" pkg/authn/callbackauthn/oidccallbackauthn/authn.go "envelope 适配"
check_grep "oidccallbackauthn" pkg/signoz/authn.go "OIDC provider 注册"
check_grep "AuthNProviderOIDC" pkg/signoz/authn.go "OIDC registry key"
check_grep "LogoutURLProvider" pkg/authn/authn.go "LogoutURLProvider 接口"
check_grep "emailVerifiedPolicy" pkg/types/authtypes/oidc.go "emailVerifiedPolicy 字段"
check_grep "enforceEmailDomain" pkg/types/authtypes/oidc.go "enforceEmailDomain 字段"
check_grep "allowJit" pkg/types/authtypes/oidc.go "allowJit 字段"
check_grep "scopes,omitempty" pkg/types/authtypes/oidc.go "scopes 字段"
check_file pkg/types/authtypes/oidc_test.go "OIDC config tests"
check_grep "SessionSSOContext" pkg/types/authtypes/session.go "SSO context 类型"
check_grep "SessionLogoutContext" pkg/types/authtypes/session.go "logout context 类型"
check_grep "GetSessionSSOContext" pkg/modules/session/session.go "接口方法"
check_grep "GetSessionLogoutContext" pkg/modules/session/session.go "接口方法"
check_grep "GetSessionSSOContext" pkg/modules/session/implsession/module.go "module 实现"
check_grep "GetSessionLogoutContext" pkg/modules/session/implsession/module.go "module 实现"
check_grep "IsAllowJIT" pkg/modules/session/implsession/module.go "JIT 开关判断"
check_grep "GetSessionSSOContext" pkg/modules/session/implsession/handler.go "handler 实现"
check_file pkg/modules/session/implsession/handler_test.go "session handler tests"
check_grep "/api/v2/sessions/sso_context" pkg/apiserver/signozapiserver/session.go "sso_context 路由"
check_grep "/api/v2/sessions/logout_context" pkg/apiserver/signozapiserver/session.go "logout_context 路由"
check_grep "/api/v2/sessions/sso_context" docs/api/openapi.yml "OpenAPI path"
check_grep "/api/v2/sessions/logout_context" docs/api/openapi.yml "OpenAPI path"
check_grep "emailVerifiedPolicy" docs/api/openapi.yml "OpenAPI OIDC schema"
check_grep "enforceEmailDomain" docs/api/openapi.yml "OpenAPI OIDC schema"
check_grep "allowJit" docs/api/openapi.yml "OpenAPI OIDC schema"
check_grep "AuthtypesSessionSSOContext" docs/api/openapi.yml "OpenAPI SSO context schema"
check_grep "AuthtypesSessionLogoutContext" docs/api/openapi.yml "OpenAPI logout context schema"
# 上游 auth_domain 文档迁移(kind/spec envelope)必须继续把 oidcConfig 整块透传成 spec,
# 否则我们的 scopes/emailVerifiedPolicy/enforceEmailDomain/allowJit 会在升级时丢失。
check_grep "oidcConfig" pkg/sqlmigration/113_restructure_auth_domain_config.go "OIDC 历史配置迁移透传"
if grep -E "go-jose/v4[^/]*// indirect" go.mod >/dev/null; then
  FAIL=$((FAIL + 1)); FAILED_ITEMS+=("[缺锚点] go.mod :: go-jose/v4 应为直接依赖(不带 // indirect)")
else
  check_grep "go-jose/v4" go.mod "go-jose 直接依赖"
fi

echo "== P0-2 OIDC 配置契约 + 管理表单 =="
check_grep "api/v1/complete/oidc" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/Providers/AuthnOIDC.tsx "Callback URL 展示"
check_grep "Post Logout Redirect URI" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/Providers/AuthnOIDC.tsx "登出回跳 URL 展示"
check_grep "convertScopesArrayToString" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/CreateEdit.utils.ts "scopes 序列化"
check_grep "convertScopesStringToArray" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/CreateEdit.utils.ts "scopes 反序列化"
check_grep "allowJit" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/CreateEdit.utils.ts "allowJit 表单映射"
# v0.140 起 OIDC payload 组装从 CreateEdit.tsx 上移到 utils,并接入上游的 kind/spec envelope
check_grep "prepareOIDCConfig" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/CreateEdit.utils.ts "OIDC payload 组装"
check_grep "scopesText" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/CreateEdit.utils.ts "scopesText 表单字段"
check_file frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/CreateEdit.utils.test.ts "CreateEdit utils 测试"
check_grep "prepareOIDCConfig" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/CreateEdit.utils.test.ts "OIDC payload 测试覆盖"
check_grep "authn-provider__copy-links" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/Providers/AuthnOIDC.tsx "copy-links 容器"
check_grep "emailVerifiedPolicy" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/Providers/AuthnOIDC.tsx "邮箱策略控件"
check_grep "__copy-links" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/Providers/Providers.styles.scss "copy-links 样式"
# 上游已删除手写 DTO(types/api/v1/domains/*),字段经 openapi -> orval 流入生成类型
check_grep "allowJit" frontend/src/api/generated/services/sigNoz.schemas.ts "生成 DTO 字段"
check_grep "emailVerifiedPolicy" frontend/src/api/generated/services/sigNoz.schemas.ts "生成 DTO 字段"
check_grep "enforceEmailDomain" frontend/src/api/generated/services/sigNoz.schemas.ts "生成 DTO 字段"
check_grep "AuthtypesSessionSSOContext" frontend/src/api/generated/services/sigNoz.schemas.ts "生成 schema"
check_grep "AuthtypesSessionLogoutContext" frontend/src/api/generated/services/sigNoz.schemas.ts "生成 schema"
# OIDC 在 OSS 构建中无条件可选(上游将其门控在企业 SAML flag 后)
check_grep "enabled: true" frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/AuthnProviderSelector.tsx "OIDC 无条件启用"

echo "== P0-3 登录页 SSO 快捷入口 + 登出 =="
check_grep "handleSSOShortcutClick" frontend/src/container/Login/index.tsx "SSO shortcut 点击"
check_grep "sso_context/get" frontend/src/container/Login/index.tsx "SSO context 拉取"
check_file frontend/src/container/Login/__tests__/Login.test.tsx "Login 测试"
check_file frontend/src/api/v2/sessions/sso_context/get.ts "sso_context client"
check_file frontend/src/api/v2/sessions/logout/get.ts "logout context client"
check_file frontend/src/types/api/v2/sessions/sso_context/get.ts "sso_context 类型"
check_file frontend/src/types/api/v2/sessions/logout/get.ts "logout 类型"
check_file frontend/src/api/generated/services/sessions/index.ts "sessions service"

echo "== P0-4 导航栏版本标签覆盖 =="
check_grep "NAV_VERSION_OVERRIDE" frontend/src/container/SideNav/SideNav.tsx "版本覆盖"
check_grep "NAV_LICENSE_TAG_OVERRIDE" frontend/src/container/SideNav/SideNav.tsx "license tag 覆盖"
check_grep "NAV_DEFAULT_LICENSE_TAG = 'Community'" frontend/src/container/SideNav/SideNav.tsx "默认 license tag 为 Community"
check_grep "NAV_VERSION_OVERRIDE" frontend/vite.config.ts "env 注入"
# 展示版本由 CI 注入(与镜像 tag 同源),硬编码默认值因此不会随 rebase 过期
check_grep "nav_version" .github/workflows/autobuild-community-dockerhub.yaml "nav_version 输出"
check_grep "VITE_NAV_VERSION_OVERRIDE" .github/workflows/autobuild-community-dockerhub.yaml "版本注入"
check_grep "VITE_NAV_LICENSE_TAG_OVERRIDE: Community" .github/workflows/autobuild-community-dockerhub.yaml "license tag 注入"

echo "== P1 CI / Docker =="
check_file .github/workflows/_autobuild-community-quality.yaml "质量门 workflow"
check_file .github/workflows/autobuild-community-dockerhub.yaml "autobuild workflow"
check_grep "BUILD_RUNNER" .github/workflows/autobuild-community-dockerhub.yaml "runner 选择"
check_grep "infra/signoz/community-build" .github/workflows/autobuild-community-dockerhub.yaml "Nexus 路径"
check_grep "ALPINE_SHA" .github/workflows/autobuild-community-dockerhub.yaml "digest 传递"
check_grep "alpine@sha256" cmd/community/Dockerfile "digest-pinned base"
check_grep "alpine@sha256" cmd/community/Dockerfile.multi-arch "digest-pinned base"
for df in cmd/community/Dockerfile cmd/community/Dockerfile.multi-arch cmd/enterprise/Dockerfile cmd/enterprise/Dockerfile.multi-arch; do
  check_grep "apk add --no-cache ca-certificates-bundle" "$df" "apk 加固"
done

echo "== Spec / 文档 =="
check_file openspec/specs/oidc-callback-authentication/spec.md "spec"
check_file openspec/specs/oidc-domain-configuration/spec.md "spec"
check_file openspec/specs/community-autobuild-action-runner/spec.md "spec"
check_file openspec/changes/archive/2026-04-18-oidc-support-customized-action-runner/proposal.md "归档 change"
check_file docs/main-switch-impact/README.md "main 切换影响评估"
check_grep "/.claude/" .gitignore "本地工具目录忽略"

echo
echo "静态校验:PASS=${PASS} FAIL=${FAIL}"
if [ "$FAIL" -gt 0 ]; then
  printf '%s\n' "${FAILED_ITEMS[@]}"
fi

if [ "${1:-}" = "--full" ]; then
  echo
  echo "== full 模式:运行测试 =="
  # 本地 Go 版本高于 go.mod 要求时(如 sonic 不兼容 go1.26),强制使用 go.mod 声明的工具链
  GO_MOD_TOOLCHAIN="go$(awk '/^go /{print $2; exit}' go.mod)"
  GOTOOLCHAIN="$GO_MOD_TOOLCHAIN" go test ./pkg/authn/callbackauthn/oidccallbackauthn/... ./pkg/types/authtypes/... ./pkg/modules/session/implsession/... ./pkg/signoz/... \
    || { FAIL=$((FAIL + 1)); FAILED_ITEMS+=("[测试失败] go test OIDC/session"); }
  if command -v openspec >/dev/null 2>&1; then
    openspec validate --all || { FAIL=$((FAIL + 1)); FAILED_ITEMS+=("[校验失败] openspec validate --all"); }
  else
    echo "(openspec CLI 不在 PATH,跳过 spec 校验)"
  fi
  if [ -d frontend/node_modules ]; then
    (cd frontend && pnpm exec jest src/container/Login src/container/OrganizationSettings/AuthDomain src/container/SideNav --silent) \
      || { FAIL=$((FAIL + 1)); FAILED_ITEMS+=("[测试失败] frontend Login/AuthDomain/SideNav tests"); }
  else
    echo "(frontend/node_modules 不存在,跳过前端测试)"
  fi
fi

echo
if [ "$FAIL" -gt 0 ]; then
  echo "结果:FAIL(${FAIL} 项未通过)——客制化不完整,勿视为同步完成"
  exit 1
fi
echo "结果:PASS——全部客制化锚点存活"
