# nexus3

`nexus3` 目前分成四类能力：

- 本地 npm 工具：`npm-cache`、`npm-pkg`
- Nexus3 命令：`backup-download`、`backup-upload`、`component-download`、`component-upload`
- Nexus2 导出命令：`nexus2-backup-download`、`nexus2-component-download`
- Nexus2 导入命令：`nexus2-backup-upload`、`nexus2-component-upload`

更多按场景组织的案例见 [docs/use-cases.md](/mnt/d/gitProjects/nexus3/docs/use-cases.md)。

旧的 `upload`、`npm-down`、`npm-pack` 已从 CLI 移除，其中 npm 相关命令改名为 `npm-cache`、`npm-pkg`。

## 快速判断

如果你的目标是：

- 给前端项目做依赖离线缓存：用 `npm-cache`
- 把本地 `node_modules` / `.pnpm` 重新打成 tarball：用 `npm-pkg`
- 备份整个 Nexus3 Maven 或 npm 仓库：用 `backup-download`
- 把备份重新恢复到 Nexus3：用 `backup-upload`
- 只迁移某一个 Maven 组件：用 `component-download` + `component-upload`
- 只迁移某一个 npm 包：用 `component-download` + `component-upload`
- 从 Nexus2 导出 Maven 或 npm 数据：用 `nexus2-backup-download` 或 `nexus2-component-download`
- 把 Nexus3 的备份数据导入 Nexus2：用 `nexus2-backup-upload` 或 `nexus2-component-upload`

## 本地 npm 工具

### `npm-cache`

用途：

- 从 `package-lock.json`
- 从 `yarn.lock`
- 或从现有 `node_modules`

解析 npm tarball 地址并下载到本地目录，适合做离线缓存或制品预热。

示例：

```bash
go build -o nexus3 .

./nexus3 npm-cache ./package-lock.json ./store

./nexus3 npm-cache ./yarn.lock ./store

./nexus3 npm-cache ./node_modules ./store

./nexus3 npm-cache -k ./package-lock.json ./store
```

### `npm-pkg`

用途：

- 扫描项目下的 `.pnpm` 或 `node_modules`
- 对每个发现的包执行一次 `npm pack`
- 把生成的 `.tgz` 放到指定目录

适合把“已经安装好的依赖”重新导出为 tarball。

示例：

```bash
go build -o nexus3 .

./nexus3 npm-pkg ./my-project ./store
```

## Nexus3 命令

### `backup-download`

用途：

- 从 Nexus3 下载整个 Maven 仓库
- 或下载整个 npm 仓库
- 生成 `nexus3-backup.json` 清单和 `assets/` 文件目录

示例：

```bash
go build -o nexus3 .

./nexus3 backup-download \
  --format maven \
  --base-url https://host \
  --repository maven-releases \
  -u admin -p 123456 \
  ./backup

./nexus3 backup-download \
  --format npm \
  --repo-url https://host/repository/npm-hosted \
  -u admin -p 123456 \
  ./backup
```

### `backup-upload`

用途：

- 把 `backup-download` 导出的备份重新恢复到 Nexus3
- Maven 按 repository path 逐个 PUT 回仓库
- npm 按 `.tgz` 逐个调用 Nexus3 `components` 上传接口

示例：

```bash
go build -o nexus3 .

./nexus3 backup-upload \
  --format maven \
  --repo-url https://host/repository/maven-releases \
  -u admin -p 123456 \
  ./backup

./nexus3 backup-upload \
  --format npm \
  --base-url https://host \
  --repository npm-hosted \
  -u admin -p 123456 \
  ./backup
```

### `component-download`

用途：

- 下载指定 Maven 组件
- 或下载指定 npm 包
- 输出为可回传的备份目录

Maven 示例：

```bash
./nexus3 component-download \
  --format maven \
  --repo-url https://host/repository/maven-releases \
  --group-id com.example \
  --artifact-id demo \
  --version 1.0.0 \
  -u admin -p 123456 \
  ./component
```

npm 示例：

