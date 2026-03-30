# TaskTracker（业务追踪）

Go 实现的业务任务与价目表管理，单二进制 + 内嵌 Web，便于部署在服务器上。

## 功能

- **任务**：公司、日期、业务、价格、价目表多选、完成状态与完成日期、月度报表与 CSV 导出
- **价目表**：服务项目与价格（多币种）
- **登录**（可选）：设置环境变量 `AUTH_PASSWORD` 后启用

## 运行

```bash
go build -o biztracker .
./biztracker
```

默认监听 `:8080`，数据目录 `./data`（可用 `DATA_DIR` 指定）。

### 环境变量

| 变量 | 说明 |
|------|------|
| `LISTEN_ADDR` | 监听地址，默认 `:8080` |
| `DATA_DIR` | 数据目录，默认 `./data` |
| `AUTH_USER` / `AUTH_PASSWORD` | 启用登录 |
| `AUTH_SECRET` | 会话签名密钥（可选） |
| `AUTH_SECURE_COOKIE` | HTTPS 下设为 `true` |

## 许可证

按仓库所有者约定使用。
