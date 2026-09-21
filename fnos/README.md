# DDNS-GO (DNSHE) for 飞牛 fnOS · ARM 应用包

把 [ddns-go](https://github.com/jeessy2/ddns-go)（已集成 **DNSHE / my.dnshe.com** 免费域名服务商）打包成飞牛 fnOS 的 `.fpk` 安装包，目标架构 **arm64**（ARM NAS / 飞牛设备）。

> 本仓库即 `zhaoyeqi0323/ddns-go` 的 fork：`DNSHE` provider 代码已合入 `dns/` 目录，外加本 `fnos/` 目录的打包工程。

## 目录结构

```
ddns-go-fnos/                      (仓库根)
├── fnos/                          # fnOS 应用包内容（最终打进 .fpk）
│   ├── manifest                   # 应用元数据（platform=arm64, service_port=9876）
│   ├── cmd/service-setup          # fnOS 生命周期脚本（启动命令/进程管理）
│   ├── config/privilege           # 运行权限（空=默认）
│   ├── config/resource            # 以 package 用户 ddns-go 运行 + 数据目录
│   ├── ui/config                  # 桌面入口（Web 跳转 9876）
│   ├── ui/images/64.png           # 桌面图标
│   ├── ICON.PNG / ICON_256.PNG    # 应用中心图标
│   ├── DDNS-GO.sc                 # 防火墙/端口转发规则
│   └── bin/ddns-go-server         # 启动包装脚本（exec ./ddns-go -l :9876 -c <数据目录>）
├── build-fnos.sh                  # 交叉编译 + 打包脚本
└── .github/workflows/build-fnos.yml  # GitHub Actions 自动出包
```

`ddns-go` 二进制本体**不入库**，由构建步骤交叉编译后放入 `fnos/` 再打包。

## 本地构建（需本机有 Go 1.23+）

在 ddns-go 仓库根目录（即包含 `go.mod` 与本 `fnos/` 目录处）执行：

```bash
./build-fnos.sh arm64 6.17.7     # 生成 ddns-go_arm64.fpk
```

- 自动 `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build` 出静态二进制；
- 把版本号写进 `fnos/manifest`；
- 优先用官方 `fnpack build`，否则退化成 `tar -czf`（社区已验证 .fpk 即 tar.gz）。

## 用 GitHub Actions 自动出包（推荐）

仓库已含 `.github/workflows/build-fnos.yml`：

- **手动触发**：Actions → Build fnOS .fpk → Run workflow，可填 `version` / `arch`。产物在 Artifacts 中下载。
- **打 tag 发版**：`git tag fnos-v6.17.7 && git push --tags`，CI 会自动在 Release 里附上 `.fpk`。

## 安装到飞牛 fnOS

1. 拿到 `ddns-go_arm64.fpk`；
2. 飞牛 OS → 应用中心 → 右上角「设置」→ **手动安装** → 上传 `.fpk`；
3. 安装完成后桌面出现 DDNS-GO 图标，打开即 `http://<NAS-IP>:9876`；
4. Web 界面里 DNS 服务商选 **DNSHE**，ID 填 `X-API-Key`，Secret 填 `X-API-Secret`，域名填完整域名（如 `home.cc.cd`）。

## 说明

- 动态 DNS 解析客户端，自动更新域名解析记录；
- 已内置 DNSHE（my.dnshe.com）免费域名：支持 `us.ci / cc.cd / de5.net / ccwu.cc` 等后缀，A/AAAA 自动解析；
- 支持 IPv4 / IPv6；
- 配置数据存于应用数据目录（`-c <数据目录>/data`）。

## 还原 / 卸载

应用中心直接卸载即可；配置文件在应用数据目录下，卸载时一并清除。
