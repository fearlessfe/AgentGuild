# Docker Compose 部署

Compose 栈包含 PostgreSQL、一次性数据库迁移、后端和前端。所有服务均不向宿主机发布端口；前端仅在 Compose 网络内暴露 `8080`，数据库和后端同样只在内部网络中可访问。Agent RSA 私钥和数据库数据分别保存在命名卷 `agentguild-keys` 与 `postgres-data` 中。

## 配置并启动

复制配置示例：

```bash
cp .env.example .env
```

编辑 `.env`，必须为其中四个空白的秘密变量填写新值后才能启动；Compose 的 `${VAR:?}` 校验会拒绝任何仍为空的必填值。数据库密码应使用不含 URI 保留字符的长随机值；`CURSOR_SECRET` 和 `SESSION_COOKIE_SECRET` 分别使用至少 32 字节的独立随机秘密；本地管理员密码至少 12 个字符。示例文件不提供任何可部署的已知秘密，也不要提交填写后的 `.env`。

直接通过 localhost HTTP 访问时保留 `SESSION_COOKIE_SECURE=false`。如果用户通过外部 TLS 终止设施以 HTTPS 访问，必须设置 `SESSION_COOKIE_SECURE=true`，防止浏览器通过明文 HTTP 发送 session cookie。

构建并等待所有长期运行服务健康：

```bash
docker compose up --build --wait
```

部署平台网关或同一 Docker 网络中的反向代理应将流量转发到 `frontend:8080`。Compose 不再提供宿主机直连地址；页面使用 `.env` 中的 `LOCAL_ADMIN_PASSWORD` 登录。

## 运维命令

查看服务、迁移退出状态和健康状态：

```bash
docker compose ps -a
```

持续查看日志：

```bash
docker compose logs -f
```

停止栈但保留数据库和 Agent 签名密钥：

```bash
docker compose down
```

不要附加 `--volumes`，否则持久化数据库和 Agent RSA 私钥会被删除。

静态检查最终渲染的 Compose 配置：

```bash
sh deploy/compose_config_test.sh
```
