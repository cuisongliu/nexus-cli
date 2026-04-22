# nexus3 使用案例

这份文档不重复参数说明，重点是回答：

- 什么时候该用哪个命令
- 源仓库和目标仓库分别怎么配
- Maven 和 npm 在 Nexus3 / Nexus2 之间怎么迁移
- `http://` 和 `-k` 什么时候该用

## 地址规则

先记住两种仓库地址形式：

- Nexus3 仓库地址：`https://host/repository/<repo>`
- Nexus2 仓库地址：`https://host/content/repositories/<repo>`

例如：

- Nexus3 Maven: `https://nexus3.example.com/repository/maven-releases`
- Nexus3 npm: `https://nexus3.example.com/repository/npm-hosted`
- Nexus2 Maven: `https://nexus2.example.com/content/repositories/releases`
- Nexus2 npm: `https://nexus2.example.com/content/repositories/npm-hosted`

## 场景一：给前端项目做离线缓存

你手里有：

- `package-lock.json`
- 或 `yarn.lock`
- 或现成的 `node_modules`

你想要：

- 把依赖对应的 tarball 全部缓存到本地目录

命令：

```bash
./nexus3 npm-cache ./package-lock.json ./offline-store
```

也可以：

```bash
./nexus3 npm-cache ./yarn.lock ./offline-store

./nexus3 npm-cache ./node_modules ./offline-store
```

如果依赖下载地址是 HTTPS 自签名证书：

```bash
./nexus3 npm-cache -k ./package-lock.json ./offline-store
```

## 场景二：把本地 node_modules 重新打成 tarball

你手里有：

- 一个已经安装过依赖的项目目录

你想要：

- 把里面的包重新 `npm pack`
- 导出一批 `.tgz`

命令：

```bash
./nexus3 npm-pkg ./my-project ./pkg-store
```

这个命令不走网络，不需要 `-k`。

## 场景三：备份整个 Nexus3 Maven 仓库

你手里有：

- 一个 Nexus3 Maven hosted 仓库

你想要：

- 把整个仓库导出成统一备份格式

命令：

```bash
./nexus3 backup-download \
  --format maven \
  --repo-url https://nexus3.example.com/repository/maven-releases \
  -u admin -p 123456 \
  ./maven-backup
```

导出结果：

- `maven-backup/nexus3-backup.json`
- `maven-backup/assets/...`

如果目标是 HTTP 服务，也可以直接写：

```bash
./nexus3 backup-download \
  --format maven \
  --repo-url http://nexus3.example.com/repository/maven-releases \
  -u admin -p 123456 \
  ./maven-backup
```

## 场景四：备份整个 Nexus3 npm 仓库

命令：

```bash
./nexus3 backup-download \
  --format npm \
  --repo-url https://nexus3.example.com/repository/npm-hosted \
  -u admin -p 123456 \
  ./npm-backup
```

如果证书不可信：

```bash
./nexus3 backup-download \
  --format npm \
  --repo-url https://nexus3.example.com/repository/npm-hosted \
  -u admin -p 123456 \
  -k \
  ./npm-backup
```

## 场景五：把整个 Nexus3 备份恢复到另一个 Nexus3

### Maven

先导出：

```bash
./nexus3 backup-download \
  --format maven \
  --repo-url https://source-nexus3/repository/maven-releases \
  -u sourceUser -p sourcePass \
  ./maven-backup
```

再导入：

```bash
./nexus3 backup-upload \
  --format maven \
  --repo-url https://target-nexus3/repository/maven-releases \
  -u targetUser -p targetPass \
  ./maven-backup
```

### npm

先导出：

```bash
./nexus3 backup-download \
  --format npm \
  --repo-url https://source-nexus3/repository/npm-hosted \
  -u sourceUser -p sourcePass \
  ./npm-backup
```

再导入：

```bash
./nexus3 backup-upload \
  --format npm \
  --repo-url https://target-nexus3/repository/npm-hosted \
  -u targetUser -p targetPass \
  ./npm-backup
```

## 场景六：只迁移一个 Maven 组件（Nexus3 -> Nexus3）

先导出单组件：

```bash
./nexus3 component-download \
  --format maven \
  --repo-url https://source-nexus3/repository/maven-releases \
  --group-id com.example \
  --artifact-id demo \
  --version 1.2.3 \
  -u sourceUser -p sourcePass \
  ./demo-component
```

再导入目标仓库：

```bash
./nexus3 component-upload \
  --format maven \
  --repo-url https://target-nexus3/repository/maven-releases \
  -u targetUser -p targetPass \
  ./demo-component
```

## 场景七：只迁移一个 npm 包（Nexus3 -> Nexus3）

先导出：

```bash
./nexus3 component-download \
  --format npm \
  --repo-url https://source-nexus3/repository/npm-hosted \
  --name @scope/demo \
  --version 1.2.3 \
  -u sourceUser -p sourcePass \
  ./demo-component
```

再导入：

```bash
./nexus3 component-upload \
  --format npm \
  --repo-url https://target-nexus3/repository/npm-hosted \
  -u targetUser -p targetPass \
  ./demo-component
```

## 场景八：把 Nexus3 备份导入 Nexus2