```bash
./nexus3 component-download \
  --format npm \
  --repo-url https://host/repository/npm-hosted \
  --name @scope/demo \
  --version 1.0.0 \
  -u admin -p 123456 \
  ./component
```

### `component-upload`

用途：

- 上传指定 Maven 组件备份
- 或上传指定 npm `.tgz` / npm 组件备份目录

Maven 示例：

```bash
./nexus3 component-upload \
  --format maven \
  --repo-url https://host/repository/maven-releases \
  -u admin -p 123456 \
  ./component
```

npm 示例：

```bash
./nexus3 component-upload \
  --format npm \
  --base-url https://host \
  --repository npm-hosted \
  -u admin -p 123456 \
  ./package.tgz
```

## Nexus2 导入命令

这组命令的目标是：

- 输入 `backup-download` 或 `component-download` 产出的 Nexus3 备份目录
- 或者输入 npm `.tgz`
- 导入到 Nexus2 仓库

### `nexus2-backup-upload`

用途：

- 把 Nexus3 的整库备份目录导入到 Nexus2
- 也支持导入一个组件备份目录，只是语义上更推荐整库备份时使用

Maven 示例：

```bash
./nexus3 nexus2-backup-upload \
  --format maven \
  --repo-url https://host/content/repositories/releases \
  -u admin -p 123456 \
  ./backup
```

npm 示例：

```bash
./nexus3 nexus2-backup-upload \
  --format npm \
  --repo-url https://host/content/repositories/npm-hosted \
  -u admin -p 123456 \
  ./backup
```

### `nexus2-component-upload`

用途：

- 把一个 Nexus3 组件备份导入到 Nexus2
- Maven 可直接输入组件备份目录
- npm 可输入组件备份目录或单个 `.tgz`

Maven 示例：

```bash
./nexus3 nexus2-component-upload \
  --format maven \
  --repo-url https://host/content/repositories/releases \
  -u admin -p 123456 \
  ./component
```

npm 示例：

```bash
./nexus3 nexus2-component-upload \
  --format npm \
  --repo-url https://host/content/repositories/npm-hosted \
  -u admin -p 123456 \
  ./package.tgz
```

## Nexus2 导出命令

这组命令的输出格式与 `backup-download` / `component-download` 一致：

- 产出 `nexus3-backup.json`
- 产出 `assets/` 目录
- 可直接导回 Nexus3
- 也可以再导回 Nexus2

### `nexus2-backup-download`

用途：

- 从 Nexus2 导出整个 Maven 仓库
- 或导出整个 npm 仓库
- 输出为统一备份格式

Maven 示例：

```bash
./nexus3 nexus2-backup-download \
  --format maven \
  --repo-url https://host/content/repositories/releases \
  -u admin -p 123456 \
  ./backup
```

npm 示例：

```bash
./nexus3 nexus2-backup-download \
  --format npm \
  --repo-url https://host/content/repositories/npm-hosted \
  -u admin -p 123456 \
  ./backup
```

### `nexus2-component-download`

用途：

- 从 Nexus2 导出指定 Maven 组件
- 或导出指定 npm 包
- 输出为单组件备份目录

Maven 示例：

```bash
./nexus3 nexus2-component-download \
  --format maven \
  --repo-url https://host/content/repositories/releases \
  --group-id com.example \
  --artifact-id demo \
  --version 1.0.0 \
  -u admin -p 123456 \
  ./component
```

npm 示例：

```bash
./nexus3 nexus2-component-download \
  --format npm \
  --repo-url https://host/content/repositories/npm-hosted \
  --name demo-pkg \
  --version 1.0.0 \
  -u admin -p 123456 \
  ./component
```

## 典型场景

### 场景一：前端依赖离线缓存

```bash
./nexus3 npm-cache ./package-lock.json ./offline-store
```

结果：

- `offline-store/` 下会得到一批 `.tgz`

### 场景二：迁移单个 Maven 组件

先从源仓库下载：

```bash
./nexus3 component-download \
  --format maven \
  --repo-url https://source-host/repository/maven-releases \
  --group-id com.example \
  --artifact-id demo \
  --version 1.2.3 \
  -u sourceUser -p sourcePass \
  ./demo-backup
```

