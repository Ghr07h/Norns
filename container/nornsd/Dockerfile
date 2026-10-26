# 通用 nornsd Server 镜像。

ARG PYTHON_IMAGE=python:3.13.15-slim
FROM ${PYTHON_IMAGE}

# Build configuration.
ARG CLAUDE_CODE_VERSION=2.1.199
ARG NODE_VERSION=24.22.1
ARG NODE_DIST_URL=https://nodejs.org/dist
ARG NORNS_RELEASE_DIR=release/linux-x64
ARG HTTP_PROXY=
ARG HTTPS_PROXY=
ARG ALL_PROXY=
ARG NO_PROXY=
ARG http_proxy=
ARG https_proxy=
ARG all_proxy=
ARG no_proxy=
ARG PIP_INDEX_URL=
ARG PIP_TRUSTED_HOST=
ARG NPM_REGISTRY=
ARG APT_SOURCE_BLOCK=

# Runtime environment.
ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    HOME=/root \
    PATH=/norns/bin:/norns/engine/cc/node_modules/.bin:${PATH} \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_BREAK_SYSTEM_PACKAGES=1 \
    PIP_INDEX_URL=${PIP_INDEX_URL} \
    PIP_TRUSTED_HOST=${PIP_TRUSTED_HOST} \
    NPM_CONFIG_UPDATE_NOTIFIER=false \
    NORNS_IMAGE_NODE_VERSION=${NODE_VERSION} \
    NORNS_IMAGE_CLAUDE_CODE_VERSION=${CLAUDE_CODE_VERSION}

# Debian 13 trixie slim 使用 DEB822 源文件。
RUN if [ -n "${APT_SOURCE_BLOCK}" ]; then \
        rm -f /etc/apt/sources.list.d/debian.sources /etc/apt/sources.list; \
        printf '%s\n' "${APT_SOURCE_BLOCK}" > /etc/apt/sources.list.d/debian.sources; \
    fi

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential pkg-config \
        bash ca-certificates curl wget git jq ripgrep tmux less file openssl vim sudo \
        unzip tar gzip xz-utils \
        procps net-tools iputils-ping dnsutils iproute2 netcat-openbsd openssh-client \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# 固定 Node.js 版本，支持 amd64 和 arm64。
RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in \
        amd64) node_arch="x64" ;; \
        arm64) node_arch="arm64" ;; \
        *) echo "unsupported Node.js target architecture: $arch" >&2; exit 1 ;; \
    esac; \
    archive="node-v${NODE_VERSION}-linux-${node_arch}.tar.xz"; \
    url="${NODE_DIST_URL%/}/v${NODE_VERSION}/${archive}"; \
    i=0; \
    while [ "$i" -lt 15 ]; do \
        i=$((i+1)); \
        wget --timeout=60 -q "$url" -O "/tmp/$archive" \
            && [ -s "/tmp/$archive" ] && break; \
        echo "[retry $i] Node.js download failed; retrying in 5 seconds" >&2; \
        rm -f "/tmp/$archive"; \
        sleep 5; \
    done; \
    [ -s "/tmp/$archive" ]; \
    tar -xJf "/tmp/$archive" -C /usr/local --strip-components=1; \
    rm -f "/tmp/$archive"; \
    [ "$(node --version)" = "v${NODE_VERSION}" ]; \
    npm --version; \
    if [ -n "${NPM_REGISTRY}" ]; then npm config set registry "${NPM_REGISTRY}"; fi

RUN pip install --no-cache-dir PyYAML==6.0.3

RUN mkdir -p \
      /norns/bin \
      /norns/config \
      /norns/domain \
      /norns/engine/cc \
      /workspace

# Claude Code 使用项目内固定引擎目录。
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

COPY ${NORNS_RELEASE_DIR}/ /norns/bin/
COPY domain/ /norns/domain/
COPY nornsd/internal/config/agent.example.yaml /norns/config/agent.example.yaml
COPY container/nornsd/entrypoint.py /usr/local/bin/nornsd-entrypoint
COPY container/nornsd/verify.py /usr/local/bin/nornsd-verify

RUN set -eux; \
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

ENTRYPOINT ["/usr/local/bin/nornsd-entrypoint"]