这是“源是 Nexus3，目标是 Nexus2”的场景。

### Maven

先从 Nexus3 导出：

```bash
./nexus3 backup-download \
  --format maven \
  --repo-url https://source-nexus3/repository/maven-snapshots \
  -u sourceUser -p sourcePass \
  ./maven-backup
```

再导入 Nexus2：

```bash
./nexus3 nexus2-backup-upload \
  --format maven \
  --repo-url https://target-nexus2/content/repositories/snapshots \
  -u targetUser -p targetPass \
  ./maven-backup
```

### npm

先从 Nexus3 导出：

```bash
./nexus3 backup-download \
  --format npm \
  --repo-url https://source-nexus3/repository/npm-hosted \
  -u sourceUser -p sourcePass \
  ./npm-backup
```

再导入 Nexus2：

```bash
./nexus3 nexus2-backup-upload \
  --format npm \
  --repo-url https://target-nexus2/content/repositories/npm-hosted \
  -u targetUser -p targetPass \
  ./npm-backup
```

## 场景九：把 Nexus2 导出后导回 Nexus3

这是“源是 Nexus2，目标是 Nexus3”的场景。

### Maven

先从 Nexus2 导出：

```bash
./nexus3 nexus2-component-download \
  --format maven \
  --repo-url https://source-nexus2/content/repositories/snapshots \
  --group-id com.example \
  --artifact-id demo \
  --version 1.0.0-SNAPSHOT \
  -u sourceUser -p sourcePass \
  ./maven-component
```

再导入 Nexus3：

```bash
./nexus3 component-upload \
  --format maven \
  --repo-url https://target-nexus3/repository/maven-snapshots \
  -u targetUser -p targetPass \
  ./maven-component
```

### npm

先从 Nexus2 导出：

```bash
./nexus3 nexus2-component-download \
  --format npm \
  --repo-url https://source-nexus2/content/repositories/npm-hosted \
  --name demo-pkg \
  --version 1.0.0 \
  -u sourceUser -p sourcePass \
  ./npm-component
```

再导入 Nexus3：

```bash
./nexus3 component-upload \
  --format npm \
  --repo-url https://target-nexus3/repository/npm-hosted \
  -u targetUser -p targetPass \
  ./npm-component
```

## 场景十：只把一个 Nexus3 组件导入 Nexus2

如果你不想整库迁移，只想迁一个组件：

### Maven

```bash
./nexus3 component-download \
  --format maven \
  --repo-url https://source-nexus3/repository/maven-releases \
  --group-id com.example \
  --artifact-id demo \
  --version 1.0.0 \
  -u sourceUser -p sourcePass \
  ./component

./nexus3 nexus2-component-upload \
  --format maven \
  --repo-url https://target-nexus2/content/repositories/releases \
  -u targetUser -p targetPass \
  ./component
```

### npm

```bash
./nexus3 component-download \
  --format npm \
  --repo-url https://source-nexus3/repository/npm-hosted \
  --name demo-pkg \
  --version 1.0.0 \
  -u sourceUser -p sourcePass \
  ./component

./nexus3 nexus2-component-upload \
  --format npm \
  --repo-url https://target-nexus2/content/repositories/npm-hosted \
  -u targetUser -p targetPass \
  ./component
```

## 场景十一：HTTP 服务

如果你的服务不是 HTTPS，而是纯 HTTP，直接把地址写成 `http://` 即可。

### Nexus3 HTTP 示例

```bash
./nexus3 backup-download \
  --format maven \
  --repo-url http://nexus3.example.com/repository/maven-releases \
  -u admin -p 123456 \
  ./backup
```

### Nexus2 HTTP 示例

```bash
./nexus3 nexus2-backup-upload \
  --format npm \
  --repo-url http://nexus2.example.com/content/repositories/npm-hosted \
  -u admin -p 123456 \
  ./backup
```

## 场景十二：自签名证书 / 跳过证书校验

如果目标是 HTTPS，但证书不受信任，带 `-k`。

### Nexus3

```bash
./nexus3 component-download \
  --format maven \
  --repo-url https://nexus3.example.com/repository/maven-releases \
  --group-id com.example \
  --artifact-id demo \
  --version 1.0.0 \
  -u admin -p 123456 \
  -k \
  ./component
```

### Nexus2

```bash
./nexus3 nexus2-backup-download \
  --format npm \
  --repo-url https://nexus2.example.com/content/repositories/npm-hosted \
  -u admin -p 123456 \
  -k \
  ./backup
```

## 推荐操作顺序

如果你要做正式迁移，建议按这个顺序：

1. 先用 `component-download` / `component-upload` 验证一个小组件
2. 再做 `backup-download` / `backup-upload` 的整库迁移
3. 对 HTTPS 自签名环境先单独验证 `-k`
4. 迁移后用仓库地址或查询接口验证导入结果

## 补充说明

- `npm-pkg` 不走网络，不需要 `-k`
- `npm-cache`、Nexus3 命令、Nexus2 命令都支持 `http://` 和 `https://`
- Nexus2 的 npm 导入使用标准 npm publish JSON 方式
- Nexus2 导出的结果会统一转换成 `nexus3-backup.json + assets/` 格式，方便后续直接导回 Nexus3 或 Nexus2
