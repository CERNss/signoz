# 客制化事实清单(Customization Manifest)

本文是本 fork 相对 SigNoz 官方代码的**全部客制化内容的事实记录**,用于:

1. rebase 最新 `main` / 同步上游后,逐项确认客制化功能仍然存在;
2. 解决冲突时判断"哪边为准";
3. 快速回答"我们改了什么"。

配套校验脚本:`docs/customizations/verify.sh`(rebase 后必跑,全部 PASS 才算同步完成)。

## 基线事实(2026-09-09 快照,已 rebase 到 v0.140.0)

- 上游基线:`v0.140.0`(= `70335dc70`,官方 SigNoz tag,位于 `upstream/main`)
- 客制化分支:`develop`(经 `rebase/v0.140.0` 在 v0.140.0 上按顺序 `git cherry-pick` 重放 7 个客制化提交,
  再 `-s ours` 并入旧 develop 谱系后快进得到——与 v0.134 轮结构一致,无需 force push)
- 本轮基线 tag:`customizations-baseline-20260909-v0140`(rebase 分支合并前的 HEAD,即本轮文档提交)
- 本轮 rebase 前 develop 备份:分支 `backup-develop-pre-v0140`(= `f04295f61`,同 `v0.134.0-jhs.2`)
- 上一轮基线 tag:
  - `customizations-baseline-20260723-v0134` → `8a9abf2b6`(v0.134.0 基线上的文档提交)
  - `v0.134.0-jhs.2` → `f04295f61`(v0.134.0 基线上的最后一个客制化提交)
  - `customizations-baseline-20260723` → `996a34587`(更早的旧历史基线)
- rebase 前的完整旧历史备份:分支 `backup-develop-pre-v0134`(HEAD `a18eecada`,
  基点旧 main `c8099a88c`,含原始 9+1 个客制化提交)——本轮未改动
