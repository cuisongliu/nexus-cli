#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./test_uploaded_package.sh \
    --repo-url https://host/repository/maven-releases \
    --username admin \
    --password admin123 \
    --group-id com.example \
    --artifact-id demo \
    --version 1.0.0 \
    [--packaging jar] \
    [--plugin-repo-url https://host/repository/maven-public] \
    [--insecure]

What it does:
  1. Checks that the uploaded POM and main artifact are reachable from Nexus
  2. Creates a temporary Maven consumer project
  3. Builds that project against the uploaded artifact
  4. Prints "success" only when the artifact is usable

Notes:
  - Requires java and mvn in PATH
  - Works with both http:// and https:// repository URLs
  - Use --insecure only for self-signed HTTPS certificates
EOF
}

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

derive_plugin_repo_url() {
  local repo_url="$1"
  if [[ "$repo_url" =~ ^(https?://[^/]+)/repository/[^/]+/?$ ]]; then
    printf '%s/repository/maven-public\n' "${BASH_REMATCH[1]}"
    return 0
  fi
  return 1
}

REPO_URL=""
PLUGIN_REPO_URL=""
USERNAME=""
PASSWORD=""
GROUP_ID=""
ARTIFACT_ID=""
VERSION=""
PACKAGING="jar"
INSECURE="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-url)
      REPO_URL="${2:-}"
      shift 2
      ;;
    --plugin-repo-url)
      PLUGIN_REPO_URL="${2:-}"
      shift 2
      ;;
    --username)
      USERNAME="${2:-}"
      shift 2
      ;;
    --password)
      PASSWORD="${2:-}"
      shift 2
      ;;
    --group-id)
      GROUP_ID="${2:-}"
      shift 2
      ;;
    --artifact-id)
      ARTIFACT_ID="${2:-}"
      shift 2
      ;;
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --packaging)
      PACKAGING="${2:-}"
      shift 2
      ;;
    --insecure)
      INSECURE="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

[[ -n "$REPO_URL" ]] || die "--repo-url is required"
[[ -n "$USERNAME" ]] || die "--username is required"
[[ -n "$PASSWORD" ]] || die "--password is required"
[[ -n "$GROUP_ID" ]] || die "--group-id is required"
[[ -n "$ARTIFACT_ID" ]] || die "--artifact-id is required"
[[ -n "$VERSION" ]] || die "--version is required"

require_cmd java
require_cmd mvn
require_cmd curl

if [[ -z "$PLUGIN_REPO_URL" ]]; then
  PLUGIN_REPO_URL="$(derive_plugin_repo_url "$REPO_URL")" || die "--plugin-repo-url is required when repo-url is not in the form https://host/repository/<name>"
fi

REPO_URL="${REPO_URL%/}"
PLUGIN_REPO_URL="${PLUGIN_REPO_URL%/}"

group_path="${GROUP_ID//./\/}"
artifact_file="${ARTIFACT_ID}-${VERSION}.${PACKAGING}"
pom_url="${REPO_URL}/${group_path}/${ARTIFACT_ID}/${VERSION}/${ARTIFACT_ID}-${VERSION}.pom"
artifact_url="${REPO_URL}/${group_path}/${ARTIFACT_ID}/${VERSION}/${artifact_file}"

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

settings_file="${tmp_dir}/settings.xml"
consumer_dir="${tmp_dir}/consumer"
local_repo="${tmp_dir}/m2"
mkdir -p "$consumer_dir/src/main/java/testpkg" "$local_repo"

curl_args=(-fsSL -u "${USERNAME}:${PASSWORD}")
if [[ "$INSECURE" == "true" ]]; then
  curl_args+=(-k)
fi

curl "${curl_args[@]}" -o /dev/null "$pom_url"
curl "${curl_args[@]}" -o /dev/null "$artifact_url"

cat >"$settings_file" <<EOF
<settings>
  <servers>
    <server>
      <id>artifact-repo</id>
      <username>${USERNAME}</username>
      <password>${PASSWORD}</password>
    </server>
    <server>
      <id>plugin-repo</id>
      <username>${USERNAME}</username>
      <password>${PASSWORD}</password>
    </server>
  </servers>
</settings>
EOF

cat >"$consumer_dir/pom.xml" <<EOF
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>local.verify</groupId>
  <artifactId>consumer-check</artifactId>
  <version>1.0.0</version>

  <repositories>
    <repository>
      <id>artifact-repo</id>
      <url>${REPO_URL}</url>
    </repository>
  </repositories>

  <pluginRepositories>
    <pluginRepository>
      <id>plugin-repo</id>
      <url>${PLUGIN_REPO_URL}</url>
    </pluginRepository>
  </pluginRepositories>

  <dependencies>
    <dependency>
      <groupId>${GROUP_ID}</groupId>
      <artifactId>${ARTIFACT_ID}</artifactId>
      <version>${VERSION}</version>
    </dependency>
  </dependencies>

  <properties>
    <maven.compiler.source>8</maven.compiler.source>
    <maven.compiler.target>8</maven.compiler.target>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
  </properties>
</project>
EOF

cat >"$consumer_dir/src/main/java/testpkg/App.java" <<'EOF'
package testpkg;

public class App {
    public static void main(String[] args) {
        System.out.println("ok");
    }
}
EOF

mvn_args=(
  -q
  -s "$settings_file"
  -Dmaven.repo.local="$local_repo"
  -f "$consumer_dir/pom.xml"
)

if [[ "$INSECURE" == "true" ]]; then
  mvn_args+=(
    -Dmaven.wagon.http.ssl.insecure=true
    -Dmaven.wagon.http.ssl.allowall=true
  )
fi

mvn "${mvn_args[@]}" -U clean package

local_pom="${local_repo}/${group_path}/${ARTIFACT_ID}/${VERSION}/${ARTIFACT_ID}-${VERSION}.pom"
local_artifact="${local_repo}/${group_path}/${ARTIFACT_ID}/${VERSION}/${artifact_file}"

[[ -f "$local_pom" ]] || die "artifact POM was not downloaded into the local Maven repository"
[[ -f "$local_artifact" ]] || die "artifact file was not downloaded into the local Maven repository"

printf 'success\n'
