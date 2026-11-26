# 独立的 Norns redteam 服务镜像，工具链与 redteam_for_benchmark 保持一致。

ARG PYTHON_IMAGE=python:3.13.15-slim
FROM ${PYTHON_IMAGE}

ARG CLAUDE_CODE_VERSION=2.1.199
ARG NODE_VERSION=24.22.1
ARG NODE_DIST_URL=https://nodejs.org/dist
ARG NORNS_RELEASE_DIR=release/linux-x64
ARG RADARE2_VERSION=6.2.4
# —— 五向补齐的版本锚点。tag/sha256 与 heimdall downloads.lock 对齐；资产名按
# Temurin 命名规则由 tag 机械推导（8u504-b01 → 8u504b01，11/21 加号转下划线）。
ARG PWNDBG_TAG=2026.07.29
ARG JAVA8_TAG=jdk8u504-b01
ARG JAVA8_ASSET=8u504b01
ARG JAVA11_TAG=jdk-11.0.32.1%2B1
ARG JAVA11_ASSET=11.0.32.1_1
ARG JAVA21_TAG=jdk-21.0.12%2B8
ARG JAVA21_ASSET=21.0.12_8
ARG MAVEN_VERSION=3.9.9
ARG JADX_VERSION=1.5.6
ARG JADX_SHA256=545ea2be9c242511bc145755cf4bda2485ade42966e096f8b4d3da2a230e8974
ARG APKTOOL_VERSION=3.0.3
ARG APKTOOL_SHA256=dbf930b076c6b9be08d57c449cacefc3bdd6b71ebd59b3066fc0e1f5b14f9423
ARG HTTPX_VERSION=1.12.0
ARG HTTPX_SHA256_AMD64=9d8439e8b6c9aa7d1e2314817a392e00d5178da3af5652f7475f88868f418f76
ARG SUBFINDER_VERSION=2.16.0
ARG SUBFINDER_SHA256_AMD64=1b7f9c608e9a5bd59e609a5e09710d63c5485e92d3d49dc2c16eb4fdbe10cb60
# Java 利用附件: rogue-jndi 锁 commit 源码现场构建; JDumpSpider 预编译 fat jar。
ARG ROGUE_JNDI_COMMIT=1aa5a5dfc09bfcd7dd50c617a6cd79167d5248d6
ARG ROGUE_JNDI_SHA256=6fa3181d80fbdec5db36b9936c6d51adb3a0f2a5cfc7d0b2d1a52507efebdcf4
ARG JDUMPSPIDER_TAG=dev-20250409T071858
ARG JDUMPSPIDER_JAR=JDumpSpider-1.1-SNAPSHOT-full.jar
ARG JDUMPSPIDER_SHA256=e8ddd7bb514f6ed0bad60efb1ae9e6f7656620b2d67fc977a3773d8ba5e55d49
# enum4linux-ng: 锁定 release tag(上游最新 release 即此), 装后核验 pip 元数据版本。
ARG ENUM4LINUX_NG_TAG=v1.3.10
# Playwright CLI: 与 npm @latest 稳定版天然错位(其依赖内钉一个 alpha 版 playwright),
# 错位即两套 chromium revision、tar 导出破 3GB 预算。CLI 经此锚点锁定; 顶层 playwright
# 库在构建期由此 CLI 实际嵌装的版本推导(playwright 层), 升级只改这一个 ARG。
ARG PLAYWRIGHT_CLI_VERSION=0.1.25

# 构建期代理。ALL_PROXY/all_proxy 不是 Docker 预定义参数，必须显式声明。
ARG HTTP_PROXY=
ARG HTTPS_PROXY=
ARG ALL_PROXY=
ARG NO_PROXY=
ARG http_proxy=
ARG https_proxy=
ARG all_proxy=
ARG no_proxy=

# 留空时使用官方软件源，build.sh --use-mirror 时注入国内源。
ARG PIP_INDEX_URL=
ARG PIP_TRUSTED_HOST=
ARG NPM_REGISTRY=
ARG APT_SOURCE_BLOCK=
# Maven Central 官方源国内拉包易超时(rogue-jndi 现场构建依赖它)。留空走官方,
# --use-mirror 时注入阿里云公共仓。
ARG MAVEN_MIRROR_URL=

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    TZ=Asia/Shanghai \
    HOME=/root \
    PATH=/norns/bin:/norns/engine/cc/node_modules/.bin:${PATH} \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_BREAK_SYSTEM_PACKAGES=1 \
    PIP_INDEX_URL=${PIP_INDEX_URL} \
    PIP_TRUSTED_HOST=${PIP_TRUSTED_HOST} \
    NUCLEI_TEMPLATES_DIR=/opt/nuclei-templates \
    NPM_CONFIG_UPDATE_NOTIFIER=false \
    NORNS_IMAGE_NODE_VERSION=${NODE_VERSION} \
    NORNS_IMAGE_CLAUDE_CODE_VERSION=${CLAUDE_CODE_VERSION} \
    NODE_PATH=/usr/local/lib/node_modules \
    JAVA_HOME=/opt/jdks/jdk21

# Debian 13 trixie slim 使用 DEB822 源文件。
RUN if [ -n "${APT_SOURCE_BLOCK}" ]; then \
        rm -f /etc/apt/sources.list.d/debian.sources /etc/apt/sources.list; \
        printf '%s\n' "${APT_SOURCE_BLOCK}" > /etc/apt/sources.list.d/debian.sources; \
    fi