- 重放方式:**必须逐个 `git cherry-pick`**。`develop` 的祖先里有一个 `-s ours` 合并
  (PR #1:`18814215c` / `c2369d820`),`git rebase` / `--onto` / `--first-parent` 会拖出
  700+ 条不相关提交。
- 工具链:Go 按 go.mod 声明版本运行(当前 1.25.7;本地 Go 1.26 会让依赖 sonic 编译失败,
  所有 go 命令加 `GOTOOLCHAIN=go1.25.7`);前端 **pnpm 10.x** + Node 22,
  checkout 后先 `cd frontend && pnpm install --frozen-lockfile`(pnpm-lock.yaml 随上游变化)
- 已知噪声:`gofmt -l pkg cmd` 在**上游 v0.140.0 本身**就会列出 7 个未格式化文件
  (`impldashboard/listfilter_visitor.go`、`querier/v2/querier_test.go`、`rules/prom_rule_task.go`、
  `rules/prom_rule_test.go`、`logstelemetryschema/json_string.go`、`cloudintegrationtypes/regions.go`、
  `metricreductionruletypes/reduction_rule.go`),与客制化无关;只需保证客制化触及的 `.go` 文件干净。

> 注意:本清单的校验锚点全部基于**文件路径 + 代码符号**,与 SHA 无关,rebase 后依然有效。

### v0.140.0 rebase 中的结构性变化(2026-09-09)

**1. kind/spec envelope(上游 `5b3b2865d`,#12472)——采用上游结构**

上游把 auth domain 载荷改成判别式信封:`auth_domain.data` 由
`{ssoEnabled, ssoType, samlConfig|googleAuthConfig|oidcConfig, roleMapping}` 变成
`{enabled, config: {kind, spec}, roleMapping}`;`/api/v1/domains` 下线,资源移到
`/api/v2/auth_domains`;provider kind `google_auth` → `google`;SAML 键改名。

我们的四个 OIDC 客制化字段(`scopes` / `emailVerifiedPolicy` / `enforceEmailDomain` /
`allowJit`)本来就是 `authtypes.OIDCConfig` 的字段,而 `OIDCConfig` 现在正是
`AuthDomainConfig` 中 `kind=oidc` 那一支的 `spec`——**字段零改名、零丢失**,只是
JSON 路径从 `oidcConfig.*` 变成 `config.spec.*`。上游同时给 `issuer`/`clientId`/
`clientSecret` 加了 `required:"true"` 和 `format:"password"`,我们照单接受。

**2. 后端访问方式迁移(纯机械)**

| 旧(v0.134) | 新(v0.140) |
|---|---|
| `authDomain.AuthDomainConfig().OIDC.X` | `oidcConfig, err := authDomain.Config().OIDCConfig()` 后 `oidcConfig.X` |
| `authDomain.AuthDomainConfig().SSOEnabled` | `authDomain.Enabled()` |
| `authDomain.AuthDomainConfig().AuthNProvider` | `authDomain.Kind()` |
| `authDomain.AuthDomainConfig().RoleMapping` | `authDomain.RoleMapping()` |

涉及 `pkg/authn/callbackauthn/oidccallbackauthn/authn.go`(437 → 472 行)与
`pkg/modules/session/implsession/module.go`。副作用:`stateSigningSecret` 由返回
`string` 改为返回 `(string, error)`(取 config 会失败)。

**3. 从上游 ee OIDC provider 移植了什么**

`git diff v0.134.0 v0.140.0 -- ee/authn/callbackauthn/oidccallbackauthn/authn.go` 的全部内容
就是上面那套 envelope 适配,外加删掉 `LoginURL` 里冗余的 kind 断言(因为
`Config().OIDCConfig()` 自带类型校验)——**上游没有任何新能力可移植**。我们保留了自己的
kind 断言(改用 `authDomain.Kind()`),因为它给出明确的 `ErrCodeAuthDomainMismatch`,
而不是泛化的类型不匹配错误。ee 目录本身未改一行。

**4. 测试适配**

`authtypes.NewAuthDomainFromConfig` 被上游删除。OIDC provider 测试改用
`NewAuthDomainFromPostableAuthDomain` + `AuthDomainConfig{Kind, Spec}` 构造;原来三处
"构造后改字段"(`EmailVerifiedPolicy` / `EnforceEmailDomain` / `Scopes`)改为构造时传
`func(*authtypes.OIDCConfig)` override,行为不变。

**5. 前端:客制化落点从 `CreateEdit.tsx` 迁到 `CreateEdit.utils.ts`**

上游把 `getGoogleAuthConfig` / `getRoleMapping` 从组件搬进 utils,并新增 `prepareConfig`
生成 kind/spec envelope。我们原来在组件里的 `getOIDCConfig` 相应变成 utils 中的
**`prepareOIDCConfig`**,由 `prepareConfig` 的 oidc 分支调用;`prepareInitialValues` 在
`config.kind === oidc` 分支上补 `scopesText` 与 `allowJit ?? true`。
**结果:`CreateEdit.tsx` 现在与上游完全一致,不再是客制化文件**(锚点表已相应调整)。
`AuthnProviderSelector.tsx` 的 OIDC 卡片 `enabled: true` 仍然是客制化(上游把 SAML 卡片
门控在 `samlEnabled`,OIDC 卡片同样默认跟随企业 flag)。

**6. 新增测试(补齐结构性变化影响到的覆盖)**

`CreateEdit.utils.test.ts` 新增:scopes 文本↔数组互转、`prepareOIDCConfig` 在空 scopes 时
省略字段、`prepareConfig` 携带四个 OIDC 策略字段进 envelope、`prepareInitialValues` 的
`scopesText` / `allowJit` 回填。上游自带的 AuthDomain 测试与 mocks 无需改动即通过。

**7. sqlmigration 113 结论(`pkg/sqlmigration/113_restructure_auth_domain_config.go`)**

该迁移的 OIDC 分支把 `legacy["oidcConfig"]` **整块 JSON 原样**搬进 `config.spec`;只有
saml(键改名)和 google(删 `redirectURI`)才重写 spec 内容。因此
`scopes` / `emailVerifiedPolicy` / `enforceEmailDomain` / `allowJit` 全部透传,
**无需在 `pkg/` 里做任何补丁**。唯一需要知情的风险:迁移会**删除**那些
`ssoType` 缺失、无法解析、或对应 provider config 缺失的历史行。

**8. P2 取代情况**

- `pkg/contextlinks/alert_link_visitor.go`(`fmt.Sprintf`+WriteString → `fmt.Fprintf`):
  上游 v0.140.0 已原生实现同一改法,**本项取代完成,从清单移除**。
- `pkg/alertmanager/alertmanagerserver/distpatcher_test.go`(TestAggrGroup 竞态时序):
  上游仍未修,继续保留我们的版本。

**9. 零冲突项**

CI workflow / Dockerfile 加固(P1)在 v0.140.0 上完全无冲突;两个 workflow 用
`go-version-file: go.mod` + Node 22,不需要跟版本升级。`.gitignore` 取并集
(上游新增 `.dev/`、`.claude/worktrees/`,我们的 `/.claude/` 等保留)。上游 README.md
在 v0.134.0 → v0.140.0 之间没有任何变化,fork README 直接沿用。

### v0.134.0 rebase 中的结构性变化(2026-07-23)

- 上游已自带 `/api/v1/complete/oidc` 路由、`CreateSessionByOIDCCallback` handler 和通用
  callback session 流程(含新角色映射 `NewRolesFromCallbackIdentity`),但 **CallbackAuthN
  注册表中没有 OIDC 实现**——我们的客制化从"自建路由"变为"注册 provider 填上游预留的槽"。
- 上游删除了手写 DTO `frontend/src/types/api/v1/domains/*`,改用 openapi→orval 生成的
  `AuthtypesGettableAuthDomainDTO` 等类型;我们的 OIDC 字段经重新生成的 openapi.yml 自动
  流入 `sigNoz.schemas.ts`。openapi.yml 由 `go run ./cmd/enterprise generate openapi` 生成,
  前端生成物由 `pnpm generate:api` 生成——两者都是生成物,rebase 后重新生成即可,不手改。
- 上游把 OIDC 选项门控在企业 `samlEnabled` flag 后(`AuthnProviderSelector.tsx`),我们改回
  `enabled: true` 让 OSS 构建可用 OIDC——这是新增的显式客制化锚点。
- 上游整体重构了集成测试目录(`tests/integration/src` → `tests/integration/tests`,删除
  `fixtures/alertutils.py`),我们在旧测试文件上的 P2 修复随之丢弃。

## 优先级定义

- **P0**:业务客制化,rebase 后**必须存在**,冲突时以本分支实现为准(除非上游原生实现了等价能力,需人工评估后替换)。
- **P1**:构建/CI 客制化,必须存在,但实现细节可随上游演进调整。
- **P2**:小修复/文档,允许被上游等价修复取代,取代后可从本清单移除。

---

## P0-1 OIDC callback 登录(后端)

OIDC 作为一等 callback 登录方式:签名 state、code 交换、id_token 校验、UserInfo claims 兜底、策略化 session 创建。

**OpenSpec**:`openspec/specs/oidc-callback-authentication/spec.md`

**锚点(文件 → 必须存在的符号)**:

| 文件 | 符号/内容 |
|---|---|
| `pkg/authn/callbackauthn/oidccallbackauthn/authn.go` | 整个包(472 行);`oidcProviderAndOAuth2Config`、`claimsFromIDToken`、`claimsFromUserInfo`、`var _ authn.LogoutURLProvider`;envelope 适配后统一经 `authDomain.Config().OIDCConfig()` 取配置 |
| `pkg/authn/callbackauthn/oidccallbackauthn/authn_test.go` | 整个文件(595 行测试);domain 由 `NewAuthDomainFromPostableAuthDomain` + `AuthDomainConfig{Kind, Spec}` 构造 |
| `pkg/signoz/authn.go` | import `oidccallbackauthn`;registry 里 `authtypes.AuthNProviderOIDC: oidcCallbackAuthN`(上游只注册 `AuthNProviderGoogle`) |
| `pkg/authn/authn.go` | `LogoutURLProvider` 接口 |
| `pkg/types/authtypes/oidc.go` | 字段 `scopes`、`emailVerifiedPolicy`(strict/warn/ignore,默认 warn,`insecureSkipEmailVerified=true`→ignore)、`enforceEmailDomain`、`allowJit`(`*bool`,`IsAllowJIT()`)。该结构体即 `AuthDomainConfig` 中 `kind=oidc` 的 `spec` |
| `pkg/types/authtypes/oidc_test.go` | 整个文件 |
| `pkg/modules/session/implsession/module.go` | `GetSessionSSOContext`、`GetSessionLogoutContext`、JIT 判断 `IsAllowJIT()`(约 line 250) |
| `pkg/modules/session/implsession/handler.go` | `GetSessionSSOContext`、`GetSessionLogoutContext` handler |
| `pkg/modules/session/implsession/handler_test.go` | 整个文件 |
| `pkg/modules/session/session.go` | Module/Handler 接口含上述两个方法 |
| `pkg/types/authtypes/session.go` | `SessionSSOContext`、`SessionLogoutContext`、`SSODomainContext` |
| `pkg/apiserver/signozapiserver/session.go` | 路由 `/api/v2/sessions/sso_context`、`/api/v2/sessions/logout_context` |
| `docs/api/openapi.yml` | 上述两个 path + `emailVerifiedPolicy` 等 schema 字段 |
| `go.mod` | `github.com/go-jose/go-jose/v4` 为直接依赖(去掉 `// indirect`) |

**行为要点**(细节以 spec 为准):

- 登录 URL:`redirect_uri=<origin>/api/v1/complete/oidc`、HMAC 签名 versioned state、`prompt=select_account`、scopes 恒含 `openid`,配置了 group 角色映射时自动补 `groups`。
- 回调:验签 → 换 code → 验 id_token → email/name/groups/role 解析,email 缺失走 UserInfo 兜底(不覆盖已有 claims)。
- 策略:`emailVerifiedPolicy` 三档;`enforceEmailDomain` 大小写不敏感匹配;`allowJit=false` 只放行已存在用户;root 用户禁止 OIDC 登录。
- 回调端点成功 303 → return URL,失败 303 → `/login?...` 带错误参数。
- logout context:provider metadata 有 `end_session_endpoint` 时返回带 `post_logout_redirect_uri=<origin>/login` 和 `client_id` 的登出 URL,否则返回空 URL(前端回退本地登出)。

## P0-2 OIDC 配置契约 + 管理表单(前端)

**OpenSpec**:`openspec/specs/oidc-domain-configuration/spec.md`

| 文件 | 符号/内容 |
|---|---|
| `frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/Providers/AuthnOIDC.tsx` | scopes/policy/JIT/域名强制/UserInfo 控件;可复制 `OIDC Callback URL`(`<origin>/api/v1/complete/oidc`)和 `Post Logout Redirect URI`(`<origin>/login`),`authn-provider__copy-links` |
| `frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/CreateEdit.utils.ts` | `convertScopesStringToArray`/`convertScopesArrayToString` 文本 ↔ 数组序列化;`prepareOIDCConfig`(剥离 `scopesText`、空 scopes 时省略该字段),由上游的 `prepareConfig` oidc 分支调用;`prepareInitialValues` 的 oidc 分支回填 `scopesText` 与 `allowJit ?? true` |
| `frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/CreateEdit.utils.test.ts` | scopes 序列化 / `prepareOIDCConfig` / `prepareConfig` 策略字段 / `prepareInitialValues` 回填的用例(其余用例为上游自带) |
| ~~`.../CreateEdit/CreateEdit.tsx`~~ | v0.140 起与上游一致(逻辑已上移至 utils),**不再是客制化文件** |
| `frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/AuthnProviderSelector.tsx` | OIDC 卡片 `enabled: true`(上游为 `samlEnabled` 企业门控) |
| `frontend/src/container/OrganizationSettings/AuthDomain/CreateEdit/Providers/Providers.styles.scss` | copy-links 样式 |
| `frontend/src/api/generated/services/sigNoz.schemas.ts` | 生成物:`AuthtypesOIDCConfigDTO` 扩展字段、`AuthtypesSessionSSOContext`、`AuthtypesSessionLogoutContext`(由 openapi.yml 经 `pnpm generate:api` 生成) |

## P0-3 登录页 SSO 快捷入口 + OIDC 登出(前端)

| 文件 | 符号/内容 |
|---|---|
| `frontend/src/container/Login/index.tsx` | `getSSOContext` 拉取、`sessionSSOContext` state、`handleSSOShortcutClick`;SSO 按钮在 Next 按钮下方 |
| `frontend/src/container/Login/Login.styles.scss` | SSO shortcut 样式 |
| `frontend/src/container/Login/__tests__/Login.test.tsx` | SSO shortcut 测试 |
| `frontend/src/api/v2/sessions/sso_context/get.ts` | `GET /api/v2/sessions/sso_context` client |
| `frontend/src/api/v2/sessions/logout/get.ts` | logout context client |
| `frontend/src/types/api/v2/sessions/{sso_context,logout}/get.ts` | 类型 |
| `frontend/src/api/generated/services/sessions/index.ts` | 生成的 sessions service |
| `frontend/src/api/index.ts`、`frontend/src/api/utils.ts` | v2 client 接线 |

## P0-4 导航栏版本标签覆盖(前端)

| 文件 | 符号/内容 |
|---|---|
| `frontend/src/container/SideNav/SideNav.tsx` | `NAV_VERSION_OVERRIDE`/`NAV_LICENSE_TAG_OVERRIDE` 读取;默认 `v0.119.0` / `Free`;`NAV_LICENSE_TAG_OPTIONS = ['Cloud','Enterprise','Free','Community']` |
| `frontend/vite.config.ts` | 两个 env 注入 |
| `frontend/example.env` | 示例变量 |

## P1-1 Community 自动构建流水线(CI)

**OpenSpec**:`openspec/specs/community-autobuild-action-runner/spec.md`

| 文件 | 符号/内容 |
|---|---|
| `.github/workflows/_autobuild-community-quality.yaml` | 可复用质量门 workflow(整个文件,上游不存在) |
| `.github/workflows/autobuild-community-dockerhub.yaml` | 整个文件,上游不存在。要点:runner 选择(dispatch input `runner` > 仓库变量 `BUILD_RUNNER` > 默认 `linux`;仓库变量现已设为 `linux`);质量门失败不构建;tag 规则(main→`latest`+branch+sha,git tag→`vX.Y.Z`);Alpine digest 解析后经 `ALPINE_SHA` build arg 传入;产物暂存按 sub2api 模式自适应:self-hosted runner 且 NEXUS_* 配置齐全时走 Nexus(`infra/signoz/community-build/<GITHUB_RUN_ID>.tar.gz`),GitHub 托管 runner 一律走 `actions/upload-artifact`(内网 Nexus 不可达) |
| `.github/workflows/gor-signoz.yaml`、`gor-signoz-community.yaml` | 各 4 行调整 |

## P1-2 Docker 镜像加固

| 文件 | 符号/内容 |
|---|---|
| `cmd/community/Dockerfile`、`cmd/community/Dockerfile.multi-arch` | `ARG ALPINE_SHA="1e42bbe2..."`(真实 digest 默认值)+ `FROM alpine@sha256:${ALPINE_SHA}`;`apk add --no-cache ca-certificates-bundle` 带 3 次重试(5s/10s 退避) |
| `cmd/enterprise/Dockerfile`、`cmd/enterprise/Dockerfile.multi-arch` | 仅 apk 重试加固;enterprise 单 arch 仍为 `FROM alpine:3.20.3`(有意保留) |

## P2 小修复与杂项(可被上游等价修复取代)

| 文件 | 内容 | 取代条件 |
|---|---|---|
| `pkg/alertmanager/alertmanagerserver/distpatcher_test.go` | TestAggrGroup 竞态时序修稳 | 上游修复同一 flaky 测试(截至 v0.140.0 未修,继续保留) |
| ~~`pkg/contextlinks/alert_link_visitor.go`~~ | `fmt.Sprintf`+WriteString → `fmt.Fprintf`(lint 修) | **已取代**:v0.140.0 上游原生实现同一改法 |
| ~~`tests/integration/fixtures/alertutils.py` 等~~ | 超时边界补 poll(**已于 v0.134 rebase 时丢弃**:上游重构整个集成测试目录) | 已取代 |
| `.gitignore` | 忽略 `/.claude/`、`/.codex/`、`/.github/prompts/`、`/.github/skills/` | 不适用,保留 |
| `README.md` | 分支变更总结(fork 自用,与上游 README 完全冲突时以本分支为准) | — |
| `docs/main-switch-impact/README.md` | 切回官方 main 的数据影响评估(关键结论:观测数据可继承;`auth_domain.data` 的 `ssoType=oidc` 组织切回后登录高风险) | — |
| `openspec/`(specs + changes/archive + config.yaml) | 全部保留 | — |

---

## Rebase / 同步上游的标准流程

1. rebase 前:确认 `git tag` 里有基线 tag,必要时新建 `customizations-baseline-<date>`。
2. 重放方式:`git checkout -b rebase/<tag> <tag>` 后**逐个 `git cherry-pick` 客制化提交**。
   不要用 `git rebase` / `--onto` / `--first-parent`——`develop` 祖先里的 `-s ours` 合并会
   拖出 700+ 条不相关提交。客制化提交清单 = `git log --oneline <上一基线>..develop`。
3. rebase 中的冲突原则:
   - P0/P1 清单内文件 → 以本分支实现为准,再把上游的无关改动手工并入;
   - 上游若原生实现了 OIDC/SSO 等价能力 → 停下来人工评估,不要机械保留;
   - 上游若做了结构性重构(如 v0.140 的 kind/spec envelope)→ **采用上游结构,把我们的字段
     和行为搬进新结构**,而不是把上游的结构改回去;
   - P2 文件 → 上游有等价修复就用上游的,并更新本清单;
   - `frontend/src/api/generated/*`、`docs/api/openapi.yml` 属生成物:冲突时一律取上游
     (`git checkout --ours`),等全部后端改动落地后统一重新生成
     (`GOTOOLCHAIN=go<go.mod 版本> go run ./cmd/enterprise generate openapi` +
     `cd frontend && pnpm generate:api`),再确认客制化字段/路径仍在。
   - **新增文件不会产生冲突,但可能编译不过**:我们的 `pkg/authn/.../oidccallbackauthn/*` 等
     文件在 git 眼里是纯新增,cherry-pick 不会报冲突,必须靠 `go build ./...` /
     `go vet` / `tsc --noEmit` 兜底。
4. 顺带确认(上游改动可能悄悄影响我们):
   - `pkg/sqlmigration/*_auth_domain*.go`:auth domain 文档迁移是否原样保留我们的 OIDC 字段;
   - `ee/authn/callbackauthn/oidccallbackauthn/authn.go` 的上游 diff:是否有值得移植进我们
     `pkg/` 版本的改进(**不要改 `ee/`**)。
5. rebase 后必跑:

```bash
bash docs/customizations/verify.sh          # 静态锚点校验(秒级)
bash docs/customizations/verify.sh --full   # 追加 go test / 前端 Login 测试 / openspec validate
```

6. 全部 PASS 后,更新本清单的"基线事实"(新 HEAD、新基点、新 tag)和"结构性变化"小节,再收工。
