FROM node:22-bookworm AS node-runtime

FROM golang:1.22-bookworm

ENV DEBIAN_FRONTEND=noninteractive \
    CI=1 \
    NO_COLOR=1 \
    PATH=/usr/local/go/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin \
    NODE_PATH=/usr/local/lib/node_modules \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

COPY --from=node-runtime /usr/local/bin/node /usr/local/bin/node
COPY --from=node-runtime /usr/local/lib/node_modules /usr/local/lib/node_modules

RUN ln -sf /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -sf /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        build-essential \
        python3 \
        python3-pip \
        python3-venv \
    && python3 -m pip install --break-system-packages --no-cache-dir pytest pytest-cov \
    && npm install -g playwright@1.56.1 \
    && npx playwright install --with-deps chromium \
    && chmod -R a+rX /ms-playwright /usr/local/lib/node_modules \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -u 1000 qsm || true
USER 1000:1000
WORKDIR /workspace
