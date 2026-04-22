# nexus3

`nexus3` 是对 [`gaojunxin/nexus-tools`](https://github.com/gaojunxin/nexus-tools/tree/main) 的 Go 版实现，保留了原项目的核心命令：

- `upload`：递归上传 Maven 本地仓库中的 `.jar` 和 `.pom` 到 Nexus
- `npm-down`：从 `package-lock.json`、`yarn.lock` 或 `node_modules` 中解析并下载 npm tarball
- `npm-pack`：扫描项目下的 `.pnpm` 或 `node_modules`，逐个执行 `npm pack`
- `upload` 和 `npm-down` 支持 `http://`、`https://`，并可用 `-k` 跳过 HTTPS 证书校验

## 用法

```bash
go build -o nexus3 .

./nexus3 upload -r http://127.0.0.1:8081/repository/maven-hosted -u admin -p 123456 ~/.m2/repository

./nexus3 upload -k -r https://nexus.example.com/repository/maven-hosted -u admin -p 123456 ~/.m2/repository

./nexus3 npm-down ./package-lock.json ./store

./nexus3 npm-down -k ./package-lock.json ./store

./nexus3 npm-pack ./my-project ./store
```

## 说明

- `upload` 要求 `-r` 指向实际的 Nexus repository 地址，例如 `http://host:8081/repository/maven-releases`
- `upload` 和 tarball 下载地址必须显式带 `http://` 或 `https://`
- `-k`/`--insecure` 会跳过 HTTPS 证书校验，适合自签名证书环境
- `npm-pack` 依赖本机已安装 `npm`
- 当前仓库里额外保留了 `LatestVersion` 的 Go 实现，可用于读取 `maven-metadata.xml` 获取三段式版本号中的最新版本