# 系统、网络、经典 Web、Samba、浏览器、misc 取证、数据库客户端和系统小件工具。
# 追加: pwn 栈(gdb-multiarch/patchelf + amd64 专属 i386 运行库)、OCR(中英文)、
# 高频小件(7z/tree/rlwrap/fcrackzip/foremost)、php-cli(PHP 源码级弱点复现/一句话
# 木马现场调试)、snmp(snmpwalk 等老派协议枚举)、ldap-utils(ldapsearch)、whatweb。
# 全列表硬装: 任何包源缺失/安装失败直接中断构建, 本镜像不再有软失败项。
RUN set -eux; \
    if [ "$(dpkg --print-architecture)" = "amd64" ]; then dpkg --add-architecture i386; fi; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        build-essential pkg-config gdb strace binutils ltrace time \
        ca-certificates curl wget git jq ripgrep tmux less file openssl vim sudo \
        unzip zip tar gzip xz-utils bsdextrautils perl \
        net-tools iputils-ping iputils-tracepath dnsutils whois iproute2 iptables netcat-openbsd \
        socat tcpdump proxychains4 sshpass openssh-client openvpn \
        krb5-user \
        nmap sqlmap hydra john dirb gobuster \
        smbclient samba-common-bin \
        libxml-writer-perl libwhisker2-perl whatweb \
        chromium imagemagick \
        steghide libimage-exiftool-perl binwalk \
        default-mysql-client postgresql-client redis-tools sqlite3 libxml2-utils \
        php-cli snmp ldap-utils \
        xxd procps psmisc lsof \
        gdb-multiarch patchelf \
        tesseract-ocr tesseract-ocr-eng tesseract-ocr-chi-sim \
        p7zip-full tree rlwrap fcrackzip foremost; \
    if [ "$(dpkg --print-architecture)" = "amd64" ]; then \
        apt-get install -y --no-install-recommends \
            libc6:i386 libgcc-s1:i386 libstdc++6:i386 zlib1g:i386; \
    fi; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in amd64|arm64) ;; *) echo "不支持的目标架构: $arch" >&2; exit 1 ;; esac; \
    mkdir -p /tmp/radare2; \
    cd /tmp/radare2; \
    package="radare2_${RADARE2_VERSION}_${arch}.deb"; \
    base_url="https://github.com/radareorg/radare2/releases/download/${RADARE2_VERSION}"; \
    curl -fL --retry 5 --connect-timeout 15 --max-time 180 "$base_url/$package" -o "$package"; \
    curl -fL --retry 5 --connect-timeout 15 --max-time 180 "$base_url/checksums.txt" -o checksums.txt; \
    sha256sum --check --ignore-missing checksums.txt; \
    apt-get update; \
    apt-get install -y --no-install-recommends "/tmp/radare2/$package"; \
    apt-get clean; \
    cd /; \
    rm -rf /tmp/radare2 /var/lib/apt/lists/*

# 直接安装 Node.js 官方二进制包，避免额外拉取 Node 镜像或引入运行时 NVM。
RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in \
        amd64) node_arch="x64" ;; \
        arm64) node_arch="arm64" ;; \
        *) echo "不支持的 Node.js 目标架构: $arch" >&2; exit 1 ;; \
    esac; \
    archive="node-v${NODE_VERSION}-linux-${node_arch}.tar.xz"; \
    url="${NODE_DIST_URL%/}/v${NODE_VERSION}/${archive}"; \
    i=0; \
    while [ "$i" -lt 15 ]; do \
        i=$((i+1)); \
        wget --timeout=60 -q "$url" -O "/tmp/$archive" \
            && [ -s "/tmp/$archive" ] && break; \
        echo "[retry $i] Node.js 下载失败，5s 后重试" >&2; \
        rm -f "/tmp/$archive"; \
        sleep 5; \
    done; \
    [ -s "/tmp/$archive" ]; \
    tar -xJf "/tmp/$archive" -C /usr/local --strip-components=1; \
    rm -f "/tmp/$archive"; \
    node_version="$(node --version)"; \
    [ "$node_version" = "v${NODE_VERSION}" ]; \
    echo "Node.js ${node_version}"; \
    npm --version; \
    if [ -n "${NPM_REGISTRY}" ]; then npm config set registry "${NPM_REGISTRY}"; fi

# 多架构 Go 扫描器、nikto、常用词表和 linpeas。
RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in \
        arm64) dalfox_arch="aarch64" ;; \
        amd64) dalfox_arch="x86_64" ;; \
        *) echo "不支持的目标架构: $arch" >&2; exit 1 ;; \
    esac; \
    echo "构建目标架构: $arch (dalfox: $dalfox_arch)"; \
    mkdir -p /usr/local/bin /usr/share/wordlists /opt/peass; \
    dl() { local url="$1" out="$2" i=0; while [ $i -lt 15 ]; do i=$((i+1)); wget -c --timeout=60 -q "$url" -O "$out" && [ -s "$out" ] && return 0; echo "[retry $i] $out 失败, 5s 后重试" >&2; sleep 5; done; return 1; }; \
    dl "https://codeload.github.com/sullo/nikto/tar.gz/refs/tags/2.6.1" /tmp/nikto.tar.gz; \
    tar -xzf /tmp/nikto.tar.gz -C /tmp; \
    mv /tmp/nikto-2.6.1 /opt/nikto; \
    ln -s /opt/nikto/program/nikto.pl /usr/local/bin/nikto; \
    chmod +x /usr/local/bin/nikto; \
    rm -f /tmp/nikto.tar.gz; \
    dl "https://github.com/projectdiscovery/nuclei/releases/download/v3.11.1/nuclei_3.11.1_linux_${arch}.zip" /tmp/nuclei.zip; \
    dl "https://github.com/projectdiscovery/katana/releases/download/v1.7.0/katana_1.7.0_linux_${arch}.zip" /tmp/katana.zip; \
    dl "https://github.com/projectdiscovery/naabu/releases/download/v2.6.1/naabu_2.6.1_linux_${arch}.zip" /tmp/naabu.zip; \
    dl "https://github.com/ffuf/ffuf/releases/download/v2.2.1/ffuf_2.2.1_linux_${arch}.tar.gz" /tmp/ffuf.tar.gz; \
    dl "https://github.com/hahwul/dalfox/releases/download/v3.2.2/dalfox-v3.2.2-linux-${dalfox_arch}.tar.gz" /tmp/dalfox.tar.gz; \
    cd /tmp; \
    unzip -q nuclei.zip -d /tmp/nb; mv /tmp/nb/nuclei /usr/local/bin/; \
    unzip -q katana.zip -d /tmp/kb; mv /tmp/kb/katana /usr/local/bin/; \
    unzip -q naabu.zip -d /tmp/ub; mv /tmp/ub/naabu /usr/local/bin/; \
    tar -xzf ffuf.tar.gz -C /tmp; mv /tmp/ffuf /usr/local/bin/; \
    tar -xzf dalfox.tar.gz -C /tmp; mv "/tmp/dalfox-v3.2.2-linux-${dalfox_arch}/dalfox" /usr/local/bin/; \
    dl "https://github.com/BishopFox/cloudfox/releases/download/v2.0.5/cloudfox-linux-${arch}.zip" /tmp/cloudfox.zip; \
    unzip -q cloudfox.zip -d /tmp/cb; find /tmp/cb -name cloudfox -type f -exec mv {} /usr/local/bin/cloudfox \; ; \
    dl "https://github.com/jpillora/chisel/releases/download/v1.12.0/chisel_1.12.0_linux_${arch}.gz" /tmp/chisel.gz; \
    gunzip -f chisel.gz; mv chisel /usr/local/bin/chisel; \
    chmod +x /usr/local/bin/* 2>/dev/null || true; \
    dl "https://github.com/peass-ng/PEASS-ng/releases/latest/download/linpeas.sh" /opt/peass/linpeas.sh; \
    chmod +x /opt/peass/linpeas.sh; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/common.txt" -o /usr/share/wordlists/common.txt; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/raft-medium-directories.txt" -o /usr/share/wordlists/raft-medium-directories.txt; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Passwords/Common-Credentials/10k-most-common.txt" -o /usr/share/wordlists/top-10000-passwords.txt; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/danielmiessler/SecLists/cb6d7645abf4a2c467bc80f84cf70bda3d60c2d6/Passwords/Honeypot-Captures/SSH-passwords_1000_14d_2026-08_LureScope.txt" -o /usr/share/wordlists/top1000.txt; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/raft-medium-files.txt" -o /usr/share/wordlists/raft-medium-files.txt; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/big.txt" -o /usr/share/wordlists/big.txt; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/rapid7/metasploit-framework/master/data/wordlists/unix_users.txt" -o /usr/share/wordlists/unix_users.txt; \
    rm -rf /tmp/*

# ---- 五向补齐(工具面对齐 Heimdall) ----

# 1. Java 攻击栈: Temurin JDK 8/11/21 全量 JDK + maven。反序列化靶/内存马/Shellcode
# 加载器编译需要真 javac, JRE 不够。release tag 复用 Heimdall 锁文件已验证的版本, 资产名按
# Temurin 命名规则由 tag 机械推导。本层为硬依赖: 15 次重试后仍失败直接中断构建。
# java/javac 默认指向 jdk21, java8/java11/javac8/javac11/mvn8/mvn11 按版本切换。
RUN set -eux; \
    jarch="$(dpkg --print-architecture)"; \
    case "$jarch" in \
        amd64) jarch="x64" ;; \
        arm64) jarch="aarch64" ;; \
        *) echo "不支持的 Java 目标架构: $jarch" >&2; exit 1 ;; \
    esac; \
    dl() { local url="$1" out="$2" i=0; while [ $i -lt 15 ]; do i=$((i+1)); wget -c --timeout=60 -q "$url" -O "$out" && [ -s "$out" ] && return 0; echo "[retry $i] $out 失败, 5s 后重试" >&2; sleep 5; done; return 1; }; \
    install_jdk() { \
        repo="$1"; jtag="$2"; asset_name="$3"; dest="$4"; \
        dl "https://github.com/${repo}/releases/download/${jtag}/${asset_name}" "/tmp/${asset_name}"; \
        if dl "https://github.com/${repo}/releases/download/${jtag}/${asset_name}.sha256.txt" "/tmp/${asset_name}.sha256.txt"; then \
            ckline="$(awk -v f="$asset_name" '$2==f' "/tmp/${asset_name}.sha256.txt")"; \
            if [ -n "$ckline" ]; then ( cd /tmp && printf '%s\n' "$ckline" | sha256sum --check --strict - ); \
            else echo "[error] ${asset_name}.sha256.txt 中无 ${asset_name} 条目, 无法强校验" >&2; exit 1; fi; \
        else echo "[warn] ${asset_name}.sha256.txt 不可得, 跳过强校验"; fi; \
        mkdir -p "$dest"; \
        tar -xzf "/tmp/${asset_name}" -C "$dest" --strip-components=1; \
        rm -f "/tmp/${asset_name}" "/tmp/${asset_name}.sha256.txt"; \
    }; \
    install_jdk adoptium/temurin8-binaries "${JAVA8_TAG}" "OpenJDK8U-jdk_${jarch}_linux_hotspot_${JAVA8_ASSET}.tar.gz" /opt/jdks/jdk8; \
    install_jdk adoptium/temurin11-binaries "${JAVA11_TAG}" "OpenJDK11U-jdk_${jarch}_linux_hotspot_${JAVA11_ASSET}.tar.gz" /opt/jdks/jdk11; \
    install_jdk adoptium/temurin21-binaries "${JAVA21_TAG}" "OpenJDK21U-jdk_${jarch}_linux_hotspot_${JAVA21_ASSET}.tar.gz" /opt/jdks/jdk21; \
    ln -sf /opt/jdks/jdk21/bin/java /usr/local/bin/java; \
    ln -sf /opt/jdks/jdk21/bin/javac /usr/local/bin/javac; \
    for v in 8 11 21; do \
        ln -sf "/opt/jdks/jdk${v}/bin/java" "/usr/local/bin/java${v}"; \
        ln -sf "/opt/jdks/jdk${v}/bin/javac" "/usr/local/bin/javac${v}"; \
        printf '#!/bin/sh\nJAVA_HOME=/opt/jdks/jdk%s\nexport JAVA_HOME\nexec /opt/maven/bin/mvn "$@"\n' "$v" > "/usr/local/bin/mvn${v}"; \
        chmod 0755 "/usr/local/bin/mvn${v}"; \
    done; \
    dl "https://archive.apache.org/dist/maven/maven-3/${MAVEN_VERSION}/binaries/apache-maven-${MAVEN_VERSION}-bin.tar.gz" /tmp/maven.tgz; \
    mkdir -p /opt/maven; \
    tar -xzf /tmp/maven.tgz -C /opt/maven --strip-components=1; \
    rm -f /tmp/maven.tgz; \
    ln -sf /opt/maven/bin/mvn /usr/local/bin/mvn; \
    java8 -version; java11 -version; java21 -version; javac -version; mvn -v

# 2. pwn 调试栈: pwndbg(2026.07.29, 与 Heimdall 同 tag), gdb/gdb-multiarch 经
# /root/.gdbinit 自动加载。clone 15 次重试后仍失败/build 失败均直接断构建。
# 注意: 该 tag 的仓库布局没有 bin/ 目录(已实查), 不存在独立 `pwndbg` 启动器;
# 不要 ln -sf /opt/pwndbg/bin/pwndbg —— 那只会造出悬空软链, 恰好骗过 dash 的
# command -v, 而 Python 侧 shutil.which 等一切会跟踪链接的检查都会踩空。
RUN set -eux; \
    i=0; \
    while [ $i -lt 15 ]; do \
        i=$((i+1)); \
        if git -c http.proxy="${HTTP_PROXY:-}" -c https.proxy="${HTTPS_PROXY:-}" \
              clone --depth 1 --branch "${PWNDBG_TAG}" https://github.com/pwndbg/pwndbg.git /opt/pwndbg 2>/dev/null; then \
            break; \
        fi; \
        echo "[retry $i] pwndbg clone 5s 后重试" >&2; rm -rf /opt/pwndbg; sleep 5; \
    done; \
    [ -d /opt/pwndbg ]; \
    cd /opt/pwndbg; \
    ./setup.sh; \
    grep -q pwndbg /root/.gdbinit; \
    test -f /opt/pwndbg/gdbinit.py; \
    cd /

# 3. 深度逆向栈: jadx(字节码↔Java) + apktool(资源/smali 齐套), sha256 硬校验。
# 依赖上面的 Java 层(wrapper 用 PATH 默认 java=jdk21)。
# 本层曾两次以 "no properly formatted checksum lines found" 断构建, 而 echo 进 stdin 的
# 校验行已在外部逐字节验证合法。故改为落盘校验(不依赖管道), 并落日志诊断: sha256sum
# 身份、ARG 值长度、校验行字节(od), 下次构建无论成败都能从日志定位真因。
RUN set -eux; \
    dl() { local url="$1" out="$2" i=0; while [ $i -lt 15 ]; do i=$((i+1)); wget -c --timeout=60 -q "$url" -O "$out" && [ -s "$out" ] && return 0; echo "[retry $i] $out 失败, 5s 后重试" >&2; sleep 5; done; return 1; }; \
    dl "https://github.com/skylot/jadx/releases/download/v${JADX_VERSION}/jadx-${JADX_VERSION}.zip" /tmp/jadx.zip; \
    echo "[dbg] sha256sum=$(command -v sha256sum) len=${#JADX_SHA256}" >&2; \
    sha256sum --version >&2; \
    printf '%s  %s\n' "${JADX_SHA256}" "/tmp/jadx.zip" > /tmp/jadx.zip.sha256; \
    od -An -c /tmp/jadx.zip.sha256 >&2; \
    sha256sum --check --strict /tmp/jadx.zip.sha256; \
    printf '%s  %s\n' "${JADX_SHA256}" "/tmp/jadx.zip" > /tmp/jadx.zip.sha256; \
    od -An -c /tmp/jadx.zip.sha256 >&2; \
    sha256sum --check --strict /tmp/jadx.zip.sha256; \
    mkdir -p /opt/jadx; unzip -q /tmp/jadx.zip -d /opt/jadx; \
    ln -sf /opt/jadx/bin/jadx /usr/local/bin/jadx; \
    ln -sf /opt/jadx/bin/jadx-gui /usr/local/bin/jadx-gui; \
    mkdir -p /opt/apktool; \
    dl "https://github.com/iBotPeaches/Apktool/releases/download/v${APKTOOL_VERSION}/apktool_${APKTOOL_VERSION}.jar" /opt/apktool/apktool.jar; \
    printf '%s  %s\n' "${APKTOOL_SHA256}" "/opt/apktool/apktool.jar" > /tmp/apktool.jar.sha256; \
    sha256sum --check --strict /tmp/apktool.jar.sha256; \
    printf '#!/bin/sh\nexec "${JAVA_CMD:-java}" -jar /opt/apktool/apktool.jar "$@"\n' > /usr/local/bin/apktool; \
    chmod 0755 /usr/local/bin/apktool; \
    rm -f /tmp/jadx.zip /tmp/jadx.zip.sha256 /tmp/apktool.jar.sha256

# 4. ProjectDiscovery 补齐: httpx(存活指纹) + subfinder(子域枚举)。
# amd64 资产带 sha256 硬校验; arm64 无锁文件校验值, 下载成功即用(同 nuclei/katana 惯例)。
RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    dl() { local url="$1" out="$2" i=0; while [ $i -lt 15 ]; do i=$((i+1)); wget -c --timeout=60 -q "$url" -O "$out" && [ -s "$out" ] && return 0; echo "[retry $i] $out 失败, 5s 后重试" >&2; sleep 5; done; return 1; }; \
    dl "https://github.com/projectdiscovery/httpx/releases/download/v${HTTPX_VERSION}/httpx_${HTTPX_VERSION}_linux_${arch}.zip" /tmp/httpx.zip; \
    if [ "$arch" = "amd64" ]; then printf '%s  %s\n' "${HTTPX_SHA256_AMD64}" "/tmp/httpx.zip" > /tmp/httpx.zip.sha256; sha256sum --check --strict /tmp/httpx.zip.sha256; fi; \
    unzip -q -o /tmp/httpx.zip -d /tmp/httpx_out; mv /tmp/httpx_out/httpx /usr/local/bin/httpx; \
    chmod 0755 /usr/local/bin/httpx; rm -rf /tmp/httpx.zip /tmp/httpx.zip.sha256 /tmp/httpx_out; \
    dl "https://github.com/projectdiscovery/subfinder/releases/download/v${SUBFINDER_VERSION}/subfinder_${SUBFINDER_VERSION}_linux_${arch}.zip" /tmp/subfinder.zip; \
    if [ "$arch" = "amd64" ]; then printf '%s  %s\n' "${SUBFINDER_SHA256_AMD64}" "/tmp/subfinder.zip" > /tmp/subfinder.zip.sha256; sha256sum --check --strict /tmp/subfinder.zip.sha256; fi; \
    unzip -q -o /tmp/subfinder.zip -d /tmp/subfinder_out; mv /tmp/subfinder_out/subfinder /usr/local/bin/subfinder; \
    chmod 0755 /usr/local/bin/subfinder; rm -rf /tmp/subfinder.zip /tmp/subfinder.zip.sha256 /tmp/subfinder_out

# 5. Java 利用附件: rogue-jndi(JNDI 回连利用原语, 锁 commit 源码现场构建——
# pom 较老, 只能用 jdk8 编译, 构建方式与 Heimdall assets.sh 一致: mvn -B -DskipTests
# package 后取 *-all.jar) + JDumpSpider(预编译 fat jar, heap dump 凭据提取)。
# 两者字节码都面向 Java 8, wrapper 默认 java8 运行, 可 JAVA_CMD 覆盖。
# rogue-jndi 现场构建依赖从 Maven 拉包: MAVEN_MIRROR_URL 非空时把 central 指到
# 该镜像(build.sh --use-mirror 注入阿里云仓), 规避 Maven Central 国内拉包超时。
# 下载/构建/产物任一环节失败均直接断构建。
RUN set -eux; \
    dl() { local url="$1" out="$2" i=0; while [ $i -lt 15 ]; do i=$((i+1)); wget -c --timeout=60 -q "$url" -O "$out" && [ -s "$out" ] && return 0; echo "[retry $i] $out 失败, 5s 后重试" >&2; sleep 5; done; return 1; }; \
    if [ -n "${MAVEN_MIRROR_URL}" ]; then \
        mkdir -p /root/.m2; \
        printf '<settings><mirrors><mirror><id>build-mirror</id><name>build mirror for central</name><url>%s</url><mirrorOf>central</mirrorOf></mirror></mirrors></settings>\n' \
            "${MAVEN_MIRROR_URL}" > /root/.m2/settings.xml; \
    fi; \
    dl "https://codeload.github.com/veracode-research/rogue-jndi/tar.gz/${ROGUE_JNDI_COMMIT}" /tmp/rogue-jndi.tar.gz; \
    printf '%s  %s\n' "${ROGUE_JNDI_SHA256}" "/tmp/rogue-jndi.tar.gz" > /tmp/rogue-jndi.tar.gz.sha256; \
    sha256sum --check --strict /tmp/rogue-jndi.tar.gz.sha256; \
    mkdir -p /tmp/rjn; tar -xzf /tmp/rogue-jndi.tar.gz -C /tmp/rjn --strip-components=1; \
    ( cd /tmp/rjn && JAVA_HOME=/opt/jdks/jdk8 mvn -B -DskipTests package ); \
    jar="$(find /tmp/rjn/target -maxdepth 1 -name '*-all.jar' -print -quit)"; \
    if [ -z "$jar" ]; then jar="$(find /tmp/rjn/target -maxdepth 1 -name '*.jar' ! -name 'original-*' -print -quit)"; fi; \
    [ -n "$jar" ]; \
    mkdir -p /opt/rogue-jndi; \
    install -m 0644 "$jar" /opt/rogue-jndi/rogue-jndi.jar; \
    printf '#!/bin/sh\nexec "${JAVA_CMD:-java8}" -jar /opt/rogue-jndi/rogue-jndi.jar "$@"\n' > /usr/local/bin/rogue-jndi; \
    chmod 0755 /usr/local/bin/rogue-jndi; \
    echo "[ok] rogue-jndi 装好"; \
    mkdir -p /opt/JDumpSpider; \
    dl "https://github.com/whwlsfb/JDumpSpider/releases/download/${JDUMPSPIDER_TAG}/${JDUMPSPIDER_JAR}" /opt/JDumpSpider/JDumpSpider.jar; \
    printf '%s  %s\n' "${JDUMPSPIDER_SHA256}" "/opt/JDumpSpider/JDumpSpider.jar" > /tmp/JDumpSpider.jar.sha256; \
    sha256sum --check --strict /tmp/JDumpSpider.jar.sha256; \
    printf '#!/bin/sh\nexec "${JAVA_CMD:-java8}" -jar /opt/JDumpSpider/JDumpSpider.jar "$@"\n' > /usr/local/bin/jdumpspider; \
    chmod 0755 /usr/local/bin/jdumpspider; \
    rm -f /tmp/JDumpSpider.jar.sha256; \
    rm -rf /tmp/rjn /tmp/rogue-jndi.tar.gz /tmp/rogue-jndi.tar.gz.sha256

# nuclei 模板库: 15 次重试后仍失败直接断构建。
RUN set -eux; \
    i=0; \
    until wget -c --timeout=60 -q "https://codeload.github.com/projectdiscovery/nuclei-templates/tar.gz/refs/heads/main" -O /tmp/nuclei-templates.tar.gz && [ -s /tmp/nuclei-templates.tar.gz ]; do \
        i=$((i+1)); [ $i -lt 15 ] || { echo "[error] nuclei-templates 下载失败(15次)" >&2; exit 1; }; \
        echo "[retry $i] nuclei-templates 5s 后重试" >&2; sleep 5; \
    done; \
    tar -xzf /tmp/nuclei-templates.tar.gz -C /tmp; \
    mv /tmp/nuclei-templates-main /opt/nuclei-templates; \
    rm -f /tmp/nuclei-templates.tar.gz

# Python 渗透库和云 CLI。PyYAML 同时供入口写 nornsd.yaml 使用。
RUN pip install --no-cache-dir \
        PyYAML==6.0.3 \
        pwntools \
        capstone ROPgadget \
        z3-solver numpy numba \
        pyjwt sympy pycryptodome unicorn \
        pymongo \
        pyftpdlib \
        paramiko \
        cryptography \
        dnspython \
        ldap3 \
        beautifulsoup4 \
        requests aiohttp websockets websocket-client \
        pypdf pexpect \
        Pillow \
        dirsearch \
        awscli \
        tccli

RUN pip install --no-cache-dir impacket

RUN pip install --no-cache-dir bloodyAD

# crypto 大数运算(gmpy2)与 WAF 指纹(wafw00f)。
RUN pip install --no-cache-dir gmpy2

RUN pip install --no-cache-dir wafw00f

# 汇编/ELF 深度分析: pyelftools(纯 Python) + keystone-engine(汇编引擎)。
RUN pip install --no-cache-dir pyelftools

RUN pip install --no-cache-dir keystone-engine

# enum4linux-ng 必须 --no-deps，避免升级 bloodyAD 固定的 cryptography 版本。
# 其运行期真实依赖是 impacket/yaml/ldap3, 三者都已是上面的硬装 pip 项。
# 锁 ENUM4LINUX_NG_TAG 安装(pip 元数据版本须与 tag 一致, 否则说明 tag 被动过/装错)。
RUN set -eux; \
    cd /tmp; \
    i=0; \
    until git -c http.proxy="${HTTP_PROXY:-}" -c https.proxy="${HTTPS_PROXY:-}" \
          clone --depth=1 --branch "${ENUM4LINUX_NG_TAG}" https://github.com/cddmp/enum4linux-ng.git /tmp/e4l 2>/dev/null; do \
        i=$((i+1)); [ $i -lt 15 ] || { echo "[error] enum4linux-ng git clone 失败(15次)" >&2; exit 1; }; \
        echo "[retry $i] enum4linux-ng clone 5s 后重试" >&2; sleep 5; \
    done; \
    pip install --no-cache-dir --no-deps /tmp/e4l; \
    pip show enum4linux-ng | grep -q "^Version: ${ENUM4LINUX_NG_TAG#v}"; \
    echo "[ok] enum4linux-ng 装好(${ENUM4LINUX_NG_TAG}, 元数据核验通过)"; \
    rm -rf /tmp/*;

# JSFinder(从前端 JS 抽 URL/敏感接口/子域) + exploitdb/searchsploit(离线漏洞库检索)
# + PHPGGC(PHP 反序列化 gadget chain 生成器, 纯 PHP 单体仅需 php>=5.6, 无 composer
# 依赖)。JSFinder 单文件脚本仅依赖 requests+bs4(已装)。exploitdb 无 Debian 包(Kali
# 专属), 浅克隆自 gitlab 官方源(github 镜像已被上游清空成只剩 README 的空壳仓库,
# 作为兜底源会静默装出无脚本无数据的假库, 不可再用), 克隆后删 .git 瘦身(本体即全量
# 检索数据, 浅克隆不支持 searchsploit --update, 镜像内可接受); searchsploit/phpggc
# 用 wrapper exec 真脚本保证路径自解析。克隆完成后本层立即做结构校验 + ms17-010
# 真实检索(注意 searchsploit 默认按列宽截断标题, 检索点验必须带 --overflow)。
# 任一下载/克隆/校验失败均直接断构建。
RUN set -eux; \
    dl() { local url="$1" out="$2" i=0; while [ $i -lt 15 ]; do i=$((i+1)); wget -c --timeout=60 -q "$url" -O "$out" && [ -s "$out" ] && return 0; echo "[retry $i] $out 失败, 5s 后重试" >&2; sleep 5; done; return 1; }; \
    mkdir -p /opt/jsfinder; \
    dl "https://raw.githubusercontent.com/Threezh1/JSFinder/master/JSFinder.py" /opt/jsfinder/JSFinder.py; \
    printf '#!/bin/sh\nexec python3 /opt/jsfinder/JSFinder.py "$@"\n' > /usr/local/bin/jsfinder; \
    chmod 0755 /usr/local/bin/jsfinder; \
    echo "[ok] JSFinder 装好"; \
    exploitdb_ok=""; \
    i=0; \
    while [ $i -lt 15 ]; do \
        i=$((i+1)); \
        if git -c http.proxy="${HTTP_PROXY:-}" -c https.proxy="${HTTPS_PROXY:-}" \
              clone --depth 1 --single-branch https://gitlab.com/exploit-database/exploitdb.git /opt/exploitdb 2>/dev/null; then \
            exploitdb_ok=1; break; \
        fi; \
        echo "[retry $i] exploitdb clone 5s 后重试" >&2; \
        rm -rf /opt/exploitdb; sleep 5; \
    done; \
    [ -n "$exploitdb_ok" ] || { echo "[error] exploitdb 克隆失败(15次)" >&2; exit 1; }; \
    rm -rf /opt/exploitdb/.git; \
    test -f /opt/exploitdb/searchsploit; \
    test -s /opt/exploitdb/files_exploits.csv; \
    printf '#!/bin/sh\nexec bash /opt/exploitdb/searchsploit "$@"\n' > /usr/local/bin/searchsploit; \
    chmod 0755 /usr/local/bin/searchsploit; \
    searchsploit --disable-colour --overflow ms17-010 | grep -q 'MS17-010'; \
    echo "[ok] exploitdb 装好(结构+检索验证通过)"; \
    i=0; \
    until git -c http.proxy="${HTTP_PROXY:-}" -c https.proxy="${HTTPS_PROXY:-}" \
          clone --depth 1 https://github.com/ambionics/phpggc.git /opt/phpggc 2>/dev/null; do \
        i=$((i+1)); [ $i -lt 15 ] || { echo "[error] phpggc clone 失败(15次)" >&2; exit 1; }; \
        echo "[retry $i] phpggc clone 5s 后重试" >&2; \
        rm -rf /opt/phpggc; sleep 5; \
    done; \
    rm -rf /opt/phpggc/.git; \
    printf '#!/bin/sh\nexec php /opt/phpggc/phpggc "$@"\n' > /usr/local/bin/phpggc; \
    chmod 0755 /usr/local/bin/phpggc; \
    phpggc -l >/dev/null; \
    echo "[ok] phpggc 装好"

# 阿里云 CLI。
RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in arm64|amd64) ;; *) echo "不支持的目标架构: $arch" >&2; exit 1 ;; esac; \
    cd /tmp; mkdir aliyun_extract; \
    dl() { local url="$1" out="$2" i=0; while [ $i -lt 15 ]; do i=$((i+1)); wget -c --timeout=60 -q "$url" -O "$out" && [ -s "$out" ] && return 0; echo "[retry $i] $out 失败, 5s 后重试" >&2; sleep 5; done; return 1; }; \
    dl "https://github.com/aliyun/aliyun-cli/releases/download/v3.4.11/aliyun-cli-linux-3.4.11-${arch}.tgz" /tmp/aliyun.tgz; \
    tar -xzf aliyun.tgz -C aliyun_extract; \
    mv aliyun_extract/aliyun /usr/local/bin/aliyun; \
    chmod +x /usr/local/bin/aliyun; \
    rm -rf /tmp/*

# Playwright Agent CLI、库与浏览器。浏览器二进制 8 次重试后仍失败直接断构建
# (CLI 与库共享同一缓存; apt chromium 是另一条独立的 headless 路线, 不替代库匹配场景)。
# NODE_PATH 使 require('playwright') 在任意目录可用(库随 npm 全局安装)。
# 版本纪律(3GB 导出预算的硬前提): @playwright/cli 内钉一个 alpha 版 playwright, 与
# npm @latest 稳定版永远错位, 错开一次就是两套完整 chromium(每套 726MB raw/约 300MB
# 压缩后)。因此顶层库不跟 @latest: 先装 CLI, 构建期从其嵌装 node_modules 读出实际
# 内嵌的版本号(全局安装为嵌套布局、spec 为精确版本无 ^, 均已实证), 顶层装同一版;
# 层尾断言缓存内恰好一套 chromium / headless shell, 上游漂移当场断构建。
# 体积纪律: install-browser 支持按浏览器名过滤
# (https://playwright.nodejs.cn/agent-cli/installation), 只装 chromium, firefox/webkit
# 从源头不下载, find 断言写死"缓存内无 firefox/webkit"这一不变式。
RUN set -eux; \
    npm install -g "@playwright/cli@${PLAYWRIGHT_CLI_VERSION}"; \
    lib_version="$(node -e "console.log(require('/usr/local/lib/node_modules/@playwright/cli/node_modules/playwright/package.json').version)")"; \
    echo "[ok] @playwright/cli 内嵌 playwright 版本: ${lib_version}, 顶层库装同版对齐" >&2; \
    npm install -g "playwright@${lib_version}"; \
    cli_ok=0; \
    for i in $(seq 1 8); do \
        if playwright-cli install-browser --with-deps chromium; then cli_ok=1; echo "[ok] playwright 浏览器装好"; break; fi; \
        echo "[retry $i] playwright 浏览器下载失败, 清缓存 5s 后重试" >&2; \
        rm -rf /root/.cache/ms-playwright; sleep 5; \
    done; \
    [ "$cli_ok" = "1" ]; \
    playwright install --with-deps chromium; \
    echo "[ok] playwright 库匹配浏览器就绪"; \
    [ "$(find /root/.cache/ms-playwright -maxdepth 1 -type d -name 'chromium-*' | wc -l)" -eq 1 ]; \
    [ "$(find /root/.cache/ms-playwright -maxdepth 1 -type d -name 'chromium_headless_shell-*' | wc -l)" -eq 1 ]; \
    [ -z "$(find /root/.cache/ms-playwright -maxdepth 1 -type d \( -name 'firefox-*' -o -name 'webkit-*' \) -print -quit)" ]; \
    rm -rf /root/.cache/ms-playwright/cli-update-check.json /tmp/*

# Claude Code 使用项目内固定引擎目录。
RUN mkdir -p /norns/bin /norns/config /norns/domain/redteam /norns/engine/cc /workspace

# Pentest-Tools 环境标记，SKILL.md 以此文件存在为环境可用标识。
RUN touch /.is_pentest_tools_env

RUN set -eux; \
    if [ -n "${NPM_REGISTRY}" ]; then \
        npm install --prefix /norns/engine/cc \
          "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
          --registry="${NPM_REGISTRY}"; \
    else \
        npm install --prefix /norns/engine/cc \
          "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}"; \
    fi; \
    npm cache clean --force; \
    /norns/engine/cc/node_modules/.bin/claude --version

# NORNS_RELEASE_DIR 由 build.sh 按主机架构选择；目录中已有的 norns-* 插件一并复制。
COPY ${NORNS_RELEASE_DIR}/ /norns/bin/
COPY nornsd/configs/nornsd/nornsd.example.yaml /norns/config/nornsd.example.yaml
COPY domain/redteam/ /norns/domain/redteam/
# 通用服务入口，不启动 benchmark 调度器。
COPY container/redteam/entrypoint.py /usr/local/bin/nornsd-entrypoint
COPY container/redteam/verify.py /usr/local/bin/nornsd-verify

# 构建期点验: 全量硬点验, 任何件缺失/不可用立即中断, 不带病出镜像。
# 覆盖各层产物: apt 件、Go 二进制、Git 克隆件、pip 件、词表、jar 附件、浏览器缓存。
RUN set -eux; \
    java -version; java8 -version; java11 -version; java21 -version; javac -version; \
    mvn -v >/dev/null; JAVA_HOME=/opt/jdks/jdk8 mvn -v >/dev/null; \
    tesseract --version >/dev/null; \
    tesseract --list-langs 2>/dev/null | grep -qx 'eng'; \
    tesseract --list-langs 2>/dev/null | grep -qx 'chi_sim'; \
    command -v gdb-multiarch patchelf 7z tree rlwrap fcrackzip foremost httpx subfinder jadx apktool php snmpwalk ldapsearch; \
    command -v whatweb wafw00f bloodyAD enum4linux-ng; \
    grep -q pwndbg /root/.gdbinit; \
    command -v cloudfox chisel aliyun phpggc jsfinder searchsploit; \
    command -v secretsdump.py wmiexec.py psexec.py mssqlclient.py GetUserSPNs.py getTGT.py smbclient.py; \
    command -v rogue-jndi jdumpspider; \
    test -f /opt/peass/linpeas.sh; \
    test -f /usr/share/wordlists/unix_users.txt; \
    searchsploit --disable-colour --overflow ms17-010 | grep -q 'MS17-010'; \
    phpggc -l | grep -q 'Laravel/RCE'; \
    python3 -m py_compile /opt/jsfinder/JSFinder.py; \
    if [ "$(dpkg --print-architecture)" = "amd64" ]; then dpkg -s libc6:i386 >/dev/null; fi; \
    test -f /opt/JDumpSpider/JDumpSpider.jar; \
    test -f /opt/rogue-jndi/rogue-jndi.jar; \
    python3 -c "from elftools.elf.elffile import ELFFile; print('pyelftools ok')"; \
    python3 -c "import PIL; print('Pillow ok')"; \
    python3 -c "import keystone; print('keystone ok')"; \
    python3 -c "import gmpy2; print('gmpy2 ok')"; \
    python3 -c "import ldap3; print('ldap3 ok')"; \
    ls /root/.cache/ms-playwright/ | grep -q 'chromium'; \
    jadx --version; \
    apktool --version

RUN set -eux; \
    test -f /norns/bin/norns; \
    test -f /norns/bin/nornsd; \
    test -f /norns/domain/redteam/extend_agents/meta.json; \
    test -d /norns/domain/redteam/custom_skills; \
    chmod 0755 /norns/bin/norns /norns/bin/nornsd \
      /usr/local/bin/nornsd-entrypoint /usr/local/bin/nornsd-verify; \
    find /norns/bin -maxdepth 1 -type f -name 'norns-*' -exec chmod 0755 {} \;; \
    chmod 0600 /norns/config/nornsd.example.yaml; \
    python3 -m py_compile /usr/local/bin/nornsd-entrypoint; \
    python3 -c "import yaml; print('pyyaml ok')"; \
    python3 -c "import json, urllib.request, subprocess, secrets, signal, re, threading, concurrent.futures; print('stdlib ok')"; \
    printf '%s\n' '{' '  "hasCompletedOnboarding": true' '}' > /root/.claude.json; \
    chmod 0600 /root/.claude.json; \
    python3 /usr/local/bin/nornsd-verify --build

USER root
WORKDIR /workspace

EXPOSE 28081
VOLUME ["/workspace"]
STOPSIGNAL SIGTERM

# 不接受位置参数；通过环境变量生成 nornsd.yaml 并启动服务。
ENTRYPOINT ["/usr/local/bin/nornsd-entrypoint"]
