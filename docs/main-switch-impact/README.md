# 切回官方 main 分支的数据影响范围说明

## 1. 目的

本文用于说明：当前自定义改造版本切回官方 `main` 分支时，哪些数据会继承，哪些能力会退化或失效，以及可接受损失范围内的最小化风险做法。

## 2. 结论摘要

- 观测数据（Metrics / Logs / Traces）通常可以继承。
- 影响主要集中在认证域配置（`auth_domain.data`）语义和 OIDC 登录行为。
- 若组织正在使用 `ssoType=oidc` 且 `ssoEnabled=true`，切回 `main` 后登录可用性有高风险。
- 自定义 CI/CD（community DockerHub workflow）会丢失，但不影响已有观测数据。

## 3. 本次自定义改造覆盖面

本地改造主要包含三类：

1. OIDC 回调认证增强
- 新增 OIDC callback provider 注册与处理逻辑。
- 增加 state 签名校验、userinfo 回填、邮箱策略校验。

2. OIDC 配置扩展
- 新增配置字段：`scopes`、`emailVerifiedPolicy`、`enforceEmailDomain`、`allowJit`。
- 前后端 OpenAPI/DTO/UI 均已同步。

3. 自定义 action runner / 构建流程
- 新增 community 质量门禁 workflow。
- 新增 DockerHub autobuild workflow 及 runner 选择逻辑。
- community Dockerfile 改为 digest pin 基础镜像。

## 4. 影响范围矩阵（切回 main）

| 范围 | 影响等级 | 说明 |
|---|---|---|
| Metrics / Logs / Traces 历史数据 | 低 | 本次改造未引入新的数据库迁移或观测数据表结构变更，通常可继承。 |
| `auth_domain` 表结构 | 低 | 表结构不变，`data` 仍为文本 JSON。 |
| `auth_domain.data` 中新增 OIDC 字段语义 | 中-高 | 官方 `main` 对新增字段不提供同等能力；若在 `main` 上编辑并保存，可能被覆盖丢失。 |
| OIDC SSO 登录可用性 | 高 | 若组织依赖 OIDC，回退至 `main` 后可能无法保持当前登录行为。 |
| OIDC 策略控制（JIT/邮箱校验/域名校验） | 高 | 自定义策略能力会退化或失效。 |
| 自定义构建发布流程 | 中 | workflow 丢失仅影响构建发布，不直接影响业务数据。 |

## 5. 即便“用不到”也可能产生的影响

即使当前不主动使用 OIDC，只要库中存在 OIDC auth domain 记录：

- 在 `main` 上读取通常可通过（未知字段常被忽略）。
- 在 `main` 上修改并保存该域配置时，新增字段可能被清理掉。
- 后续再切回自定义分支时，策略值可能无法完整恢复（取决于是否保留过备份）。

## 6. 切换前建议（可接受丢失前提下）

### 6.1 最小备份

至少备份 `auth_domain` 全量配置：

```sql
SELECT id, name, org_id, data, created_at, updated_at
FROM auth_domain
ORDER BY updated_at DESC;
```

### 6.2 识别受影响组织

```sql
SELECT id, name, org_id
FROM auth_domain
WHERE data LIKE '%"ssoType":"oidc"%'
  AND data LIKE '%"ssoEnabled":true%';
```

### 6.3 登录兜底

切换前确认至少有一个可用的本地密码管理员账号，避免 SSO 不可用导致锁死。

## 7. 风险接受建议

若你已接受“回到 `main` 后 OIDC 定制能力丢失”：

- 可以直接切回 `main`。
- 但建议保留一份 `auth_domain` 备份，方便未来恢复策略。
- 建议在切换后做一次最小验证：登录入口、组织切换、基础查询、告警页可用性。

## 8. 何时需要回滚到自定义版本

出现以下任一情况建议立即回滚：

- OIDC 登录全部失败且短期无法切换为密码登录。
- 关键组织用户无法入站（JIT/邮箱策略变更导致）。
- 认证配置被误保存后造成大面积 SSO 异常。
