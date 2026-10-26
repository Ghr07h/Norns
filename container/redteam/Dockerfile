# 独立的 Norns redteam 服务镜像，仅以 Python slim 为基础。

ARG PYTHON_IMAGE=python:3.13.15-slim
FROM ${PYTHON_IMAGE}

ARG CLAUDE_CODE_VERSION=2.1.199
ARG NODE_VERSION=24.22.1
ARG NODE_DIST_URL=https://nodejs.org/dist
ARG NORNS_RELEASE_DIR=release/linux-x64

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
    NORNS_IMAGE_CLAUDE_CODE_VERSION=${CLAUDE_CODE_VERSION}

# Debian 13 trixie slim 使用 DEB822 源文件。
RUN if [ -n "${APT_SOURCE_BLOCK}" ]; then \
        rm -f /etc/apt/sources.list.d/debian.sources /etc/apt/sources.list; \
        printf '%s\n' "${APT_SOURCE_BLOCK}" > /etc/apt/sources.list.d/debian.sources; \
    fi

# 系统、网络、经典 Web、Samba 和浏览器工具。
RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential pkg-config \
        ca-certificates curl wget git jq ripgrep tmux less file openssl vim sudo \
        unzip tar gzip xz-utils bsdextrautils perl \
        net-tools iputils-ping dnsutils whois iproute2 netcat-openbsd \
        socat tcpdump proxychains4 sshpass openssh-client openvpn \
        krb5-user \
        nmap sqlmap hydra john dirb gobuster \
        smbclient samba-common-bin \
        libxml-writer-perl libwhisker2-perl \
        chromium imagemagick \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

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
    if dl "https://github.com/BishopFox/cloudfox/releases/download/v2.0.5/cloudfox-linux-${arch}.zip" /tmp/cloudfox.zip; then \
        unzip -q cloudfox.zip -d /tmp/cb; find /tmp/cb -name cloudfox -type f -exec mv {} /usr/local/bin/cloudfox \; ; \
    else echo "[warn] cloudfox 下载失败(15次重试后), 已跳过"; fi; \
    if dl "https://github.com/jpillora/chisel/releases/download/v1.12.0/chisel_1.12.0_linux_${arch}.gz" /tmp/chisel.gz; then \
        gunzip -f chisel.gz; mv chisel /usr/local/bin/chisel; \
    else echo "[warn] chisel 下载失败, 已跳过"; fi; \
    chmod +x /usr/local/bin/* 2>/dev/null || true; \
    if dl "https://github.com/peass-ng/PEASS-ng/releases/latest/download/linpeas.sh" /opt/peass/linpeas.sh; then \
        chmod +x /opt/peass/linpeas.sh; \
    else echo "[warn] linpeas 下载失败, 已跳过"; fi; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/common.txt" -o /usr/share/wordlists/common.txt || echo "[warn] common.txt 下载失败"; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/raft-medium-directories.txt" -o /usr/share/wordlists/raft-medium-directories.txt || echo "[warn] raft-medium 下载失败"; \
    curl -fsSL --retry 5 --retry-delay 3 "https://raw.githubusercontent.com/danielmiessler/SecLists/master/Passwords/Common-Credentials/10k-most-common.txt" -o /usr/share/wordlists/top-10000-passwords.txt || echo "[warn] top-10000 下载失败"; \
    rm -rf /tmp/*

# nuclei 模板库是可选资源，下载失败不阻断镜像构建。
RUN set -eux; \
    i=0; \
    until wget -c --timeout=60 -q "https://codeload.github.com/projectdiscovery/nuclei-templates/tar.gz/refs/heads/main" -O /tmp/nuclei-templates.tar.gz && [ -s /tmp/nuclei-templates.tar.gz ]; do \
        i=$((i+1)); [ $i -lt 15 ] || { echo "[warn] nuclei-templates 下载失败, 已跳过"; exit 0; }; \
        echo "[retry $i] nuclei-templates 5s 后重试" >&2; sleep 5; \
    done; \
    tar -xzf /tmp/nuclei-templates.tar.gz -C /tmp; \
    mv /tmp/nuclei-templates-main /opt/nuclei-templates; \
    rm -f /tmp/nuclei-templates.tar.gz

# Python 渗透库和云 CLI。PyYAML 同时供 nornsd 容器入口使用。
RUN pip install --no-cache-dir \
        PyYAML==6.0.3 \
        pwntools \
        pymongo \
        pyftpdlib \
        paramiko \
        cryptography \
        dnspython \
        beautifulsoup4 \
        requests \
        dirsearch \
        awscli \
        tccli

RUN pip install --no-cache-dir impacket \
    || echo "[warn] impacket 安装失败"

RUN pip install --no-cache-dir bloodyAD \
    || echo "[warn] bloodyAD 安装失败"

# enum4linux-ng 必须 --no-deps，避免升级 bloodyAD 固定的 cryptography 版本。
RUN set -eux; \
    cd /tmp; \
    i=0; \
    until git -c http.proxy="${HTTP_PROXY:-}" -c https.proxy="${HTTPS_PROXY:-}" \
          clone --depth=1 https://github.com/cddmp/enum4linux-ng.git /tmp/e4l 2>/dev/null; do \
        i=$((i+1)); [ $i -lt 15 ] || { echo "[warn] enum4linux-ng git clone 失败(15次), 已跳过"; break; }; \
        echo "[retry $i] enum4linux-ng clone 5s 后重试" >&2; sleep 5; \
    done; \
    if [ -d /tmp/e4l ]; then \
        { pip install --no-cache-dir --no-deps /tmp/e4l && echo "[ok] enum4linux-ng 装好"; } \
        || echo "[warn] enum4linux-ng 安装失败, 已跳过"; \
    fi; \
    rm -rf /tmp/*

# 阿里云 CLI。
RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in arm64|amd64) ;; *) echo "不支持的目标架构: $arch" >&2; exit 1 ;; esac; \
    cd /tmp; mkdir aliyun_extract; \
    dl() { local url="$1" out="$2" i=0; while [ $i -lt 15 ]; do i=$((i+1)); wget -c --timeout=60 -q "$url" -O "$out" && [ -s "$out" ] && return 0; echo "[retry $i] $out 失败, 5s 后重试" >&2; sleep 5; done; return 1; }; \
    if dl "https://github.com/aliyun/aliyun-cli/releases/download/v3.4.11/aliyun-cli-linux-3.4.11-${arch}.tgz" /tmp/aliyun.tgz; then \
        tar -xzf aliyun.tgz -C aliyun_extract; \
        mv aliyun_extract/aliyun /usr/local/bin/aliyun; \
        chmod +x /usr/local/bin/aliyun; \
    else echo "[warn] 阿里云 CLI 下载失败, 已跳过"; fi; \
    rm -rf /tmp/*

# Playwright Agent CLI 与浏览器。浏览器下载失败为可接受的可选缺失。
RUN set -eux; \
    npm install -g @playwright/cli@latest; \
    for i in $(seq 1 8); do \
        if playwright-cli install-browser --with-deps; then echo "[ok] playwright 浏览器装好"; break; fi; \
        echo "[retry $i] playwright 浏览器下载失败, 清缓存 5s 后重试" >&2; \
        rm -rf /root/.cache/ms-playwright; sleep 5; \
    done; \
    rm -rf /root/.cache/ms-playwright/cli-update-check.json /tmp/*

# Claude Code 使用项目内固定引擎目录。
RUN mkdir -p /norns/bin /norns/config /norns/domain/redteam /norns/engine/cc /workspace

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
COPY nornsd/internal/config/agent.example.yaml /norns/config/agent.example.yaml
COPY domain/redteam/ /norns/domain/redteam/
COPY container/redteam/entrypoint.py /usr/local/bin/nornsd-entrypoint
COPY container/redteam/verify.py /usr/local/bin/nornsd-verify

RUN set -eux; \
    test -f /norns/bin/norns; \
    test -f /norns/bin/nornsd; \
    test -f /norns/domain/redteam/norns_domain_agents/meta.json; \
    test -d /norns/domain/redteam/skills; \
    chmod 0755 /norns/bin/norns /norns/bin/nornsd \
      /usr/local/bin/nornsd-entrypoint /usr/local/bin/nornsd-verify; \
    find /norns/bin -maxdepth 1 -type f -name 'norns-*' -exec chmod 0755 {} \;; \
    chmod 0600 /norns/config/agent.example.yaml; \
    printf '%s\n' '{' '  "hasCompletedOnboarding": true' '}' > /root/.claude.json; \
    chmod 0600 /root/.claude.json; \
    python3 /usr/local/bin/nornsd-verify --build

USER root
WORKDIR /workspace

EXPOSE 28081
VOLUME ["/workspace"]
STOPSIGNAL SIGTERM

# Runtime 配置只从同名环境变量读取。位置参数不会绕过配置入口。
ENTRYPOINT ["/usr/local/bin/nornsd-entrypoint"]