再上传到目标仓库：

```bash
./nexus3 component-upload \
  --format maven \
  --repo-url https://target-host/repository/maven-releases \
  -u targetUser -p targetPass \
  ./demo-backup
```

### 场景三：迁移整个 npm 仓库

先备份：

```bash
./nexus3 backup-download \
  --format npm \
  --repo-url https://source-host/repository/npm-hosted \
  -u sourceUser -p sourcePass \
  ./npm-backup
```

再恢复：

```bash
./nexus3 backup-upload \
  --format npm \
  --repo-url https://target-host/repository/npm-hosted \
  -u targetUser -p targetPass \
  ./npm-backup
```

### 场景四：把 Nexus3 备份导入 Nexus2

先从 Nexus3 导出：

```bash
./nexus3 backup-download \
  --format maven \
  --repo-url https://source-host/repository/maven-releases \
  -u sourceUser -p sourcePass \
  ./maven-backup
```

再导入 Nexus2：

```bash
./nexus3 nexus2-backup-upload \
  --format maven \
  --repo-url https://target-host/content/repositories/releases \
  -u targetUser -p targetPass \
  ./maven-backup
```

npm 也是同样的思路：

```bash
./nexus3 backup-download \
  --format npm \
  --repo-url https://source-host/repository/npm-hosted \
  -u sourceUser -p sourcePass \
  ./npm-backup

./nexus3 nexus2-backup-upload \
  --format npm \
  --repo-url https://target-host/content/repositories/npm-hosted \
  -u targetUser -p targetPass \
  ./npm-backup
```

### 场景五：从 Nexus2 导出再回传 Nexus3

先从 Nexus2 导出：

```bash
./nexus3 nexus2-component-download \
  --format maven \
  --repo-url https://source-host/content/repositories/snapshots \
  --group-id com.example \
  --artifact-id demo \
  --version 1.0.0-SNAPSHOT \
  -u sourceUser -p sourcePass \
  ./component
```

再导入 Nexus3：

```bash
./nexus3 component-upload \
  --format maven \
  --repo-url https://target-host/repository/maven-snapshots \
  -u targetUser -p targetPass \
  ./component
```

npm 也是同样流程：

```bash
./nexus3 nexus2-component-download \
  --format npm \
  --repo-url https://source-host/content/repositories/npm-hosted \
  --name demo-pkg \
  --version 1.0.0 \
  -u sourceUser -p sourcePass \
  ./component

./nexus3 component-upload \
  --format npm \
  --repo-url https://target-host/repository/npm-hosted \
  -u targetUser -p targetPass \
  ./component
```

## 备份目录结构

`backup-download` 或 `component-download` 的输出大致如下：

```text
backup/
├── nexus3-backup.json
└── assets/
    └── ...
```

Maven 示例：

```text
backup/
├── nexus3-backup.json
└── assets/
    └── com/example/demo/1.0.0/
        ├── demo-1.0.0.jar
        └── demo-1.0.0.pom
```

npm 示例：

```text
backup/
├── nexus3-backup.json
└── assets/
    └── @scope/demo/-/
        └── demo-1.0.0.tgz
```

## 参数说明

- `-k` / `--insecure`：跳过 HTTPS 证书校验，适合自签名证书环境
- 所有 Nexus 地址都必须显式带 `http://` 或 `https://`
- `--base-url` 传 Nexus 根地址，例如 `https://host`
- `--repository` 传仓库名，例如 `maven-releases`、`npm-hosted`
- `--repo-url` 传完整仓库地址，例如 `https://host/repository/maven-releases`
- Nexus2 的 `--repo-url` 形式通常是 `https://host/content/repositories/<repo>`

## 当前实现范围

- Nexus3：支持 Maven / npm 的整库备份恢复、指定组件下载上传
- Nexus2：支持 Maven / npm 的整库导出、指定组件导出，以及把 Nexus3 备份数据导入到 Maven / npm 仓库
- 当前仓库额外保留了 `LatestVersion` 的 Go 实现，可用于读取 `maven-metadata.xml` 获取三段式版本号中的最新版本
