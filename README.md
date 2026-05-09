# SigNoz Docker 镜像依赖加固说明

本文说明 commit `400ea5b4c` 中 Dockerfile 的调整内容、OpenSpec 依据、实际效果和验证情况。

## OpenSpec 依据

当前仓库没有未完成的 OpenSpec change；与本次改动最相关的是已归档并同步到主规格的：

- `openspec/specs/community-autobuild-action-runner/spec.md`
- 需求：`Community Dockerfile SHALL use digest-pinned base image`

该规格要求社区镜像构建使用 `alpine@sha256:<digest>` 作为基础镜像，并由 workflow 将解析出的
Alpine digest 通过 `ALPINE_SHA` build arg 传给 Dockerfile。归档设计中也说明了这样做的目的：
让社区镜像基础层更确定，减少基础镜像漂移和供应链歧义。

本次 README 按这个 OpenSpec 说明社区镜像相关变更；enterprise Dockerfile 的改动属于同类构建稳定性
扩展，不是该 OpenSpec 规格的直接要求。

## 变更文件

| 文件 | 与 OpenSpec 的关系 | 本次改动 |
| --- | --- | --- |
| `cmd/community/Dockerfile` | 直接相关 | 设置真实 `ALPINE_SHA` 默认 digest，并加固 CA 证书安装 |
| `cmd/community/Dockerfile.multi-arch` | 同策略扩展 | 设置真实 `ALPINE_SHA` 默认 digest，并加固 CA 证书安装 |
| `cmd/enterprise/Dockerfile` | 额外稳定性扩展 | 保持 `alpine:3.20.3`，加固 CA 证书安装 |
| `cmd/enterprise/Dockerfile.multi-arch` | 额外稳定性扩展 | 设置真实 `ALPINE_SHA` 默认 digest，并加固 CA 证书安装 |

## 改了什么

### 1. 将部分 Dockerfile 的 Alpine digest 默认值从占位符改为真实值

以下 Dockerfile 现在提供真实的 `ALPINE_SHA` 默认值：

- `cmd/community/Dockerfile`
- `cmd/community/Dockerfile.multi-arch`
- `cmd/enterprise/Dockerfile.multi-arch`

当前默认值为：

```dockerfile
ARG ALPINE_SHA="1e42bbe2508154c9126d48c2b8a75420c3544343bf86fd041fb7527e017a4b4a"
```

此前默认值是占位符：

```dockerfile
ARG ALPINE_SHA="pass-a-valid-docker-sha-otherwise-this-will-fail"
```

效果是：本地或非 workflow 构建时，如果没有显式传入 `ALPINE_SHA`，这些 Dockerfile 也能使用一个
确定的 Alpine digest，而不是因为占位符直接失败。

### 2. 将 CA 证书安装改为 `apk add --no-cache ca-certificates-bundle`

四个 Dockerfile 都不再使用下面这种写法：

```dockerfile
RUN apk update && \
    apk add ca-certificates && \
    rm -rf /var/cache/apk/*
```

现在改为安装 Alpine 的 CA bundle：

```dockerfile
apk add --no-cache ca-certificates-bundle
```

`--no-cache` 会避免把 APK index 写进镜像层，因此不再需要手动清理 `/var/cache/apk/*`。

### 3. 给 APK 安装增加重试

四个 Dockerfile 都为 APK 安装增加了最多 3 次重试：

```dockerfile
RUN set -eu; \
    attempt=1; \
    max=3; \
    until apk add --no-cache ca-certificates-bundle; do \
        if [ "${attempt}" -ge "${max}" ]; then \
            echo "apk add failed after ${max} attempts"; \
            exit 1; \
        fi; \
        sleep_time=$((attempt * 5)); \
        echo "apk add failed (attempt ${attempt}/${max}), retrying in ${sleep_time}s..."; \
        sleep "${sleep_time}"; \
        attempt=$((attempt + 1)); \
    done
```

重试等待时间为：

- 第 1 次失败后等待 5 秒
- 第 2 次失败后等待 10 秒
- 第 3 次仍失败则退出构建

## 效果怎么样

- 社区镜像继续符合 OpenSpec 中 digest-pinned base image 的方向。
- 社区 Dockerfile 默认 `ALPINE_SHA` 不再是占位符，非 workflow 构建路径更容易成功。
- multi-arch Dockerfile 与普通 Dockerfile 的 CA 证书安装方式保持一致。
- APK 仓库或网络短暂抖动时，构建不会立刻失败，会自动重试。
- `apk add --no-cache` 避免缓存 APK index，镜像层更干净。
- 运行时镜像仍包含 HTTPS 访问需要的 CA 证书 bundle。
- enterprise 普通 Dockerfile 本次没有改成 digest-pinned base image，仍为 `FROM alpine:3.20.3`；
  本次只对它的 CA 证书安装过程做了加固。

## 失败行为

如果 APK 安装连续 3 次都失败，构建会明确退出，并输出：

```text
apk add failed after 3 attempts
```

这样可以区分短暂网络抖动和持续性的依赖源问题，真实问题不会被静默吞掉。

## 验证情况

已执行格式和空白检查：

```bash
git diff --check -- cmd/community/Dockerfile cmd/community/Dockerfile.multi-arch cmd/enterprise/Dockerfile cmd/enterprise/Dockerfile.multi-arch
```

结果：通过。

未执行完整 Docker build，因为这些 Dockerfile 依赖 build context 中已经存在的目标二进制，例如：

- `target/linux-${TARGETARCH}/signoz-community`
- `target/linux-${TARGETARCH}/signoz`
- `target/linux-${ARCH}/signoz-community`
- `target/linux-${ARCH}/signoz`

## 影响范围

- 只影响 Docker 镜像构建过程。
- 不改变 SigNoz 应用运行时代码。
- 不改变前端或后端业务逻辑。
- 不改变 OpenSpec 中 OIDC 相关能力。
