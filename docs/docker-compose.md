# Docker Compose 部署

Compose 栈包含 PostgreSQL、一次性数据库迁移、后端和前端。只有前端端口会发布到宿主机；数据库和后端仅在 Compose 内部网络中可访问。Agent RSA 私钥和数据库数据分别保存在命名卷 `agentguild-keys` 与 `postgres-data` 中。

## 配置并启动

复制配置示例：

```bash
cp .env.example .env
```

编辑 `.env`，必须为其中四个空白的秘密变量填写新值后才能启动；Compose 的 `${VAR:?}` 校验会拒绝任何仍为空的必填值。数据库密码应使用不含 URI 保留字符的长随机值；`CURSOR_SECRET` 和 `SESSION_COOKIE_SECRET` 分别使用至少 32 字节的独立随机秘密；本地管理员密码至少 12 个字符。示例文件不提供任何可部署的已知秘密，也不要提交填写后的 `.env`。

构建并等待所有长期运行服务健康：

```bash
docker compose up --build --wait
```

浏览器访问 `http://localhost:8080/login`。如果修改了 `APP_PORT`，请相应替换 URL 中的端口。页面使用 `.env` 中的 `LOCAL_ADMIN_PASSWORD` 登录。

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
